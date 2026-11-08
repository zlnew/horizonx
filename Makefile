# --- Variables ---
APP_NAME=horizonx-server
AGENT_NAME=horizonx-agent

# Entry Points
SERVER_ENTRY=./cmd/server/main.go
AGENT_ENTRY=./cmd/agent/main.go
MIGRATE_SRC=./cmd/migrate/main.go
SEED_SRC=./cmd/seed/main.go
CLI_ENTRY=./cmd/horizonx

# Binaries Output
BIN_DIR=bin
SERVER_BIN=$(BIN_DIR)/server
AGENT_BIN=$(BIN_DIR)/agent
MIGRATE_BIN=$(BIN_DIR)/migrate
SEED_BIN=$(BIN_DIR)/seed
CLI_BIN=$(BIN_DIR)/horizonx

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
	@echo "   • compiling cli..."
	@go build -o $(CLI_BIN) $(CLI_ENTRY)

fmt:
	gofmt -w ./cmd ./internal

fmt-check:
	@test -z "$$(gofmt -l ./cmd ./internal)" || (echo "gofmt: files need formatting:"; gofmt -l ./cmd ./internal; exit 1)

vet:
	go vet ./...

test:
	go test ./... -count=1

race:
	go test -race ./... -count=1

lint:
	@command -v golangci-lint >/dev/null 2>&1 || (echo "golangci-lint not installed. Install: https://golangci-lint.run/docs/welcome/install/"; exit 1)
	golangci-lint run ./...

mocks:
	@command -v mockery >/dev/null 2>&1 || (echo "mockery not installed. Install: go install github.com/vektra/mockery/v2@latest"; exit 1)
	mockery

# test-unit excludes horizonx/internal/app: its tests assert host ports 4858/4859
# are free, but the local HorizonX dev stack occupies them
# (horizonx-local on 4858, horizonx-dashboard-local on 4859).
test-unit:
	go test -count=1 $$(go list ./... | grep -v '^horizonx/internal/app$$')

# check is the hermetic local gate (no network, no Postgres/Redis, no docker).
# It must stay hermetic and must not require golangci-lint or mockery,
# which are not installed here.
# NOTE: fmt-check is intentionally excluded from aggregates because the repo
# is not yet gofmt-clean and CI never checked formatting.
check: vet test-unit build

# check-all mirrors CI (vet + full test + build) and requires host ports
# 4858/4859 to be free (the local HorizonX dev stack occupies them).
check-all: vet test build

# test-integration runs the opt-in Postgres migration integration test.
# Requires Docker and the postgres:16-alpine image. Intentionally NOT part
# of check/check-all: it spins up a throwaway container (never personal-postgres).
test-integration:
	go test -tags=integration ./internal/adapters/postgres/... -count=1

help:
	@echo "Available targets:"
	@echo "  build      - compile all binaries into $(BIN_DIR)/"
	@echo "  fmt        - format ./cmd and ./internal with gofmt"
	@echo "  fmt-check  - fail if gofmt would reformat any file"
	@echo "  vet        - go vet ./..."
	@echo "  test       - go test ./... -count=1"
	@echo "  test-unit  - go test all packages except horizonx/internal/app"
	@echo "  race       - go test -race ./... -count=1"
	@echo "  lint       - golangci-lint (requires installed binary)"
	@echo "  mocks      - mockery (requires installed binary)"
	@echo "  check      - vet + test-unit + build (hermetic local gate)"
	@echo "  check-all  - vet + test + build (CI parity; needs 4858/4859 free)"
	@echo "  test-integration - migration integration test (Docker; NOT in check)"
	@echo "  help       - print this message"

.PHONY: build fmt fmt-check vet test test-unit test-integration race lint mocks check check-all help
