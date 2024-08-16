package timingLock

import (
	"bytes"
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"
)

type dummyLogger struct {
	sync.RWMutex
	buf bytes.Buffer
}

func (l *dummyLogger) Warnf(format string, v ...any) {
	msg := fmt.Sprintf(format, v...)
	l.Lock()
	l.buf.WriteString(msg)
	l.Unlock()
}

func createDummyLogger() *dummyLogger {
	return &dummyLogger{}
}

func TestInit(t *testing.T) {
	log := createDummyLogger()
	for _, typeMutex := range []struct {
		name     string
		getMutex func() Locker
	}{
		{"lock", func() Locker { return &Mutex{} }},
		{"rw lock", func() Locker { return &RWMutex{} }},
	} {
		t.Run(typeMutex.name, func(t *testing.T) {
			for _, test := range []struct {
				tname string
				log   Logger
				twfl  time.Duration
				tul   time.Duration
			}{
				{"no logger", nil, time.Second, time.Second},
				{"no twfl", log, 0, time.Second},
				{"no tul", log, time.Second, 0},
			} {
				t.Run(test.tname, func(t *testing.T) {
					l := typeMutex.getMutex()
					defer func() {
						r := recover()
						err, ok := r.(error)
						if !ok {
							t.Fatalf("Expected and error, got %T (%v)", r, r)
						}
						if err != errInvalidInitArgs {
							t.Fatalf("Expected error %q, got %v", errInvalidInitArgs, r)
						}
					}()
					l.Init("test", test.log, test.twfl, test.tul)
				})
			}
			t.Run("mutex can init only once", func(t *testing.T) {
				l := typeMutex.getMutex()
				l.Init("test", log, time.Second, 2*time.Second)
				defer func() {
					r := recover()
					err, ok := r.(error)
					if !ok {
						t.Fatalf("Expected and error, got %T (%v)", r, r)
					}
					if err != errAlreadyInit {
						t.Fatalf("Expected error %q, got %v", errAlreadyInit, r)
					}
				}()
				l.Init("try to init again", log, 2*time.Second, time.Second)
			})
		})
	}

}

func rwbar(l *RWMutex, m, v int) int {
	switch m {
	case 1:
		l.RLock()
	case 2:
		if !l.TryRLock() {
			return 0
		}
	case 3:
		l.Lock()
	case 4:
		if !l.TryLock() {
			return 0
		}
	default:
		panic(fmt.Errorf("Unexpected mode: %v", m))
	}
	v *= 2
	switch m {
	case 1, 2:
		l.RUnlock()
	case 3, 4:
		l.Unlock()
	default:
		panic(fmt.Errorf("Unexpected mode: %v", m))
	}
	return v
}
func rwfoo(l *RWMutex, m, v int) int {
	return rwbar(l, m, v)
}

func TestRWMutexTimeWaitingForLock(t *testing.T) {
	for _, test := range []struct {
		name string
		mode int
	}{
		{"rlock", 1},
		{"tryrlock", 2},
		{"lock", 3},
		{"trylock", 4},
	} {
		t.Run(test.name, func(t *testing.T) {
			var l RWMutex
			var wg sync.WaitGroup

			log := createDummyLogger()
			l.Init("test", log, 250*time.Millisecond, time.Hour)

			wg.Add(1)
			l.Lock()
			go func() {
				defer wg.Done()
				rwfoo(&l, test.mode, 1234)
			}()
			time.Sleep(500 * time.Millisecond)
			l.Unlock()
			wg.Wait()

			var lt string
			var bufEmpty bool
			switch test.mode {
			case 1:
				lt = "RLock"
			case 2:
				lt = "TryRLock"
				bufEmpty = true
			case 3:
				lt = "Lock"
			case 4:
				lt = "TryLock"
				bufEmpty = true
			}

			buf := log.buf.Bytes()
			if bufEmpty {
				if len(buf) != 0 {
					t.Fatalf("Expected no warning, got:\n%s", buf)
				}
			} else {
				if !bytes.Contains(buf, []byte(fmt.Sprintf("%q %s was acquired after", "test", lt))) {
					t.Fatalf("No indication that %s took too long:\n%s", lt, buf)
				}
				if !bytes.Contains(buf, []byte("rwbar")) || !bytes.Contains(buf, []byte("rwfoo")) {
					t.Fatalf("No sign of the stack trace")
				}
			}
		})
	}
}

