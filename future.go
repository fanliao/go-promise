/*
Package promise provides a complete promise and future implementation.
A quick start sample:


fu := Start(func()(resp interface{}, err error){
    resp, err := http.Get("http://example.com/")
    return
})
//do somthing...
resp, err := fu.Get()
*/
package promise

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

type callbackType int

const (
	CALLBACK_DONE callbackType = iota
	CALLBACK_FAIL
	CALLBACK_ALWAYS
)

//pipe presents the chain promise
//pipe结构表示一个链式调用的Promise
type pipe struct {
	pipeDoneTask, pipeFailTask func(v interface{}) *Future
	pipePromise                *Promise
}

//
//保存Future状态数据的结构
type futureVal struct {
	dones, fails, always []func(v interface{})
	pipe
	r unsafe.Pointer
}

//返回与链式调用相关的对象
func (this *futureVal) getPipe(isResolved bool) (func(v interface{}) *Future, *Promise) {
	if isResolved {
		return this.pipeDoneTask, this.pipePromise
	} else {
		return this.pipeFailTask, this.pipePromise
	}
}

//Future代表一个异步任务的readonly-view
//Future provides a read-only view of promise, the value is set by using promise.Resolve, Reject and Cancel methods
type Future struct {
	Id       int
	oncePipe *sync.Once
	chOut    chan *PromiseResult
	chEnd    chan struct{}
	//指向futureVal的指针，程序要保证该指针指向的对象内容不会发送变化，任何变化都必须生成新对象并通过原子操作更新指针，以避免lock
	v            unsafe.Pointer
	cancelStatus int32
}

//请求取消异步任务
//RequestCancel request to cancel the promise
//It don't mean the promise be surely cancelled
func (this *Future) RequestCancel() bool {
	ccstatus := atomic.LoadInt32(&this.cancelStatus)
	if ccstatus == 0 {
		atomic.CompareAndSwapInt32(&this.cancelStatus, 0, 1)
		return true
	} else {
		return false
	}
}

//获得任务是否已经被Cancel
//IsCancelled returns true if the promise is cancelled, otherwise false
func (this *Future) IsCancelled() bool {
	ccstatus := atomic.LoadInt32(&this.cancelStatus)
	return ccstatus == 2
}

func (this *Future) GetChan() chan *PromiseResult {
	return this.chOut
}

//Get函数将一直阻塞直到任务完成,并返回任务的结果
//如果任务已经完成，后续的Get将直接返回任务结果
func (this *Future) Get() (interface{}, error) {
	<-this.chEnd
	return getFutureReturnVal(this.result())
}

//Get函数将一直阻塞直到任务完成或超过指定的Timeout时间
//如果任务已经完成，后续的Get将直接返回任务结果
//mm的单位是毫秒
func (this *Future) GetOrTimeout(mm int) (interface{}, error, bool) {
	if mm == 0 {
		mm = 10
	} else {
		mm = mm * 1000 * 1000
	}

	select {
	case <-time.After((time.Duration)(mm) * time.Nanosecond):
		return nil, nil, true
	case <-this.chEnd:
		r, err := getFutureReturnVal(this.result())
		return r, err, false
	}
}

//添加一个任务成功完成时的回调，如果任务已经成功完成，则直接执行回调函数
//传递给Done函数的参数与Reslove函数的参数相同
func (this *Future) Done(callback func(v interface{})) *Future {
	this.handleOneCallback(callback, CALLBACK_DONE)
	return this
}

//添加一个任务失败时的回调，如果任务已经失败，则直接执行回调函数
//传递给Fail函数的参数与Reject函数的参数相同
func (this *Future) Fail(callback func(v interface{})) *Future {
	this.handleOneCallback(callback, CALLBACK_FAIL)
	return this
}

//添加一个回调函数，该函数将在任务完成后执行，无论成功或失败
//传递给Always回调的参数根据成功或失败状态，与Reslove或Reject函数的参数相同
func (this *Future) Always(callback func(v interface{})) *Future {
	this.handleOneCallback(callback, CALLBACK_ALWAYS)
	return this
}

//for Pipe api, the new Promise object will be return
//New Promise task object should be started after current Promise be done or failed
//链式添加异步任务，可以同时定制Done或Fail状态下的链式异步任务，并返回一个新的异步对象。如果对此对象执行Done，Fail，Always操作，则新的回调函数将会被添加到链式的异步对象中
//如果调用的参数超过2个，那第2个以后的参数将会被忽略
//Pipe只能调用一次，第一次后的调用将被忽略
func (this *Future) Pipe(callbacks ...(func(v interface{}) *Future)) (result *Future, ok bool) {
	if len(callbacks) == 0 ||
		(len(callbacks) == 1 && callbacks[0] == nil) ||
		(len(callbacks) > 1 && callbacks[0] == nil && callbacks[1] == nil) {
		result = this
		fmt.Println("return false")
		return
	}

	this.oncePipe.Do(func() {
		for {
			v := this.val()
			r := (*PromiseResult)(v.r)
			if r != nil {
				result = this
				if r.Typ == RESULT_SUCCESS && callbacks[0] != nil {
					result = (callbacks[0](r.Result))
				} else if r.Typ == RESULT_FAILURE && len(callbacks) > 1 && callbacks[1] != nil {
					result = (callbacks[1](r.Result))
				}
			} else {
				newVal := *v
				newVal.pipeDoneTask = callbacks[0]
				if len(callbacks) > 1 {
					newVal.pipeFailTask = callbacks[1]
				}
				newVal.pipePromise = NewPromise()
				//通过CAS操作检测Future对象的原始状态未发生改变，否则需要重试
				if atomic.CompareAndSwapPointer(&this.v, unsafe.Pointer(v), unsafe.Pointer(&newVal)) {
					result = newVal.pipePromise.Future
					break
				}
			}
		}
		ok = true
	})
	return
}

func (this *Future) result() *PromiseResult {
	val := this.val()
	return (*PromiseResult)(val.r)
}

func (this *Future) val() *futureVal {
	r := atomic.LoadPointer(&this.v)
	return (*futureVal)(r)
}

//处理单个回调函数的添加请求
func (this *Future) handleOneCallback(callback func(v interface{}), t callbackType) {
	if callback == nil {
		return
	}

	for {
		v := this.val()
		r := (*PromiseResult)(v.r)
		if r == nil {
			newVal := *v
			switch t {
			case CALLBACK_DONE:
				newVal.dones = append(newVal.dones, callback)
			case CALLBACK_FAIL:
				newVal.fails = append(newVal.fails, callback)
			case CALLBACK_ALWAYS:
				newVal.always = append(newVal.always, callback)
			}
			if atomic.CompareAndSwapPointer(&this.v, unsafe.Pointer(v), unsafe.Pointer(&newVal)) {
				break
			}
		} else {
			if (t == CALLBACK_DONE && r.Typ == RESULT_SUCCESS) ||
				(t == CALLBACK_FAIL && r.Typ == RESULT_FAILURE) ||
				(t == CALLBACK_ALWAYS && r.Typ != RESULT_CANCELLED) {
				callback(r.Result)
			}
			break
		}
	}
}
