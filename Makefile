BINARY  := bin/c4forge
PKG     := github.com/Meidorislav/c4-forge
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short=12 HEAD 2>/dev/null)
LDFLAGS := -s -w -X $(PKG)/internal/buildinfo.Version=$(VERSION) -X $(PKG)/internal/buildinfo.Commit=$(COMMIT)

# Local database from docker-compose.yml. Both can be overridden, e.g.
# `make db-up run C4FORGE_DB_PORT=5433`.
C4FORGE_DB_PORT      ?= 5432
C4FORGE_DATABASE_URL ?= postgres://c4forge:c4forge@localhost:$(C4FORGE_DB_PORT)/c4forge?sslmode=disable
export C4FORGE_DB_PORT

.PHONY: build run test lint fmt tidy db-up db-down db-reset

build: ## Build the binary into bin/
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/c4forge

run: ## Run the server with human-readable logs against the local database (see db-up)
	C4FORGE_DATABASE_URL="$(C4FORGE_DATABASE_URL)" C4FORGE_LOG_FORMAT=text C4FORGE_LOG_LEVEL=debug go run ./cmd/c4forge

test: ## Run all tests with the race detector
	go test -race ./...

lint: ## Run linters (requires golangci-lint v2)
	golangci-lint run

fmt: ## Format code
	golangci-lint fmt

tidy: ## Tidy go.mod and go.sum
	go mod tidy

db-up: ## Start the local PostgreSQL and wait until it is healthy
	docker compose up -d --wait postgres

db-down: ## Stop the local PostgreSQL, keeping its data
	docker compose down

db-reset: ## Stop the local PostgreSQL and delete its data
	docker compose down --volumes
