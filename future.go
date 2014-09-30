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
	"bytes"
	"errors"
	"fmt"
	"math/rand"
	"runtime"
	"strconv"
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

type resultType int

const (
	RESULT_SUCCESS resultType = iota
	RESULT_FAILURE
	RESULT_CANCELLED
)

//代表异步任务的结果

//PromiseResult presents the result of a promise
type PromiseResult struct {
	Result interface{} //result of the Promise
	Typ    resultType  //success, failure, or cancelled?
}

//处理链式调用

//pipe presents the chain promise
type pipe struct {
	pipeDoneTask, pipeFailTask func(v interface{}) *Future
	pipePromise                *Promise
}

//异步任务

//Promise describe an object that acts as a proxy for a result
//that is initially unknown, usually because the computation of its
//value is yet incomplete (refer to wikipedia).
type Promise struct {
	onceEnd *sync.Once
	*Future
}

//Cancel表示任务正常完成

//Cancel method set the status of promise to cancelled
//if promise is cancelled, Get() will return nil and CancelledError
func (this *Promise) Cancel() (e error) {
	return this.end(&PromiseResult{&CancelledError{}, RESULT_CANCELLED})
}

//Reslove表示任务正常完成

//Resolve method set the value of promise, and the status will be changed to resolved
//if promise is resolved, Get() will return the value
func (this *Promise) Resolve(v interface{}) (e error) {
	return this.end(&PromiseResult{v, RESULT_SUCCESS})
}

//Reject表示任务失败

//Resolve method set the error value of promise, and the status will be changed to rejected
//if promise is rejected, Get() will return the error value
func (this *Promise) Reject(err error) (e error) {
	return this.end(&PromiseResult{err, RESULT_FAILURE})
}

//EnableCanceller set a Promise can be cancelled
func (this *Promise) EnableCanceller() *Promise {
	atomic.CompareAndSwapInt32(&this.cancelStatus, -1, 0)
	//ccstatus := atomic.LoadInt32(&this.cancelStatus)
	//if ccstatus >= 0 {
	//	return &canceller{&this.cancelStatus}
	//}
	//if this.canceller == nil {
	//	this.canceller = &canceller{}
	//}
	return this
}

//添加一个任务成功完成时的回调，如果任务已经成功完成，则直接执行回调函数
//传递给Done函数的参数与Reslove函数的参数相同

//Done register a callback function for resolved status
//if promise is already resolved, the callback will immediately called.
func (this *Promise) Done(callback func(v interface{})) *Promise {
	this.Future.Done(callback)
	return this
}

//添加一个任务失败时的回调，如果任务已经失败，则直接执行回调函数
//传递给Fail函数的参数与Reject函数的参数相同

//Fail register a callback function for rejected status
//if promise is already rejected, the callback will immediately called.
func (this *Promise) Fail(callback func(v interface{})) *Promise {
	this.Future.Fail(callback)
	return this
}

//添加一个回调函数，该函数将在任务完成后执行，无论成功或失败
//传递给Always回调的参数根据成功或失败状态，与Reslove或Reject函数的参数相同

//Always register a callback function for rejected and resolved status
//if promise is already rejected or resolved, the callback will immediately called.
func (this *Promise) Always(callback func(v interface{})) *Promise {
	this.Future.Always(callback)
	return this
}

//Cancel一个任务的interface

//Canceller used to check if the promise be requested to cancel, and set the cancelled status
type Canceller interface {
	IsCancellationRequested() bool
	SetCancelled()
}

//保存Future状态数据的结构
type futureVal struct {
	dones, fails, always []func(v interface{})
	pipe
	r unsafe.Pointer
}

//Future代表一个异步任务的readonly-view

//Future provides a read-only view of promise, the value is set by using promise.Resolve, Reject and Cancel methods
type Future struct {
	Id       int
	oncePipe *sync.Once
	//lock     *sync.Mutex
	chOut chan *PromiseResult
	chEnd chan struct{}
	//dones, fails, always []func(v interface{})
	//pipe
	////r          *PromiseResult
	//r unsafe.Pointer
	//指向futureVal的指针，程序要保证该指针指向的对象内容不会发送变化，任何变化都必须生成新对象并通过原子操作更新指针，以避免lock
	v unsafe.Pointer
	//*canceller //*PromiseCanceller
	cancelStatus int32
}

