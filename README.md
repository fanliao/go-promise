[home]: github.com/fanliao/go-promise

go-promise is a Go promise and future library.

Inspired by [Futures and promises]()

## Installation

    $ go get github.com/fanliao/go-promise

## Features

* Future and Promise
  * ```NewPromise()```
  * ```promise.Future```
* Promise and Future callbacks
  * ```.Done(...)```
  * ```.Fail(...)```
  * ```.Always(...)```
* Get the value of the future
  * ```.Get() ```
  * ```.GetOrTimeout()```
* Multiple promises
  * ```WhenAll(f1, f2, f3, ...)```
  * ```WhenAny(f1, f2, f3, ...)```
* Pipe
  * ```.Pipe(futureWithDone, futureWithFail)```
* Cancel the future
  * ```.EnableCanceller()```
  * ```.RequestCancel()```
* Function value wrappers
  * ```Start(func() []interface{})```
  * ```StartCanCancel(func(canceller Canceller) []interface{})```
* Immediate wrappers
  * ```Wrap(interface{})```
* Chain API
  * ```Start(taskDone).Done(done1).Fail(fail1).Always(alwaysForDone1).Pipe(f1, f2).Done(done2)```

	
## Quick start

### Promise and Future 

```go
import "github.com/fanliao/go-promise"
import "net/http"

p := promise.NewPromise()
p.Done(func(v ...interface{}) {
   ...
}).Always(func(v ...interface{}) {
   ...
}).Fail(func(v ...interface{}) {
   ...
})

go func(){
	url := "http://example.com/"
	
	resp, err := http.Get(url)
	defer resp.Body.Close()
	if err != nil {
		// handle error
		...
		p.Reject(url, err)
	}
	p.Resolve(url, resp.Body)
}()
r, typ := p.Get()
```

If you want to provide a read-only view, you can get a future variable:

```go
p.Future //cannot Resolve, Reject and EnableCanceller for a future
```

Can use Start function to submit a future task, it will return a future variable, so cannot Resolve or Reject the future outside of Start function:

```go
import "github.com/fanliao/go-promise"
import "net/http"

task := func()(r []interface{}){
	url := "http://example.com/"
	
	resp, err := http.Get(url)
	defer resp.Body.Close()
	if err != nil {
		// handle error
		...
		return url, err, false
	}
	return url, resp.Body, true
}

f := Start(task).Done(func(v ...interface{}) {
   ...
}).Always(func(v ...interface{}) {
   ...
}).Fail(func(v ...interface{}) {
   ...
})
r, typ := f.Get()
```

### Get the result of future

```go
f := promise.Start(func() []interface{} {
	time.Sleep(500 * time.Millisecond)
	return []interface{}{1, "ok", true}  
})
v, typ := f.Get()  //return []interface{}{1, "ok"}, RESULT_SUCCESS

f := promise.Start(func() []interface{} {
	time.Sleep(500 * time.Millisecond)
	return []interface{}{1, "fail", false}  
})
v, typ := f.Get()  //return []interface{}{1, "fail"},  RESULT_FAILURE
```

Can wait until the future task to complete, then return its result

```go
f := promise.Start(func() []interface{} {
	time.Sleep(500 * time.Millisecond)
	return []interface{}{1, "ok", true}  
})
v, typ, ok := f.GetOrTimeout(100)  //return nil, 0, false
```

### Waits for multiple futures

Creates a future that will complete when all of the supplied future have completed.
```go
task1 := func() (r []interface{}) {
	time.Sleep(100 * time.Millisecond)
	r = []interface{}{10, "ok", true}
	return
}
task2 := func() (r []interface{}) {
	time.Sleep(200 * time.Millisecond)
	r = []interface{}{20, "ok2", true}
}
f := WhenAll(Start(task1), Start(task2))
r, ok := f.Get()
```

If any future is failure, the future returnd by WhenAll will be failure
```go
task1 := func() (r []interface{}) {
	time.Sleep(100 * time.Millisecond)
	r = []interface{}{10, "ok", true}
	return
}
task2 := func() (r []interface{}) {
	time.Sleep(200 * time.Millisecond)
	r = []interface{}{20, "fail2", false}
}
f := WhenAll(Start(task1), Start(task2))
r, ok := f.Get()
```

Creates a future that will complete when any of the supplied tasks have completed.
```go
task1 := func() (r []interface{}) {
	time.Sleep(100 * time.Millisecond)
	r = []interface{}{10, "ok", true}
	return
}
task2 := func() (r []interface{}) {
	time.Sleep(200 * time.Millisecond)
	r = []interface{}{20, "fail2", false}
}
f := WhenAny(Start(task1), Start(task2))
r, ok := f.Get()
```

### Promise pipelining

```go
task1 := func() (r []interface{}) {
	time.Sleep(100 * time.Millisecond)
	r = []interface{}{10, "ok", true}
	return
}
task2 := func(v ...interface{}) *Future {
	return Start(func() []interface{} {
		time.Sleep(100 * time.Millisecond)
		return []interface{}{v[0].(int) * 2, v[1].(string) + "2", true}
	})
}
f := Start(task1).Pipe(task2)
r, ok := f.Get()
```

### Cancel the future

If need cancel a future, need send a canceller object to 
```go
import "github.com/fanliao/go-promise"
import "net/http"

p := promise.NewPromise().EnableCanceller()

go func(canceller Canceller){
	for i < 50 {
		if canceller.IsCancellationRequested() {
			p.Cancel(0)
			return 0
		}
		time.Sleep(100 * time.Millisecond)
	}
	p.Resolve(1)
}(p.Canceller())
f.RequestCancel()

r, typ := p.Get()
```

Or can use StartCanCancel() to submit a future task which can be cancelled
```go
task := func(canceller Canceller) []interface{} {
	for i < 50 {
		if canceller.IsCancellationRequested() {
			canceller.SetCancelled()
			return 0
		}
		time.Sleep(100 * time.Millisecond)
	}
	return 1
}
f := StartCanCancel(task1)
time.Sleep(200 * time.Millisecond)
f.RequestCancel()

r, ok := f.Get()
```

When call WhenAny() function, if a future is completed correctly, then will try to check if other future is enable cancel. If yes, will request cancelling the future


## Document



## License

