package promise

import (
	"errors"
	"fmt"
	c "github.com/smartystreets/goconvey/convey"
	//"strings"
	"testing"
	"time"
)

const (
	TASK_END      = "task be end,"
	CALL_DONE     = "callback done,"
	CALL_FAIL     = "callback fail,"
	CALL_ALWAYS   = "callback always,"
	WAIT_TASK     = "wait task end,"
	GET           = "get task result,"
	DONE_Pipe_END = "task Pipe done be end,"
	FAIL_Pipe_END = "task Pipe fail be end,"
)

// errorLinq is a trivial implementation of error.
type myError struct {
	val interface{}
}

func (e *myError) Error() string {
	return fmt.Sprintf("%v", e.val)
}

func newMyError(v interface{}) *myError {
	return &myError{v}
}

var order []string
var tObj *testing.T

var taskDone func() (interface{}, error) = func() (interface{}, error) {
	time.Sleep(500 * time.Millisecond)
	order = append(order, TASK_END)
	return "ok", nil
}
var taskFail func() (interface{}, error) = func() (interface{}, error) {
	time.Sleep(500 * time.Millisecond)
	order = append(order, TASK_END)
	return nil, newMyError("fail")
}

var done func(v interface{}) = func(v interface{}) {
	time.Sleep(50 * time.Millisecond)
	order = append(order, CALL_DONE)
	c.Convey("When done function be called", tObj, func() {
		c.So(v, c.ShouldEqual, "ok")
	})
}
var alwaysForDone func(v interface{}) = func(v interface{}) {
	order = append(order, CALL_ALWAYS)
	c.Convey("When alwaysForDone function be called", tObj, func() {
		c.So(v, c.ShouldEqual, "ok")
	})
}
var fail func(v interface{}) = func(v interface{}) {
	time.Sleep(50 * time.Millisecond)
	order = append(order, CALL_FAIL)
	c.Convey("When alwaysForDone function be called", tObj, func() {
		c.So(v.(*myError).val, c.ShouldEqual, "fail")
	})
}
var alwaysForFail func(v interface{}) = func(v interface{}) {
	order = append(order, CALL_ALWAYS)
	//AreEqual(v.(*myError).val, []interface{}{10, "fail"}, tObj)
	c.Convey("When alwaysForDone function be called", tObj, func() {
		c.So(v.(*myError).val, c.ShouldEqual, "fail")
	})
}

func TestResolveAndReject(t *testing.T) {
	c.Convey("When Future is resolved", t, func() {
		p := NewPromise()
		go func() {
			time.Sleep(50 * time.Millisecond)
			p.Resolve("ok")
		}()
		r, err := p.Get()
		c.So(r, c.ShouldEqual, "ok")
		c.So(err, c.ShouldBeNil)
	})

	c.Convey("When Future is rejected", t, func() {
		p := NewPromise()
		go func() {
			time.Sleep(50 * time.Millisecond)
			p.Reject(errors.New("fail"))
		}()
		r, err := p.Get()
		c.So(err, c.ShouldNotBeNil)
		c.So(r, c.ShouldEqual, nil)
	})
}

func TestCancel1(t *testing.T) {
	c.Convey("When Future is cancelled", t, func() {
		p := NewPromise()
		go func() {
			time.Sleep(50 * time.Millisecond)
			p.Cancel()
		}()
		r, err := p.Get()
		c.So(r, c.ShouldBeNil)
		c.So(err, c.ShouldHaveSameTypeAs, &CancelledError{})
	})
}

