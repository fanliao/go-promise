package promise

import (
	"errors"
	"fmt"
	c "github.com/smartystreets/goconvey/convey"
	"reflect"
	"strconv"
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

func TestResolveAndReject(t *testing.T) {
	c.Convey("When Promise is resolved", t, func() {
		p := NewPromise()
		go func() {
			time.Sleep(50 * time.Millisecond)
			p.Resolve("ok")
		}()
		r, err := p.Get()
		c.So(r, c.ShouldEqual, "ok")
		c.So(err, c.ShouldBeNil)
	})

	c.Convey("When Promise is rejected", t, func() {
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
	c.Convey("When Promise is cancelled", t, func() {
		p := NewPromise()
		go func() {
			time.Sleep(50 * time.Millisecond)
			p.Cancel()
		}()
		r, err := p.Get()
		c.So(r, c.ShouldBeNil)
		c.So(err, c.ShouldHaveSameTypeAs, &CancelledError{})
	})
	i := 0
	task := func(canceller Canceller) (interface{}, error) {
		for i < 50 {
			if canceller.IsCancellationRequested() {
				canceller.SetCancelled()
				return nil, nil
			}
			time.Sleep(100 * time.Millisecond)
		}
		panic("exception")
	}

	f := Start(task)
	f.RequestCancel()
	r, err := f.Get()
	AreEqual(f.IsCancelled(), true, t)
	AreEqual(r, nil, t)
	AreEqual(err.Error(), (&CancelledError{}).Error(), t)

	task = func(canceller Canceller) (interface{}, error) {
		time.Sleep(100 * time.Millisecond)
		return 1, nil
	}
	f = Start(task)
	c := f.RequestCancel()
	AreEqual(c, true, t)
	r, err = f.Get()
	AreEqual(r, 1, t)
	AreEqual(err, nil, t)

	AreEqual(f.IsCancelled(), false, t)

	task1 := func() (interface{}, error) {
		time.Sleep(100 * time.Millisecond)
		return 1, nil
	}
	f = Start(task1)
	c = f.RequestCancel()
	AreEqual(c, false, t)
	r, err = f.Get()
	AreEqual(r, 1, t)
	AreEqual(err, nil, t)

	AreEqual(f.IsCancelled(), false, t)
}

func TestGetOrTimeout(t *testing.T) {
	timout := 50 * time.Millisecond
	c.Convey("When Promise is unfinished", t, func() {
		p := NewPromise()
		go func() {
			<-time.After(timout)
			p.Resolve("ok")
		}()
		r, err, timeout := p.GetOrTimeout(10)
		c.So(timeout, c.ShouldEqual, true)
		c.So(r, c.ShouldBeNil)
		c.So(err, c.ShouldBeNil)

		c.Convey("When Promise is resolved", func() {
			r, err, timeout := p.GetOrTimeout(50)
			c.So(timeout, c.ShouldEqual, false)
			c.So(r, c.ShouldEqual, "ok")
			c.So(err, c.ShouldBeNil)
		})
	})

	c.Convey("When Promise is rejected", t, func() {
		p := NewPromise()
		go func() {
			<-time.After(timout)
			p.Reject(errors.New("fail"))
		}()
		r, err, timeout := p.GetOrTimeout(53)
		c.So(timeout, c.ShouldEqual, false)
		c.So(r, c.ShouldBeNil)
		c.So(err, c.ShouldNotBeNil)
	})

	c.Convey("When Promise is cancelled", t, func() {
		p := NewPromise()
		go func() {
			<-time.After(timout)
			p.Cancel()
		}()
		r, err, timeout := p.GetOrTimeout(53)
		c.So(timeout, c.ShouldEqual, false)
		c.So(r, c.ShouldBeNil)
		c.So(err, c.ShouldHaveSameTypeAs, &CancelledError{})
	})
}

func TestGetChan(t *testing.T) {
	timout := 50 * time.Millisecond
	c.Convey("When Promise is resolved", t, func() {
		p := NewPromise()
		go func() {
			<-time.After(timout)
			p.Resolve("ok")
		}()
		fr, ok := <-p.GetChan()
		c.So(fr.Result, c.ShouldEqual, "ok")
		c.So(fr.Typ, c.ShouldEqual, RESULT_SUCCESS)
		c.So(ok, c.ShouldBeTrue)
	})

	c.Convey("When Promise is rejected", t, func() {
		p := NewPromise()
		go func() {
			<-time.After(timout)
			p.Reject(errors.New("fail"))
		}()
		fr, ok := <-p.GetChan()
		c.So(fr.Result, c.ShouldNotBeNil)
		c.So(fr.Typ, c.ShouldEqual, RESULT_FAILURE)
		c.So(ok, c.ShouldBeTrue)
	})

	c.Convey("When Promise is cancelled", t, func() {
		p := NewPromise()
		go func() {
			<-time.After(timout)
			p.Cancel()
		}()
		fr, ok := <-p.GetChan()
		c.So(fr.Result, c.ShouldBeNil)
		c.So(fr.Typ, c.ShouldEqual, RESULT_CANCELLED)
		c.So(ok, c.ShouldBeTrue)
	})
}

func TestFuture(t *testing.T) {
	var fu *Future
	c.Convey("When Future is resolved", t, func() {
		func() {
			p := NewPromise()
			go func() {
				time.Sleep(50 * time.Millisecond)
				p.Resolve("ok")
			}()
			fu = p.Future
		}()
		r, err := fu.Get()
		c.So(r, c.ShouldEqual, "ok")
		c.So(err, c.ShouldBeNil)
	})

	c.Convey("When Future is rejected", t, func() {
		func() {
			p := NewPromise()
			go func() {
				time.Sleep(50 * time.Millisecond)
				p.Reject(errors.New("fail"))
			}()
			fu = p.Future
		}()
		r, err := fu.Get()
		c.So(err, c.ShouldNotBeNil)
		c.So(r, c.ShouldEqual, nil)
	})

	c.Convey("When Future is cancelled", t, func() {
		func() {
			p := NewPromise()
			go func() {
				time.Sleep(50 * time.Millisecond)
				p.Cancel()
			}()
			fu = p.Future
		}()
		r, err := fu.Get()
		c.So(err, c.ShouldNotBeNil)
		c.So(r, c.ShouldEqual, nil)
	})

}

func TestCallbacks(t *testing.T) {
	timout := 50 * time.Millisecond
	done, always, fail := false, false, false

	p := NewPromise()
	go func() {
		<-time.After(timout)
		p.Resolve("ok")
	}()

	c.Convey("When Promise is resolved", t, func() {
		p.Done(func(v interface{}) {
			done = true
			c.Convey("The argument of Done should be 'ok'", t, func() {
				c.So(v, c.ShouldEqual, "ok")
			})
		}).Always(func(v interface{}) {
			always = true
			c.Convey("The argument of Always should be 'ok'", t, func() {
				c.So(v, c.ShouldEqual, "ok")
			})
		}).Fail(func(v interface{}) {
			fail = true
			panic("Unexpected calling")
		})
		r, err := p.Get()

		//The code after Get() and the callback will be concurrent run
		//So sleep 52 ms to wait all callback be done
		time.Sleep(52 * time.Millisecond)

		c.So(r, c.ShouldEqual, "ok")
		c.So(err, c.ShouldBeNil)
		c.So(done, c.ShouldEqual, true)
		c.So(always, c.ShouldEqual, true)
		c.So(fail, c.ShouldEqual, false)
	})

	c.Convey("When adding the callback after Promise is resolved", t, func() {
		done, always, fail := false, false, false
		p.Done(func(v interface{}) {
			done = true
			c.Convey("The argument of Done should be 'ok'", func() {
				c.So(v, c.ShouldEqual, "ok")
			})
		}).Always(func(v interface{}) {
			always = true
			c.Convey("The argument of Always should be 'ok'", func() {
				c.So(v, c.ShouldEqual, "ok")
			})
		}).Fail(func(v interface{}) {
			fail = true
			panic("Unexpected calling")
		})
		c.So(done, c.ShouldEqual, true)
		c.So(always, c.ShouldEqual, true)
		c.So(fail, c.ShouldEqual, false)
	})

	var e *error = nil
	done, always, fail = false, false, false
	p = NewPromise()
	go func() {
		<-time.After(timout)
		p.Reject(errors.New("fail"))
	}()

	c.Convey("When Promise is rejected", t, func() {
		p.Done(func(v interface{}) {
			done = true
			panic("Unexpected calling")
		}).Always(func(v interface{}) {
			always = true
			c.Convey("The argument of Always should be error", t, func() {
				c.So(v, c.ShouldImplement, e)
			})
		}).Fail(func(v interface{}) {
			fail = true
			c.Convey("The argument of Fail should be error", t, func() {
				c.So(v, c.ShouldImplement, e)
			})
		})
		r, err := p.Get()

		time.Sleep(52 * time.Millisecond)

		c.So(r, c.ShouldEqual, nil)
		c.So(err, c.ShouldNotBeNil)
		c.So(done, c.ShouldEqual, false)
		c.So(always, c.ShouldEqual, true)
		c.So(fail, c.ShouldEqual, true)
	})

	c.Convey("When adding the callback after Promise is rejected", t, func() {
		done, always, fail = false, false, false
		p.Done(func(v interface{}) {
			done = true
			panic("Unexpected calling")
		}).Always(func(v interface{}) {
			always = true
			c.Convey("The argument of Always should be error", func() {
				c.So(v, c.ShouldImplement, e)
			})
		}).Fail(func(v interface{}) {
			fail = true
			c.Convey("The argument of Fail should be error", func() {
				c.So(v, c.ShouldImplement, e)
			})
		})
		c.So(done, c.ShouldEqual, false)
		c.So(always, c.ShouldEqual, true)
		c.So(fail, c.ShouldEqual, true)
	})

	done, always, fail = false, false, false
	p = NewPromise()
	go func() {
		<-time.After(timout)
		p.Cancel()
	}()

	c.Convey("When Promise is cancelled", t, func() {
		done, always, fail = false, false, false
		p.Done(func(v interface{}) {
			done = true
		}).Always(func(v interface{}) {
			always = true
		}).Fail(func(v interface{}) {
			fail = true
		})
		r, err := p.Get()

		time.Sleep(52 * time.Millisecond)

		c.So(r, c.ShouldBeNil)
		c.So(err, c.ShouldNotBeNil)
		c.So(done, c.ShouldEqual, false)
		c.So(always, c.ShouldEqual, false)
		c.So(fail, c.ShouldEqual, false)
	})

	c.Convey("When adding the callback after Promise is cancelled", t, func() {
		done, always, fail = false, false, false
		p.Done(func(v interface{}) {
			done = true
		}).Always(func(v interface{}) {
			always = true
		}).Fail(func(v interface{}) {
			fail = true
		})
		c.So(done, c.ShouldEqual, false)
		c.So(always, c.ShouldEqual, false)
		c.So(fail, c.ShouldEqual, false)
	})

}

func TestStart(t *testing.T) {
	c.Convey("Test start func()", t, func() {
	})

	c.Convey("Test start func()(interface{}, error)", t, func() {
	})

	c.Convey("Test start func(canceller Canceller)", t, func() {
	})

	c.Convey("Test start func(canceller Canceller)(interface{}, error)", t, func() {
	})
}

func TestPipeWhenDone(t *testing.T) {
	timout := 50 * time.Millisecond
	taskDonePipe := func(v interface{}) *Future {
		return Start(func() (interface{}, error) {
			<-time.After(timout)
			return v.(string) + "2", nil
		})
	}

	taskFailPipe := func(v interface{}) *Future {
		return Start(func() (interface{}, error) {
			<-time.After(timout)
			return "fail2", nil
		})
	}

	c.Convey("test Done branch for Pipe function", t, func() {
		p := NewPromise()
		go func() {
			<-time.After(timout)
			p.Resolve("ok")
		}()
		fu, ok := p.Pipe(taskDonePipe, taskFailPipe)
		r, err := fu.Get()

		c.So(r, c.ShouldEqual, "ok2")
		c.So(err, c.ShouldBeNil)
		c.So(ok, c.ShouldEqual, true)
	})

	c.Convey("test Fail branch for Pipe function", t, func() {
		p := NewPromise()
		go func() {
			<-time.After(timout)
			p.Reject(errors.New("fail"))
		}()
		fu, ok := p.Pipe(taskDonePipe, taskFailPipe)
		r, err := fu.Get()

		c.So(r, c.ShouldEqual, "fail2")
		c.So(err, c.ShouldBeNil)
		c.So(ok, c.ShouldEqual, true)
	})

	c.Convey("test pipe twice", t, func() {
		p := NewPromise()
		_, ok := p.Pipe(taskDonePipe, taskFailPipe)
		c.So(ok, c.ShouldEqual, true)
		_, ok = p.Pipe(taskDonePipe, taskFailPipe)
		c.So(ok, c.ShouldEqual, false)
	})
}

func TestWhenAny(t *testing.T) {
	c.Convey("Test WhenAny", t, func() {
		whenTwoTask := func(t1 int, t2 int) *Future {
			timeouts := []time.Duration{time.Duration(t1), time.Duration(t2)}
			getTask := func(i int) func() (interface{}, error) {
				return func() (interface{}, error) {
					if timeouts[i] > 0 {
						time.Sleep(timeouts[i] * time.Millisecond)
						return "ok" + strconv.Itoa(i), nil
					} else {
						time.Sleep((-1 * timeouts[i]) * time.Millisecond)
						return nil, newMyError("fail" + strconv.Itoa(i))
					}
				}
			}
			task0 := getTask(0)
			task1 := getTask(1)
			f := WhenAny(Start(task0), Start(task1))
			return f
		}

		c.Convey("When all tasks completed, and task 1 be first to complete", func() {
			r, err := whenTwoTask(200, 250).Get()
			c.So(r, c.ShouldEqual, "ok0")
			c.So(err, c.ShouldBeNil)
		})

		c.Convey("When all tasks completed, and task 2 be first to complete", func() {
			r, err := whenTwoTask(280, 250).Get()
			c.So(r, c.ShouldEqual, "ok1")
			c.So(err, c.ShouldBeNil)
		})

		c.Convey("When all tasks failed", func() {
			r, err := whenTwoTask(-280, -250).Get()
			errs := err.(*AggregateError).InnerErrs
			c.So(r, c.ShouldBeNil)
			c.So(errs[0].(*myError).val, c.ShouldEqual, "fail0")
			c.So(errs[1].(*myError).val, c.ShouldEqual, "fail1")
		})

		c.Convey("When one task completed", func() {
			r, err := whenTwoTask(-280, 150).Get()
			c.So(r, c.ShouldEqual, "ok1")
			c.So(err, c.ShouldBeNil)
		})

		c.Convey("When no task be passed", func() {
			r, err := WhenAny().Get()
			c.So(r, c.ShouldBeNil)
			c.So(err, c.ShouldBeNil)
		})
	})

	c.Convey("Test WhenAny with task can be cancelled", t, func() {
		var c1, c2 bool
		startTwoCanCancelTask := func(t1 int, t2 int) *Future {
			timeouts := []time.Duration{time.Duration(t1), time.Duration(t2)}
			getTask := func(i int) func(canceller Canceller) (interface{}, error) {
				return func(canceller Canceller) (interface{}, error) {
					for j := 0; j < 10; j++ {
						if timeouts[i] > 0 {
							time.Sleep(timeouts[i] * time.Millisecond)
						} else {
							time.Sleep((-1 * timeouts[i]) * time.Millisecond)
						}
						if canceller.IsCancellationRequested() {
							canceller.SetCancelled()
							if i == 0 {
								c1 = true
							} else {
								c2 = true
							}
							return nil, nil
						}
					}
					if timeouts[i] > 0 {
						return "ok" + strconv.Itoa(i), nil
					} else {
						return nil, newMyError("fail" + strconv.Itoa(i))
					}
				}
			}
			task0 := getTask(0)
			task1 := getTask(1)
			f := WhenAny(Start(task0), Start(task1))
			return f
		}
		c.Convey("When task 1 is the first to complete, task 2 will be cancelled", func() {
			r, err := startTwoCanCancelTask(10, 250).Get()

			c.So(r, c.ShouldEqual, "ok0")
			c.So(err, c.ShouldBeNil)
			time.Sleep(1000 * time.Millisecond)
			c.So(c2, c.ShouldEqual, true)
		})

		c.Convey("When task 2 is the first to complete, task 1 will be cancelled", func() {
			r, err := startTwoCanCancelTask(200, 10).Get()

			c.So(r, c.ShouldEqual, "ok1")
			c.So(err, c.ShouldBeNil)
			time.Sleep(1000 * time.Millisecond)
			c.So(c1, c.ShouldEqual, true)
		})
	})
}

func TestWhenAnyTrue(t *testing.T) {
	c1, c2 := false, false
	startTwoCanCancelTask := func(t1 int, t2 int, predicate func(interface{}) bool) *Future {
		timeouts := []time.Duration{time.Duration(t1), time.Duration(t2)}
		getTask := func(i int) func(canceller Canceller) (interface{}, error) {
			return func(canceller Canceller) (interface{}, error) {
				for j := 0; j < 10; j++ {
					if timeouts[i] > 0 {
						time.Sleep(timeouts[i] * time.Millisecond)
					} else {
						time.Sleep((-1 * timeouts[i]) * time.Millisecond)
					}
					if canceller.IsCancellationRequested() {
						canceller.SetCancelled()
						if i == 0 {
							c1 = true
						} else {
							c2 = true
						}
						return nil, nil
					}
				}
				if timeouts[i] > 0 {
					return "ok" + strconv.Itoa(i), nil
				} else {
					return nil, newMyError("fail" + strconv.Itoa(i))
				}
			}
		}
		task0 := getTask(0)
		task1 := getTask(1)
		f := WhenAnyTrue(predicate, Start(task0), Start(task1))
		return f
	}
	//第一个任务先完成，第二个后完成，并且设定条件为返回值==第一个的返回值
	c.Convey("When the task1 is the first to complete, and predicate returns true", t, func() {
		r, err := startTwoCanCancelTask(30, 250, func(v interface{}) bool {
			return v.(string) == "ok0"
		}).Get()
		c.So(r, c.ShouldEqual, "ok0")
		c.So(err, c.ShouldBeNil)
		time.Sleep(1000 * time.Millisecond)
		c.So(c2, c.ShouldEqual, true)
	})

	//第一个任务后完成，第二个先完成，并且设定条件为返回值==第二个的返回值
	c.Convey("When the task2 is the first to complete, and predicate returns true", t, func() {
		c1, c2 = false, false
		r, err := startTwoCanCancelTask(230, 50, func(v interface{}) bool {
			return v.(string) == "ok1"
		}).Get()
		c.So(r, c.ShouldEqual, "ok1")
		c.So(err, c.ShouldBeNil)
		time.Sleep(1000 * time.Millisecond)
		c.So(c1, c.ShouldEqual, true)
	})

	//第一个任务后完成，第二个先完成，并且设定条件为返回值不等于任意一个任务的返回值
	c.Convey("When the task2 is the first to complete, and predicate always returns false", t, func() {
		c1, c2 = false, false
		r, err := startTwoCanCancelTask(30, 250, func(v interface{}) bool {
			return v.(string) == "ok11"
		}).Get()
		c.So(r, c.ShouldEqual, false)
		c.So(err, c.ShouldBeNil)
		time.Sleep(1000 * time.Millisecond)
		c.So(c1, c.ShouldEqual, false)
		c.So(c2, c.ShouldEqual, false)
	})
}

func TestWhenAll(t *testing.T) {
	startTwoTask := func(t1 int, t2 int, wait bool) (f *Future) {
		timeouts := []time.Duration{time.Duration(t1), time.Duration(t2)}
		getTask := func(i int) func() (interface{}, error) {
			return func() (interface{}, error) {
				if timeouts[i] > 0 {
					time.Sleep(timeouts[i] * time.Millisecond)
					return "ok" + strconv.Itoa(i), nil
				} else {
					time.Sleep((-1 * timeouts[i]) * time.Millisecond)
					return nil, newMyError("fail" + strconv.Itoa(i))
				}
			}
		}
		task0 := getTask(0)
		task1 := getTask(1)
		if wait {
			f = WaitAll(task0, task1)
		} else {
			f = WhenAllFuture(Start(task0), Start(task1))
		}
		return f
	}
	c.Convey("Test WhenAllFuture", t, func() {
		whenTwoTask := func(t1 int, t2 int) *Future {
			return startTwoTask(t1, t2, false)
		}
		c.Convey("When all tasks completed, and the task1 is the first to complete", func() {
			r, err := whenTwoTask(200, 230).Get()
			c.So(r, shouldSlicesReSame, []interface{}{"ok0", "ok1"})
			c.So(err, c.ShouldBeNil)
		})

		c.Convey("When all tasks completed, and the task1 is the first to complete", func() {
			r, err := whenTwoTask(230, 200).Get()
			c.So(r, shouldSlicesReSame, []interface{}{"ok0", "ok1"})
			c.So(err, c.ShouldBeNil)
		})

		c.Convey("When task1 failed, but task2 is completed", func() {
			r, err := whenTwoTask(-250, 210).Get()
			c.So(err.(*AggregateError).InnerErrs[0].(*myError).val, c.ShouldEqual, "fail0")
			c.So(r, c.ShouldBeNil)
		})

		c.Convey("When all tasks failed", func() {
			r, err := whenTwoTask(-250, -210).Get()
			c.So(err.(*AggregateError).InnerErrs[0].(*myError).val, c.ShouldEqual, "fail0")
			c.So(r, c.ShouldBeNil)
		})

		c.Convey("When no task be passed", func() {
			r, err := WhenAllFuture().Get()
			c.So(r, shouldSlicesReSame, []interface{}{})
			c.So(err, c.ShouldBeNil)
		})
	})

	c.Convey("Test WaitAll", t, func() {
		waitTwoTask := func(t1 int, t2 int) *Future {
			return startTwoTask(t1, t2, true)
		}
		c.Convey("When all tasks completed, and the task1 is the first to complete", func() {
			r, err := waitTwoTask(200, 250).Get()
			c.So(r, shouldSlicesReSame, []interface{}{"ok0", "ok1"})
			c.So(err, c.ShouldBeNil)
		})

		c.Convey("When all tasks completed, and the task2 is the first to complete", func() {
			r, err := waitTwoTask(250, 210).Get()
			c.So(r, shouldSlicesReSame, []interface{}{"ok0", "ok1"})
			c.So(err, c.ShouldBeNil)
		})

		c.Convey("When task1 failed, and task2 completed", func() {
			r, err := waitTwoTask(-250, 210).Get()
			c.So(err.(*AggregateError).InnerErrs[0].(*myError).val, c.ShouldEqual, "fail0")
			c.So(r, c.ShouldBeNil)
		})

		c.Convey("When all tasks failed", func() {
			r, err := waitTwoTask(-250, -210).Get()
			c.So(err.(*AggregateError).InnerErrs[0].(*myError).val, c.ShouldEqual, "fail0")
			c.So(err.(*AggregateError).InnerErrs[1].(*myError).val, c.ShouldEqual, "fail1")
			c.So(r, c.ShouldBeNil)
		})

		c.Convey("When no task be passed", func() {
			r, err := WaitAll().Get()
			c.So(r, shouldSlicesReSame, []interface{}{})
			c.So(err, c.ShouldBeNil)
		})
	})
}

func TestWrap(t *testing.T) {
	r, err := Wrap(10).Get()
	AreEqual(r, 10, t)
	AreEqual(err, nil, t)
}

func shouldSlicesReSame(actual interface{}, expected ...interface{}) string {
	actualSlice, expectedSlice := reflect.ValueOf(actual), reflect.ValueOf(expected[0])
	if actualSlice.Kind() != expectedSlice.Kind() {
		return fmt.Sprintf("Expected1: '%v'\nActual:   '%v'\n", expected[0], actual)
	}

	if actualSlice.Kind() != reflect.Slice {
		return fmt.Sprintf("Expected2: '%v'\nActual:   '%v'\n", expected[0], actual)
	}

	if actualSlice.Len() != expectedSlice.Len() {
		return fmt.Sprintf("Expected3: '%v'\nActual:   '%v'\n", expected[0], actual)
	}

	for i := 0; i < actualSlice.Len(); i++ {
		if !reflect.DeepEqual(actualSlice.Index(i).Interface(), expectedSlice.Index(i).Interface()) {
			return fmt.Sprintf("Expected4: '%v'\nActual:   '%v'\n", expected[0], actual)
		}
	}
	return ""
}