func rwbaz(l *RWMutex, m int) bool {
	switch m {
	case 1:
		l.RLock()
	case 2:
		if !l.TryRLock() {
			return false
		}
	case 3:
		l.Lock()
	case 4:
		if !l.TryLock() {
			return false
		}
	default:
		panic(fmt.Errorf("Unexpected mode: %v", m))
	}
	rwbat(l, m)
	return true
}

func rwbat(l *RWMutex, m int) {
	time.Sleep(500 * time.Millisecond)
	switch m {
	case 1, 2:
		l.RUnlock()
	case 3, 4:
		l.Unlock()
	default:
		panic(fmt.Errorf("Unexpected mode: %v", m))
	}
}

func TestRWMutexTimeUnderLock(t *testing.T) {
	for _, test := range []struct {
		name string
		mode int
	}{
		{"rlock", 1},
		{"tryrlock", 2},
		{"lock", 3},
		{"trylock", 4},
	} {
		t.Run(test.name, func(t *testing.T) {
			var l RWMutex

			log := createDummyLogger()
			l.Init("test", log, time.Hour, 250*time.Millisecond)

			if !rwbaz(&l, test.mode) {
				t.Fatal("Unexpected result from locking")
			}

			var lt string
			switch test.mode {
			case 1, 2:
				lt = "RLock"
			case 3, 4:
				lt = "Lock"
			}

			buf := log.buf.Bytes()
			if !bytes.Contains(buf, []byte(fmt.Sprintf("%q %s was held for", "test", lt))) {
				t.Fatalf("No indication that %s was held for too long:\n%s", lt, buf)
			}
			if !bytes.Contains(buf, []byte("rwbaz")) || !bytes.Contains(buf, []byte("rwbat")) {
				t.Fatalf("No sign of the stack trace")
			}
		})
	}
}

func TestRWMutexNotInitializedOkToUse(t *testing.T) {
	var (
		l RWMutex
		v int
		w int
	)
	l.Lock()
	v = 1
	time.Sleep(2 * defaultInterval)
	l.Unlock()
	if v != 1 {
		t.Fatalf("Invalid v value=%v", v)
	}
	l.RLock()
	w = v
	time.Sleep(2 * defaultInterval)
	l.RUnlock()
	if w != 1 {
		t.Fatalf("Invalid w value=%v", w)
	}
	if !l.TryLock() {
		t.Fatal("Should not have failed the TryLock")
	}
	v = 2
	time.Sleep(2 * defaultInterval)
	l.Unlock()
	if v != 2 {
		t.Fatalf("Invalid v value=%v", v)
	}
	if !l.TryRLock() {
		t.Fatal("Should not have failed the TryRLock")
	}
	w = v
	time.Sleep(2 * defaultInterval)
	l.RUnlock()
	if w != 2 {
		t.Fatalf("Invalid w value=%v", w)
	}
}

func TestRWMutexConcurrentCalls(t *testing.T) {
	log := createDummyLogger()
	wg := sync.WaitGroup{}
	total := 300
	wg.Add(total)
	l := &RWMutex{}
	l.Init("test", log, 150*time.Millisecond, 250*time.Millisecond)
	for i := 0; i < total; i++ {
		go func() {
			defer wg.Done()
		ITER_LOOP:
			for i := 0; i < 10000; i++ {
				mode := rand.Intn(4)
				switch mode {
				case 0:
					l.RLock()
				case 1:
					if !l.TryRLock() {
						continue ITER_LOOP
					}
				case 2:
					l.Lock()
				case 3:
					if !l.TryLock() {
						continue ITER_LOOP
					}
				}
				time.Sleep(time.Duration(rand.Int63n(300)))
				switch mode {
				case 0, 1:
					l.RUnlock()
				default:
					l.Unlock()
				}
			}
		}()
	}
	wg.Wait()
}

