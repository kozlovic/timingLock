# timingLock

You can replace the use of a `sync.RWMutex` or `sync.Mutex` with the corresponding mutex from this library to get a report of log with the stack where getting the lock took longer than a specified threshold or when a (r)lock was held longer than a specified threshold.

## Time capture

The library captures the current time at regular intervals (currently `100ms`) but can be altered when the library is loaded by setting the environment variable `TIMING_LOCK_INTERVAL` to the desired interval (e.g: `TIMING_LOCK_INTERVAL=10ms`).

The advantage is that it minimizes performance impact, however, it does not necessarily accurately capture the real time it took to get the lock or how long the lock was held.

That is fine since the use-case is really when a lock takes way too long to be acquired or is held for longer than expected. Meaning that the thresholds used for the report will be high enough that the granularity of the time capture should be enough.

## How to use

Replace the normal `sync.RWMutex` (or `sync.Mutex`) by one from this library.

```go
var l timingLock.RWMutex
```
The mutex could be used as-is, but in this case no timing is done and obviously no report will be made.

You need to initialize the lock. Note that it cannot be made in the same go routine that already locked this mutex, a dead-lock would occur.

The `Init()` method requires a name (could be empty string if you want), a logger that implements the `Warnf(format string, v ...any)` method and two `time.Duration` that specify the thresholds to detect the time it takes to acquire the lock and the time spent under the lock. Those 3 last parameters are required to be non-nil (for the logger) and positive (for the durations).

```go
l.Init("MyLock", log, time.Second, 2*time.Second)
```

## Produced logs

If it took more than 1 second to acquire the lock, you would see something similar to this:

```
"MyLock" RLock was acquired after 1.501319s
goroutine 33 [running]:
runtime/debug.Stack()
    /usr/local/go/src/runtime/debug/stack.go:24 +0x6c
github.com/kozlovic/timingLock.(*RWMutex).RLock(0xc000100480)
    /Users/ik/dev/go/src/github.com/kozlovic/timingLock/lock.go:102 +0xe0
github.com/kozlovic/timingLock.rwbar(0xc000100480, 0x1, 0x4d2)
    /Users/ik/dev/go/src/github.com/kozlovic/timingLock/lock_test.go:86 +0x70
github.com/kozlovic/timingLock.rwfoo(...)
    /Users/ik/dev/go/src/github.com/kozlovic/timingLock/lock_test.go:112
github.com/kozlovic/timingLock.TestRWMutexTimeWaitingForLock.func1.1()
    /Users/ik/dev/go/src/github.com/kozlovic/timingLock/lock_test.go:136 +0x8c
created by github.com/kozlovic/timingLock.TestRWMutexTimeWaitingForLock.func1 in goroutine 16
    /Users/ik/dev/go/src/github.com/kozlovic/timingLock/lock_test.go:134 +0x230
```

If the lock was held for more than 2 seconds, you would see something similar to this:

```
"MyLock" RLock was held for 2.499957s
goroutine 52 [running]:
runtime/debug.Stack()
    /usr/local/go/src/runtime/debug/stack.go:24 +0x6c
github.com/kozlovic/timingLock.(*RWMutex).RUnlock(0xc00008e1e0)
    /Users/ik/dev/go/src/github.com/kozlovic/timingLock/lock.go:119 +0xa8
github.com/kozlovic/timingLock.rwbat(0xc00008e1e0, 0x1)
    /Users/ik/dev/go/src/github.com/kozlovic/timingLock/lock_test.go:199 +0x64
github.com/kozlovic/timingLock.rwbaz(0xc00008e1e0, 0x1)
    /Users/ik/dev/go/src/github.com/kozlovic/timingLock/lock_test.go:191 +0xb4
github.com/kozlovic/timingLock.TestRWMutexTimeUnderLock.func1(0xc0000a8340)
    /Users/ik/dev/go/src/github.com/kozlovic/timingLock/lock_test.go:223 +0x108
testing.tRunner(0xc0000a8340, 0xc0000a2040)
    /usr/local/go/src/testing/testing.go:1689 +0x184
created by testing.(*T).Run in goroutine 51
    /usr/local/go/src/testing/testing.go:1742 +0x5e8
```
