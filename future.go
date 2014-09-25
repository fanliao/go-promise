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
	"time"
)

var Debug = false

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

//pipe presents the chain of promise
type pipe struct {
	pipeDoneTask, pipeFailTask func(v interface{}) *Future
	pipePromise                *Promise
}

//异步任务

//Promise describe an object that acts as a proxy for a result
//that is initially unknown, usually because the computation of its
//value is yet incomplete (refer to wikipedia).
type Promise struct {
	*Future
}

//Cancel表示任务正常完成

//Cancel method set the status of promise to cancelled
//if promise is cancelled, Get() will return nil and CancelledError
func (this *Promise) Cancel() (e error) {
	return this.setEnd(&PromiseResult{&CancelledError{}, RESULT_CANCELLED})
}

//Reslove表示任务正常完成

//Resolve method set the value of promise, and the status will be changed to resolved
//if promise is resolved, Get() will return the value
func (this *Promise) Resolve(v interface{}) (e error) {
	return this.setEnd(&PromiseResult{v, RESULT_SUCCESS})
}

//Reject表示任务失败

//Resolve method set the error value of promise, and the status will be changed to rejected
//if promise is rejected, Get() will return the error value
func (this *Promise) Reject(err error) (e error) {
	return this.setEnd(&PromiseResult{err, RESULT_FAILURE})
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

//Future代表一个异步任务的readonly-view

//Future provides a read-only view of promise, the value is set by using promise.Resolve, Reject and Cancel methods
type Future struct {
	Id                   int
	s                    status         //Promise的状态
	runChan              chan *cmd      //接受命令的chan
	r                    *PromiseResult //保存执行结果
	dones, fails, always []func(v interface{})
	pipe
	*canceller               //*PromiseCanceller              //执行Cancel功能的对象
	chEnd      chan struct{} //表示Promise已经结束的chan
}

type cmdType int

const (
	END cmdType = iota
	ENABLE_CANCELLER
	GET_CANCELLER
	REQUEST_CANCELLER
	PIPE
	CANCEL
	DONE
	FAIL
	ALWAYS
)

type status int

const (
	NEW status = iota
	RESOLVED
	REJECTED
	ENABLED_CANCELLER
	REQUESTED_CANCELLER
	CANCELLED
)

type cmd struct {
	typ   cmdType
	val   interface{}
	rChan chan interface{}
}

func (this *Promise) start() {
	go func() {
		for v := range this.runChan {
			switch v.typ {
			case END:
				v.rChan <- this.end(v.val.(*PromiseResult))
				return
			case GET_CANCELLER:
				v.rChan <- this.canceller
			case ENABLE_CANCELLER:
				this.enableCanceller()
			case REQUEST_CANCELLER:
				v.rChan <- this.requestCancel()
			case PIPE:
				v.rChan <- this.pipe((v.val.([]func(v interface{}) *Future))...)
			case DONE:
				this.handleOneCallback(v.val.(func(interface{})), CALLBACK_DONE)
			case FAIL:
				this.handleOneCallback(v.val.(func(interface{})), CALLBACK_FAIL)
			case ALWAYS:
				this.handleOneCallback(v.val.(func(interface{})), CALLBACK_ALWAYS)
			}
		}
	}()
}

//EnableCanceller set a Promise can be cancelled
func (this *Promise) EnableCanceller() *Promise {
	//execWithLock(this.lock, func() {
	select {
	case <-this.chEnd:
	case this.runChan <- &cmd{typ: ENABLE_CANCELLER, val: nil, rChan: nil}:
	}
	return this
}

//获取Canceller接口，在异步任务内可以通过此对象查询任务是否已经被取消

//Canceller provides a canceller related to future
//if Canceller return nil, the futrue cannot be cancelled
func (this *Future) Canceller() (c Canceller) {
	ch := make(chan interface{})
	select {
	case <-this.chEnd:
		return this.canceller
	case this.runChan <- &cmd{typ: GET_CANCELLER, val: nil, rChan: ch}:
		rv := <-ch
		return rv.(Canceller)
	}

}

//RequestCancel request to cancel the promise
//It don't mean the promise be surely cancelled
func (this *Future) RequestCancel() (r bool) {
	ch := make(chan interface{})
	select {
	case <-this.chEnd:
		return false
	case this.runChan <- &cmd{typ: REQUEST_CANCELLER, val: nil, rChan: ch}:
		c := <-ch
		return c.(bool)
	}
}

//判断任务是否已经被要求取消

//IsCancellationRequested returns true if the promise is requested to cancelled,
//otherwise false.
func (this *Future) IsCancellationRequested() (r bool) {
	c := this.Canceller()
	if c != nil {
		r = this.canceller.IsCancellationRequested()
	} else {
		r = false
	}
	printfln(this, "IsCancellationRequested? return %t", r)
	return
}

//获得任务是否已经被Cancel

//IsCancelled returns true if the promise is cancelled, otherwise false
func (this *Future) IsCancelled() (r bool) {
	c := this.Canceller()
	if c != nil {
		r = c.(*canceller).IsCancelled()
	} else {
		r = false
	}
	printfln(this, "IsCancelled? return %t", r)
	return
}

func (this *Future) result() (r *PromiseResult) {
	r = this.r
	printfln(this, "get result = %#v", r)
	return
}

//EnableCanceller set a Promise can be cancelled
func (this *Promise) enableCanceller() *Promise {
	if this.canceller == nil {
		this.canceller = &canceller{new(once), new(once)}
		printfln(this.Future, "enable canceller")
	}
	return this
}

//请求取消异步任务

//RequestCancel request to cancel the promise
//It don't mean the promise be surely cancelled
func (this *Future) requestCancel() (r bool) {
	if this.result() != nil || this.canceller == nil {
		r = false
	} else {
		this.canceller.RequestCancel()
		r = true
	}
	printfln(this, "Request Cancel, return %t", r)
	return
}

func (this *Future) GetChan() chan *PromiseResult {
	c := make(chan *PromiseResult)
	go func() {
		<-this.chEnd
		c <- this.result()
	}()
	return c
}

//Get函数将一直阻塞直到任务完成,并返回任务的结果
//如果任务已经完成，后续的Get将直接返回任务结果
func (this *Future) Get() (r interface{}, e error) {
	<-this.chEnd
	r, e = getFutureReturnVal(this.result()) //r, typ
	printfln(this, "get future result, r = %#v, e = %#v", r, e)
	return
}

//Get函数将一直阻塞直到任务完成或超过指定的Timeout时间
//如果任务已经完成，后续的Get将直接返回任务结果
//mm的单位是毫秒
func (this *Future) GetOrTimeout(mm int) (r interface{}, e error, t bool) {
	if mm == 0 {
		mm = 10
	} else {
		mm = mm * 1000 * 1000
	}

	select {
	case <-time.After((time.Duration)(mm) * time.Nanosecond):
		r, e, t = nil, nil, true
	case <-this.chEnd:
		t = false
		r, e = getFutureReturnVal(this.result()) //r, typ
	}
	printfln(this, "get future result or timeout, r = %#v, e = %#v, timeout = %#t", r, e, t)
	return
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
	this.addOneCallback(callback, CALLBACK_DONE)
	return this
}

//添加一个任务失败时的回调，如果任务已经失败，则直接执行回调函数
//传递给Fail函数的参数与Reject函数的参数相同
func (this *Future) Fail(callback func(v interface{})) *Future {
	this.addOneCallback(callback, CALLBACK_FAIL)
	return this
}

//添加一个回调函数，该函数将在任务完成后执行，无论成功或失败
//传递给Always回调的参数根据成功或失败状态，与Reslove或Reject函数的参数相同
func (this *Future) Always(callback func(v interface{})) *Future {
	this.addOneCallback(callback, CALLBACK_ALWAYS)
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
		return
	}

	ch := make(chan interface{})
	select {
	case <-this.chEnd:
		r := this.r
		//如果chEnd能够读，那么说明Promise已经处于end状态
		if r.Typ == RESULT_SUCCESS && callbacks[0] != nil {
			result = (callbacks[0](r.Result))
		} else if r.Typ == RESULT_FAILURE && len(callbacks) > 1 && callbacks[1] != nil {
			result = (callbacks[1](r.Result))
		} else {
			result = this
		}
	case this.runChan <- &cmd{typ: PIPE, val: callbacks, rChan: ch}:
		//如果能写入runChan，那么说明Promise还未end
		result = (<-ch).(*Future)
		ok = true
	}
	return

}