func bar(l *Mutex, m, v int) int {
	switch m {
	case 1:
		l.Lock()
	case 2:
		if !l.TryLock() {
			return 0
		}
	default:
		panic(fmt.Errorf("Unexpected mode: %v", m))
	}
	v *= 2
	l.Unlock()
	return v
}
func foo(l *Mutex, m, v int) int {
	return bar(l, m, v)
}

func TestMutexTimeWaitingForLock(t *testing.T) {
	for _, test := range []struct {
		name string
		mode int
	}{
		{"lock", 1},
		{"trylock", 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			var l Mutex
			var wg sync.WaitGroup

			log := createDummyLogger()
			l.Init("test", log, 250*time.Millisecond, time.Hour)

			wg.Add(1)
			l.Lock()
			go func() {
				defer wg.Done()
				foo(&l, test.mode, 1234)
			}()
			time.Sleep(500 * time.Millisecond)
			l.Unlock()
			wg.Wait()

			buf := log.buf.Bytes()
			if test.mode == 2 {
				if len(buf) != 0 {
					t.Fatalf("Expected no warning, got:\n%s", buf)
				}
			} else {
				if !bytes.Contains(buf, []byte(fmt.Sprintf("%q Lock was acquired after", "test"))) {
					t.Fatalf("No indication that Lock took too long:\n%s", buf)
				}
				if !bytes.Contains(buf, []byte("bar")) || !bytes.Contains(buf, []byte("foo")) {
					t.Fatalf("No sign of the stack trace")
				}
			}
		})
	}
}

func baz(l *Mutex, m int) bool {
	switch m {
	case 1:
		l.Lock()
	case 2:
		if !l.TryLock() {
			return false
		}
	default:
		panic(fmt.Errorf("Unexpected mode: %v", m))
	}
	bat(l, m)
	return true
}

func bat(l *Mutex, m int) {
	time.Sleep(500 * time.Millisecond)
	switch m {
	case 1, 2:
		l.Unlock()
	default:
		panic(fmt.Errorf("Unexpected mode: %v", m))
	}
}

func TestMutexTimeUnderLock(t *testing.T) {
	for _, test := range []struct {
		name string
		mode int
	}{
		{"lock", 1},
		{"trylock", 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			var l Mutex

			log := createDummyLogger()
			l.Init("test", log, time.Hour, 250*time.Millisecond)

			if !baz(&l, test.mode) {
				t.Fatal("Unexpected result from locking")
			}

			buf := log.buf.Bytes()
			if !bytes.Contains(buf, []byte(fmt.Sprintf("%q Lock was held for", "test"))) {
				t.Fatalf("No indication that Lock was held for too long:\n%s", buf)
			}
			if !bytes.Contains(buf, []byte("baz")) || !bytes.Contains(buf, []byte("bat")) {
				t.Fatalf("No sign of the stack trace")
			}
		})
	}
}

func TestMutexNotInitializedOkToUse(t *testing.T) {
	var (
		l Mutex
		v int
	)
	l.Lock()
	v = 1
	time.Sleep(2 * defaultInterval)
	l.Unlock()
	if v != 1 {
		t.Fatalf("Invalid v value=%v", v)
	}
	if !l.TryLock() {
		t.Fatal("Should not have failed the TryLock")
	}
	v = 2
	time.Sleep(2 * defaultInterval)
	l.Unlock()
	if v != 2 {
		t.Fatalf("Invalid v value=%v", v)
	}
}

func TestMutexConcurrentCalls(t *testing.T) {
	log := createDummyLogger()
	wg := sync.WaitGroup{}
	total := 300
	wg.Add(total)
	l := &Mutex{}
	l.Init("test", log, 150*time.Millisecond, 250*time.Millisecond)
	for i := 0; i < total; i++ {
		go func() {
			defer wg.Done()
			for i := 0; i < 10000; i++ {
				if rand.Intn(2) == 0 {
					l.Lock()
				} else if !l.TryLock() {
					continue
				}
				time.Sleep(time.Duration(rand.Int63n(300)))
				l.Unlock()
			}
		}()
	}
	wg.Wait()
}