func (this *Future) result() *PromiseResult {
	//r := atomic.LoadPointer(&this.r)
	val := this.val()
	return (*PromiseResult)(val.r)
}

func (this *Future) val() *futureVal {
	r := atomic.LoadPointer(&this.v)
	return (*futureVal)(r)
}

//获取Canceller接口，在异步任务内可以通过此对象查询任务是否已经被取消

//Canceller provides a canceller related to future
//if Canceller return nil, the futrue cannot be cancelled
func (this *Future) Canceller() Canceller {
	ccstatus := atomic.LoadInt32(&this.cancelStatus)
	if ccstatus >= 0 {
		return &canceller{&this.cancelStatus}
	} else {
		return nil
	}
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
	//if this.result() != nil || this.canceller == nil {
	//	return false
	//} else {
	//	this.canceller.RequestCancel()
	//	return true
	//}
}

//判断任务是否已经被要求取消

//IsCancellationRequested returns true if the promise is requested to cancelled,
//otherwise false.
func (this *Future) IsCancellationRequested() bool {
	c := this.Canceller()
	if c != nil {
		return c.IsCancellationRequested()
	} else {
		return false
	}
}

//设置任务为已被取消状态

//SetCancelled set the status that presents the promise is cancelled,
func (this *Future) SetCancelled() {
	c := this.Canceller()
	if c != nil && this.result() == nil {
		c.SetCancelled()
	}
}

//获得任务是否已经被Cancel

//IsCancelled returns true if the promise is cancelled, otherwise false
func (this *Future) IsCancelled() bool {
	ccstatus := atomic.LoadInt32(&this.cancelStatus)
	return ccstatus == 2
	//if this.canceller != nil {
	//	return this.canceller.IsCancelled()
	//} else {
	//	return false
	//}
}

func (this *Future) GetChan() chan *PromiseResult {
	return this.chOut
}

//Get函数将一直阻塞直到任务完成,并返回任务的结果
//如果任务已经完成，后续的Get将直接返回任务结果
func (this *Future) Get() (interface{}, error) {
	<-this.chEnd
	//if fr, ok := <-this.chOut; ok {
	//	return getFutureReturnVal(fr) //fr.Result, fr.Typ
	//} else {
	return getFutureReturnVal(this.result()) //r, typ
	//}
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
		//if ok {
		//	r, err := getFutureReturnVal(fr)
		//	return r, err, false
		//} else {
		r, err := getFutureReturnVal(this.result())
		return r, err, false
		//}

	}
}

func getFutureReturnVal(r *PromiseResult) (interface{}, error) {
	if r.Typ == RESULT_SUCCESS {
		return r.Result, nil
	} else if r.Typ == RESULT_FAILURE {
		return nil, getError(r.Result)
	} else {
		return nil, getError(r.Result) //&CancelledError{}
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
		//r := this.result()
		//if r != nil {
		//	result = this
		//	if r.Typ == RESULT_SUCCESS && callbacks[0] != nil {
		//		result = (callbacks[0](r.Result))
		//	} else if r.Typ != RESULT_FAILURE && len(callbacks) > 1 && callbacks[1] != nil {
		//		result = (callbacks[1](r.Result))
		//	}
		//} else {
		//	execWithLock(this.lock, func() {
		//		this.pipeDoneTask = callbacks[0]
		//		if len(callbacks) > 1 {
		//			this.pipeFailTask = callbacks[1]
		//		}
		//		this.pipePromise = NewPromise()
		//		result = this.pipePromise.Future
		//	})
		//}
		ok = true
		fmt.Println("return true")
	})
	return
}

type canceller struct {
	status *int32
}

//Cancel任务
func (this *canceller) RequestCancel() {
	//只有当状态==0（表示初始状态）时才可以请求取消任务
	atomic.CompareAndSwapInt32(this.status, 0, 1)
}

