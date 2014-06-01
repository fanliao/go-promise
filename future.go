package promise

import (
	"bytes"
	"errors"
	"fmt"
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
func (this *Promise) Cancel() (e error) {
	return this.end(&PromiseResult{&CancelledError{}, RESULT_CANCELLED})
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
		this.canceller = &canceller{}
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
	//r          *PromiseResult
	r          unsafe.Pointer
	*canceller //*PromiseCanceller
}

func (this *Future) result() *PromiseResult {
	r := atomic.LoadPointer(&this.r)
	return (*PromiseResult)(r)
}

//获取Canceller接口，在异步任务内可以通过此对象查询任务是否已经被取消
func (this *Future) Canceller() Canceller {
	return this.canceller
}

//取消异步任务
func (this *Future) RequestCancel() bool {
	if this.result() != nil || this.canceller == nil {
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
	if this.canceller != nil && this.result() == nil {
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
		return getFutureReturnVal(this.result()) //r, typ
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
			r, err := getFutureReturnVal(this.result())
			return r, err, false
		}

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
		return
	}

	this.oncePipe.Do(func() {
		r := this.result()
		if r != nil {
			result = this
			if r.Typ == RESULT_SUCCESS && callbacks[0] != nil {
				result = (callbacks[0](r.Result))
			} else if r.Typ != RESULT_FAILURE && len(callbacks) > 1 && callbacks[1] != nil {
				result = (callbacks[1](r.Result))
			}
		} else {
			execWithLock(this.lock, func() {
				this.pipeDoneTask = callbacks[0]
				if len(callbacks) > 1 {
					this.pipeFailTask = callbacks[1]
				}
				this.pipePromise = NewPromise()
				result = this.pipePromise.Future
			})
		}
		ok = true
	})
	return
}

type canceller struct {
	//lockC       *sync.Mutex
	isRequested int32
	isCancelled int32
}

//Cancel任务
func (this *canceller) RequestCancel() {
	atomic.StoreInt32(&this.isRequested, 1)
}

//已经被要求取消任务
func (this *canceller) IsCancellationRequested() (r bool) {
	return atomic.LoadInt32(&this.isRequested) == 1
}

//设置任务已经被Cancel
func (this *canceller) SetCancelled() {
	atomic.StoreInt32(&this.isCancelled, 1)
}

//任务已经被Cancel
func (this *canceller) IsCancelled() (r bool) {
	return atomic.LoadInt32(&this.isCancelled) == 1
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
		this.setResult(r)

		//fmt.Println("send", r)
		//让Get函数可以返回
		this.chOut <- r
		close(this.chOut)
		//fmt.Println("close done", r)

		if r.Typ != RESULT_CANCELLED {
			//任务完成后调用回调函数
			execCallback(r, this.dones, this.fails, this.always)

			//fmt.Println("after callback", r)
			pipeTask, pipePromise := this.getPipe(r.Typ == RESULT_SUCCESS)
			this.startPipe(pipeTask, pipePromise)
		}
		e = nil
	})
	return
}

//set this.r
func (this *Promise) setResult(r *PromiseResult) {
	atomic.StorePointer(&this.r, unsafe.Pointer(r))
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
		f := pipeTask(this.result().Result)
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
			(t == CALLBACK_ALWAYS && r.Typ != RESULT_CANCELLED) {
			callback(r.Result)
		}
	}
	if f := this.addCallback(pendingAction, finalAction); f != nil {
		f()
	}
}

//添加回调函数的框架函数
func (this *Future) addCallback(pendingAction func(), finalAction func(*PromiseResult)) (fun func()) {
	r := this.result()
	if r == nil {
		execWithLock(this.lock, func() {
			pendingAction()
		})
		fun = nil
	} else {
		fun = func() { finalAction(r) }
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
				//fmt.Println("whenall reject", errs)
				e := newAggregateError("Error appears in WhenAll:", errs)
				//fmt.Println("whenall reject2", e.Error())
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
