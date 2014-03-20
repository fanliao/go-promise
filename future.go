package promise

import (
	"errors"
	"fmt"
	"sync"
	"time"
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
type PromiseResult struct {
	result []interface{}
	typ    resultType
}

//处理链式调用
type pipe struct {
	pipeDoneTask, pipeFailTask func(v ...interface{}) *Future
	pipePromise                *Promise
}

//异步任务
type Promise struct {
	onceEnd *sync.Once
	*Future
}

//Reslove表示任务正常完成
func (this *Promise) Reslove(v ...interface{}) (e error) {
	return this.end(&PromiseResult{v, RESULT_SUCCESS})
}

//Reject表示任务失败
func (this *Promise) Reject(v ...interface{}) (e error) {
	return this.end(&PromiseResult{v, RESULT_FAILURE})
}

//Set a Promise can be cancelled
func (this *Promise) EnableCanceller() *Promise {
	if this.canceller == nil {
		this.canceller = &canceller{new(sync.Mutex), false, false}
	}
	return this
}

//添加一个任务成功完成时的回调，如果任务已经成功完成，则直接执行回调函数
//传递给Done函数的参数与Reslove函数的参数相同
func (this *Promise) Done(callback func(v ...interface{})) *Promise {
	this.Future.Done(callback)
	return this
}

//添加一个任务失败时的回调，如果任务已经失败，则直接执行回调函数
//传递给Fail函数的参数与Reject函数的参数相同
func (this *Promise) Fail(callback func(v ...interface{})) *Promise {
	this.Future.Fail(callback)
	return this
}

//添加一个回调函数，该函数将在任务完成后执行，无论成功或失败
//传递给Always回调的参数根据成功或失败状态，与Reslove或Reject函数的参数相同
func (this *Promise) Always(callback func(v ...interface{})) *Promise {
	this.Future.Always(callback)
	return this
}

//Cancel一个任务的interface
type Canceller interface {
	IsCancellationRequested() bool
	SetIsCancelled()
}

//Future代表一个异步任务的readonly-view
type Future struct {
	oncePipe             *sync.Once
	lock                 *sync.Mutex
	chOut                chan *PromiseResult
	dones, fails, always []func(v ...interface{})
	pipe
	r          *PromiseResult
	*canceller //*PromiseCanceller
}

//获取Canceller接口，在异步任务内可以通过此对象查询任务是否已经被取消
func (this *Future) Canceller() Canceller {
	return this.canceller
}

//取消异步任务
func (this *Future) Cancel() bool {
	if this.canceller != nil {
		this.canceller.Cancel()
		return true
	} else {
		return false
	}
}

//获得任务是否已经被要求取消
func (this *Future) IsCancellationRequested() bool {
	if this.canceller != nil {
		return this.canceller.IsCancellationRequested()
	} else {
		return false
	}
}

//设置任务为已被取消状态
func (this *Future) SetIsCancelled() {
	if this.canceller != nil {
		this.canceller.SetIsCancelled()
	}
}

//获得任务是否已经被Cancel
func (this *Future) IsCancelled() bool {
	if this.canceller != nil {
		return this.canceller.IsCancelled()
	} else {
		return false
	}
}

//Get函数将一直阻塞直到任务完成,并返回任务的结果
//如果任务已经完成，后续的Get将直接返回任务结果
func (this *Future) Get() ([]interface{}, resultType) {
	if fr, ok := <-this.chOut; ok {
		return fr.result, fr.typ
	} else {
		r, typ := this.r.result, this.r.typ
		return r, typ
	}
}

//Get函数将一直阻塞直到任务完成或超过指定的Timeout时间
//如果任务已经完成，后续的Get将直接返回任务结果
//mm的单位是毫秒
func (this *Future) GetOrTimeout(mm int) ([]interface{}, resultType, bool) {
	if mm == 0 {
		mm = 10
	} else {
		mm = mm * 1000 * 1000
	}
	select {
	case <-time.After((time.Duration)(mm) * time.Nanosecond):
		return nil, 0, true
	case fr, ok := <-this.chOut:
		if ok {
			return fr.result, fr.typ, false
		} else {
			r, typ := this.r.result, this.r.typ
			return r, typ, false
		}

	}
}

//添加一个任务成功完成时的回调，如果任务已经成功完成，则直接执行回调函数
//传递给Done函数的参数与Reslove函数的参数相同
func (this *Future) Done(callback func(v ...interface{})) *Future {
	this.handleOneCallback(callback, CALLBACK_DONE)
	return this
}

//添加一个任务失败时的回调，如果任务已经失败，则直接执行回调函数
//传递给Fail函数的参数与Reject函数的参数相同
func (this *Future) Fail(callback func(v ...interface{})) *Future {
	this.handleOneCallback(callback, CALLBACK_FAIL)
	return this
}

//添加一个回调函数，该函数将在任务完成后执行，无论成功或失败
//传递给Always回调的参数根据成功或失败状态，与Reslove或Reject函数的参数相同
func (this *Future) Always(callback func(v ...interface{})) *Future {
	this.handleOneCallback(callback, CALLBACK_ALWAYS)
	return this
}

//for Pipe api, the new Promise object will be return
//New Promise task object should be started after current Promise be done or failed
//链式添加异步任务，可以同时定制Done或Fail状态下的链式异步任务，并返回一个新的异步对象。如果对此对象执行Done，Fail，Always操作，则新的回调函数将会被添加到链式的异步对象中
//如果调用的参数超过2个，那第2个以后的参数将会被忽略
//Pipe只能调用一次，第一次后的调用将被忽略
func (this *Future) Pipe(callbacks ...(func(v ...interface{}) *Future)) (result *Future, ok bool) {
	if len(callbacks) == 0 ||
		(len(callbacks) == 1 && callbacks[0] == nil) ||
		(len(callbacks) > 1 && callbacks[0] == nil && callbacks[1] == nil) {
		result = this
		return
	}

	this.oncePipe.Do(func() {
		this.lock.Lock()
		defer this.lock.Unlock()
		if this.r != nil {
			f := this

			if this.r.typ == RESULT_SUCCESS && callbacks[0] != nil {
				f = (callbacks[0](this.r.result...))
			} else if this.r.typ != RESULT_FAILURE && len(callbacks) > 1 && callbacks[1] != nil {
				f = (callbacks[1](this.r.result...))
			}
			result = f
		} else {
			this.pipeDoneTask = callbacks[0]
			if len(callbacks) > 1 {
				this.pipeFailTask = callbacks[1]
			}
			this.pipePromise = NewPromise()
			result = this.pipePromise.Future
		}
		ok = true
	})
	return
}

type canceller struct {
	lock        *sync.Mutex
	isRequested bool
	isCancelled bool
}

//Cancel任务
func (this *canceller) Cancel() {
	this.lock.Lock()
	defer this.lock.Unlock()
	this.isRequested = true
}

//已经被要求取消任务
func (this *canceller) IsCancellationRequested() bool {
	this.lock.Lock()
	defer this.lock.Unlock()
	return this.isRequested
}

//设置任务已经被Cancel
func (this *canceller) SetIsCancelled() {
	this.lock.Lock()
	defer this.lock.Unlock()
	this.isCancelled = true
}

//任务已经被Cancel
func (this *canceller) IsCancelled() bool {
	this.lock.Lock()
	defer this.lock.Unlock()
	return this.isCancelled
}

//完成一个任务
func (this *Promise) end(r *PromiseResult) (e error) { //r *PromiseResult) {
	defer func() {
		e = getError(recover())
	}()
	e = errors.New("Cannoy resolve/reject more than once")
	this.onceEnd.Do(func() {
		//r := <-this.chIn
		this.setResult(r)

		//让Get函数可以返回
		this.chOut <- r
		close(this.chOut)

		//任务完成后调用回调函数
		execCallback(r, this.dones, this.fails, this.always)

		this.startPipe()
		e = nil
	})
	return
}

//返回与链式调用相关的对象
func (this *Future) getPipe(isResolved bool) (func(v ...interface{}) *Future, *Promise) {
	this.lock.Lock()
	defer this.lock.Unlock()
	if isResolved {
		return this.pipeDoneTask, this.pipePromise
	} else {
		return this.pipeFailTask, this.pipePromise
	}
}

//set this.r
func (this *Promise) setResult(r *PromiseResult) {
	this.lock.Lock()
	defer this.lock.Unlock()
	this.r = r
}

func (this *Future) startPipe() {
	//处理链式异步任务
	pipeTask, pipePromise := this.getPipe(this.r.typ == RESULT_SUCCESS)
	var f *Future
	if pipeTask != nil {
		f = pipeTask(this.r.result...)
	} else {
		f = this
	}

	f.Done(func(v ...interface{}) {
		pipePromise.Reslove(v...)
	}).Fail(func(v ...interface{}) {
		pipePromise.Reject(v...)
	})

}

//执行回调函数
func execCallback(r *PromiseResult, dones []func(v ...interface{}), fails []func(v ...interface{}), always []func(v ...interface{})) {
	var callbacks []func(v ...interface{})
	if r.typ == RESULT_SUCCESS {
		callbacks = dones
		fmt.Println(r, "ok")
	} else {
		callbacks = fails
		fmt.Println(r, "fail")
	}

	forFs := func(s []func(v ...interface{})) {
		forSlice(s, func(f func(v ...interface{})) { f(r.result...) })
	}

	forFs(callbacks)
	forFs(always)

}

//处理单个回调函数的添加请求
func (this *Future) handleOneCallback(callback func(v ...interface{}), t callbackType) {
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
	}
	finalAction := func(r *PromiseResult) {
		if (t == CALLBACK_DONE && r.typ == RESULT_SUCCESS) ||
			(t == CALLBACK_FAIL && r.typ == RESULT_FAILURE) ||
			(t == CALLBACK_ALWAYS) {
			callback(r.result...)
		}
	}
	if f := this.addCallback(pendingAction, finalAction); f != nil {
		f()
	}
}