//for Pipe api, the new Promise object will be return
//New Promise task object should be started after current Promise be done or failed
//链式添加异步任务，可以同时定制Done或Fail状态下的链式异步任务，并返回一个新的异步对象。如果对此对象执行Done，Fail，Always操作，则新的回调函数将会被添加到链式的异步对象中
//如果调用的参数超过2个，那第2个以后的参数将会被忽略
//Pipe只能调用一次，第一次后的调用将被忽略
func (this *Future) addOneCallback(callback func(v interface{}), t callbackType) *Future {
	if callback == nil {
		return this
	}

	var ct cmdType
	switch t {
	case CALLBACK_DONE:
		ct = DONE
	case CALLBACK_FAIL:
		ct = FAIL
	case CALLBACK_ALWAYS:
		ct = ALWAYS
	}

	select {
	case <-this.chEnd:
		//如果chEnd能够读，那么说明Promise已经处于end状态
		r := this.r
		if r.Typ == RESULT_SUCCESS && (t == CALLBACK_DONE || t == CALLBACK_ALWAYS) {
			callback(r.Result)
		} else if r.Typ == RESULT_FAILURE && (t == CALLBACK_FAIL || t == CALLBACK_ALWAYS) {
			callback(r.Result)
		}
	case this.runChan <- &cmd{typ: ct, val: callback, rChan: nil}:
		//如果能写入runChan，那么说明Promise还未end
	}
	return this

}

