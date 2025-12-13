package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"

	_ "github.com/jackc/pgx/v5/stdlib"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	if err := godotenv.Load(); err != nil {
		fmt.Println("Warning: .env file not found")
	}

	defaultDSN := os.Getenv("DATABASE_URL")
	dsn := flag.String("dsn", defaultDSN, "database url")
	flag.Parse()

	if *dsn == "" {
		log.Fatal("DSN required via flag -dsn or DATABASE_URL env")
	}

	db, err := sql.Open("pgx", *dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatal("Cannot ping DB:", err)
	}

	seedAdmin(db)
}

func seedAdmin(db *sql.DB) {
	email := "admin@horizonx.local"
	password := "password"

	if envEmail := os.Getenv("DB_ADMIN_EMAIL"); envEmail != "" {
		email = envEmail
	}

	if envPass := os.Getenv("DB_ADMIN_PASSWORD"); envPass != "" {
		password = envPass
	}

	hashed, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)

	query := `
		INSERT INTO users (name, email, password, role_id) 
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (email) DO UPDATE SET password = excluded.password;
	`

	_, err := db.Exec(query, "Admin", email, string(hashed), 1)
	if err != nil {
		log.Fatalf("Failed to seed admin: %v", err)
	}

	fmt.Printf("✅ User Seeded!\n   User: %s\n   Pass: %s\n", email, password)
}