//添加回调函数的框架函数
func (this *Future) addCallback(pendingAction func(), finalAction func(*PromiseResult)) func() {
	this.lock.Lock()
	defer this.lock.Unlock()

	if this.r == nil {
		pendingAction()
		return nil
	} else {
		r := this.r
		return func() {
			finalAction(r)
		}
	}
}

func StartCanCancel(action func(canceller Canceller) []interface{}) *Future {
	return start(action, true)
}

func Start(action func() []interface{}) *Future {
	return start(action, false)
}

func StartCanCancel0(action func(canceller Canceller)) *Future {
	return start(func(canceller Canceller) []interface{} {
		action(canceller)
		return make([]interface{}, 0, 0)
	}, true)
}

func Start0(action func()) *Future {
	return start(func() []interface{} {
		action()
		return make([]interface{}, 0, 0)
	}, false)
}

//异步执行一个函数。如果最后一个返回值为bool，则将认为此值代表异步任务成功或失败。如果函数抛出error，则认为异步任务失败
func start(action interface{}, canCancel bool) *Future {
	fu := NewPromise()

	var action1 func() []interface{}
	var action2 func(canceller Canceller) []interface{}
	if canCancel {
		action2 = action.(func(canceller Canceller) []interface{})
		fu.EnableCanceller()
	} else {
		action1 = action.(func() []interface{})
	}

	go func() {
		defer func() {
			if e := recover(); e != nil {
				fu.Reject(e)
			}
		}()

		var r []interface{}
		if canCancel {
			r = action2(fu.Canceller())
		} else {
			r = action1()
		}
		if l := len(r); l > 0 {
			if done, ok := r[l-1].(bool); ok {
				if done {
					fu.Reslove(r[:l-1]...)
				} else {
					fu.Reject(r[:l-1]...)
				}
			} else {
				fu.Reslove(r...)
			}
		} else {
			fu.Reslove(r...)
		}
	}()

	return fu.Future
}