//完成一个任务
func (this *Promise) setEnd(r *PromiseResult) (e error) { //r *PromiseResult) {
	ch := make(chan interface{})
	select {
	case <-this.chEnd:
		//如果chEnd能够读，那么说明Promise已经处于end状态
	case this.runChan <- &cmd{typ: END, val: r, rChan: ch}:
		//如果能写入runChan，那么说明Promise还未end
		r := <-ch
		if r == nil {
			e = nil
		} else {
			e = (<-ch).(error)
		}
	}
	return
}

type canceller struct {
	isRequested *once
	isCancelled *once
}

//Cancel任务
func (this *canceller) RequestCancel() {
	this.isRequested.Done()
}

//已经被要求取消任务
func (this *canceller) IsCancellationRequested() (r bool) {
	return this.isRequested.IsDone()
}

//设置任务已经被Cancel
func (this *canceller) SetCancelled() {
	this.isCancelled.Done()
}

//任务已经被Cancel
func (this *canceller) IsCancelled() (r bool) {
	return this.isCancelled.IsDone()
}

//完成一个任务
func (this *Promise) end(r *PromiseResult) (e error) { //r *PromiseResult) {
	defer func() {
		if err := getError(recover()); err != nil {
			e = err
			printfln(this.Future, "\nerror in end(): %#v", err)
			e = newErrorWithStacks(e)
			panic(e)
		}
	}()
	if this.r != nil {
		e = errors.New("Cannot resolve/reject/cancel more than once")
	}

	if r.Typ == RESULT_SUCCESS {
		printfln(this.Future, "set future result %#v", r)
	} else {
		printfln(this.Future, "set future result with error or cancelled %#v", r.Typ)
	}

	this.setResult(r)
	//让Get函数可以返回
	close(this.chEnd)

	//任务完成后调用回调函数
	printfln(this.Future, "before call callback %#v %#v %#v", this.dones, this.fails, this.always)
	execCallback(r, this.dones, this.fails, this.always)

	pipeTask, pipePromise := this.getPipe(r.Typ == RESULT_SUCCESS)
	this.startPipe(r, pipeTask, pipePromise)

	return
}

func (this *Promise) pipe(callbacks ...(func(v interface{}) *Future)) (result *Future) {
	this.pipeDoneTask = callbacks[0]
	if len(callbacks) > 1 {
		this.pipeFailTask = callbacks[1]
	}
	this.pipePromise = NewPromise()
	result = this.pipePromise.Future
	return
}

//set this.r
func (this *Promise) setResult(r *PromiseResult) {
	this.r = r
}

//返回与链式调用相关的对象
func (this *Future) getPipe(isResolved bool) (func(v interface{}) *Future, *Promise) {
	if isResolved {
		return this.pipeDoneTask, this.pipePromise
	} else {
		return this.pipeFailTask, this.pipePromise
	}
}

func (this *Future) startPipe(r *PromiseResult, pipeTask func(v interface{}) *Future, pipePromise *Promise) {
	//处理链式异步任务
	//var f *Future
	if pipeTask != nil {
		f := pipeTask(r.Result)
		f.Done(func(v interface{}) {
			pipePromise.Resolve(v)
		}).Fail(func(v interface{}) {
			pipePromise.Reject(getError(v))
		})
	}

}