func TestGetOrTimeOut(t *testing.T) {
	p := NewPromise()
	go func() {
		_, _ = <-time.After((time.Duration)50 * time.Millisecond)
		time.Sleep(50 * time.Millisecond)
		p.Resolve("ok")
	}()
	c.Convey("When Future is unfinished", t, func() {
		r, err, timeout := p.GetOrTimeout(10)
		c.So(timeout, c.ShouldEqual, true)
		c.So(r, c.ShouldBeNil)
		c.So(err, c.ShouldBeNil)
	})
	c.Convey("When Future is resolved", t, func() {
		r, err, timeout := p.GetOrTimeout(50)
		c.So(timeout, c.ShouldEqual, false)
		c.So(r, c.ShouldEqual, "ok")
		c.So(err, c.ShouldBeNil)
	})

	p = NewPromise()
	go func() {
		time.Sleep(50 * time.Millisecond)
		p.Reject(errors.New("fail"))
	}()
	c.Convey("When Future is rejected", t, func() {
		r, err, timeout := p.GetOrTimeout(53)
		c.So(timeout, c.ShouldEqual, false)
		c.So(r, c.ShouldBeNil)
		c.So(err, c.ShouldNotBeNil)
	})

	p = NewPromise()
	go func() {
		time.Sleep(50 * time.Millisecond)
		p.Cancel()
	}()
	c.Convey("When Future is cancelled", t, func() {
		r, err, timeout := p.GetOrTimeout(53)
		c.So(timeout, c.ShouldEqual, false)
		c.So(r, c.ShouldBeNil)
		c.So(err, c.ShouldHaveSameTypeAs, &CancelledError{})
	})
}

//func TestCallbacks(t *testing.T) {
//	tObj = t
//	order = make([]string, 0, 10)
//	f := Start(func() (interface{}, error) {
//		time.Sleep(50 * time.Millisecond)
//		return "ok", nil
//	}).Done(func(v interface{}) {
//		c.Convey("When done function be called", t, func() {
//			c.So(v, c.ShouldEqual, "ok")
//		})
//	}).Always(func(v interface{}) {
//		c.Convey("When always function be called", t, func() {
//			c.So(v, c.ShouldEqual, "ok")
//		})
//	}).Fail(func(v interface{}) {
//		panic("Unexpected calling")
//	})

//	c.Convey("When Future is resolved", t, func() {
//		r, err := f.Get()

//		//The code after Get() and the callback will be concurrent run
//		//So sleep 500 ms to wait all callback be done
//		time.Sleep(50 * time.Millisecond)

//		c.So(r, c.ShouldEqual, "ok")
//		c.So(err, c.ShouldBeNil)
//	})

//	c.Convey("When add the callback after Future is resolved", t, func() {
//		f.Done(func(v interface{}) {
//			c.Convey("When done function be called", func() {
//				c.So(v, c.ShouldEqual, "ok")
//			})
//		}).Always(func(v interface{}) {
//			c.Convey("When always function be called", func() {
//				c.So(v, c.ShouldEqual, "ok")
//			})
//		}).Fail(func(v interface{}) {
//			panic("Unexpected calling")
//		})
//	})
//}

//func TestDoneAlways(t *testing.T) {
//	/*tObj = t
//	order = make([]string, 0, 10)
//	f := Start(func() (interface{}, error) {
//			time.Sleep(50 * time.Millisecond)
//			return "ok", nil
//		}).Done(func(v interface{}) {
//			c.Convey("When done function be called", t, func() {
//				c.So(v, c.ShouldEqual, "ok")
//			})
//		}).Always(func(v interface{}) {
//			c.Convey("When always function be called", t, func() {
//				c.So(v, c.ShouldEqual, "ok")
//			})
//		}).Fail(func(v interface{}) {
//			panic("Unexpected calling")
//		})

//	c.Convey("When a Future be successed", t, func() {
//		r, err := f.Get()

//		//The code after Get() and the callback will be concurrent run
//		//So sleep 500 ms to wait all callback be done
//		time.Sleep(50 * time.Millisecond)

//		c.So(r, c.ShouldEqual, "ok")
//		c.So(err, c.ShouldBeNil)
//	})

//	c.Convey("When add the callback after Future is resolved", t, func() {
//		f.Done( func(v interface{}) {
//			c.Convey("When done function be called", t, func() {
//				c.So(v, c.ShouldEqual, "ok")
//			})
//		}).Always(func(v interface{}) {
//			c.Convey("When always function be called", t, func() {
//				c.So(v, c.ShouldEqual, "ok")
//			})
//		}).Fail(func(v interface{}) {
//			panic("Unexpected calling")
//		})
//	})*/
//}

//func TestFailAlways(t *testing.T) {
//	tObj = t
//	order = make([]string, 0, 10)
//	f := Start(taskFail).Fail(fail).Always(alwaysForFail).Fail(fail)

