package promise

import (
	"fmt"
	//"errors"
	"strings"
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
	return []interface{}{10, "ok"}, nil
}
var taskFail func() (interface{}, error) = func() (interface{}, error) {
	time.Sleep(500 * time.Millisecond)
	order = append(order, TASK_END)
	return nil, newMyError([]interface{}{10, "fail"})
}

var done func(v interface{}) = func(v interface{}) {
	time.Sleep(50 * time.Millisecond)
	order = append(order, CALL_DONE)
	AreEqual(v, []interface{}{10, "ok"}, tObj)
}
var alwaysForDone func(v interface{}) = func(v interface{}) {
	order = append(order, CALL_ALWAYS)
	AreEqual(v, []interface{}{10, "ok"}, tObj)
}
var fail func(v interface{}) = func(v interface{}) {
	time.Sleep(50 * time.Millisecond)
	order = append(order, CALL_FAIL)
	AreEqual(v.(*myError).val, []interface{}{10, "fail"}, tObj)
}
var alwaysForFail func(v interface{}) = func(v interface{}) {
	order = append(order, CALL_ALWAYS)
	AreEqual(v.(*myError).val, []interface{}{10, "fail"}, tObj)
}

//func TestDoneAlways(t *testing.T) {
//	tObj = t
//	order = make([]string, 0, 10)
//	f := Start(taskDone).Done(done).Always(alwaysForDone).Done(done)

//	r, err := f.Get()
//	order = append(order, GET)
//	//The code after Get() and the callback will be concurrent run
//	//So sleep 500 ms to wait all callback be done
//	time.Sleep(500 * time.Millisecond)

//	//The always callback run after all done or fail callbacks be done
//	AreEqual(order, []string{TASK_END, GET, CALL_DONE, CALL_DONE, CALL_ALWAYS}, t)
//	AreEqual(r, []interface{}{10, "ok"}, t)
//	AreEqual(err, nil, t)
//	t.Log(f.r)

//	//if task be done, the callback function will be immediately called
//	f.Done(done).Fail(fail)
//	AreEqual(order, []string{TASK_END, GET, CALL_DONE, CALL_DONE, CALL_ALWAYS, CALL_DONE}, t)
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
//	AreEqual(err.(*myError).val, []interface{}{10, "fail"}, t)

//}

func TestPipeWhenDone(t *testing.T) {
	tObj = t
	taskDonePipe := func(v interface{}) *Future {
		return Start(func() (interface{}, error) {
			vs := v.([]interface{})
			time.Sleep(100 * time.Millisecond)
			order = append(order, DONE_Pipe_END)
			return []interface{}{vs[0].(int) * 2, vs[1].(string) + "2"}, nil
		})
	}

	taskFailPipe := func(v interface{}) *Future {
		return Start(func() (interface{}, error) {
			vs := v.(*myError).val.([]interface{})
			time.Sleep(100 * time.Millisecond)
			order = append(order, FAIL_Pipe_END)
			return nil, newMyError([]interface{}{vs[0].(int) * 2, vs[1].(string) + "2"})
		})
	}

	SubmitWithCallback := func(task func() (interface{}, error)) (*Future, bool) {
		return Start(task).Done(done).Fail(fail).
			Pipe(taskDonePipe, taskFailPipe)
	}

	//test Done branch for Pipe function
	order = make([]string, 0, 10)
	f, isOk := SubmitWithCallback(taskDone)
	r, err := f.Get()
	order = append(order, GET)
	time.Sleep(300 * time.Millisecond)

	AreEqual(order, []string{TASK_END, CALL_DONE, DONE_Pipe_END, GET}, t)
	AreEqual(r, []interface{}{20, "ok2"}, t)
	AreEqual(err, nil, t)
	AreEqual(isOk, true, t)

	//test fail branch for Pipe function
	order = make([]string, 0, 10)
	f, isOk = SubmitWithCallback(taskFail)
	r, err = f.Get()
	order = append(order, GET)
	time.Sleep(300 * time.Millisecond)

	AreEqual(order, []string{TASK_END, CALL_FAIL, FAIL_Pipe_END, GET}, t)
	AreEqual(err.(*myError).val, []interface{}{20, "fail2"}, t)
	AreEqual(r, nil, t)
	AreEqual(isOk, true, t)

	f, isOk = f.Pipe(taskDonePipe, taskFailPipe)
	//t.Log("isok?", isOk, f, f.oncePipe)
	AreEqual(isOk, true, t)
	_, _ = f.Get()
}

