package agent

import "sync"

// keyedMutex serializes work per key. Used to prevent two jobs for the same
// application (deploy, rollback, restart, health check...) from running
// concurrently on the same workdir — 10 job workers would otherwise race
// git pulls and docker compose operations in the same directory (P1-7).
type keyedMutex struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

func newKeyedMutex() *keyedMutex {
	return &keyedMutex{locks: make(map[string]*sync.Mutex)}
}

// Lock acquires (or blocks for) the mutex for key. The returned unlock func
// releases it and cleans up the map entry when no one else is waiting.
func (k *keyedMutex) Lock(key string) func() {
	k.mu.Lock()
	m, ok := k.locks[key]
	if !ok {
		m = &sync.Mutex{}
		k.locks[key] = m
	}
	k.mu.Unlock()

	m.Lock()

	return func() {
		m.Unlock()

		k.mu.Lock()
		if m.TryLock() {
			// No waiter left — safe to drop the entry.
			delete(k.locks, key)
			m.Unlock()
		}
		k.mu.Unlock()
	}
}
