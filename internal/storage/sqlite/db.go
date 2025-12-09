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
	dsn := fmt.Sprintf("file:%s?_busy_timeout=5000&_journal_mode=WAL&_foreign_keys=on&_synchronous=NORMAL", dbPath)

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

	return db, nil
}