//	r, err := f.Get()
//	order = append(order, GET)
//	time.Sleep(500 * time.Millisecond)

//	AreEqual(order, []string{TASK_END, GET, CALL_FAIL, CALL_FAIL, CALL_ALWAYS}, t)
//	AreEqual(r, nil, t)
//	AreEqual(err.(*myError).val, "fail", t)

//}

//func TestPipeWhenDone(t *testing.T) {
//	tObj = t
//	taskDonePipe := func(v interface{}) *Future {
//		return Start(func() (interface{}, error) {
//			time.Sleep(100 * time.Millisecond)
//			order = append(order, DONE_Pipe_END)
//			return v.(string) + "2", nil
//		})
//	}

//	taskFailPipe := func(v interface{}) *Future {
//		return Start(func() (interface{}, error) {
//			val := v.(*myError).val.(string)
//			time.Sleep(100 * time.Millisecond)
//			order = append(order, FAIL_Pipe_END)
//			return nil, newMyError(val + "2")
//		})
//	}

//	SubmitWithCallback := func(task func() (interface{}, error)) (*Future, bool) {
//		return Start(task).Done(done).Fail(fail).
//			Pipe(taskDonePipe, taskFailPipe)
//	}

//	//test Done branch for Pipe function
//	order = make([]string, 0, 10)
//	f, isOk := SubmitWithCallback(taskDone)
//	r, err := f.Get()
//	order = append(order, GET)
//	time.Sleep(300 * time.Millisecond)

//	AreEqual(order, []string{TASK_END, CALL_DONE, DONE_Pipe_END, GET}, t)
//	AreEqual(r, "ok2", t)
//	AreEqual(err, nil, t)
//	AreEqual(isOk, true, t)

//	//test fail branch for Pipe function
//	order = make([]string, 0, 10)
//	f, isOk = SubmitWithCallback(taskFail)
//	r, err = f.Get()
//	order = append(order, GET)
//	time.Sleep(300 * time.Millisecond)

//	AreEqual(order, []string{TASK_END, CALL_FAIL, FAIL_Pipe_END, GET}, t)
//	AreEqual(err.(*myError).val, "fail2", t)
//	AreEqual(r, nil, t)
//	AreEqual(isOk, true, t)

//	f, isOk = f.Pipe(taskDonePipe, taskFailPipe)
//	//t.Log("isok?", isOk, f, f.oncePipe)
//	AreEqual(isOk, true, t)
//	_, _ = f.Get()
//}

//func TestGetOrTimeout(t *testing.T) {
//	tObj = t
//	order = make([]string, 0, 10)
//	AreEqual(order, []string{}, t)
//	f := Start(taskDone)

//	AreEqual(order, []string{}, t)
//	//timeout
//	r, err, timeout := f.GetOrTimeout(100)
//	AreEqual(timeout, true, t)

//	order = append(order, GET)
//	AreEqual(order, []string{GET}, t)
//	//get return value
//	r, err, timeout = f.GetOrTimeout(470)
//	AreEqual(timeout, false, t)
//	AreEqual(order, []string{GET, TASK_END}, t)
//	AreEqual(r, "ok", t)
//	AreEqual(err, nil, t)

//	//if task be done and timeout is 0, still can get return value
//	r, err, timeout = f.GetOrTimeout(0)
//	AreEqual(timeout, false, t)
//	AreEqual(r, "ok", t)
//	AreEqual(err, nil, t)
//}

//func TestException(t *testing.T) {
//	order = make([]string, 0, 10)
//	task := func() (interface{}, error) {
//		time.Sleep(500 * time.Millisecond)
//		order = append(order, "task be end,")
//		panic("unknown exception")
//		return "ok", nil
//	}

//	f := Start(task).Done(func(v interface{}) {
//		time.Sleep(200 * time.Millisecond)
//		order = append(order, "run Done callback,")
//	}).Always(func(v interface{}) {
//		order = append(order, "run Always callback,")
//		if !strings.Contains(v.(error).Error(), "unknown exception") {
//			t.Log("Failed! actual", v)
//			t.Fail()
//		}
//		//AreEqual(v, []interface{}{"exception"}, t)
//	}).Fail(func(v interface{}) {
//		order = append(order, "run Fail callback,")
//		if !strings.Contains(v.(error).Error(), "unknown exception") {
//			t.Log("Failed! actual", v)
//			t.Fail()
//		}
//	})