func TestGetOrTimeout(t *testing.T) {
	tObj = t
	order = make([]string, 0, 10)
	AreEqual(order, []string{}, t)
	f := Start(taskDone)

	AreEqual(order, []string{}, t)
	//timeout
	r, err, timeout := f.GetOrTimeout(100)
	AreEqual(timeout, true, t)

	order = append(order, GET)
	AreEqual(order, []string{GET}, t)
	//get return value
	r, err, timeout = f.GetOrTimeout(470)
	AreEqual(timeout, false, t)
	AreEqual(order, []string{GET, TASK_END}, t)
	AreEqual(r, []interface{}{10, "ok"}, t)
	AreEqual(err, nil, t)

	//if task be done and timeout is 0, still can get return value
	r, err, timeout = f.GetOrTimeout(0)
	AreEqual(timeout, false, t)
	AreEqual(r, []interface{}{10, "ok"}, t)
	AreEqual(err, nil, t)
}

func TestException(t *testing.T) {
	order = make([]string, 0, 10)
	task := func() (interface{}, error) {
		time.Sleep(500 * time.Millisecond)
		order = append(order, "task be end,")
		panic("unknown exception")
		return []interface{}{10, "ok"}, nil
	}

	f := Start(task).Done(func(v interface{}) {
		time.Sleep(200 * time.Millisecond)
		order = append(order, "run Done callback,")
	}).Always(func(v interface{}) {
		order = append(order, "run Always callback,")
		if !strings.Contains(v.(error).Error(), "unknown exception") {
			t.Log("Failed! actual", v)
			t.Fail()
		}
		//AreEqual(v, []interface{}{"exception"}, t)
	}).Fail(func(v interface{}) {
		order = append(order, "run Fail callback,")
		if !strings.Contains(v.(error).Error(), "unknown exception") {
			t.Log("Failed! actual", v)
			t.Fail()
		}
	})

	r, err := f.Get()
	time.Sleep(200 * time.Millisecond)
	AreEqual(order, []string{"task be end,", "run Fail callback,", "run Always callback,"}, t)
	if !strings.Contains(err.Error(), "unknown exception") {
		t.Log("Failed! actual", err.Error())
		t.Fail()
	}
	AreEqual(r, nil, t)

}

func TestWhenAny(t *testing.T) {
	startTwoTask := func(t1 int, t2 int) *Future {
		timeout1 := time.Duration(t1)
		timeout2 := time.Duration(t2)
		task1 := func() (interface{}, error) {
			if timeout1 > 0 {
				time.Sleep(timeout1 * time.Millisecond)
				return []interface{}{10, "ok"}, nil
			} else {
				time.Sleep((-1 * timeout1) * time.Millisecond)
				return nil, newMyError([]interface{}{-10, "fail"})
			}
		}
		task2 := func() (interface{}, error) {
			if timeout2 > 0 {
				time.Sleep(timeout2 * time.Millisecond)
				return []interface{}{20, "ok2"}, nil
			} else {
				time.Sleep((-1 * timeout2) * time.Millisecond)
				return nil, newMyError([]interface{}{-20, "fail2"})
			}
		}
		f := WhenAny(Start(task1), Start(task2))
		return f
	}

	r, err := startTwoTask(200, 250).Get()
	AreEqual(r, []interface{}{10, "ok"}, t)
	AreEqual(err, nil, t)

	r, err = startTwoTask(280, 250).Get()
	AreEqual(r, []interface{}{20, "ok2"}, t)
	AreEqual(err, nil, t)

	r, err = startTwoTask(-280, -250).Get()
	errs := err.(*AggregateError).InnerErrs
	AreEqual(errs[0].(*myError).val, []interface{}{-10, "fail"}, t)
	AreEqual(errs[1].(*myError).val, []interface{}{-20, "fail2"}, t)
	AreEqual(r, nil, t)

	r, err = startTwoTask(-280, 150).Get()
	AreEqual(r, []interface{}{20, "ok2"}, t)
	AreEqual(err, nil, t)

	r, err = WhenAny().Get()
	AreEqual(r, nil, t)
	AreEqual(err, nil, t)

	var c1, c2 bool
	startTwoCanCancelTask := func(t1 int, t2 int) *Future {
		timeout1 := time.Duration(t1)
		timeout2 := time.Duration(t2)
		task1 := func(canceller Canceller) (r interface{}, err error) {
			for i := 0; i < 10; i++ {
				if timeout1 > 0 {
					time.Sleep(timeout1 * time.Millisecond)
				} else {
					time.Sleep((-1 * timeout1) * time.Millisecond)
				}
				if canceller.IsCancellationRequested() {
					t.Log("cancel 1")
					canceller.SetCancelled()
					c1 = true
					return nil, nil
				}
			}
			if timeout1 > 0 {
				return []interface{}{10, "ok"}, nil
			} else {
				return nil, newMyError([]interface{}{-10, "fail"})
			}
		}
		task2 := func(canceller Canceller) (r interface{}, err error) {
			for i := 0; i < 10; i++ {
				if timeout2 > 0 {
					time.Sleep(timeout2 * time.Millisecond)
				} else {
					time.Sleep((-1 * timeout2) * time.Millisecond)
				}
				if canceller.IsCancellationRequested() {
					t.Log("cancel 2")
					canceller.SetCancelled()
					c2 = true
					return nil, nil
				}
			}
			if timeout2 > 0 {
				return []interface{}{20, "ok2"}, nil
			} else {
				return nil, newMyError([]interface{}{-20, "fail2"})
			}
		}
		f := WhenAny(StartCanCancel(task1), StartCanCancel(task2))
		return f
	}
	r, err = startTwoCanCancelTask(10, 250).Get()
	AreEqual(r, []interface{}{10, "ok"}, t)
	AreEqual(err, nil, t)
	time.Sleep(1000 * time.Millisecond)
	AreEqual(c2, true, t)

	//r, ok = startTwoCanCancelTask(280, 15).Get()
	//AreEqual(r, []interface{}{20, "ok2"}, t)
	//AreEqual(ok, RESULT_SUCCESS, t)
	//AreEqual(c1, true, t)
}

