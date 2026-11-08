//go:build integration

package postgres_test

import (
	"context"
	"fmt"
	"math/rand"
	"os/exec"
	"testing"
	"time"

	"horizonx/internal/adapters/postgres"

	"github.com/golang-migrate/migrate/v4"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestRetentionPurge spins up a throwaway postgres:16-alpine container (never
// personal-postgres), migrates up, seeds jobs/logs rows both past and inside
// the TTL, runs the retention purge, and asserts old rows are gone while
// fresh rows survive. Reuses freePort/runDocker/waitForDB/newMigrator from
// migrations_integration_test.go.
func TestRetentionPurge(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Fatalf("docker binary not found in PATH: install Docker to run the integration tests (make test-integration requires Docker)")
	}

	name := fmt.Sprintf("hx-test-retention-%d-%d", time.Now().UnixNano(), rand.Intn(100000))
	password := "hx-test-pw"
	user := "postgres"
	dbName := "hx_test"

	port, err := freePort()
	if err != nil {
		t.Fatalf("could not find a free host port: %v", err)
	}
	if port == 5432 {
		t.Fatalf("selected port 5432 is reserved for personal-postgres; refusing to run")
	}

	runDocker(t, "run", "-d", "--rm",
		"--name", name,
		"-e", "POSTGRES_PASSWORD="+password,
		"-e", "POSTGRES_DB="+dbName,
		"-p", fmt.Sprintf("127.0.0.1:%d:5432", port),
		"postgres:16-alpine")
	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f", name).Run()
	})

	dsn := fmt.Sprintf("postgres://%s:%s@127.0.0.1:%d/%s?sslmode=disable", user, password, port, dbName)
	db := waitForDB(t, dsn)
	defer db.Close()

	m := newMigrator(t, db)
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("migrate up failed: %v", err)
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("could not create pgxpool: %v", err)
	}
	defer pool.Close()

	var serverID uuid.UUID
	err = pool.QueryRow(ctx,
		`INSERT INTO servers (name, ip_address, api_token) VALUES ('retention-test', '127.0.0.1', $1) RETURNING id`,
		"retention-token-"+uuid.NewString(),
	).Scan(&serverID)
	if err != nil {
		t.Fatalf("could not seed server: %v", err)
	}

	oldTime := time.Now().UTC().Add(-60 * 24 * time.Hour)
	freshTime := time.Now().UTC()

	seedJob := func(queuedAt time.Time) int64 {
		var id int64
		err := pool.QueryRow(ctx,
			`INSERT INTO jobs (trace_id, server_id, type, queued_at) VALUES ($1, $2, 'deploy', $3) RETURNING id`,
			uuid.New(), serverID, queuedAt,
		).Scan(&id)
		if err != nil {
			t.Fatalf("could not seed job: %v", err)
		}
		return id
	}
	oldJob1 := seedJob(oldTime)
	oldJob2 := seedJob(oldTime.Add(time.Hour))
	freshJob := seedJob(freshTime)

	seedLog := func(ts time.Time, jobID *int64) int64 {
		var id int64
		err := pool.QueryRow(ctx,
			`INSERT INTO logs (timestamp, level, source, action, trace_id, job_id, message) VALUES ($1, 'info', 'test', 'test', $2, $3, 'retention') RETURNING id`,
			ts, uuid.New(), jobID,
		).Scan(&id)
		if err != nil {
			t.Fatalf("could not seed log: %v", err)
		}
		return id
	}
	// Logs reference the fresh job so job deletion cannot cascade them away
	// (logs.job_id is ON DELETE SET NULL anyway; jobs are purged first).
	oldLog1 := seedLog(oldTime, &freshJob)
	oldLog2 := seedLog(oldTime.Add(time.Hour), &freshJob)
	freshLog := seedLog(freshTime, &freshJob)

	repo := postgres.NewRetentionRepository(pool)
	jobCutoff := time.Now().UTC().Add(-30 * 24 * time.Hour)
	logCutoff := time.Now().UTC().Add(-14 * 24 * time.Hour)

	deletedJobs, err := repo.DeleteJobsOlderThan(ctx, jobCutoff, 1000)
	if err != nil {
		t.Fatalf("DeleteJobsOlderThan failed: %v", err)
	}
	if deletedJobs != 2 {
		t.Errorf("expected 2 jobs deleted, got %d", deletedJobs)
	}

	deletedLogs, err := repo.DeleteLogsOlderThan(ctx, logCutoff, 1000)
	if err != nil {
		t.Fatalf("DeleteLogsOlderThan failed: %v", err)
	}
	if deletedLogs != 2 {
		t.Errorf("expected 2 logs deleted, got %d", deletedLogs)
	}

	assertRowGone := func(table string, id int64) {
		t.Helper()
		var exists bool
		if err := db.QueryRow(fmt.Sprintf(`SELECT EXISTS (SELECT 1 FROM %s WHERE id = $1)`, table), id).Scan(&exists); err != nil {
			t.Fatalf("could not check %s id %d: %v", table, id, err)
		}
		if exists {
			t.Errorf("expected %s id %d to be purged", table, id)
		}
	}
	assertRowSurvives := func(table string, id int64) {
		t.Helper()
		var exists bool
		if err := db.QueryRow(fmt.Sprintf(`SELECT EXISTS (SELECT 1 FROM %s WHERE id = $1)`, table), id).Scan(&exists); err != nil {
			t.Fatalf("could not check %s id %d: %v", table, id, err)
		}
		if !exists {
			t.Errorf("expected %s id %d to survive the purge", table, id)
		}
	}

	assertRowGone("jobs", oldJob1)
	assertRowGone("jobs", oldJob2)
	assertRowSurvives("jobs", freshJob)
	assertRowGone("logs", oldLog1)
	assertRowGone("logs", oldLog2)
	assertRowSurvives("logs", freshLog)

	// Purge is idempotent: a second run deletes nothing.
	again, err := repo.DeleteJobsOlderThan(ctx, jobCutoff, 1000)
	if err != nil {
		t.Fatalf("second DeleteJobsOlderThan failed: %v", err)
	}
	if again != 0 {
		t.Errorf("expected second jobs purge to delete 0, got %d", again)
	}
}
