//go:build integration

package postgres_test

import (
	"database/sql"
	"fmt"
	"math/rand"
	"net"
	"os/exec"
	"testing"
	"time"

	"horizonx/internal/adapters/postgres"

	"github.com/golang-migrate/migrate/v4"
	pgMigrate "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// This test NEVER touches personal-postgres (127.0.0.1:5432) or any existing
// database: it starts a throwaway postgres:16-alpine container on a random
// free host port with a unique name, and removes it via t.Cleanup.
// Requires Docker and the postgres:16-alpine image. Opt-in only:
// run with `make test-integration` (tags=integration); NOT part of make check.
func TestMigrationsUpDownUp(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Fatalf("docker binary not found in PATH: install Docker to run the integration tests (make test-integration requires Docker)")
	}

	name := fmt.Sprintf("hx-test-migrations-%d-%d", time.Now().UnixNano(), rand.Intn(100000))
	password := "hx-test-pw"
	user := "postgres"
	dbName := "hx_test"

	port, err := freePort()
	if err != nil {
		t.Fatalf("could not find a free host port: %v", err)
	}
	// Guard: the throwaway container must never sit on personal-postgres' port.
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
	assertTablesExist(t, db)

	if err := m.Down(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("migrate down failed (migrations are not reversible): %v", err)
	}
	assertTablesGone(t, db)

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("migrate up (second time) failed: %v", err)
	}
	assertTablesExist(t, db)

	v, dirty, err := m.Version()
	if err != nil {
		t.Fatalf("could not read migration version: %v", err)
	}
	if dirty {
		t.Fatalf("migration version %d is dirty", v)
	}
	if v == 0 {
		t.Fatalf("expected a nonzero migration version after up")
	}
	t.Logf("final migration version: %d", v)
}

// expectedTables are created by the up migrations (see migrations/*.up.sql).
var expectedTables = []string{
	"roles",
	"permissions",
	"role_has_permissions",
	"users",
	"servers",
	"server_metrics",
	"applications",
	"environment_variables",
	"deployments",
	"jobs",
	"logs",
	"audit_logs",
	"settings",
	"alert_rules",
	"alert_history",
}

func newMigrator(t *testing.T, db *sql.DB) *migrate.Migrate {
	t.Helper()

	driver, err := pgMigrate.WithInstance(db, &pgMigrate.Config{})
	if err != nil {
		t.Fatalf("could not create migrate driver: %v", err)
	}

	src, err := iofs.New(postgres.MigrationsFS, "migrations")
	if err != nil {
		t.Fatalf("could not create migration source: %v", err)
	}

	m, err := migrate.NewWithInstance("iofs", src, "postgres", driver)
	if err != nil {
		t.Fatalf("could not initialise migrate (same path as cmd/migrate): %v", err)
	}

	return m
}

func assertTablesExist(t *testing.T, db *sql.DB) {
	t.Helper()

	for _, table := range expectedTables {
		var exists bool
		err := db.QueryRow(
			`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema='public' AND table_name=$1)`,
			table,
		).Scan(&exists)
		if err != nil {
			t.Fatalf("could not check table %q: %v", table, err)
		}
		if !exists {
			t.Errorf("expected table %q to exist after migrate up", table)
		}
	}
}

func assertTablesGone(t *testing.T, db *sql.DB) {
	t.Helper()

	for _, table := range expectedTables {
		var exists bool
		err := db.QueryRow(
			`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema='public' AND table_name=$1)`,
			table,
		).Scan(&exists)
		if err != nil {
			t.Fatalf("could not check table %q: %v", table, err)
		}
		if exists {
			t.Errorf("expected table %q to be gone after migrate down", table)
		}
	}
}

func waitForDB(t *testing.T, dsn string) *sql.DB {
	t.Helper()

	deadline := time.Now().Add(90 * time.Second)
	var db *sql.DB
	for {
		var err error
		db, err = sql.Open("pgx", dsn)
		if err == nil {
			if pingErr := db.Ping(); pingErr == nil {
				return db
			} else {
				err = pingErr
			}
		}
		_ = db.Close()

		if time.Now().After(deadline) {
			t.Fatalf("throwaway Postgres did not become ready within 90s (image postgres:16-alpine must be present locally, e.g. `docker pull postgres:16-alpine`): last error: %v", err)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func runDocker(t *testing.T, args ...string) {
	t.Helper()

	out, err := exec.Command("docker", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("docker %v failed: %v\n%s\n(if the image is missing, run: docker pull postgres:16-alpine)", args, err, out)
	}
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}
