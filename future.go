package promise

import (
	"bytes"
	"errors"
	"fmt"
	"runtime"
	"strconv"
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
	Result interface{}
	Typ    resultType
}

//处理链式调用
type pipe struct {
	pipeDoneTask, pipeFailTask func(v interface{}) *Future
	pipePromise                *Promise
}

//异步任务
type Promise struct {
	onceEnd *sync.Once
	*Future
}

//Cancel表示任务正常完成
func (this *Promise) Cancel(v interface{}) (e error) {
	return this.end(&PromiseResult{v, RESULT_CANCELLED})
}

//Reslove表示任务正常完成
func (this *Promise) Resolve(v interface{}) (e error) {
	return this.end(&PromiseResult{v, RESULT_SUCCESS})
}

//Reject表示任务失败
func (this *Promise) Reject(err error) (e error) {
	return this.end(&PromiseResult{err, RESULT_FAILURE})
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
func (this *Promise) Done(callback func(v interface{})) *Promise {
	this.Future.Done(callback)
	return this
}

//添加一个任务失败时的回调，如果任务已经失败，则直接执行回调函数
//传递给Fail函数的参数与Reject函数的参数相同
func (this *Promise) Fail(callback func(v interface{})) *Promise {
	this.Future.Fail(callback)
	return this
}

//添加一个回调函数，该函数将在任务完成后执行，无论成功或失败
//传递给Always回调的参数根据成功或失败状态，与Reslove或Reject函数的参数相同
func (this *Promise) Always(callback func(v interface{})) *Promise {
	this.Future.Always(callback)
	return this
}

//Cancel一个任务的interface
type Canceller interface {
	IsCancellationRequested() bool
	SetCancelled()
}

//Future代表一个异步任务的readonly-view
type Future struct {
	oncePipe             *sync.Once
	lock                 *sync.Mutex
	chOut                chan *PromiseResult
	dones, fails, always []func(v interface{})
	pipe
	r          *PromiseResult
	*canceller //*PromiseCanceller
}

//获取Canceller接口，在异步任务内可以通过此对象查询任务是否已经被取消
func (this *Future) Canceller() Canceller {
	return this.canceller
}

//取消异步任务
func (this *Future) RequestCancel() bool {
	if this.r != nil || this.canceller == nil {
		return false
	} else {
		this.canceller.RequestCancel()
		return true
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
func (this *Future) SetCancelled() {
	if this.canceller != nil && this.r == nil {
		this.canceller.SetCancelled()
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

func (this *Future) GetChan() chan *PromiseResult {
	return this.chOut
}

//Get函数将一直阻塞直到任务完成,并返回任务的结果
//如果任务已经完成，后续的Get将直接返回任务结果
func (this *Future) Get() (interface{}, error) {
	if fr, ok := <-this.chOut; ok {
		return getFutureReturnVal(fr) //fr.Result, fr.Typ
	} else {
		//r, typ := this.r.Result, this.r.Typ
		return getFutureReturnVal(this.r) //r, typ
	}
}

func getFutureReturnVal(r *PromiseResult) (interface{}, error) {
	if r.Typ == RESULT_SUCCESS {
		return r.Result, nil
	} else if r.Typ == RESULT_FAILURE {
		return nil, getError(r.Result)
	} else {
		return nil, &CancelledError{}
	}
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
	case fr, ok := <-this.chOut:
		if ok {
			r, err := getFutureReturnVal(fr)
			return r, err, false
		} else {
			r, err := getFutureReturnVal(this.r)
			return r, err, false
		}

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
		return
	}

	this.oncePipe.Do(func() {
		execWithLock(this.lock, func() {
			if this.r != nil {
				result = this
				if this.r.Typ == RESULT_SUCCESS && callbacks[0] != nil {
					result = (callbacks[0](this.r.Result))
				} else if this.r.Typ != RESULT_FAILURE && len(callbacks) > 1 && callbacks[1] != nil {
					result = (callbacks[1](this.r.Result))
				}
			} else {
				this.pipeDoneTask = callbacks[0]
				if len(callbacks) > 1 {
					this.pipeFailTask = callbacks[1]
				}
				this.pipePromise = NewPromise()
				result = this.pipePromise.Future
			}
		})
		ok = true
	})
	return
}

type canceller struct {
	lockC       *sync.Mutex
	isRequested bool
	isCancelled bool
}

//Cancel任务
func (this *canceller) RequestCancel() {
	execWithLock(this.lockC, func() {
		this.isRequested = true
	})
}

//已经被要求取消任务
func (this *canceller) IsCancellationRequested() (r bool) {
	execWithLock(this.lockC, func() {
		r = this.isRequested
	})
	return
}

//设置任务已经被Cancel
func (this *canceller) SetCancelled() {
	execWithLock(this.lockC, func() {
		this.isCancelled = true
	})
}

//任务已经被Cancel
func (this *canceller) IsCancelled() (r bool) {
	execWithLock(this.lockC, func() {
		r = this.isCancelled
	})
	return
}

//完成一个任务
func (this *Promise) end(r *PromiseResult) (e error) { //r *PromiseResult) {
	defer func() {
		if e = getError(recover()); e != nil {
			//TODO: how to handle the errors appears in callback?
			fmt.Println("error in end", e)

			buf := bytes.NewBufferString("")
			pcs := make([]uintptr, 50)
			num := runtime.Callers(2, pcs)
			for _, v := range pcs[0:num] {
				fun := runtime.FuncForPC(v)
				file, line := fun.FileLine(v)
				name := fun.Name()
				//fmt.Println(name, file + ":", line)
				writeStrings(buf, []string{name, " ", file, ":", strconv.Itoa(line), "\n"})
			}
			fmt.Println(buf.String())
		}
	}()
	e = errors.New("Cannot resolve/reject/cancel more than once")
	this.onceEnd.Do(func() {
		this.setResult(r)

		//让Get函数可以返回
		this.chOut <- r
		close(this.chOut)

		if r.Typ != RESULT_CANCELLED {
			//任务完成后调用回调函数
			execCallback(r, this.dones, this.fails, this.always)

			pipeTask, pipePromise := this.getPipe(this.r.Typ == RESULT_SUCCESS)
			this.startPipe(pipeTask, pipePromise)
		}
		e = nil
	})
	return
}

//set this.r
func (this *Promise) setResult(r *PromiseResult) {
	this.lock.Lock()
	defer this.lock.Unlock()
	this.r = r
}

//返回与链式调用相关的对象
func (this *Future) getPipe(isResolved bool) (func(v interface{}) *Future, *Promise) {
	this.lock.Lock()
	defer this.lock.Unlock()
	if isResolved {
		return this.pipeDoneTask, this.pipePromise
	} else {
		return this.pipeFailTask, this.pipePromise
	}
}

func (this *Future) startPipe(pipeTask func(v interface{}) *Future, pipePromise *Promise) {
	//处理链式异步任务
	//var f *Future
	if pipeTask != nil {
		f := pipeTask(this.r.Result)
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
		if (t == CALLBACK_DONE && r.Typ == RESULT_SUCCESS) ||
			(t == CALLBACK_FAIL && r.Typ == RESULT_FAILURE) ||
			(t == CALLBACK_ALWAYS) {
			callback(r.Result)
		}
	}
	if f := this.addCallback(pendingAction, finalAction); f != nil {
		f()
	}
}

//添加回调函数的框架函数
func (this *Future) addCallback(pendingAction func(), finalAction func(*PromiseResult)) (r func()) {
	execWithLock(this.lock, func() {
		if this.r == nil {
			pendingAction()
			r = nil
		} else {
			r = func() { finalAction(this.r) }
		}
	})
	return
}

func StartCanCancel(action func(canceller Canceller) (interface{}, error)) *Future {
	return start(action, true)
}

func Start(action func() (interface{}, error)) *Future {
	return start(action, false)
}

func StartCanCancel0(action func(canceller Canceller)) *Future {
	return start(func(canceller Canceller) (interface{}, error) {
		action(canceller)
		return nil, nil
	}, true)
}

func Start0(action func()) *Future {
	return start(func() (interface{}, error) {
		action()
		return nil, nil
	}, false)
}

//异步执行一个函数。如果最后一个返回值为bool，则将认为此值代表异步任务成功或失败。如果函数抛出error，则认为异步任务失败
func start(act interface{}, canCancel bool) *Future {
	fu := NewPromise()

	if canCancel {
		fu.EnableCanceller()
	}

	go func() {
		defer func() {
			if e := recover(); e != nil {
				fu.Reject(newErrorWithStacks(e))
			}
		}()

		var r interface{}
		var err error
		if canCancel {
			r, err = (act.(func(canceller Canceller) (interface{}, error)))(fu.Canceller())
		} else {
			r, err = (act.(func() (interface{}, error)))()
		}

		if fu.IsCancelled() {
			fu.Cancel(r)
		} else {
			if err == nil {
				fu.Resolve(r)
			} else {
				fu.Reject(err)
			}
			//if l := len(r); l > 0 {
			//	if done, ok := r[l-1].(bool); ok {
			//		if done {
			//			fu.Resolve(r[:l-1]...)
			//		} else {
			//			fu.Reject(r[:l-1]...)
			//		}
			//	} else {
			//		fu.Resolve(r...)
			//	}
			//} else {
			//	fu.Resolve(r...)
			//}
		}
	}()

	return fu.Future
}

func Wrap(value interface{}) *Future {
	fu := NewPromise()
	fu.Resolve(value)
	//if values, ok := value.([]interface{}); ok {
	//	fu.Resolve(values...)
	//} else {
	//}
	return fu.Future
}

//Factory function for Promise
func NewPromise() *Promise {
	f := &Promise{new(sync.Once),
		&Future{
			new(sync.Once), new(sync.Mutex),
			make(chan *PromiseResult, 1),
			make([]func(v interface{}), 0, 8),
			make([]func(v interface{}), 0, 8),
			make([]func(v interface{}), 0, 4),
			pipe{}, nil, nil,
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
	nf := NewPromise()
	errs := make([]error, len(fs))
	chFails := make(chan anyPromiseResult)

	for i, f := range fs {
		k := i
		f.Done(func(v interface{}) {
			nf.Resolve(v)
		}).Fail(func(v interface{}) {
			chFails <- anyPromiseResult{v, k}
		})
	}

	if len(fs) == 0 {
		nf.Resolve(nil)
	} else {
		go func() {
			j := 0
			for {
				select {
				case r := <-chFails:
					errs[r.i] = getError(r.result)
					if j++; j == len(fs) {
						nf.Reject(newAggregateError("Error appears in WhenAny:", errs))
						break
					}
				case _ = <-nf.chOut:
					//if a future be success, will try to cancel oter future
					for _, f := range fs {
						if c := f.Canceller(); c != nil {
							f.RequestCancel()
						}
					}
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
		f.Resolve(nil)
	} else {
		go func() {
			rs := make([]interface{}, len(fs))
			errs := make([]error, len(fs))
			allOk := true
			for i, f := range fs {
				//if a future be failure, then will try to cancel other futures
				if !allOk {
					for j := i; j < len(fs); j++ {
						if c := fs[j].Canceller(); c != nil {
							fs[j].RequestCancel()
						}
					}
				}
				rs[i], errs[i] = f.Get()

				if errs[i] != nil {
					allOk = false
				}
			}
			for i, r := range rs {
				if allOk {
					rs[i] = r
				} else {
					rs[i] = errs[i] //append(r.([]interface{}), typs[i])
				}
			}
			if allOk {
				f.Resolve(rs)
			} else {
				f.Reject(newAggregateError("Error appears in WhenAll:", errs))
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
