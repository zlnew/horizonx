package main

import (
	"database/sql"
	"flag"
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

func main() {
	// Flags
	envFile := flag.String("env-file", "", "path to .env file to load (optional)")
	cmd := flag.String("op", "", "operation: up, down, version, force")
	steps := flag.Int("steps", 0, "number of steps for up/down (0 = all); version number for force")
	dsn := flag.String("dsn", "", "database url (postgres://user:pass@host:port/db); overrides DATABASE_URL env")
	flag.Parse()

	// Load env file if specified, otherwise try default .env (ignore error)
	if *envFile != "" {
		if err := godotenv.Load(*envFile); err != nil {
			log.Fatalf("could not load env file %q: %v", *envFile, err)
		}
	} else {
		_ = godotenv.Load()
	}

	// Resolve DSN: flag > env
	if *dsn == "" {
		*dsn = os.Getenv("DATABASE_URL")
	}

	// Validate required args
	if *cmd == "" {
		fmt.Fprintf(os.Stderr, "Error: -op is required\n\n")
		printUsage()
		os.Exit(1)
	}
	if *dsn == "" {
		fmt.Fprintf(os.Stderr, "Error: -dsn is required (or set DATABASE_URL)\n\n")
		printUsage()
		os.Exit(1)
	}
	if *cmd == "force" && *steps == 0 {
		fmt.Fprintf(os.Stderr, "Error: -steps is required for force operation\n\n")
		printUsage()
		os.Exit(1)
	}

	// Connect
	db, err := sql.Open("pgx", *dsn)
	if err != nil {
		log.Fatalf("could not open db connection: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("could not reach database: %v", err)
	}

	driver, err := pgMigrate.WithInstance(db, &pgMigrate.Config{})
	if err != nil {
		log.Fatalf("could not create migrate driver: %v", err)
	}

	src, err := iofs.New(postgres.MigrationsFS, "migrations")
	if err != nil {
		log.Fatalf("could not create migration source: %v", err)
	}

	m, err := migrate.NewWithInstance("iofs", src, "postgres", driver)
	if err != nil {
		log.Fatalf("could not initialise migrate: %v", err)
	}

	// Print current version before running
	if v, dirty, verr := m.Version(); verr == nil {
		log.Printf("Current version: %d (dirty: %v)", v, dirty)
	} else if verr == migrate.ErrNilVersion {
		log.Printf("Current version: none (fresh database)")
	}

	log.Printf("Running: %s (steps: %d)", *cmd, *steps)

	switch *cmd {
	case "up":
		if *steps > 0 {
			err = m.Steps(*steps)
		} else {
			err = m.Up()
		}

	case "down":
		if *steps > 0 {
			err = m.Steps(-(*steps))
		} else {
			err = m.Down()
		}

	case "version":
		v, dirty, verr := m.Version()
		if verr != nil && verr != migrate.ErrNilVersion {
			log.Fatalf("could not get version: %v", verr)
		}
		if verr == migrate.ErrNilVersion {
			fmt.Println("Version: none (no migrations applied)")
		} else {
			fmt.Printf("Version: %d, Dirty: %v\n", v, dirty)
		}
		return

	case "force":
		err = m.Force(*steps)

	default:
		fmt.Fprintf(os.Stderr, "Error: unknown operation %q\n\n", *cmd)
		printUsage()
		os.Exit(1)
	}

	if err != nil {
		if err == migrate.ErrNoChange {
			log.Println("No changes detected, already up to date.")
		} else {
			log.Fatalf("Migration failed: %v", err)
		}
	} else {
		// Print new version after success
		if v, dirty, verr := m.Version(); verr == nil {
			log.Printf("Migration successful. Now at version: %d (dirty: %v)", v, dirty)
		} else {
			log.Println("Migration successful.")
		}
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `Usage: migrate [OPTIONS]

Options:
  -op <operation>    Operation to run: up, down, version, force (required)
  -dsn <url>         Database URL: postgres://user:pass@host:port/db
                     Overrides DATABASE_URL env var
  -env-file <path>   Path to a .env file to load (optional)
  -steps <n>         For up/down: number of steps (0 = all)
                     For force: target version number (required)

Examples:
  # Apply all pending migrations
  migrate -op=up -dsn=postgres://user:pass@localhost:5432/horizonx

  # Apply 2 migrations
  migrate -op=up -steps=2

  # Roll back last migration
  migrate -op=down -steps=1

  # Roll back all
  migrate -op=down

  # Print current version
  migrate -op=version

  # Force version (mark as applied without running)
  migrate -op=force -steps=5

  # Load DSN from a custom env file
  migrate -op=up -env-file=/var/lib/horizonx/server.env
`)
}