//	r, err := f.Get()
//	time.Sleep(200 * time.Millisecond)
//	AreEqual(order, []string{"task be end,", "run Fail callback,", "run Always callback,"}, t)
//	if !strings.Contains(err.Error(), "unknown exception") {
//		t.Log("Failed! actual", err.Error())
//		t.Fail()
//	}
//	AreEqual(r, nil, t)

//}

//func TestWhenAny(t *testing.T) {
//	whenTwoTask := func(t1 int, t2 int) *Future {
//		timeout1 := time.Duration(t1)
//		timeout2 := time.Duration(t2)
//		task1 := func() (interface{}, error) {
//			if timeout1 > 0 {
//				time.Sleep(timeout1 * time.Millisecond)
//				return "ok", nil
//			} else {
//				time.Sleep((-1 * timeout1) * time.Millisecond)
//				return nil, newMyError("fail")
//			}
//		}
//		task2 := func() (interface{}, error) {
//			if timeout2 > 0 {
//				time.Sleep(timeout2 * time.Millisecond)
//				return "ok2", nil
//			} else {
//				time.Sleep((-1 * timeout2) * time.Millisecond)
//				return nil, newMyError("fail2")
//			}
//		}
//		f := WhenAny(Start(task1), Start(task2))
//		return f
//	}

//	r, err := whenTwoTask(200, 250).Get()
//	AreEqual(r, "ok", t)
//	AreEqual(err, nil, t)

//	r, err = whenTwoTask(280, 250).Get()
//	AreEqual(r, "ok2", t)
//	AreEqual(err, nil, t)

//	r, err = whenTwoTask(-280, -250).Get()
//	errs := err.(*AggregateError).InnerErrs
//	AreEqual(errs[0].(*myError).val, "fail", t)
//	AreEqual(errs[1].(*myError).val, "fail2", t)
//	AreEqual(r, nil, t)

//	r, err = whenTwoTask(-280, 150).Get()
//	AreEqual(r, "ok2", t)
//	AreEqual(err, nil, t)

//	r, err = WhenAny().Get()
//	AreEqual(r, nil, t)
//	AreEqual(err, nil, t)

//	var c1, c2 bool
//	startTwoCanCancelTask := func(t1 int, t2 int) *Future {
//		timeout1 := time.Duration(t1)
//		timeout2 := time.Duration(t2)
//		task1 := func(canceller Canceller) (r interface{}, err error) {
//			for i := 0; i < 10; i++ {
//				if timeout1 > 0 {
//					time.Sleep(timeout1 * time.Millisecond)
//				} else {
//					time.Sleep((-1 * timeout1) * time.Millisecond)
//				}
//				if canceller.IsCancellationRequested() {
//					t.Log("cancel 1")
//					canceller.SetCancelled()
//					c1 = true
//					return nil, nil
//				}
//			}
//			if timeout1 > 0 {
//				return "ok", nil
//			} else {
//				return nil, newMyError("fail")
//			}
//		}
//		task2 := func(canceller Canceller) (r interface{}, err error) {
//			for i := 0; i < 10; i++ {
//				if timeout2 > 0 {
//					time.Sleep(timeout2 * time.Millisecond)
//				} else {
//					time.Sleep((-1 * timeout2) * time.Millisecond)
//				}
//				if canceller.IsCancellationRequested() {
//					t.Log("cancel 2")
//					canceller.SetCancelled()
//					c2 = true
//					return nil, nil
//				}
//			}
//			if timeout2 > 0 {
//				return "ok2", nil
//			} else {
//				return nil, newMyError("fail2")
//			}
//		}
//		f := WhenAny(Start(task1), Start(task2))
//		return f
//	}
//	r, err = startTwoCanCancelTask(10, 250).Get()
//	AreEqual(r, "ok", t)
//	AreEqual(err, nil, t)
//	time.Sleep(1000 * time.Millisecond)
//	AreEqual(c2, true, t)

//	//r, ok = startTwoCanCancelTask(280, 15).Get()
//	//AreEqual(r, []interface{}{20, "ok2"}, t)
//	//AreEqual(ok, RESULT_SUCCESS, t)
//	//AreEqual(c1, true, t)
//}

