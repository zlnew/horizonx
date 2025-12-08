// Package sqlite
package sqlite

import (
	"database/sql"
	"fmt"
	"time"

	"horizonx-server/internal/logger"

	_ "github.com/mattn/go-sqlite3"
)

func NewSqliteDB(dbPath string, log logger.Logger) (*sql.DB, error) {
	dsn := fmt.Sprintf("file:%s?_busy_timeout=5000&_journal_mode=WAL&_foreign_keys=on", dbPath)

	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("database not responding: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(5 * time.Minute)

	log.Info("sqlite connection established successfully")

	if err := runMigration(db); err != nil {
		return nil, err
	}

	return db, nil
}

func runMigration(db *sql.DB) error {
	query := `
	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		email TEXT NOT NULL UNIQUE,
		password TEXT NOT NULL
	);
	`
	_, err := db.Exec(query)
	if err != nil {
		return fmt.Errorf("failed to migrate users table: %w", err)
	}
	return nil
}
