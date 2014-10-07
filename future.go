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

//pipe presents a promise that will be chain call
type pipe struct {
	pipeDoneTask, pipeFailTask func(v interface{}) *Future
	pipePromise                *Promise
}

//getPipe returns piped Future task function and pipe Promise by the status of current Promise.
func (this *pipe) getPipe(isResolved bool) (func(v interface{}) *Future, *Promise) {
	if isResolved {
		return this.pipeDoneTask, this.pipePromise
	} else {
		return this.pipeFailTask, this.pipePromise
	}
}

//futureVal stores the internal state of Future.
type futureVal struct {
	dones, fails, always []func(v interface{})
	pipes                []*pipe
	r                    unsafe.Pointer
}

//Future provides a read-only view of promise,
//the value is set by using Resolve, Reject and Cancel methods of related Promise
type Future struct {
	Id    int //Id can be used as identity of Future
	chOut chan *PromiseResult
	chEnd chan struct{}
	//指向futureVal的指针，程序要保证该指针指向的对象内容不会发送变化，任何变化都必须生成新对象并通过原子操作更新指针，以避免lock
	v            unsafe.Pointer
	cancelStatus int32
}

//RequestCancel request to cancel the promise
//It don't mean the promise be surely cancelled, please refer to canceller.RequestCancel()
func (this *Future) RequestCancel() bool {
	ccstatus := atomic.LoadInt32(&this.cancelStatus)
	if ccstatus == 0 {
		atomic.CompareAndSwapInt32(&this.cancelStatus, 0, 1)
		return true
	} else {
		return false
	}
}

//IsCancelled returns true if the promise is cancelled, otherwise false
func (this *Future) IsCancelled() bool {
	ccstatus := atomic.LoadInt32(&this.cancelStatus)
	return ccstatus == 2
}

//GetChan returns a channel than can be used to receive result of Promise
func (this *Future) GetChan() chan *PromiseResult {
	return this.chOut
}

//Get will block current goroutines until the Future is resolved/rejected/cancelled.
//If Future is resolved, value and nil will be returned
//If Future is rejected, nil and error will be returned.
//If Future is cancelled, nil and CANCELLED error will be returned.
func (this *Future) Get() (val interface{}, err error) {
	<-this.chEnd
	return getFutureReturnVal(this.result())
}

//GetOrTimeout is similar to Get(), but GetOrTimeout will not block after timeout.
//If GetOrTimeout returns with a timeout, timeout value will be true in return values.
//The unit of paramter is millisecond.
func (this *Future) GetOrTimeout(mm int) (val interface{}, err error, timout bool) {
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

//Done registers a callback function that will be called when Promise is resolved.
//If promise is already resolved, the callback will immediately called.
//The value of Promise will be paramter of Done callback function.
func (this *Future) Done(callback func(v interface{})) *Future {
	this.handleOneCallback(callback, CALLBACK_DONE)
	return this
}

//Fail registers a callback function that will be called when Promise is rejected.
//If promise is already rejected, the callback will immediately called.
//The error of Promise will be paramter of Fail callback function.
func (this *Future) Fail(callback func(v interface{})) *Future {
	this.handleOneCallback(callback, CALLBACK_FAIL)
	return this
}

//Always register a callback function that will be called when Promise is rejected or resolved.
//If promise is already rejected or resolved, the callback will immediately called.
//According to the status of Promise, value or error will be paramter of Always callback function.
//Value is the paramter if Promise is resolved, or error is the paramter if Promise is rejected.
//Always callback will be not called if Promise be called.
func (this *Future) Always(callback func(v interface{})) *Future {
	this.handleOneCallback(callback, CALLBACK_ALWAYS)
	return this
}

//Pipe registers one or two functions that returns a Future, and returns a proxy of pipeline Future.
//First function will be called when Future is resolved, the returned Future will be as pipeline Future.
//Secondary function will be called when Futrue is rejected, the returned Future will be as pipeline Future.
func (this *Future) Pipe(callbacks ...(func(v interface{}) *Future)) (result *Future, ok bool) {
	if len(callbacks) == 0 ||
		(len(callbacks) == 1 && callbacks[0] == nil) ||
		(len(callbacks) > 1 && callbacks[0] == nil && callbacks[1] == nil) {
		result = this
		fmt.Println("return false")
		return
	}

	//this.oncePipe.Do(func() {
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
			newPipe := &pipe{}
			newPipe.pipeDoneTask = callbacks[0]
			if len(callbacks) > 1 {
				newPipe.pipeFailTask = callbacks[1]
			}
			newPipe.pipePromise = NewPromise()

			newVal := *v
			newVal.pipes = append(newVal.pipes, newPipe)
			//通过CAS操作检测Future对象的原始状态未发生改变，否则需要重试
			if atomic.CompareAndSwapPointer(&this.v, unsafe.Pointer(v), unsafe.Pointer(&newVal)) {
				result = newPipe.pipePromise.Future
				break
			}
		}
	}
	ok = true
	//})
	return
}

//result uses Atomic load to return result of the Future
func (this *Future) result() *PromiseResult {
	val := this.val()
	return (*PromiseResult)(val.r)
}

//val uses Atomic load to return state value of the Future
func (this *Future) val() *futureVal {
	r := atomic.LoadPointer(&this.v)
	return (*futureVal)(r)
}

//handleOneCallback registers a callback function
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

			//so use CAS to ensure that the state of Future is not changed,
			//if the state is changed, will retry CAS operation.
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