func Wrap(value interface{}) *Future {
	fu := NewPromise()
	fu.Reslove(value)
	return fu.Future
}

//Factory function for Promise
func NewPromise() *Promise {
	f := &Promise{new(sync.Once),
		&Future{
			new(sync.Once), new(sync.Mutex),
			make(chan *PromiseResult, 1),
			make([]func(v ...interface{}), 0, 8),
			make([]func(v ...interface{}), 0, 8),
			make([]func(v ...interface{}), 0, 4),
			pipe{}, nil, nil,
		},
	}
	return f
}

type anyPromiseResult struct {
	result []interface{}
	i      int
}

//产生一个新的Promise，如果列表中任意1个Promise完成，则Promise完成, 否则将触发Reject，参数为包含所有Promise的Reject返回值的slice
func WhenAny(fs ...*Future) *Future {
	nf := NewPromise()
	errors := make([]interface{}, len(fs), len(fs))
	chFails := make(chan anyPromiseResult)

	for i, f := range fs {
		k := i
		f.Done(func(v ...interface{}) {
			nf.Reslove(v...)
		}).Fail(func(v ...interface{}) {
			chFails <- anyPromiseResult{v, k}
		})
	}

	if len(fs) == 0 {
		nf.Reslove()
	} else {
		go func() {
			j := 0
			for {
				select {
				case r := <-chFails:
					errors[r.i] = r.result
					if j++; j == len(fs) {
						nf.Reject(errors...)
						break
					}
				case _ = <-nf.chOut:
					break
				}
			}
		}()
	}
	return nf.Future
}

//产生一个新的Promise，如果列表中所有Promise都成功完成，则Promise成功完成，否则失败
func WhenAll(fs ...*Future) *Future {
	f := NewPromise()
	if len(fs) == 0 {
		f.Reslove()
	} else {
		go func() {
			rs := make([]interface{}, len(fs))
			allOk := true
			for i, f := range fs {
				r, typ := f.Get()
				r = append(r, typ)
				rs[i] = r
				if typ != RESULT_SUCCESS {
					allOk = false
				}
			}
			if allOk {
				f.Reslove(rs...)
			} else {
				f.Reject(rs...)
			}
		}()
	}

	return f.Future
}

func forSlice(s []func(v ...interface{}), f func(func(v ...interface{}))) {
	for _, e := range s {
		f(e)
	}
}

type stringer interface {
	String() string
}

func getError(i interface{}) (e error) {
	if i != nil {
		switch v := i.(type) {
		case error:
			e = v
		case stringer:
			e = errors.New(v.String())
		default:
			e = errors.New("unknow error")
		}
	}
	return
}
