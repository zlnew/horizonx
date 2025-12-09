package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"

	"horizonx-server/internal/storage/sqlite"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite3"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/mattn/go-sqlite3"
)

func main() {
	cmd := flag.String("op", "", "operation: up, down, version, force")
	steps := flag.Int("steps", 0, "number of steps for up/down (0 = all)")
	dbPath := flag.String("db", "horizonx.db", "path to sqlite database file")
	flag.Parse()

	if *cmd == "" {
		fmt.Println("Usage: go run cmd/sqlite-migrate/main.go -op=[up|down|version] -steps=[n] -db=[path]")
		os.Exit(1)
	}

	dsn := fmt.Sprintf("file:%s?_busy_timeout=5000&_journal_mode=WAL&_foreign_keys=on", *dbPath)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	driver, err := sqlite3.WithInstance(db, &sqlite3.Config{})
	if err != nil {
		log.Fatalf("could not create driver: %v", err)
	}

	src, err := iofs.New(sqlite.MigrationsFS, "migrations")
	if err != nil {
		log.Fatalf("could not create source driver: %v", err)
	}

	m, err := migrate.NewWithInstance("iofs", src, "sqlite3", driver)
	if err != nil {
		log.Fatalf("could not create migrate instance: %v", err)
	}

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
		v, dirty, err := m.Version()
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("Version: %d, Dirty: %v\n", v, dirty)
		return
	case "force":
		if *steps == 0 {
			log.Fatal("please specify version to force")
		}
		err = m.Force(*steps)
	default:
		log.Fatal("unknown command")
	}

	if err != nil {
		if err == migrate.ErrNoChange {
			fmt.Println("No changes detected.")
		} else {
			log.Fatalf("Migration failed: %v", err)
		}
	} else {
		fmt.Println("Migration success!")
	}
}