//已经被要求取消任务
func (this *canceller) IsCancellationRequested() (r bool) {
	return atomic.LoadInt32(this.status) == 1
}

//设置任务已经被Cancel
func (this *canceller) SetCancelled() {
	atomic.StoreInt32(this.status, 2)
}

//任务已经被Cancel
func (this *canceller) IsCancelled() (r bool) {
	return atomic.LoadInt32(this.status) == 2
}

//完成一个任务
func (this *Promise) end(r *PromiseResult) (e error) { //r *PromiseResult) {
	defer func() {
		if err := getError(recover()); err != nil {
			e = err
			fmt.Println("\nerror in end():", err)
		}
	}()
	e = errors.New("Cannot resolve/reject/cancel more than once")
	this.onceEnd.Do(func() {
		//fmt.Println("send future result", r)
		for {
			v := this.val()
			newVal := *v
			newVal.r = unsafe.Pointer(r)

			if atomic.CompareAndSwapPointer(&this.v, unsafe.Pointer(v), unsafe.Pointer(&newVal)) {
				//fmt.Println("send", r)
				this.chOut <- r
				//让Get函数可以返回
				close(this.chEnd)
				//fmt.Println("close done", r)

				if r.Typ != RESULT_CANCELLED {
					//任务完成后调用回调函数
					execCallback(r, v.dones, v.fails, v.always)

					//fmt.Println("after callback", r)
					pipeTask, pipePromise := v.getPipe(r.Typ == RESULT_SUCCESS)
					startPipe(r, pipeTask, pipePromise)
				}
				e = nil
				break
			}
			//this.setResult(r)

			////fmt.Println("send", r)
			//this.chOut <- r
			////让Get函数可以返回
			//close(this.chEnd)
			////fmt.Println("close done", r)

			//if r.Typ != RESULT_CANCELLED {
			//	//任务完成后调用回调函数
			//	execCallback(r, this.dones, this.fails, this.always)

			//	//fmt.Println("after callback", r)
			//	pipeTask, pipePromise := this.getPipe(r.Typ == RESULT_SUCCESS)
			//	this.startPipe(pipeTask, pipePromise)
			//}
			//e = nil
		}
	})
	return
}

////set this.r
//func (this *Promise) setResult(r *PromiseResult) {
//	atomic.StorePointer(&this.r, unsafe.Pointer(r))
//}

//返回与链式调用相关的对象
func (this *futureVal) getPipe(isResolved bool) (func(v interface{}) *Future, *Promise) {
	//this.lock.Lock()
	//defer this.lock.Unlock()
	if isResolved {
		return this.pipeDoneTask, this.pipePromise
	} else {
		return this.pipeFailTask, this.pipePromise
	}
}

func startPipe(r *PromiseResult, pipeTask func(v interface{}) *Future, pipePromise *Promise) {
	//处理链式异步任务
	//var f *Future
	if pipeTask != nil {
		f := pipeTask(r.Result)
		f.Done(func(v interface{}) {
			pipePromise.Resolve(v)
		}).Fail(func(v interface{}) {
			pipePromise.Reject(getError(v))
		})
		//} else {
		//	f = this
	}

}

//执行回调函数
func execCallback(r *PromiseResult, dones []func(v interface{}), fails []func(v interface{}), always []func(v interface{})) {
	var callbacks []func(v interface{})
	if r.Typ == RESULT_SUCCESS {
		callbacks = dones
	} else {
		callbacks = fails
	}

	forFs := func(s []func(v interface{})) {
		forSlice(s, func(f func(v interface{})) { f(r.Result) })
	}

	forFs(callbacks)
	forFs(always)

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
	//pendingAction := func() {
	//	switch t {
	//	case CALLBACK_DONE:
	//		this.dones = append(this.dones, callback)
	//	case CALLBACK_FAIL:
	//		this.fails = append(this.fails, callback)
	//	case CALLBACK_ALWAYS:
	//		this.always = append(this.always, callback)
	//	}
	//}
	//finalAction := func(r *PromiseResult) {
	//	if (t == CALLBACK_DONE && r.Typ == RESULT_SUCCESS) ||
	//		(t == CALLBACK_FAIL && r.Typ == RESULT_FAILURE) ||
	//		(t == CALLBACK_ALWAYS && r.Typ != RESULT_CANCELLED) {
	//		callback(r.Result)
	//	}
	//}
	//if f := this.addCallback(pendingAction, finalAction); f != nil {
	//	f()
	//}
}