//func TestWhenAnyTrue(t *testing.T) {
//	c1, c2 := false, false
//	startTwoCanCancelTask := func(t1 int, t2 int, predicate func(interface{}) bool) *Future {
//		timeout1 := time.Duration(t1)
//		timeout2 := time.Duration(t2)
//		task1 := func(canceller Canceller) (r interface{}, err error) {
//			for i := 0; i < 10; i++ {
//				if timeout1 > 0 {
//					time.Sleep(timeout1 * time.Millisecond)
//				} else {
//					time.Sleep((-1 * timeout1) * time.Millisecond)
//				}
//				if canceller.IsCancellationRequested() {
//					t.Log("cancel 1")
//					canceller.SetCancelled()
//					c1 = true
//					return nil, nil
//				}
//			}
//			if timeout1 > 0 {
//				return 10, nil
//			} else {
//				return nil, newMyError(-10)
//			}
//		}
//		task2 := func(canceller Canceller) (r interface{}, err error) {
//			for i := 0; i < 10; i++ {
//				if timeout2 > 0 {
//					time.Sleep(timeout2 * time.Millisecond)
//				} else {
//					time.Sleep((-1 * timeout2) * time.Millisecond)
//				}
//				if canceller.IsCancellationRequested() {
//					t.Log("cancel 2")
//					canceller.SetCancelled()
//					c2 = true
//					return nil, nil
//				}
//			}
//			if timeout2 > 0 {
//				return 20, nil
//			} else {
//				return nil, newMyError(-20)
//			}
//		}
//		f := WhenAnyTrue(predicate, Start(task1), Start(task2))
//		return f
//	}
//	//第一个任务先完成，第二个后完成，并且设定条件为返回值==第一个的返回值
//	r, err := startTwoCanCancelTask(30, 250, func(v interface{}) bool { return v.(int) == 10 }).Get()
//	AreEqual(r, 10, t)
//	AreEqual(err, nil, t)

//	time.Sleep(1000 * time.Millisecond)
//	AreEqual(c2, true, t)

//	//第一个任务后完成，第二个先完成，并且设定条件为返回值==第二个的返回值
//	c1, c2 = false, false
//	r, err = startTwoCanCancelTask(250, 30, func(v interface{}) bool { return v.(int) == 20 }).Get()
//	AreEqual(r, 20, t)
//	AreEqual(err, nil, t)

//	time.Sleep(1000 * time.Millisecond)
//	AreEqual(c1, true, t)

//	//第一个任务后完成，第二个先完成，并且设定条件为返回值不等于任意一个任务的返回值
//	c1, c2 = false, false
//	r, err = startTwoCanCancelTask(10, 250, func(v interface{}) bool { return v.(int) == 200 }).Get()
//	AreEqual(r, false, t)
//	AreEqual(err, nil, t)

//	time.Sleep(1000 * time.Millisecond)
//	AreEqual(c1, false, t)
//	AreEqual(c2, false, t)

//	//r, ok = startTwoCanCancelTask(280, 15).Get()
//	//AreEqual(r, []interface{}{20, "ok2"}, t)
//	//AreEqual(ok, RESULT_SUCCESS, t)
//	//AreEqual(c1, true, t)
//}

//func TestWhenAll(t *testing.T) {
//	startTwoTask := func(t1 int, t2 int, wait bool) (f *Future) {
//		timeout1 := time.Duration(t1)
//		timeout2 := time.Duration(t2)
//		task1 := func() (r interface{}, err error) {
//			if timeout1 > 0 {
//				time.Sleep(timeout1 * time.Millisecond)
//				return "ok", nil
//			} else {
//				time.Sleep((-1 * timeout1) * time.Millisecond)
//				return nil, newMyError("fail")
//			}
//		}
//		task2 := func() (r interface{}, err error) {
//			if timeout2 > 0 {
//				time.Sleep(timeout2 * time.Millisecond)
//				return "ok2", nil
//			} else {
//				time.Sleep((-1 * timeout2) * time.Millisecond)
//				return nil, newMyError("fail2")
//			}
//		}
//		if wait {
//			f = WaitAll(task1, task2)
//		} else {
//			f = WhenAllFuture(Start(task1), Start(task2))
//		}
//		return f
//	}
//	whenTwoTask := func(t1 int, t2 int) *Future {
//		return startTwoTask(t1, t2, false)
//	}
//	r, err := whenTwoTask(200, 250).Get()
//	AreEqual(r, []interface{}{"ok", "ok2"}, t)
//	AreEqual(err, nil, t)

