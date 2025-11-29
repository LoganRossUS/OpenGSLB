.PHONY: help build test test-integration test-env-up test-env-down lint clean

BINARY_NAME=opengslb
GO=go

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

build: ## Build the binary
	$(GO) build -o $(BINARY_NAME) ./cmd/opengslb

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

clean: ## Clean build artifacts
	rm -f $(BINARY_NAME)
	rm -f coverage.out coverage.html
	$(MAKE) test-env-down 2>/dev/null || true