////添加回调函数的框架函数
//func (this *Future) addCallback(pendingAction func(), finalAction func(*PromiseResult)) (fun func()) {
//	r := this.result()
//	if r == nil {
//		execWithLock(this.lock, func() {
//			pendingAction()
//		})
//		fun = nil
//	} else {
//		fun = func() { finalAction(r) }
//	}

//	return
//}

//异步或同步执行一个函数。并以Future包装函数返回值返回
func Start(act interface{}, syncs ...bool) *Future {
	pr := NewPromise()
	if f, ok := act.(*Future); ok {
		return f
	}
	switch v := act.(type) {
	case func(Canceller) (interface{}, error):
		pr.EnableCanceller()
		_ = v
	case func(Canceller):
		pr.EnableCanceller()
		_ = v
	}

	action := getAct(pr, act)
	if syncs != nil && len(syncs) > 0 && !syncs[0] {
		r, err := action()
		if pr.IsCancelled() {
			//fmt.Println("cancel", r)
			pr.Cancel()
		} else {
			if err == nil {
				//fmt.Println("resolve", r, stack)
				pr.Resolve(r)
			} else {
				//fmt.Println("reject1===", err, "\n")
				pr.Reject(err)
			}
		}
	} else {
		go func() {
			r, err := action()
			if pr.IsCancelled() {
				//fmt.Println("cancel", r)
				pr.Cancel()
			} else {
				if err == nil {
					//fmt.Println("resolve", r, stack)
					pr.Resolve(r)
				} else {
					//fmt.Println("reject1===", err, "\n")
					pr.Reject(err)
				}
			}
		}()
	}

	return pr.Future
}

//执行一个函数或直接返回一个值，如果是可Cancel的函数，需要传递canceller对象
func getAct(pr *Promise, act interface{}) (f func() (r interface{}, err error)) {
	var (
		act1 func() (interface{}, error)
		act2 func(Canceller) (interface{}, error)
	)
	canCancel := false

	switch v := act.(type) {
	case func() (interface{}, error):
		act1 = v
	case func(Canceller) (interface{}, error):
		canCancel = true
		act2 = v
	case func():
		act1 = func() (interface{}, error) {
			v()
			return nil, nil
		}
	case func(Canceller):
		canCancel = true
		act2 = func(canceller Canceller) (interface{}, error) {
			v(canceller)
			return nil, nil
		}
	default:
		//r = v
		//return
	}

	var canceller Canceller = nil
	if pr != nil && canCancel {
		pr.EnableCanceller()
		canceller = pr.Canceller()
	}
	f = func() (r interface{}, err error) {
		return execute(canceller, act1, act2, canCancel)
	}
	return
}

func execute(canceller Canceller, act func() (interface{}, error), actCancel func(Canceller) (interface{}, error), canCancel bool) (r interface{}, err error) {

	defer func() {
		if e := recover(); e != nil {
			err = newErrorWithStacks(e)
		}
	}()

	if canCancel {
		r, err = actCancel(canceller)
	} else {
		r, err = act()
	}

	return
}

func Wrap(value interface{}) *Future {
	pr := NewPromise()
	if e, ok := value.(error); !ok {
		pr.Resolve(value)
	} else {
		pr.Reject(e)
	}

	return pr.Future
}

//Factory function for Promise
func NewPromise() *Promise {
	val := &futureVal{
		make([]func(v interface{}), 0, 8),
		make([]func(v interface{}), 0, 8),
		make([]func(v interface{}), 0, 4),
		pipe{}, nil,
	}
	f := &Promise{new(sync.Once),
		&Future{
			rand.Int(),
			new(sync.Once), //new(sync.Mutex),
			make(chan *PromiseResult, 1),
			make(chan struct{}),
			unsafe.Pointer(val),
			-1,
		},
	}
	return f
}

