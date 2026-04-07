# --- Variables ---
APP_NAME=horizonx-server
AGENT_NAME=horizonx-agent

# Entry Points
SERVER_ENTRY=./cmd/server/main.go
AGENT_ENTRY=./cmd/agent/main.go
MIGRATE_SRC=./cmd/migrate/main.go
SEED_SRC=./cmd/seed/main.go

# Binaries Output
BIN_DIR=bin
SERVER_BIN=$(BIN_DIR)/server
AGENT_BIN=$(BIN_DIR)/agent
MIGRATE_BIN=$(BIN_DIR)/migrate
SEED_BIN=$(BIN_DIR)/seed

# --- Build Commands ---
build:
	@echo "building binaries..."
	@mkdir -p $(BIN_DIR)
	@echo "   • compiling server..."
	@go build -o $(SERVER_BIN) $(SERVER_ENTRY)
	@echo "   • compiling agent..."
	@go build -o $(AGENT_BIN) $(AGENT_ENTRY)
	@echo "   • compiling migrate & seed..."
	@go build -o $(MIGRATE_BIN) $(MIGRATE_SRC)
	@go build -o $(SEED_BIN) $(SEED_SRC)

.PHONY: build
