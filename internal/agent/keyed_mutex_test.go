package agent

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// P1-7: same key serializes; different keys run freely.
func TestKeyedMutexSerializesSameKey(t *testing.T) {
	km := newKeyedMutex()

	var active, maxActive int
	var mu sync.Mutex

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			unlock := km.Lock("app-1")
			defer unlock()

			mu.Lock()
			active++
			if active > maxActive {
				maxActive = active
			}
			mu.Unlock()

			time.Sleep(5 * time.Millisecond)

			mu.Lock()
			active--
			mu.Unlock()
		}()
	}
	wg.Wait()

	if maxActive != 1 {
		t.Fatalf("expected max 1 concurrent holder for same key, got %d", maxActive)
	}
}

func TestKeyedMutexDifferentKeysRunConcurrently(t *testing.T) {
	km := newKeyedMutex()

	var active, maxActive int
	var mu sync.Mutex

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			unlock := km.Lock(fmt.Sprintf("app-%d", i))
			defer unlock()

			mu.Lock()
			active++
			if active > maxActive {
				maxActive = active
			}
			mu.Unlock()

			time.Sleep(5 * time.Millisecond)

			mu.Lock()
			active--
			mu.Unlock()
		}(i)
	}
	wg.Wait()

	if maxActive < 2 {
		t.Fatalf("expected concurrent execution across different keys, max was %d", maxActive)
	}
}
