package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"horizonx/internal/adapters/postgres"
	"horizonx/internal/application/role"
	"horizonx/internal/application/user"
	"horizonx/internal/domain"

	"github.com/joho/godotenv"
)

func main() {
	// Flags
	envFile := flag.String("env-file", "", "path to .env file to load (optional)")
	dsn := flag.String("dsn", "", "database url (postgres://user:pass@host:port/db); overrides DATABASE_URL env")
	adminEmail := flag.String("admin-email", "", "admin user email; overrides DB_ADMIN_EMAIL env (default: admin@horizonx.local)")
	adminPass := flag.String("admin-password", "", "admin user password; overrides DB_ADMIN_PASSWORD env (default: password)")
	skipRoles := flag.Bool("skip-roles", false, "skip seeding roles and permissions")
	skipAdmin := flag.Bool("skip-admin", false, "skip seeding the admin user")
	timeout := flag.Duration("timeout", 30*time.Second, "context timeout for seed operations")
	flag.Parse()

	// Load env file if specified, otherwise try default .env (ignore error)
	if *envFile != "" {
		if err := godotenv.Load(*envFile); err != nil {
			log.Fatalf("could not load env file %q: %v", *envFile, err)
		}
	} else {
		if err := godotenv.Load(); err != nil {
			log.Println("No .env file found, relying on environment variables and flags")
		}
	}

	// Resolve DSN: flag > env
	if *dsn == "" {
		*dsn = os.Getenv("DATABASE_URL")
	}
	if *dsn == "" {
		fmt.Fprintf(os.Stderr, "Error: -dsn is required (or set DATABASE_URL)\n\n")
		printUsage()
		os.Exit(1)
	}

	// Resolve admin credentials: flag > env > default
	if *adminEmail == "" {
		*adminEmail = os.Getenv("DB_ADMIN_EMAIL")
	}
	if *adminEmail == "" {
		*adminEmail = "admin@horizonx.local"
	}

	if *adminPass == "" {
		*adminPass = os.Getenv("DB_ADMIN_PASSWORD")
	}
	if *adminPass == "" {
		*adminPass = "password"
	}

	// Warn about insecure defaults in a visible way
	if *adminPass == "password" {
		log.Println("WARNING: using default admin password 'password' — change this in production!")
	}

	// Connect
	dbPool, err := postgres.Init(*dsn)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer dbPool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	// Track overall result
	failed := false

	if !*skipRoles {
		roleRepo := postgres.NewRoleRepository(dbPool)
		roleSvc := role.NewService(roleRepo)
		if err := seedRolePermissions(ctx, roleSvc); err != nil {
			log.Printf("[x] roles/permissions: %v", err)
			failed = true
		}
	} else {
		log.Println("[~] Skipping roles and permissions (--skip-roles)")
	}

	if !*skipAdmin {
		userRepo := postgres.NewUserRepository(dbPool)
		userSvc := user.NewService(userRepo)
		if err := seedAdmin(ctx, userSvc, *adminEmail, *adminPass); err != nil {
			log.Printf("[x] admin user: %v", err)
			failed = true
		}
	} else {
		log.Println("[~] Skipping admin user (--skip-admin)")
	}

	if failed {
		log.Println("Seeding completed with errors.")
		os.Exit(1)
	}

	log.Println("Seeding completed successfully.")
}

func seedRolePermissions(ctx context.Context, roleSvc domain.RoleService) error {
	if err := roleSvc.SyncPermissions(ctx); err != nil {
		return fmt.Errorf("failed to sync roles and permissions: %w", err)
	}
	log.Println("[v] Roles and permissions seeded")
	return nil
}

func seedAdmin(ctx context.Context, userSvc domain.UserService, email, password string) error {
	req := domain.UserSaveRequest{
		Name:     "Admin",
		Email:    email,
		Password: password,
		RoleID:   1,
	}
	if err := userSvc.Create(ctx, req); err != nil {
		return fmt.Errorf("failed to create admin user: %w", err)
	}
	log.Printf("[v] Admin seeded | Email: %s", email)
	return nil
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `Usage: seed [OPTIONS]

Options:
  -dsn <url>              Database URL: postgres://user:pass@host:port/db
                          Overrides DATABASE_URL env var
  -env-file <path>        Path to a .env file to load (optional)
  -admin-email <email>    Admin user email (default: admin@horizonx.local)
                          Overrides DB_ADMIN_EMAIL env var
  -admin-password <pass>  Admin user password (default: password)
                          Overrides DB_ADMIN_PASSWORD env var
  -skip-roles             Skip seeding roles and permissions
  -skip-admin             Skip seeding the admin user
  -timeout <duration>     Context timeout for seed operations (default: 30s)

Examples:
  # Seed everything with defaults
  seed -dsn=postgres://user:pass@localhost:5432/horizonx

  # Custom admin credentials
  seed -dsn=postgres://... -admin-email=ops@company.com -admin-password=str0ng!

  # Load config from env file
  seed -env-file=/var/lib/horizonx/server.env

  # Only seed roles, skip admin
  seed -dsn=postgres://... -skip-admin

  # Only seed admin, skip roles
  seed -dsn=postgres://... -skip-roles
`)
}
