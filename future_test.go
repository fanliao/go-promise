package promise

import (
	//"fmt"
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

var order []string
var tObj *testing.T

var taskDone func() []interface{} = func() []interface{} {
	time.Sleep(500 * time.Millisecond)
	order = append(order, TASK_END)
	return []interface{}{10, "ok", true}
}
var taskFail func() []interface{} = func() []interface{} {
	time.Sleep(500 * time.Millisecond)
	order = append(order, TASK_END)
	return []interface{}{10, "fail", false}
}

var done func(v ...interface{}) = func(v ...interface{}) {
	time.Sleep(50 * time.Millisecond)
	order = append(order, CALL_DONE)
	AreEqual(v, []interface{}{10, "ok"}, tObj)
}
var alwaysForDone func(v ...interface{}) = func(v ...interface{}) {
	order = append(order, CALL_ALWAYS)
	AreEqual(v, []interface{}{10, "ok"}, tObj)
}
var fail func(v ...interface{}) = func(v ...interface{}) {
	time.Sleep(50 * time.Millisecond)
	order = append(order, CALL_FAIL)
	AreEqual(v, []interface{}{10, "fail"}, tObj)
}
var alwaysForFail func(v ...interface{}) = func(v ...interface{}) {
	order = append(order, CALL_ALWAYS)
	AreEqual(v, []interface{}{10, "fail"}, tObj)
}

func TestDoneAlways(t *testing.T) {
	tObj = t
	order = make([]string, 0, 10)
	f := Start(taskDone).Done(done).Always(alwaysForDone).Done(done)

	r, ok := f.Get()
	order = append(order, GET)
	//The code after Get() and the callback will be concurrent run
	//So sleep 500 ms to wait all callback be done
	time.Sleep(500 * time.Millisecond)

	//The always callback run after all done or fail callbacks be done
	AreEqual(order, []string{TASK_END, GET, CALL_DONE, CALL_DONE, CALL_ALWAYS}, t)
	AreEqual(r, []interface{}{10, "ok"}, t)
	AreEqual(ok, true, t)

	//if task be done, the callback function will be immediately called
	f.Done(done).Fail(fail)
	AreEqual(order, []string{TASK_END, GET, CALL_DONE, CALL_DONE, CALL_ALWAYS, CALL_DONE}, t)
}

func TestFailAlways(t *testing.T) {
	tObj = t
	order = make([]string, 0, 10)
	f := Start(taskFail).Fail(fail).Always(alwaysForFail).Fail(fail)

	r, ok := f.Get()
	order = append(order, GET)
	time.Sleep(500 * time.Millisecond)

	AreEqual(order, []string{TASK_END, GET, CALL_FAIL, CALL_FAIL, CALL_ALWAYS}, t)
	AreEqual(r, []interface{}{10, "fail"}, t)
	AreEqual(ok, false, t)

}

func TestPipeWhenDone(t *testing.T) {
	tObj = t
	taskDonePipe := func(v ...interface{}) *Future {
		return Start(func() []interface{} {
			time.Sleep(100 * time.Millisecond)
			order = append(order, DONE_Pipe_END)
			return []interface{}{v[0].(int) * 2, v[1].(string) + "2", true}
		})
	}

	taskFailPipe := func(v ...interface{}) *Future {
		return Start(func() []interface{} {
			time.Sleep(100 * time.Millisecond)
			order = append(order, FAIL_Pipe_END)
			return []interface{}{v[0].(int) * 2, v[1].(string) + "2", false}
		})
	}

	SubmitWithCallback := func(task func() []interface{}) (*Future, bool) {
		return Start(task).Done(done).Fail(fail).
			Pipe(taskDonePipe, taskFailPipe)
	}

	//test Done branch for Pipe function
	order = make([]string, 0, 10)
	f, isOk := SubmitWithCallback(taskDone)
	r, ok := f.Get()
	order = append(order, GET)
	time.Sleep(300 * time.Millisecond)

	AreEqual(order, []string{TASK_END, CALL_DONE, DONE_Pipe_END, GET}, t)
	AreEqual(r, []interface{}{20, "ok2"}, t)
	AreEqual(ok, true, t)
	AreEqual(isOk, true, t)

	//test fail branch for Pipe function
	order = make([]string, 0, 10)
	f, isOk = SubmitWithCallback(taskFail)
	r, ok = f.Get()
	order = append(order, GET)
	time.Sleep(300 * time.Millisecond)

	AreEqual(order, []string{TASK_END, CALL_FAIL, FAIL_Pipe_END, GET}, t)
	AreEqual(r, []interface{}{20, "fail2"}, t)
	AreEqual(ok, false, t)
	AreEqual(isOk, true, t)

	f, isOk = f.Pipe(taskDonePipe, taskFailPipe)
	t.Log("isok?", isOk, f, f.oncePipe)
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
	r, ok, timeout := f.GetOrTimeout(100)
	AreEqual(timeout, true, t)

	order = append(order, GET)
	AreEqual(order, []string{GET}, t)
	//get return value
	r, ok, timeout = f.GetOrTimeout(470)
	AreEqual(timeout, false, t)
	AreEqual(order, []string{GET, TASK_END}, t)
	AreEqual(r, []interface{}{10, "ok"}, t)
	AreEqual(ok, true, t)

	//if task be done and timeout is 0, still can get return value
	r, ok, timeout = f.GetOrTimeout(0)
	AreEqual(timeout, false, t)
	AreEqual(r, []interface{}{10, "ok"}, t)
	AreEqual(ok, true, t)
}

func TestException(t *testing.T) {
	order = make([]string, 0, 10)
	task := func() []interface{} {
		time.Sleep(500 * time.Millisecond)
		order = append(order, "task be end,")
		panic("exception")
		return []interface{}{10, "ok", true}
	}

	f := Start(task).Done(func(v ...interface{}) {
		time.Sleep(200 * time.Millisecond)
		order = append(order, "run Done callback,")
	}).Always(func(v ...interface{}) {
		order = append(order, "run Always callback,")
		AreEqual(v, []interface{}{"exception"}, t)
	}).Fail(func(v ...interface{}) {
		order = append(order, "run Fail callback,")
		AreEqual(v, []interface{}{"exception"}, t)
	})

	r, ok := f.Get()
	time.Sleep(200 * time.Millisecond)
	AreEqual(order, []string{"task be end,", "run Fail callback,", "run Always callback,"}, t)
	AreEqual(r, []interface{}{"exception"}, t)
	AreEqual(ok, false, t)

}

func TestAny(t *testing.T) {
	startTwoTask := func(t1 int, t2 int) *Future {
		timeout1 := time.Duration(t1)
		timeout2 := time.Duration(t2)
		task1 := func() (r []interface{}) {
			if timeout1 > 0 {
				time.Sleep(timeout1 * time.Millisecond)
				r = []interface{}{10, "ok", true}
			} else {
				time.Sleep((-1 * timeout1) * time.Millisecond)
				r = []interface{}{-10, "fail", false}
			}
			return
		}
		task2 := func() (r []interface{}) {
			if timeout2 > 0 {
				time.Sleep(timeout2 * time.Millisecond)
				r = []interface{}{20, "ok2", true}
			} else {
				time.Sleep((-1 * timeout2) * time.Millisecond)
				r = []interface{}{-20, "fail2", false}
			}
			return
		}
		f := WhenAny(Start(task1), Start(task2))
		return f
	}

	r, ok := startTwoTask(200, 250).Get()
	AreEqual(r, []interface{}{10, "ok"}, t)
	AreEqual(ok, true, t)

	r, ok = startTwoTask(280, 250).Get()
	AreEqual(r, []interface{}{20, "ok2"}, t)
	AreEqual(ok, true, t)

	r, ok = startTwoTask(-280, -250).Get()
	AreEqual(r, []interface{}{[]interface{}{-10, "fail"}, []interface{}{-20, "fail2"}}, t)
	AreEqual(ok, false, t)

	r, ok = startTwoTask(-280, 150).Get()
	AreEqual(r, []interface{}{20, "ok2"}, t)
	AreEqual(ok, true, t)

	r, ok = WhenAny().Get()
	AreEqual(r, *new([]interface{}), t)
	AreEqual(ok, true, t)
}

func TestWhen(t *testing.T) {
	startTwoTask := func(t1 int, t2 int) *Future {
		timeout1 := time.Duration(t1)
		timeout2 := time.Duration(t2)
		task1 := func() (r []interface{}) {
			if timeout1 > 0 {
				time.Sleep(timeout1 * time.Millisecond)
				r = []interface{}{10, "ok", true}
			} else {
				time.Sleep((-1 * timeout1) * time.Millisecond)
				r = []interface{}{-10, "fail", false}
			}
			return
		}
		task2 := func() (r []interface{}) {
			if timeout2 > 0 {
				time.Sleep(timeout2 * time.Millisecond)
				r = []interface{}{20, "ok2", true}
			} else {
				time.Sleep((-1 * timeout2) * time.Millisecond)
				r = []interface{}{-20, "fail2", false}
			}
			return
		}
		f := WhenAll(Start(task1), Start(task2))
		return f
	}
	r, ok := startTwoTask(200, 250).Get()
	AreEqual(r, []interface{}{[]interface{}{10, "ok", true}, []interface{}{20, "ok2", true}}, t)
	AreEqual(ok, true, t)

	r, ok = startTwoTask(250, 210).Get()
	AreEqual(r, []interface{}{[]interface{}{10, "ok", true}, []interface{}{20, "ok2", true}}, t)
	AreEqual(ok, true, t)

	r, ok = startTwoTask(-250, 210).Get()
	AreEqual(r, []interface{}{[]interface{}{-10, "fail", false}, []interface{}{20, "ok2", true}}, t)
	AreEqual(ok, false, t)

	r, ok = startTwoTask(-250, -210).Get()
	AreEqual(r, []interface{}{[]interface{}{-10, "fail", false}, []interface{}{-20, "fail2", false}}, t)
	AreEqual(ok, false, t)

	r, ok = WhenAll().Get()
	AreEqual(r, *new([]interface{}), t)
	AreEqual(ok, true, t)
}

func TestWrap(t *testing.T) {
	r, ok := Wrap(10).Get()
	AreEqual(r, []interface{}{10}, t)
	AreEqual(ok, true, t)
}

func TestCancel(t *testing.T) {
	i := 0
	task := func(canceller Canceller) []interface{} {
		for i < 50 {
			if canceller.IsCancellationRequested() {
				canceller.SetIsCancelled()
				return nil
			}
			time.Sleep(100 * time.Millisecond)
		}
		panic("exception")
	}

	f := StartCanCancel(task)
	f.Cancel()
	r, ok := f.Get()
	AreEqual(f.IsCancelled(), true, t)
	AreEqual(len(r), 0, t)
	AreEqual(ok, true, t)

	task = func(canceller Canceller) []interface{} {
		time.Sleep(100 * time.Millisecond)
		return []interface{}{1}
	}
	f = StartCanCancel(task)
	f.Cancel()
	r, ok = f.Get()
	AreEqual(r, []interface{}{1}, t)
	AreEqual(ok, true, t)
	AreEqual(f.IsCancelled(), false, t)

	task1 := func() []interface{} {
		time.Sleep(100 * time.Millisecond)
		return []interface{}{1}
	}
	f = Start(task1)
	c := f.Cancel()
	r, ok = f.Get()
	AreEqual(r, []interface{}{1}, t)
	AreEqual(ok, true, t)
	AreEqual(f.IsCancelled(), false, t)
	AreEqual(c, false, t)

}
