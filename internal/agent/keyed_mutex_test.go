package agent

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// P1-7: two goroutines locking the same key must not run concurrently.
func TestKeyedMutexSerializesSameKey(t *testing.T) {
	km := newKeyedMutex()

	var concurrent, maxConcurrent atomic.Int64
	var wg sync.WaitGroup

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			unlock := km.Lock("app-1")
			defer unlock()

			n := concurrent.Add(1)
			// Track the high-water mark of simultaneous holders.
			for {
				old := maxConcurrent.Load()
				if n <= old || maxConcurrent.CompareAndSwap(old, n) {
					break
				}
			}
			time.Sleep(2 * time.Millisecond)
			concurrent.Add(-1)
		}()
	}

	wg.Wait()

	if max := maxConcurrent.Load(); max != 1 {
		t.Fatalf("expected max 1 concurrent holder for same key, got %d", max)
	}
}

// P1-7: different keys must NOT block each other.
func TestKeyedMutexAllowsDifferentKeys(t *testing.T) {
	km := newKeyedMutex()

	done := make(chan struct{})

	unlockA := km.Lock("app-1")
	defer unlockA()
	// A is held; B must still acquire immediately.
	unlockB := km.Lock("app-2")
	unlockB()
	close(done)
	<-done
}

// P1-7: unlock must not panic on the second call (unlock func is idempotent-safe
// via map cleanup even though the mutex itself would panic — we just ensure the
// map doesn't leak entries).
func TestKeyedMutexUnlockCleansMap(t *testing.T) {
	km := newKeyedMutex()

	unlock := km.Lock("app-1")
	unlock()

	km.mu.Lock()
	_, stillThere := km.locks["app-1"]
	km.mu.Unlock()
	if stillThere {
		t.Fatal("expected lock entry removed from map after unlock")
	}
}