type anyPromiseResult struct {
	result interface{}
	i      int
}

//产生一个新的Promise，如果列表中任意1个Promise完成，则Promise完成, 否则将触发Reject，参数为包含所有Promise的Reject返回值的slice
func WhenAny(fs ...*Future) *Future {
	return WhenAnyTrue(nil, fs...)
}

//产生一个新的Promise，如果列表中任意1个Promise完成并且返回值符合条件，则Promise完成并返回true
//如果所有Promise完成并且返回值都不符合条件，则Promise完成并返回false,
//否则将触发Reject，参数为包含所有Promise的Reject返回值的slice
func WhenAnyTrue(predicate func(interface{}) bool, fs ...*Future) *Future {
	if predicate == nil {
		predicate = func(v interface{}) bool { return true }
	}

	nf, rs := NewPromise(), make([]interface{}, len(fs))
	chFails, chDones := make(chan anyPromiseResult), make(chan anyPromiseResult)

	go func() {
		for i, f := range fs {
			k := i
			f.Done(func(v interface{}) {
				//nf.Resolve(v)
				defer func() { _ = recover() }()
				chDones <- anyPromiseResult{v, k}
			}).Fail(func(v interface{}) {
				defer func() { _ = recover() }()
				chFails <- anyPromiseResult{v, k}
			})
		}
	}()

	//var result interface{}
	if len(fs) == 0 {
		nf.Resolve(nil)
	} else if len(fs) == 1 {
		select {
		case r := <-chFails:
			//fmt.Println("get err")
			errs := make([]error, 1)
			errs[0] = getError(r.result)
			nf.Reject(newAggregateError("Error appears in WhenAnyTrue:", errs))
		case r := <-chDones:
			if predicate(r.result) {
				nf.Resolve(r.result)
			} else {
				nf.Resolve(false)
			}
		}
	} else {
		go func() {
			defer func() {
				if e := recover(); e != nil {
					//fmt.Println("reject2", newErrorWithStacks(e))
					nf.Reject(newErrorWithStacks(e))
				}
			}()
			j := 0
			//fmt.Println("start for")
			for {
				select {
				case r := <-chFails:
					//fmt.Println("get err")
					rs[r.i] = getError(r.result)
				case r := <-chDones:
					//fmt.Println("get return", r)
					if predicate(r.result) {
						//try to cancel other futures
						for _, f := range fs {
							if c := f.Canceller(); c != nil {
								f.RequestCancel()
							}
						}

						//close the channel for avoid the send side be blocked
						closeChan := func(c chan anyPromiseResult) {
							defer func() { _ = recover() }()
							close(c)
						}
						closeChan(chDones)
						closeChan(chFails)

						//Resolve the future and return result
						nf.Resolve(r.result)
						return
					} else {
						rs[r.i] = r.result
					}
				}

				if j++; j == len(fs) {
					//fmt.Println("receive all")
					errs, k := make([]error, j), 0
					for _, r := range rs {
						switch val := r.(type) {
						case error:
							errs[k] = val
							k++
						default:
						}
					}
					if k > 0 {
						nf.Reject(newAggregateError("Error appears in WhenAnyTrue:", errs[0:j]))
					} else {
						nf.Resolve(false)
					}
					break
				}
			}
			//fmt.Println("exit start")

		}()
	}
	return nf.Future
}

func WaitAll(acts ...interface{}) (fu *Future) {
	pr := NewPromise()
	fu = pr.Future

	if len(acts) == 0 {
		pr.Resolve([]interface{}{})
		return
	}

	//paralActs := acts[0 : len(acts)-1]
	f1 := WhenAll(acts[0 : len(acts)-1]...)

	p := NewPromise()
	r, err := getAct(p, acts[len(acts)-1])()

	r1, err1 := f1.Get()
	if err != nil || err1 != nil {
		errs := newAggregateError("Error appears in WhenAll:", make([]error, 0, len(acts)))
		if err1 != nil {
			errs.InnerErrs = append(errs.InnerErrs, (err1.(*AggregateError).InnerErrs)...)
		}
		if err != nil {
			errs.InnerErrs = append(errs.InnerErrs, err)
		}
		pr.Reject(errs)
	} else {
		rs := r1.([]interface{})
		rs = append(rs, r)
		pr.Resolve(rs)
	}
	//}
	return
}

