package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	if err := godotenv.Load(); err != nil {
		fmt.Println("Warning: .env file not found")
	}

	dbPath := flag.String("db", "horizonx.db", "path to db")
	flag.Parse()

	dsn := fmt.Sprintf("file:%s?_busy_timeout=5000&_journal_mode=WAL&_foreign_keys=on", *dbPath)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	email := "owner@horizonx.local"
	rawPassword := "password"

	if envPass := os.Getenv("DB_OWNER_PASSWORD"); envPass != "" {
		rawPassword = envPass
	}

	hashed, _ := bcrypt.GenerateFromPassword([]byte(rawPassword), bcrypt.DefaultCost)

	query := `
		INSERT INTO users (name, email, password, role_id) 
		VALUES (?, ?, ?, ?)
		ON CONFLICT(email) DO UPDATE SET password = excluded.password;
	`
	_, err = db.Exec(query, "Super Owner", email, string(hashed), 1)
	if err != nil {
		log.Fatalf("Failed to seed owner: %v", err)
	}

	fmt.Printf("Seeding Success!\nUser: %s\nPass: %s\n", email, rawPassword)
}