//	r, err = whenTwoTask(250, 210).Get()
//	AreEqual(r, []interface{}{"ok", "ok2"}, t)
//	AreEqual(err, nil, t)

//	r, err = whenTwoTask(-250, 210).Get()
//	AreEqual(err.(*AggregateError).InnerErrs[0].(*myError).val, "fail", t)
//	//AreEqual(err.(*AggregateError).InnerErrs[1], nil, t)
//	AreEqual(r, nil, t)

//	r, err = whenTwoTask(-250, -210).Get()
//	AreEqual(err.(*AggregateError).InnerErrs[0].(*myError).val, "fail", t)
//	//AreEqual(err.(*AggregateError).InnerErrs[1].(*myError).val, []interface{}{-20, "fail2"}, t)
//	AreEqual(r, nil, t)

//	r, err = WhenAllFuture().Get()
//	AreEqual(r, []interface{}{}, t)
//	AreEqual(err, nil, t)

//	waitTwoTask := func(t1 int, t2 int) *Future {
//		return startTwoTask(t1, t2, true)
//	}
//	r, err = waitTwoTask(200, 250).Get()
//	AreEqual(r, []interface{}{"ok", "ok2"}, t)
//	AreEqual(err, nil, t)

//	r, err = waitTwoTask(250, 210).Get()
//	AreEqual(r, []interface{}{"ok", "ok2"}, t)
//	AreEqual(err, nil, t)

//	r, err = waitTwoTask(-250, 210).Get()
//	AreEqual(err.(*AggregateError).InnerErrs[0].(*myError).val, "fail", t)
//	//AreEqual(err.(*AggregateError).InnerErrs[1], nil, t)
//	AreEqual(r, nil, t)

//	r, err = waitTwoTask(-250, -210).Get()
//	AreEqual(err.(*AggregateError).InnerErrs[0].(*myError).val, "fail", t)
//	AreEqual(err.(*AggregateError).InnerErrs[1].(*myError).val, "fail2", t)
//	AreEqual(r, nil, t)

//	r, err = WaitAll().Get()
//	AreEqual(r, []interface{}{}, t)
//	AreEqual(err, nil, t)
//}

//func TestWrap(t *testing.T) {
//	r, err := Wrap(10).Get()
//	AreEqual(r, 10, t)
//	AreEqual(err, nil, t)

//}

//func TestCancel(t *testing.T) {
//	i := 0
//	task := func(canceller Canceller) (interface{}, error) {
//		for i < 50 {
//			if canceller.IsCancellationRequested() {
//				canceller.SetCancelled()
//				return nil, nil
//			}
//			time.Sleep(100 * time.Millisecond)
//		}
//		panic("exception")
//	}

//	f := Start(task)
//	f.RequestCancel()
//	r, err := f.Get()
//	AreEqual(f.IsCancelled(), true, t)
//	AreEqual(r, nil, t)
//	AreEqual(err.Error(), (&CancelledError{}).Error(), t)

//	task = func(canceller Canceller) (interface{}, error) {
//		time.Sleep(100 * time.Millisecond)
//		return 1, nil
//	}
//	f = Start(task)
//	c := f.RequestCancel()
//	AreEqual(c, true, t)
//	r, err = f.Get()
//	AreEqual(r, 1, t)
//	AreEqual(err, nil, t)

//	AreEqual(f.IsCancelled(), false, t)

//	task1 := func() (interface{}, error) {
//		time.Sleep(100 * time.Millisecond)
//		return 1, nil
//	}
//	f = Start(task1)
//	c = f.RequestCancel()
//	AreEqual(c, false, t)
//	r, err = f.Get()
//	AreEqual(r, 1, t)
//	AreEqual(err, nil, t)

//	AreEqual(f.IsCancelled(), false, t)

//}