//执行回调函数
func execCallback(r *PromiseResult, dones []func(v interface{}), fails []func(v interface{}), always []func(v interface{})) {
	var callbacks []func(v interface{})
	if r.Typ == RESULT_SUCCESS {
		callbacks = dones
	} else if r.Typ == RESULT_FAILURE {
		callbacks = fails
	} else {
		return
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
	pendingAction := func() {
		switch t {
		case CALLBACK_DONE:
			this.dones = append(this.dones, callback)
		case CALLBACK_FAIL:
			this.fails = append(this.fails, callback)
		case CALLBACK_ALWAYS:
			this.always = append(this.always, callback)
		}
		printfln(this, "add pending action, callback is %#v, type is %#v", callback, t)
	}
	finalAction := func(r *PromiseResult) {
		if (t == CALLBACK_DONE && r.Typ == RESULT_SUCCESS) ||
			(t == CALLBACK_FAIL && r.Typ == RESULT_FAILURE) ||
			(t == CALLBACK_ALWAYS && r.Typ != RESULT_CANCELLED) {
			printfln(this, "before call final action, r isnull? type is %#v", r == nil, t)
			callback(r.Result)
			printfln(this, "after call final action, r isnull? type is %#v", r == nil, t)
		}
	}
	if f := this.addCallback(pendingAction, finalAction); f != nil {
		f()
	}
}

//添加回调函数的框架函数
func (this *Future) addCallback(pendingAction func(), finalAction func(*PromiseResult)) (fun func()) {
	r := this.result()
	println("addcallback", r)
	if r == nil {
		pendingAction()
		fun = nil
	} else {
		//fun = func() { finalAction(r) }
	}

	return
}

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
			//println("cancel", r)
			pr.Cancel()
		} else {
			if err == nil {
				//println("resolve", r, stack)
				pr.Resolve(r)
			} else {
				//println("reject1===", err, "\n")
				pr.Reject(err)
			}
		}
	} else {
		go func() {
			r, err := action()
			if pr.IsCancelled() {
				//println("cancel", r)
				pr.Cancel()
			} else {
				if err == nil {
					//println("resolve", r, stack)
					pr.Resolve(r)
				} else {
					//println("reject1===", err, "\n")
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
	f := &Promise{
		&Future{
			rand.Int(),
			NEW,
			make(chan *cmd),
			nil,
			make([]func(v interface{}), 0, 8),
			make([]func(v interface{}), 0, 8),
			make([]func(v interface{}), 0, 4),
			pipe{}, &canceller{new(once), new(once)},
			//make(chan *PromiseResult),
			make(chan struct{}),
		},
	}
	f.start()
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
			//println("get err")
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
					//println("reject2", newErrorWithStacks(e))
					nf.Reject(newErrorWithStacks(e))
				}
			}()
			j := 0
			//println("start for")
			for {
				select {
				case r := <-chFails:
					//println("get err")
					rs[r.i] = getError(r.result)
				case r := <-chDones:
					//println("get return", r)
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
					//println("receive all")
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
			//println("exit start")

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
	f := NewPromise()
	rs := make([]interface{}, len(fs))
	errs := make([]error, 0, len(fs))

	if len(fs) == 0 {
		f.Resolve([]interface{}{})
	} else {
		go func() {
			allOk := true
			cancelRequested := false
			for i, f := range fs {
				//if a future be failure, then will try to cancel other futures
				if !allOk && !cancelRequested {
					for j := i; j < len(fs); j++ {
						if c := fs[j].Canceller(); c != nil {
							fs[j].RequestCancel()
						}
					}
					cancelRequested = true
					//have capture the error in before Future, so will discards the other Future
					break
				}
				r, err := f.Get()
				if err != nil {
					allOk = false
					errs = append(errs, err)
					break
				}
				rs[i] = r

			}

			if allOk {
				f.Resolve(rs)
			} else {
				//println("whenall reject", errs)
				e := newAggregateError("Error appears in WhenAll:", errs)
				//println("whenall reject2", e.Error())
				f.Reject(e)
			}
		}()
	}

	return f.Future
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
		//println(name, file + ":", line)
		writeStrings(buf, []string{name, " ", file, ":", strconv.Itoa(line), "\n"})
	}
	return errors.New(buf.String())
}

func writeStrings(buf *bytes.Buffer, strings []string) {
	for _, s := range strings {
		buf.WriteString(s)
	}
}

func println(a ...interface{}) (n int, err error) {
	if Debug {
		return fmt.Println(a...)
	} else {
		return 0, nil
	}
}

func printf(this *Future, format string, a ...interface{}) (n int, err error) {
	if Debug {
		log := fmt.Sprintf(format, a...)
		log = log + fmt.Sprintf(". promise id = %#v", this.Id)
		return fmt.Print(log)
	} else {
		return 0, nil
	}
}

func printfln(this *Future, format string, a ...interface{}) (n int, err error) {
	if Debug {
		log := fmt.Sprintf(format, a...)
		log = log + fmt.Sprintf(". promise id = %#v\n", this.Id)
		return fmt.Print(log)
	} else {
		return 0, nil
	}
}
