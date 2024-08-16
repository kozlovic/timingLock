package timingLock

import (
	"errors"
	"os"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"
)

var (
	errAlreadyInit     = errors.New("the lock can be initialized only once")
	errInvalidInitArgs = errors.New("invalid arguments passed to Init()")
)

var curTime atomic.Int64

const defaultInterval = 100 * time.Millisecond

func init() {
	const envVarName = "TIMING_LOCK_INTERVAL"
	var interval = defaultInterval
	if v := os.Getenv(envVarName); v != "" {
		if dur, err := time.ParseDuration(v); err != nil {
			panic(err)
		} else {
			interval = dur
		}
	}
	// Store the current time now, and then every "interval".
	curTime.Store(time.Now().UnixNano())
	go func() {
		tm := time.NewTicker(interval)
		for range tm.C {
			curTime.Store(time.Now().UnixNano())
		}
	}()
}

type Logger interface {
	Warnf(format string, v ...any)
}

type Locker interface {
	sync.Locker
	Init(name string, log Logger, twfl, tul time.Duration)
}

type RWMutex struct {
	// This will be check'ed by the APIs to know if we should trace.
	init atomic.Bool
	// Used to know when to start/end timing how long we are under a read lock.
	readers atomic.Int32
	rstart  atomic.Int64
	// Real RWMutex to be used internally.
	mu sync.RWMutex
	// Start when the Lock() is acquired.
	wstart int64
	// Logger where warnings are issued to.
	log Logger
	// This is the name that will be printed as a header of the warning
	name string
	// These are the threshold to report a warning.
	// Time waiting for the lock
	twfl int64
	// Time under the lock
	tul int64
}

// =================================================
// == 					RWMutex					  ==
// =================================================

func (l *RWMutex) Init(name string, log Logger, twfl, tul time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.log != nil {
		panic(errAlreadyInit)
	}
	if log == nil || twfl <= 0 || tul <= 0 {
		panic(errInvalidInitArgs)
	}
	// Since we can initialize only once, these values will be accessed
	// without lock in the mutex APIs.
	l.name, l.log, l.twfl, l.tul = name, log, int64(twfl), int64(tul)
	l.init.Store(true)
}

func (l *RWMutex) RLock() {
	var start int64
	trace := l.init.Load()
	if trace {
		start = curTime.Load()
	}
	l.mu.RLock()
	if trace {
		if n := l.readers.Add(1); n == 1 {
			l.rstart.Store(curTime.Load())
		}
		if dur := curTime.Load() - start; dur >= l.twfl {
			stack := debug.Stack()
			l.log.Warnf("%q RLock was acquired after %v\n%s", l.name, time.Duration(dur), stack)
		}
	}
}

func (l *RWMutex) RUnlock() {
	if l.init.Load() {
		// Capture the "start" time before decrementing readers count, because
		// if it were to go to 0 and another go routine calls RLock(), it would
		// reset l.rstart to the current time.
		// So although possibly the time under the RLock() should have been
		// extended by the new go routine, we will check/report the duration
		// just before that event.
		start := l.rstart.Load()
		if n := l.readers.Add(-1); n == 0 {
			if dur := curTime.Load() - start; dur >= l.tul {
				stack := debug.Stack()
				l.log.Warnf("%q RLock was held for %v\n%s", l.name, time.Duration(dur), stack)
			}
		}
	}
	l.mu.RUnlock()
}

func (l *RWMutex) TryRLock() bool {
	var start int64
	trace := l.init.Load()
	if trace {
		start = curTime.Load()
	}
	if l.mu.TryRLock() {
		if trace {
			if n := l.readers.Add(1); n == 1 {
				l.rstart.Store(curTime.Load())
			}
			if dur := curTime.Load() - start; dur >= l.twfl {
				stack := debug.Stack()
				l.log.Warnf("%q TryRLock was acquired after %v\n%s", l.name, time.Duration(dur), stack)
			}
		}
		return true
	}
	return false
}

func (l *RWMutex) Lock() {
	var start int64
	trace := l.init.Load()
	if trace {
		start = curTime.Load()
	}
	l.mu.Lock()
	if trace {
		now := curTime.Load()
		l.wstart = now
		if dur := now - start; dur >= l.twfl {
			stack := debug.Stack()
			l.log.Warnf("%q Lock was acquired after %v\n%s", l.name, time.Duration(dur), stack)
		}
	}
}

func (l *RWMutex) Unlock() {
	if l.init.Load() {
		if dur := curTime.Load() - l.wstart; dur >= l.tul {
			stack := debug.Stack()
			l.log.Warnf("%q Lock was held for %v\n%s", l.name, time.Duration(dur), stack)
		}
	}
	l.mu.Unlock()
}

func (l *RWMutex) TryLock() bool {
	var start int64
	trace := l.init.Load()
	if trace {
		start = curTime.Load()
	}
	if l.mu.TryLock() {
		if trace {
			now := curTime.Load()
			l.wstart = now
			if dur := now - start; dur >= l.twfl {
				stack := debug.Stack()
				l.log.Warnf("%q Lock was acquired after %v\n%s", l.name, time.Duration(dur), stack)
			}
		}
		return true
	}
	return false
}

// =================================================
// == 					Mutex					  ==
// =================================================

type Mutex struct {
	// This will be check'ed by the APIs to know if we should trace.
	init atomic.Bool
	// Real Mutex to be used internally.
	mu sync.Mutex
	// Start when the Lock() is acquired.
	wstart int64
	// Logger where warnings are issued to.
	log Logger
	// This is the name that will be printed as a header of the warning
	name string
	// These are the threshold to report a warning.
	// Time waiting for the lock
	twfl int64
	// Time under the lock
	tul int64
}

func (l *Mutex) Init(name string, log Logger, twfl, tul time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.log != nil {
		panic(errAlreadyInit)
	}
	if log == nil || twfl <= 0 || tul <= 0 {
		panic(errInvalidInitArgs)
	}
	// Since we can initialize only once, these values will be accessed
	// without lock in the mutex APIs.
	l.name, l.log, l.twfl, l.tul = name, log, int64(twfl), int64(tul)
	l.init.Store(true)
}

func (l *Mutex) Lock() {
	var start int64
	trace := l.init.Load()
	if trace {
		start = curTime.Load()
	}
	l.mu.Lock()
	if trace {
		now := curTime.Load()
		l.wstart = now
		if dur := now - start; dur >= l.twfl {
			stack := debug.Stack()
			l.log.Warnf("%q Lock was acquired after %v\n%s", l.name, time.Duration(dur), stack)
		}
	}
}

func (l *Mutex) Unlock() {
	if l.init.Load() {
		if dur := curTime.Load() - l.wstart; dur >= l.tul {
			stack := debug.Stack()
			l.log.Warnf("%q Lock was held for %v\n%s", l.name, time.Duration(dur), stack)
		}
	}
	l.mu.Unlock()
}

func (l *Mutex) TryLock() bool {
	var start int64
	trace := l.init.Load()
	if trace {
		start = curTime.Load()
	}
	if l.mu.TryLock() {
		if trace {
			now := curTime.Load()
			l.wstart = now
			if dur := now - start; dur >= l.twfl {
				stack := debug.Stack()
				l.log.Warnf("%q Lock was acquired after %v\n%s", l.name, time.Duration(dur), stack)
			}
		}
		return true
	}
	return false
}
