package promise

import (
	"fmt"
	"sync"
	"sync/atomic"
)

// Once is an object that will perform exactly one action.
// It is copied from go source code and add IsDone() method
type once struct {
	m    sync.Mutex
	done uint32
}

// Do calls the function f if and only if Do is being called for the
// first time for this instance of Once.  In other words, given
// 	var once Once
// if once.Do(f) is called multiple times, only the first call will invoke f,
// even if f has a different value in each invocation.  A new instance of
// Once is required for each function to execute.
//
// Do is intended for initialization that must be run exactly once.  Since f
// is niladic, it may be necessary to use a function literal to capture the
// arguments to a function to be invoked by Do:
// 	config.once.Do(func() { config.init(filename) })
//
// Because no call to Do returns until the one call to f returns, if f causes
// Do to be called, it will deadlock.
//
func (o *once) Do(f func()) {
	if atomic.LoadUint32(&o.done) == 1 {
		return
	}
	// Slow-path.
	o.m.Lock()
	defer o.m.Unlock()
	if atomic.LoadUint32(&o.done) == 0 {
		f()
		atomic.StoreUint32(&o.done, 1)
		fmt.Println("set done = 1")
	}
}

func (o *once) Done() {
	if atomic.LoadUint32(&o.done) == 1 {
		return
	}
	// Slow-path.
	o.m.Lock()
	defer o.m.Unlock()
	if atomic.LoadUint32(&o.done) == 0 {
		atomic.StoreUint32(&o.done, 1)
	}
}

func (o *once) IsDone() bool {
	if atomic.LoadUint32(&o.done) == 1 {
		return true
	}
	// Slow-path.
	o.m.Lock()
	defer o.m.Unlock()
	d := atomic.LoadUint32(&o.done)
	fmt.Println("d =", d)
	return d != 0
}
