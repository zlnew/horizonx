//go:build integration

package postgres

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"os/exec"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// This test NEVER touches personal-postgres (127.0.0.1:5432) or any existing
// database: it starts a throwaway postgres:16-alpine container on a random
// free host port with a unique name, and removes it via t.Cleanup.
// Requires Docker and the postgres:16-alpine image (`docker pull postgres:16-alpine`).
// Opt-in only: run with `make test-integration` (tags=integration); NOT part of make check.
//
// It simulates N replicas with N independent *pgxpool.Pool instances against the
// same database. That is the same session-level mechanism real replicas use,
// but it is NOT two live server processes: a passing run means the advisory-lock
// primitive excludes across sessions, not that a deployed pair was observed.
func TestAdvisoryLockerAcrossReplicas(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Fatalf("docker binary not found in PATH: install Docker to run the integration tests (make test-integration requires Docker)")
	}

	name := fmt.Sprintf("hx-test-advisory-%d-%d", time.Now().UnixNano(), rand.Intn(100000))
	password := "hx-test-pw"
	user := "postgres"
	dbName := "hx_test"

	port, err := findFreePort()
	if err != nil {
		t.Fatalf("could not find a free host port: %v", err)
	}
	// Guard: the throwaway container must never sit on personal-postgres' port.
	if port == 5432 {
		t.Fatalf("selected port 5432 is reserved for personal-postgres; refusing to run")
	}

	dockerRun(t, "run", "-d", "--rm",
		"--name", name,
		"-e", "POSTGRES_PASSWORD="+password,
		"-e", "POSTGRES_DB="+dbName,
		"-p", fmt.Sprintf("127.0.0.1:%d:5432", port),
		"postgres:16-alpine")
	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f", name).Run()
	})

	dsn := fmt.Sprintf("postgres://%s:%s@127.0.0.1:%d/%s?sslmode=disable", user, password, port, dbName)

	// Two separate pools model two server replicas: each has its own sessions,
	// so advisory locks taken on one are visible to the other.
	replicaA := waitForPostgresPool(t, dsn)
	replicaB := waitForPostgresPool(t, dsn)
	t.Cleanup(func() {
		replicaA.Close()
		replicaB.Close()
	})

	lockerA := NewAdvisoryLocker(replicaA)
	lockerB := NewAdvisoryLocker(replicaB)

	ctx := context.Background()

	t.Run("mutual exclusion across two replicas", func(t *testing.T) {
		releaseA, acquired, err := lockerA.TryLock(ctx, "metrics_collect")
		if err != nil || !acquired {
			t.Fatalf("replica A TryLock: acquired=%v err=%v, want acquired=true", acquired, err)
		}

		if _, acquiredB, errB := lockerB.TryLock(ctx, "metrics_collect"); errB != nil || acquiredB {
			t.Fatalf("replica B TryLock while A holds: acquired=%v err=%v, want acquired=false", acquiredB, errB)
		}

		releaseA()

		releaseB, acquiredB, errB := lockerB.TryLock(ctx, "metrics_collect")
		if errB != nil || !acquiredB {
			t.Fatalf("replica B TryLock after A released: acquired=%v err=%v, want acquired=true", acquiredB, errB)
		}
		releaseB()
	})

	t.Run("different names do not collide", func(t *testing.T) {
		releaseMetrics, acquired, err := lockerA.TryLock(ctx, "metrics_collect")
		if err != nil || !acquired {
			t.Fatalf("replica A metrics lock: acquired=%v err=%v", acquired, err)
		}
		defer releaseMetrics()

		releaseReaper, acquired, err := lockerB.TryLock(ctx, "job_reaper")
		if err != nil || !acquired {
			t.Fatalf("replica B job_reaper lock while A holds metrics: acquired=%v err=%v, want acquired=true", acquired, err)
		}
		releaseReaper()
	})

	t.Run("release is idempotent and frees the lock", func(t *testing.T) {
		release, acquired, err := lockerA.TryLock(ctx, "application_health_check")
		if err != nil || !acquired {
			t.Fatalf("replica A lock: acquired=%v err=%v", acquired, err)
		}

		release() // first call releases
		release() // second call must be a no-op, not a panic

		releaseB, acquiredB, errB := lockerB.TryLock(ctx, "application_health_check")
		if errB != nil || !acquiredB {
			t.Fatalf("replica B TryLock after idempotent release: acquired=%v err=%v, want acquired=true", acquiredB, errB)
		}
		releaseB()
	})

	t.Run("session advisory locks are re-entrant", func(t *testing.T) {
		// This is the property that forces the destroy-on-failure path in
		// TryLock: within one session, taking the same advisory lock twice
		// succeeds the second time. A session that skimmed the unlock and was
		// then returned to the pool would silently hand out a lock it still
		// holds, breaking mutual exclusion.
		//
		// We use a raw session here, not the locker, because the locker is free
		// to take a different pooled connection per call and would not exercise
		// re-entrancy.
		conn, err := replicaA.Acquire(ctx)
		if err != nil {
			t.Fatalf("acquire raw session: %v", err)
		}
		defer conn.Release()

		key := advisoryLockKey("reentrancy_probe")

		var first bool
		if err := conn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", key).Scan(&first); err != nil {
			t.Fatalf("first try_advisory_lock: %v", err)
		}
		if !first {
			t.Fatal("first try_advisory_lock in a fresh session returned false, want true")
		}

		var second bool
		if err := conn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", key).Scan(&second); err != nil {
			t.Fatalf("second try_advisory_lock: %v", err)
		}
		if !second {
			t.Error("second try_advisory_lock on the same session returned false; re-entrancy assumption is wrong")
		}

		// A different session still cannot take it while this one holds it.
		var other bool
		if err := replicaB.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", key).Scan(&other); err != nil {
			t.Fatalf("other-session try_advisory_lock: %v", err)
		}
		if other {
			t.Error("a different session acquired the key while the first session holds it")
		}

		// Unlock twice to drain the recursion before returning the session.
		var unlocked bool
		if err := conn.QueryRow(ctx, "SELECT pg_advisory_unlock($1)", key).Scan(&unlocked); err != nil {
			t.Fatalf("first pg_advisory_unlock: %v", err)
		}
		if err := conn.QueryRow(ctx, "SELECT pg_advisory_unlock($1)", key).Scan(&unlocked); err != nil {
			t.Fatalf("second pg_advisory_unlock: %v", err)
		}
	})
}

func dockerRun(t *testing.T, args ...string) {
	t.Helper()

	out, err := exec.Command("docker", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("docker %v failed: %v\n%s\n(if the image is missing, run: docker pull postgres:16-alpine)", args, err, out)
	}
}

func findFreePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func waitForPostgresPool(t *testing.T, dsn string) *pgxpool.Pool {
	t.Helper()

	deadline := time.Now().Add(90 * time.Second)
	var lastErr error

	for {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		pool, err := pgxpool.New(ctx, dsn)
		if err == nil {
			err = pool.Ping(ctx)
		}
		cancel()

		if err == nil {
			return pool
		}
		if pool != nil {
			pool.Close()
		}
		lastErr = err

		if time.Now().After(deadline) {
			t.Fatalf("throwaway Postgres did not become ready within 90s (image postgres:16-alpine must be present locally, e.g. `docker pull postgres:16-alpine`): last error: %v", lastErr)
		}
		time.Sleep(500 * time.Millisecond)
	}
}
