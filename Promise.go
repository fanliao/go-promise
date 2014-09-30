package promise

import (
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"unsafe"
)

var (
	CANCELLED error = &CancelledError{}
)

type CancelledError struct {
}

func (e *CancelledError) Error() string {
	return "Task be cancelled"
}

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
	atomic.StoreInt32(&this.cancelStatus, 2)
	return this.end(&PromiseResult{CANCELLED, RESULT_CANCELLED})
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
	return this
}

//获取Canceller接口，在异步任务内可以通过此对象查询任务是否已经被取消
//Canceller provides a canceller related to future
//if Canceller return nil, the futrue cannot be cancelled
func (this *Promise) Canceller() Canceller {
	ccstatus := atomic.LoadInt32(&this.cancelStatus)
	if ccstatus >= 0 {
		return &canceller{this}
	} else {
		return nil
	}
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
		for {
			v := this.val()
			newVal := *v
			newVal.r = unsafe.Pointer(r)

			if atomic.CompareAndSwapPointer(&this.v, unsafe.Pointer(v), unsafe.Pointer(&newVal)) {
				this.chOut <- r
				//让Get函数可以返回
				close(this.chEnd)

				if r.Typ != RESULT_CANCELLED {
					//任务完成后调用回调函数
					execCallback(r, v.dones, v.fails, v.always)

					pipeTask, pipePromise := v.getPipe(r.Typ == RESULT_SUCCESS)
					startPipe(r, pipeTask, pipePromise)
				}
				e = nil
				break
			}
		}
	})
	return
}

//Cancel一个任务的interface
//Canceller used to check if the promise be requested to cancel, and set the cancelled status
type Canceller interface {
	IsCancellationRequested() bool
	Cancel()
}

type canceller struct {
	p *Promise
}

//Cancel任务
func (this *canceller) RequestCancel() {
	//只有当状态==0（表示初始状态）时才可以请求取消任务
	atomic.CompareAndSwapInt32(&this.p.cancelStatus, 0, 1)
}

//已经被要求取消任务
func (this *canceller) IsCancellationRequested() (r bool) {
	return atomic.LoadInt32(&this.p.cancelStatus) == 1
}

//设置任务已经被Cancel
func (this *canceller) Cancel() {
	this.p.Cancel()
}

//任务已经被Cancel
func (this *canceller) IsCancelled() (r bool) {
	return atomic.LoadInt32(&this.p.cancelStatus) == 2
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
