BINARY  := bin/c4forge
PKG     := github.com/Meidorislav/c4-forge
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short=12 HEAD 2>/dev/null)
LDFLAGS := -s -w -X $(PKG)/internal/buildinfo.Version=$(VERSION) -X $(PKG)/internal/buildinfo.Commit=$(COMMIT)

.PHONY: build run test lint fmt tidy

build: ## Build the binary into bin/
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/c4forge

run: ## Run the server with human-readable logs
	C4FORGE_LOG_FORMAT=text C4FORGE_LOG_LEVEL=debug go run ./cmd/c4forge

test: ## Run all tests with the race detector
	go test -race ./...

lint: ## Run linters (requires golangci-lint v2)
	golangci-lint run

fmt: ## Format code
	golangci-lint fmt

tidy: ## Tidy go.mod and go.sum
	go mod tidy
