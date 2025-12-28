.PHONY: help build build-cli build-all test test-integration test-env-up test-env-down lint clean \
	build-linux-amd64 build-windows-amd64 build-release dist-clean

BINARY_NAME=opengslb
CLI_BINARY_NAME=opengslb-cli
GO=go
DIST_DIR=dist
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

build: ## Build the main binary
	$(GO) build -o $(BINARY_NAME) ./cmd/opengslb

build-cli: ## Build the CLI binary
	$(GO) build -o $(CLI_BINARY_NAME) ./cmd/opengslb-cli

build-all: build build-cli ## Build all binaries

test: ## Run unit tests
	$(GO) test -race ./...

test-coverage: ## Run tests with coverage
	$(GO) test -race -coverprofile=coverage.out -covermode=atomic ./...
	$(GO) tool cover -html=coverage.out -o coverage.html

lint: ## Run linters
	golangci-lint run

lint-fix: ## Run linters and fix issues
	golangci-lint run --fix

test-env-up: ## Start integration test environment
	./scripts/test-env-up.sh

test-env-down: ## Stop integration test environment
	./scripts/test-env-down.sh

test-integration: test-env-up ## Run integration tests
	$(GO) test -tags=integration -v ./test/integration/...
	$(MAKE) test-env-down

test-integration-only: ## Run integration tests (assumes env is up)
	$(GO) test -tags=integration -v ./test/integration/...

docker-build: ## Build Docker image
	docker build -t $(BINARY_NAME):local .

# Cross-platform release builds
VERSION_PKG=github.com/loganrossus/OpenGSLB/pkg/version
LDFLAGS=-s -w -X '$(VERSION_PKG).Version=$(VERSION)'

build-linux-amd64: ## Build for Linux amd64
	@mkdir -p $(DIST_DIR)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GO) build -ldflags="$(LDFLAGS)" -o $(DIST_DIR)/$(BINARY_NAME)-linux-amd64 ./cmd/opengslb

build-windows-amd64: ## Build for Windows amd64
	@mkdir -p $(DIST_DIR)
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 $(GO) build -ldflags="$(LDFLAGS)" -o $(DIST_DIR)/$(BINARY_NAME)-windows-amd64.exe ./cmd/opengslb

build-release: build-linux-amd64 build-windows-amd64 ## Build release binaries for all platforms
	@echo "Release binaries built in $(DIST_DIR)/"
	@ls -la $(DIST_DIR)/

dist-clean: ## Clean distribution directory
	rm -rf $(DIST_DIR)

clean: ## Clean build artifacts
	rm -f $(BINARY_NAME) $(CLI_BINARY_NAME)
	rm -f coverage.out coverage.html
	$(MAKE) test-env-down 2>/dev/null || true
