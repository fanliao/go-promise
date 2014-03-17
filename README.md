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
  * ```.Cancel()```
* Function wrappers
  * ```Wrap(interface{})```
* Immediate value wrappers
  * ```Start(func() []interface{})```
  * ```StartCanCancel(func(canceller Canceller) []interface{})```
* Chain API
  * ```Start(taskDone).Done(done1).Fail(fail1).Always(alwaysForDone1).Pipe(f1, f2).Done(done2)```

	
## Quick start

### Promise and Future 

```go
import "github.com/fanliao/go-promise"

p := promise.NewPromise()
p.Done(func(v ...interface{}) {
   ...
}).Always(func(v ...interface{}) {
   ...
}).Fail(func(v ...interface{}) {
   ...
})
```

With the promise variable, you can then trigger actions:

```go
p.Resolve("ok", 1);
p.Reject("fail", 2);
```

If you want to provide a read-only view, you can get a future variable:

```go
p.Future //cannot Resolve, Reject and EnableCanceller for a future
```

You use Start() to submit a function:

```go
f := promise.Start(func() []interface{} {
	time.Sleep(500 * time.Millisecond)
	//the last return value is true means the promise is resolved with 1, ok
	return []interface{}{1, "ok", true}  
}).Done(func(v ...interface{}) {
   //v is slice []interface{}{1, "ok"}
   ...
}).Always(func(v ...interface{}) {
   ...
}).Fail(func(v ...interface{}) {
   ...
})
```

### Get the value of future

```go
f := promise.Start(func() []interface{} {
	time.Sleep(500 * time.Millisecond)
	//the last return value is true means the promise is resolved with 1, ok
	return []interface{}{1, "ok", true}  
}).Done(func(v ...interface{}) {
   //v is slice []interface{}{1, "ok"}
   ...
}).Always(func(v ...interface{}) {
   ...
}).Fail(func(v ...interface{}) {
   ...
})
```


## Document



## License

