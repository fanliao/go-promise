package promise

import (
	"sync/atomic"
)

type anyPromiseResult struct {
	result interface{}
	i      int
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
			pr.Cancel()
		} else {
			if err == nil {
				pr.Resolve(r)
			} else {
				pr.Reject(err)
			}
		}
	} else {
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

func Wrap(value interface{}) *Future {
	pr := NewPromise()
	if e, ok := value.(error); !ok {
		pr.Resolve(value)
	} else {
		pr.Reject(e)
	}

	return pr.Future
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
				defer func() { _ = recover() }()
				chDones <- anyPromiseResult{v, k}
			}).Fail(func(v interface{}) {
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
				nf.Resolve(false)
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
						nf.Resolve(false)
					}
					break
				}
			}
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
						wf.Reject(e)
					}
				})
			}
		}()
	}

	return wf.Future
}
