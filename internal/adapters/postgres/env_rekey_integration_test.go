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
	"horizonx/internal/security"

	"github.com/golang-migrate/migrate/v4"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestEnvVarLazyReencrypt spins up a throwaway postgres:16-alpine container
// (never personal-postgres), migrates up, inserts an env var encrypted under
// a legacy key, and asserts that reading via ListEnvVars succeeds and lazy-rewrites
// the row encrypted under the primary key.
func TestEnvVarLazyReencrypt(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Fatalf("docker binary not found in PATH: install Docker to run the integration tests (make test-integration requires Docker)")
	}

	name := fmt.Sprintf("hx-test-env-reencrypt-%d-%d", time.Now().UnixNano(), rand.Intn(100000))
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

	// Seed server and application
	var serverID uuid.UUID
	err = pool.QueryRow(ctx,
		`INSERT INTO servers (name, ip_address, api_token)
		 VALUES ('test-server', '10.0.0.1', 'dummy-token')
		 RETURNING id`).Scan(&serverID)
	if err != nil {
		t.Fatalf("could not seed server: %v", err)
	}

	var appID int64
	err = pool.QueryRow(ctx,
		`INSERT INTO applications (server_id, name, repo_name, repo_url, branch, status)
		 VALUES ($1, 'test-app', 'test-repo', 'https://github.com/test/repo', 'main', 'running')
		 RETURNING id`, serverID).Scan(&appID)
	if err != nil {
		t.Fatalf("could not seed application: %v", err)
	}

	primaryKey := security.KeyFromSecret("new-env-encryption-key")
	legacyKey := security.KeyFromSecret("old-jwt-secret")
	plaintext := "API_SECRET=shhh-legacy-secret"

	// Encrypt row with legacy key and store directly in DB
	legacyCiphertext, err := security.Encrypt(plaintext, legacyKey)
	if err != nil {
		t.Fatalf("failed to encrypt with legacy key: %v", err)
	}

	var envVarID int64
	err = pool.QueryRow(ctx,
		`INSERT INTO environment_variables (application_id, key, value, value_encrypted, is_preview, created_at, updated_at)
		 VALUES ($1, 'API_SECRET', $2, TRUE, FALSE, NOW(), NOW())
		 RETURNING id`,
		appID, legacyCiphertext,
	).Scan(&envVarID)
	if err != nil {
		t.Fatalf("could not insert legacy env var: %v", err)
	}

	// Create repository with primaryKey and legacyKey
	repo := postgres.NewApplicationRepository(pool, primaryKey, legacyKey)

	// ListEnvVars should return plaintext and trigger lazy re-encryption
	envVars, err := repo.ListEnvVars(ctx, appID)
	if err != nil {
		t.Fatalf("ListEnvVars failed: %v", err)
	}
	if len(envVars) != 1 {
		t.Fatalf("expected 1 env var, got %d", len(envVars))
	}
	if envVars[0].Value != plaintext {
		t.Fatalf("expected plaintext %q, got %q", plaintext, envVars[0].Value)
	}

	// Verify that row in DB is now encrypted with primaryKey
	var storedCiphertext string
	var storedEncrypted bool
	err = pool.QueryRow(ctx, `SELECT value, value_encrypted FROM environment_variables WHERE id = $1`, envVarID).
		Scan(&storedCiphertext, &storedEncrypted)
	if err != nil {
		t.Fatalf("failed to query stored env var: %v", err)
	}

	if !storedEncrypted {
		t.Fatalf("expected stored env var to be encrypted")
	}
	if storedCiphertext == legacyCiphertext {
		t.Fatalf("expected stored ciphertext to be rewritten, but it is still the legacy ciphertext")
	}

	// Verify primary key can decrypt it directly
	decryptedWithPrimary, err := security.Decrypt(storedCiphertext, primaryKey)
	if err != nil {
		t.Fatalf("failed to decrypt rewritten ciphertext with primary key: %v", err)
	}
	if decryptedWithPrimary != plaintext {
		t.Fatalf("decrypted with primary mismatch: got %q, want %q", decryptedWithPrimary, plaintext)
	}

	// Verify legacy key CANNOT decrypt it anymore (proves it was rewritten to primary)
	if _, err := security.Decrypt(storedCiphertext, legacyKey); err == nil {
		t.Fatalf("legacy key unexpectedly succeeded decrypting primary ciphertext")
	}

	// Second read: should read cleanly with primary key without errors
	envVars2, err := repo.ListEnvVars(ctx, appID)
	if err != nil {
		t.Fatalf("second ListEnvVars failed: %v", err)
	}
	if envVars2[0].Value != plaintext {
		t.Fatalf("second read plaintext mismatch: got %q, want %q", envVars2[0].Value, plaintext)
	}
}
