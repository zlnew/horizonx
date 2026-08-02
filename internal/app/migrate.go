package app

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	"horizonx/internal/adapters/postgres"

	"github.com/golang-migrate/migrate/v4"
	pgMigrate "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"
)

// RunMigrate applies database migrations against the DSN resolved from the
// -dsn flag (if set), the DATABASE_URL env var, or a .env file. It powers both
// the standalone `horizonx migrate` CLI command and the production compose
// migrate service, so a control plane always boots on a migrated schema.
//
// op: up | down | version | force (steps carries the version for force).
func RunMigrate(envFile, dsn, op string, steps int) error {
	// Load env file if specified, otherwise try default .env (ignore error).
	if envFile != "" {
		if err := godotenv.Load(envFile); err != nil {
			return fmt.Errorf("could not load env file %q: %w", envFile, err)
		}
	} else {
		_ = godotenv.Load()
	}

	// Resolve DSN: flag > env.
	if dsn == "" {
		dsn = os.Getenv("DATABASE_URL")
	}

	if op == "" {
		return fmt.Errorf("-op is required")
	}
	if dsn == "" {
		return fmt.Errorf("-dsn is required (or set DATABASE_URL)")
	}
	if op == "force" && steps == 0 {
		return fmt.Errorf("-steps is required for force operation")
	}

	// Connect.
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("could not open db connection: %w", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		return fmt.Errorf("could not reach database: %w", err)
	}

	driver, err := pgMigrate.WithInstance(db, &pgMigrate.Config{})
	if err != nil {
		return fmt.Errorf("could not create migrate driver: %w", err)
	}

	src, err := iofs.New(postgres.MigrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("could not create migration source: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", src, "postgres", driver)
	if err != nil {
		return fmt.Errorf("could not initialise migrate: %w", err)
	}

	// Print current version before running.
	if v, dirty, verr := m.Version(); verr == nil {
		log.Printf("Current version: %d (dirty: %v)", v, dirty)
	} else if verr == migrate.ErrNilVersion {
		log.Printf("Current version: none (fresh database)")
	}

	log.Printf("Running: %s (steps: %d)", op, steps)

	switch op {
	case "up":
		if steps > 0 {
			err = m.Steps(steps)
		} else {
			err = m.Up()
		}

	case "down":
		if steps > 0 {
			err = m.Steps(-steps)
		} else {
			err = m.Down()
		}

	case "version":
		v, dirty, verr := m.Version()
		if verr != nil && verr != migrate.ErrNilVersion {
			return fmt.Errorf("could not get version: %w", verr)
		}
		if verr == migrate.ErrNilVersion {
			fmt.Println("Version: none (no migrations applied)")
		} else {
			fmt.Printf("Version: %d, Dirty: %v\n", v, dirty)
		}
		return nil

	case "force":
		err = m.Force(steps)

	default:
		return fmt.Errorf("unknown operation %q", op)
	}

	if err != nil {
		if err == migrate.ErrNoChange {
			log.Println("No changes detected, already up to date.")
		} else {
			return fmt.Errorf("migration failed: %w", err)
		}
	} else {
		// Print new version after success.
		if v, dirty, verr := m.Version(); verr == nil {
			log.Printf("Migration successful. Now at version: %d (dirty: %v)", v, dirty)
		} else {
			log.Println("Migration successful.")
		}
	}

	return nil
}
