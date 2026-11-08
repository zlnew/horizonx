// Package postgres
package postgres

import (
	"context"
	"fmt"
	"hash/fnv"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	// advisoryLockKeyPrefix seeds the FNV-64a hash so worker lock keys live in
	// their own keyspace, well away from any other advisory-lock user.
	advisoryLockKeyPrefix = "horizonx:worker:"

	// advisoryLockAcquireTimeout bounds the pool acquisition and the
	// try-lock statement so a saturated pool cannot block a worker forever.
	advisoryLockAcquireTimeout = 5 * time.Second

	// advisoryLockReleaseTimeout bounds the unlock statement. The release
	// closure deliberately runs on a context detached from the caller's, so
	// a cancelled parent context (e.g. shutdown) cannot skip the unlock.
	advisoryLockReleaseTimeout = 5 * time.Second
)

// AdvisoryLocker coordinates worker runs across server replicas using
// Postgres session-level advisory locks. All replicas sharing one database
// derive the same key for a given worker name, so only one replica holds the
// lock for that worker at a time.
type AdvisoryLocker struct {
	pool *pgxpool.Pool
}

// NewAdvisoryLocker returns a locker backed by the given pool. The pool must
// stay open for the lifetime of any lock handed out by the locker.
func NewAdvisoryLocker(pool *pgxpool.Pool) *AdvisoryLocker {
	return &AdvisoryLocker{pool: pool}
}

// advisoryLockKey derives a stable int64 lock key from a worker name.
//
// FNV-64a over a fixed prefix means every replica maps the same worker name to
// the same key. A hash collision between two different worker names would only
// cause one of those workers to skip a run unnecessarily; it cannot cause two
// different workers to share a lock, because the collision makes the second
// worker see the key as already held rather than aliasing the first.
func advisoryLockKey(name string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(advisoryLockKeyPrefix + name))
	return int64(h.Sum64())
}

// TryLock attempts to acquire the cluster-wide advisory lock for the named
// worker. On success it returns a non-nil release function that must be called
// when the run finishes. When another replica holds the lock it returns
// (nil, false, nil). A non-nil error means the lock state is unknown and the
// caller must not run the worker (fail closed).
func (l *AdvisoryLocker) TryLock(ctx context.Context, name string) (release func(), acquired bool, err error) {
	acquireCtx, cancel := context.WithTimeout(ctx, advisoryLockAcquireTimeout)
	defer cancel()

	// Hold a dedicated connection for the whole run: session-level locks are
	// tied to the session and must not be returned to the pool mid-run.
	conn, err := l.pool.Acquire(acquireCtx)
	if err != nil {
		return nil, false, fmt.Errorf("postgres: acquire advisory lock connection for %q: %w", name, err)
	}

	key := advisoryLockKey(name)

	// Session-level (not _xact_) variant: the lock must outlive the single
	// statement and stay held while the worker runs.
	var locked bool
	if err := conn.QueryRow(acquireCtx, "SELECT pg_try_advisory_lock($1)", key).Scan(&locked); err != nil {
		// The session never acquired the lock, so it is safe to return to the
		// pool untouched.
		conn.Release()
		return nil, false, fmt.Errorf("postgres: try advisory lock for %q: %w", name, err)
	}

	if !locked {
		// Another replica holds the lock. This session never acquired it, so
		// there is nothing to leak: return it immediately.
		conn.Release()
		return nil, false, nil
	}

	var once sync.Once
	release = func() {
		once.Do(func() {
			// Detach from the caller's context: during shutdown that context
			// is already cancelled, and the unlock must still run.
			unlockCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), advisoryLockReleaseTimeout)
			defer cancel()

			if _, err := conn.Exec(unlockCtx, "SELECT pg_advisory_unlock($1)", key); err != nil {
				// Postgres session-level advisory locks are re-entrant within
				// a session: a session that still holds lock K can acquire K
				// again. If the explicit unlock failed, returning this session
				// to the pool would let a later run handed the same session
				// "acquire" a lock this run still believes it holds, silently
				// breaking mutual exclusion. Destroy the session instead.
				//
				// Do NOT "simplify" this back to conn.Release().
				closeCtx, closeCancel := context.WithTimeout(context.Background(), advisoryLockReleaseTimeout)
				defer closeCancel()
				_ = conn.Hijack().Close(closeCtx)
				return
			}

			conn.Release()
		})
	}

	return release, true, nil
}