func WhenAll(acts ...interface{}) (fu *Future) {
	pr := NewPromise()
	fu = pr.Future

	if len(acts) == 0 {
		pr.Resolve([]interface{}{})
		return
	}

	fs := make([]*Future, len(acts))
	for i, act := range acts {
		fs[i] = Start(act)
	}
	fu = WhenAllFuture(fs...)
	return
}

//产生一个新的Future，如果列表中所有Future都成功完成，则Promise成功完成，否则失败
func WhenAllFuture(fs ...*Future) *Future {
	wf := NewPromise()
	rs := make([]interface{}, len(fs))

	if len(fs) == 0 {
		wf.Resolve([]interface{}{})
	} else {
		n := int32(len(fs))
		go func() {
			isCancelled := int32(0)
			for i, f := range fs {
				j := i
				f.Done(func(v interface{}) {
					rs[j] = v
					if atomic.AddInt32(&n, -1) == 0 {
						wf.Resolve(rs)
					}
				}).Fail(func(v interface{}) {
					if atomic.CompareAndSwapInt32(&isCancelled, 0, 1) {
						//try to cancel all futures
						for k, f1 := range fs {
							if k != j {
								f1.RequestCancel()
							}
						}

						errs := make([]error, 0, 1)
						errs = append(errs, v.(error))
						e := newAggregateError("Error appears in WhenAll:", errs)
						//fmt.Println("whenall reject2", e.Error())
						wf.Reject(e)
					}
				})
			}
		}()
	}

	return wf.Future
}

func execWithLock(lock *sync.Mutex, act func()) {
	lock.Lock()
	defer lock.Unlock()
	act()
}

func forSlice(s []func(v interface{}), f func(func(v interface{}))) {
	for _, e := range s {
		f(e)
	}
}

//Error handling struct and functions------------------------------
type stringer interface {
	String() string
}

func getError(i interface{}) (e error) {
	if i != nil {
		switch v := i.(type) {
		case error:
			e = v
		case string:
			e = errors.New(v)
		default:
			if s, ok := i.(stringer); ok {
				e = errors.New(s.String())
			} else {
				e = errors.New(fmt.Sprintf("%v", i))
			}
		}
	}
	return
}

type CancelledError struct {
}

func (e *CancelledError) Error() string {
	return "Task be cancelled"
}

type AggregateError struct {
	s         string
	InnerErrs []error
}

func (e *AggregateError) Error() string {
	if e.InnerErrs == nil {
		return e.s
	} else {
		buf := bytes.NewBufferString(e.s)
		buf.WriteString("\n\n")
		for i, ie := range e.InnerErrs {
			if ie == nil {
				continue
			}
			buf.WriteString("error appears in Future ")
			buf.WriteString(strconv.Itoa(i))
			buf.WriteString(": ")
			buf.WriteString(ie.Error())
			buf.WriteString("\n")
		}
		buf.WriteString("\n")
		return buf.String()
	}
}

func newAggregateError(s string, innerErrors []error) *AggregateError {
	return &AggregateError{newErrorWithStacks(s).Error(), innerErrors}
}

func newErrorWithStacks(i interface{}) (e error) {
	err := getError(i)
	buf := bytes.NewBufferString(err.Error())
	buf.WriteString("\n")

	pcs := make([]uintptr, 50)
	num := runtime.Callers(2, pcs)
	for _, v := range pcs[0:num] {
		fun := runtime.FuncForPC(v)
		file, line := fun.FileLine(v)
		name := fun.Name()
		//fmt.Println(name, file + ":", line)
		writeStrings(buf, []string{name, " ", file, ":", strconv.Itoa(line), "\n"})
	}
	return errors.New(buf.String())
}

func writeStrings(buf *bytes.Buffer, strings []string) {
	for _, s := range strings {
		buf.WriteString(s)
	}
}
