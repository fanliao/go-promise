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

//CancelledError present the Future object is cancelled.
type CancelledError struct {
}

func (e *CancelledError) Error() string {
	return "Task be cancelled"
}

//resultType present the type of Future final status.
type resultType int

const (
	RESULT_SUCCESS resultType = iota
	RESULT_FAILURE
	RESULT_CANCELLED
)

//PromiseResult presents the result of a promise.
//If Typ is RESULT_SUCCESS, Result field will present the returned value of Future task.
//If Typ is RESULT_FAILURE, Result field will present a related error .
//If Typ is RESULT_CANCELLED, Result field will be null.
type PromiseResult struct {
	Result interface{} //result of the Promise
	Typ    resultType  //success, failure, or cancelled?
}

//Promise presents an object that acts as a proxy for a result.
//that is initially unknown, usually because the computation of its
//value is yet incomplete (refer to wikipedia).
//You can use Resolve/Reject/Cancel to set the final result of Promise.
//Future can return a read-only placeholder view of result.
type Promise struct {
	onceEnd *sync.Once
	*Future
}

//Cancel sets the status of promise to RESULT_CANCELLED.
//If promise is cancelled, Get() will return nil and CANCELLED error.
//All callback functions will be not called if Promise is cancalled.
func (this *Promise) Cancel() (e error) {
	atomic.StoreInt32(&this.cancelStatus, 2)
	return this.setResult(&PromiseResult{CANCELLED, RESULT_CANCELLED})
}

//Resolve sets the value for promise, and the status will be changed to RESULT_SUCCESS.
//if promise is resolved, Get() will return the value and nil error.
func (this *Promise) Resolve(v interface{}) (e error) {
	return this.setResult(&PromiseResult{v, RESULT_SUCCESS})
}

//Resolve sets the error for promise, and the status will be changed to RESULT_FAILURE.
//if promise is rejected, Get() will return nil and the related error value.
func (this *Promise) Reject(err error) (e error) {
	return this.setResult(&PromiseResult{err, RESULT_FAILURE})
}

//EnableCanceller sets a Promise can be cancelled.
func (this *Promise) EnableCanceller() *Promise {
	atomic.CompareAndSwapInt32(&this.cancelStatus, -1, 0)
	return this
}

//Canceller returns a canceller related to future.
//If Canceller return nil, the futrue cannot be cancelled.
func (this *Promise) Canceller() Canceller {
	ccstatus := atomic.LoadInt32(&this.cancelStatus)
	if ccstatus >= 0 {
		return &canceller{this}
	} else {
		return nil
	}
}

//OnSuccess registers a callback function that will be called when Promise is resolved.
//If promise is already resolved, the callback will immediately called.
//The value of Promise will be paramter of Done callback function.
func (this *Promise) OnSuccess(callback func(v interface{})) *Promise {
	this.Future.OnSuccess(callback)
	return this
}

//OnFailure registers a callback function that will be called when Promise is rejected.
//If promise is already rejected, the callback will immediately called.
//The error of Promise will be paramter of Fail callback function.
func (this *Promise) OnFailure(callback func(v interface{})) *Promise {
	this.Future.OnFailure(callback)
	return this
}

//OnComplete register a callback function that will be called when Promise is rejected or resolved.
//If promise is already rejected or resolved, the callback will immediately called.
//According to the status of Promise, value or error will be paramter of Always callback function.
//Value is the paramter if Promise is resolved, or error is the paramter if Promise is rejected.
//Always callback will be not called if Promise be called.
func (this *Promise) OnComplete(callback func(v interface{})) *Promise {
	this.Future.OnComplete(callback)
	return this
}

//OnCancel registers a callback function that will be called when Promise is cancelled.
//If promise is already cancelled, the callback will immediately called.
func (this *Promise) OnCancel(callback func()) *Promise {
	this.Future.OnCancel(callback)
	return this
}

//setResult sets the value and final status of Promise, it will only be executed for once
func (this *Promise) setResult(r *PromiseResult) (e error) { //r *PromiseResult) {
	defer func() {
		if err := getError(recover()); err != nil {
			e = err
			fmt.Println("\nerror in setResult():", err)
		}
	}()

	e = errors.New("Cannot resolve/reject/cancel more than once")
	this.onceEnd.Do(func() {
		for {
			v := this.loadVal()
			newVal := *v
			newVal.r = unsafe.Pointer(r)

			//Use CAS operation to ensure that the state of Promise isn't changed.
			//If the state is changed, must get latest state and try to call CAS again.
			//No ABA issue in this case because address of all objects are different.
			if atomic.CompareAndSwapPointer(&this.val, unsafe.Pointer(v), unsafe.Pointer(&newVal)) {
				//chOut will be returned in GetChan(), so send the result to chOut
				this.chOut <- r

				//Close chEnd then all Get() and GetOrTimeout() will be unblocked
				close(this.chEnd)

				//call callback functions and start the Promise pipeline
				execCallback(r, v.dones, v.fails, v.always, v.cancels)
				for _, pipe := range v.pipes {
					pipeTask, pipePromise := pipe.getPipe(r.Typ == RESULT_SUCCESS)
					startPipe(r, pipeTask, pipePromise)
				}
				e = nil
				break
			}
		}
	})
	return
}

//Canceller is used to check if the Promise be requested to cancel and cancel the Promise
//It usually be passed to the real act function for letting act function can cancel the execution.
type Canceller interface {
	IsCancellationRequested() bool
	Cancel()
}

//canceller provides an implement of Canceller interface.
//It will be passed to Future task function as paramter of function
type canceller struct {
	p *Promise
}

//RequestCancel sets the status of Promise to CancellationRequested.
//It don't mean the promise be surely cancelled.
//If Future task detects CancellationRequested status, the execution can be stopped.
//Future task must call Cancel() method of Canceller interface to set Future to Cancelled status
func (this *canceller) RequestCancel() {
	//只有当状态==0（表示初始状态）时才可以请求取消任务
	atomic.CompareAndSwapInt32(&this.p.cancelStatus, 0, 1)
}

//IsCancellationRequested returns true if Future task is requested to cancel, otherwise false.
//Future task function can use this method to detect if Future is requested to cancel.
func (this *canceller) IsCancellationRequested() (r bool) {
	return atomic.LoadInt32(&this.p.cancelStatus) == 1
}

//Cancel sets Future task to CANCELLED status
func (this *canceller) Cancel() {
	this.p.Cancel()
}

//IsCancelled returns true if Future task is cancelld, otherwise false.
func (this *canceller) IsCancelled() (r bool) {
	return atomic.LoadInt32(&this.p.cancelStatus) == 2
}

//Factory function for Promise
func NewPromise() *Promise {
	val := &futureVal{
		make([]func(v interface{}), 0, 8),
		make([]func(v interface{}), 0, 8),
		make([]func(v interface{}), 0, 4),
		make([]func(), 0, 2),
		make([]*pipe, 0, 4), nil,
	}
	f := &Promise{new(sync.Once),
		&Future{
			rand.Int(),
			make(chan *PromiseResult, 1),
			make(chan struct{}),
			unsafe.Pointer(val),
			-1,
		},
	}
	return f
}
