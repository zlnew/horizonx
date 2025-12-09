APP_NAME=horizonx-server
ENTRY=./cmd/horizonx-server
MIGRATE_TOOL=cmd/migrate/main.go
DB_PATH=horizonx.db
MIGRATION_DIR=internal/storage/sqlite/migrations

# --- App Commands ---
build:
	@echo "Building binary..."
	@go build -o bin/$(APP_NAME) $(ENTRY)

run:
	@go run $(ENTRY)

clean:
	@rm -rf bin/$(APP_NAME)
	@rm -f $(DB_PATH)

# --- Database Commands ---
migrate-up:
	@go run $(MIGRATE_TOOL) -op=up -db=$(DB_PATH)

migrate-down:
	@go run $(MIGRATE_TOOL) -op=down -steps=1 -db=$(DB_PATH)

migrate-fresh:
	@echo "Resetting database..."
	@go run $(MIGRATE_TOOL) -op=down -db=$(DB_PATH)
	@go run $(MIGRATE_TOOL) -op=up -db=$(DB_PATH)
	@echo "Database fresh and clean!"

migrate-version:
	@go run $(MIGRATE_TOOL) -op=version -db=$(DB_PATH)

migrate-create:
	@test -n "$(name)" || (echo "Error: name is required"; exit 1)
	@echo "Creating migration files..."
	@mkdir -p $(MIGRATION_DIR)
	@touch $(MIGRATION_DIR)/$$(date +%Y%m%d%H%M%S)_$(name).up.sql
	@touch $(MIGRATION_DIR)/$$(date +%Y%m%d%H%M%S)_$(name).down.sql
	@echo "Files created in $(MIGRATION_DIR)"

seed:
	@go run cmd/seed/main.go -db=$(DB_PATH)

.PHONY: build run clean migrate-up migrate-down migrate-fresh migrate-version migrate-create