func TestWhenAll(t *testing.T) {
	startTwoTask := func(t1 int, t2 int) *Future {
		timeout1 := time.Duration(t1)
		timeout2 := time.Duration(t2)
		task1 := func() (r interface{}, err error) {
			if timeout1 > 0 {
				time.Sleep(timeout1 * time.Millisecond)
				return []interface{}{10, "ok"}, nil
			} else {
				time.Sleep((-1 * timeout1) * time.Millisecond)
				return nil, newMyError([]interface{}{-10, "fail"})
			}
		}
		task2 := func() (r interface{}, err error) {
			if timeout2 > 0 {
				time.Sleep(timeout2 * time.Millisecond)
				return []interface{}{20, "ok2"}, nil
			} else {
				time.Sleep((-1 * timeout2) * time.Millisecond)
				return nil, newMyError([]interface{}{-20, "fail2"})
			}
		}
		f := WhenAll(Start(task1), Start(task2))
		return f
	}
	r, err := startTwoTask(200, 250).Get()
	AreEqual(r, []interface{}{[]interface{}{10, "ok"}, []interface{}{20, "ok2"}}, t)
	AreEqual(err, nil, t)

	r, err = startTwoTask(250, 210).Get()
	AreEqual(r, []interface{}{[]interface{}{10, "ok"}, []interface{}{20, "ok2"}}, t)
	AreEqual(err, nil, t)

	r, err = startTwoTask(-250, 210).Get()
	AreEqual(err.(*AggregateError).InnerErrs[0].(*myError).val, []interface{}{-10, "fail"}, t)
	AreEqual(err.(*AggregateError).InnerErrs[1], nil, t)
	AreEqual(r, nil, t)

	r, err = startTwoTask(-250, -210).Get()
	AreEqual(err.(*AggregateError).InnerErrs[0].(*myError).val, []interface{}{-10, "fail"}, t)
	AreEqual(err.(*AggregateError).InnerErrs[1].(*myError).val, []interface{}{-20, "fail2"}, t)
	AreEqual(r, nil, t)

	r, err = WhenAll().Get()
	AreEqual(r, nil, t)
	AreEqual(err, nil, t)

}

func TestWrap(t *testing.T) {
	r, err := Wrap(10).Get()
	AreEqual(r, 10, t)
	AreEqual(err, nil, t)

}

func TestCancel(t *testing.T) {
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

	f := StartCanCancel(task)
	f.RequestCancel()
	r, err := f.Get()
	AreEqual(f.IsCancelled(), true, t)
	AreEqual(r, nil, t)
	AreEqual(err.Error(), (&CancelledError{}).Error(), t)

	task = func(canceller Canceller) (interface{}, error) {
		time.Sleep(100 * time.Millisecond)
		return 1, nil
	}
	f = StartCanCancel(task)
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
