package promise

import (
	"sync/atomic"
)

type anyPromiseResult struct {
	result interface{}
	i      int
}

//Start calls act function and return a Future that presents the result.
//If option paramter is true, the act function will be sync called.
func Start(act interface{}, syncs ...bool) *Future {
	pr := NewPromise()
	if f, ok := act.(*Future); ok {
		return f
	}

	action := getAct(pr, act)
	if syncs != nil && len(syncs) > 0 && !syncs[0] {
		//sync call
		r, err := action()
		if pr.IsCancelled() {
			pr.Cancel()
		} else {
			if err == nil {
				pr.Resolve(r)
			} else {
				pr.Reject(err)
			}
		}
	} else {
		//async call
		go func() {
			r, err := action()
			if pr.IsCancelled() {
				pr.Cancel()
			} else {
				if err == nil {
					pr.Resolve(r)
				} else {
					pr.Reject(err)
				}
			}
		}()
	}

	return pr.Future
}

//Wrap return a Future that presents the wrapped value
func Wrap(value interface{}) *Future {
	pr := NewPromise()
	if e, ok := value.(error); !ok {
		pr.Resolve(value)
	} else {
		pr.Reject(e)
	}

	return pr.Future
}

//WhenAny returns a Future.
//If any Future is resolved, this Future will be resolved and return result of resolved Future.
//Otherwise will rejected with results slice returned by all Futures
func WhenAny(fs ...*Future) *Future {
	return WhenAnyMatched(nil, fs...)
}

//WhenAnyMatched returns a Future.
//If any Future is resolved and match the predicate, this Future will be resolved and return result of resolved Future.
//Otherwise will rejected with a AggregateError included results slice returned by all Futures
func WhenAnyMatched(predicate func(interface{}) bool, fs ...*Future) *Future {
	if predicate == nil {
		predicate = func(v interface{}) bool { return true }
	}

	nf, rs := NewPromise(), make([]interface{}, len(fs))
	chFails, chDones := make(chan anyPromiseResult), make(chan anyPromiseResult)

	go func() {
		for i, f := range fs {
			k := i
			f.OnSuccess(func(v interface{}) {
				defer func() { _ = recover() }()
				chDones <- anyPromiseResult{v, k}
			}).OnFailure(func(v interface{}) {
				defer func() { _ = recover() }()
				chFails <- anyPromiseResult{v, k}
			})
		}
	}()

	if len(fs) == 0 {
		nf.Resolve(nil)
	} else if len(fs) == 1 {
		select {
		case r := <-chFails:
			errs := make([]error, 1)
			errs[0] = getError(r.result)
			nf.Reject(newAggregateError("Error appears in WhenAnyTrue:", errs))
		case r := <-chDones:
			if predicate(r.result) {
				nf.Resolve(r.result)
			} else {
				nf.Reject(&NoMatchedError{})
			}
		}
	} else {
		go func() {
			defer func() {
				if e := recover(); e != nil {
					nf.Reject(newErrorWithStacks(e))
				}
			}()

			j := 0
			for {
				select {
				case r := <-chFails:
					rs[r.i] = getError(r.result)
				case r := <-chDones:
					if predicate(r.result) {
						//try to cancel other futures
						for _, f := range fs {
							f.RequestCancel()
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
						nf.Reject(&NoMatchedError{})
					}
					break
				}
			}
		}()
	}
	return nf.Future
}

//WaitAll returns a Future.
//If any Future is resolved and match the predicate, this Future will be resolved and return result of resolved Future.
//Otherwise will rejected with results slice returned by all Futures
func WaitAll(acts ...interface{}) (fu *Future) {
	pr := NewPromise()
	fu = pr.Future

	if len(acts) == 0 {
		pr.Resolve([]interface{}{})
		return
	}

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
	return
}

//WhenAll receives function slice and returns a Future.
//If all Futures are resolved, this Future will be resolved and return results slice.
//Otherwise will rejected with results slice returned by all Futures
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

//WhenAll receives Futures slice and returns a Future.
//If all Futures are resolved, this Future will be resolved and return results slice.
//If any Future is cancelled, this Future will be cancelled.
//Otherwise will rejected with results slice returned by all Futures.
func WhenAllFuture(fs ...*Future) *Future {
	wf := NewPromise()
	rs := make([]interface{}, len(fs))

	if len(fs) == 0 {
		wf.Resolve([]interface{}{})
	} else {
		n := int32(len(fs))
		cancelOthers := func(j int) {
			for k, f1 := range fs {
				if k != j {
					f1.RequestCancel()
				}
			}
		}

		go func() {
			isCancelled := int32(0)
			for i, f := range fs {
				j := i

				f.OnSuccess(func(v interface{}) {
					rs[j] = v
					if atomic.AddInt32(&n, -1) == 0 {
						wf.Resolve(rs)
					}
				}).OnFailure(func(v interface{}) {
					if atomic.CompareAndSwapInt32(&isCancelled, 0, 1) {
						//try to cancel all futures
						cancelOthers(j)

						errs := make([]error, 0, 1)
						errs = append(errs, v.(error))
						e := newAggregateError("Error appears in WhenAll:", errs)
						wf.Reject(e)
					}
				}).OnCancel(func() {
					if atomic.CompareAndSwapInt32(&isCancelled, 0, 1) {
						//try to cancel all futures
						cancelOthers(j)

						wf.EnableCanceller().Cancel()
					}
				})
			}
		}()
	}

	return wf.Future
}
