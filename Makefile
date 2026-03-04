.PHONY: help build test test-integration test-e2e lint fmt clean run docker-build docker-run setup-dev dev-up dev-down dev-reset

# Binary name
BINARY_NAME=cmdr
VERSION?=0.1.0
BUILD_DIR=bin

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=$(GOCMD) fmt

# Docker Compose command (detect V2 or V1)
DOCKER_COMPOSE=$(shell if docker compose version > /dev/null 2>&1; then echo "docker compose"; else echo "docker-compose"; fi)

# Build flags
LDFLAGS=-ldflags "-X main.Version=$(VERSION)"

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-20s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# ============================================================================
# Development
# ============================================================================

setup-dev: ## Set up local development environment
	@echo "Setting up development environment..."
	@chmod +x scripts/setup-dev.sh
	@./scripts/setup-dev.sh

dev-up: ## Start development services (PostgreSQL, Jaeger)
	@echo "Starting development services with $(DOCKER_COMPOSE)..."
	$(DOCKER_COMPOSE) up -d postgres jaeger
	@echo "Waiting for services to be ready..."
	@sleep 3
	@echo "✅ Development services started"
	@echo "   - PostgreSQL: localhost:5432"
	@echo "   - Jaeger UI:  http://localhost:16686"

dev-down: ## Stop development services
	@echo "Stopping development services..."
	$(DOCKER_COMPOSE) down

dev-reset: ## Reset development database
	@echo "Resetting development database..."
	$(DOCKER_COMPOSE) down -v
	$(DOCKER_COMPOSE) up -d postgres
	@echo "Waiting for PostgreSQL to be ready..."
	@sleep 3
	@echo "✅ Database reset complete"

# ============================================================================
# Build
# ============================================================================

build: ## Build the binary
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/cmdr

run: build ## Build and run the service
	@echo "Running $(BINARY_NAME)..."
	@if [ ! -f .env ]; then echo "⚠️  Warning: .env file not found. Copy .env.example to .env"; fi
	@set -a; [ -f .env ] && . ./.env; set +a; $(BUILD_DIR)/$(BINARY_NAME) serve

# ============================================================================
# Testing
# ============================================================================

test: ## Run unit tests
	@echo "Running unit tests..."
	@if [ ! -f .env ]; then echo "⚠️  Warning: .env file not found for test config"; fi
	@set -a; [ -f .env ] && . ./.env; set +a; \
	$(GOTEST) -run '^$$' ./pkg/...; \
	TEST_PKGS=$$(go list -f '{{if or (gt (len .TestGoFiles) 0) (gt (len .XTestGoFiles) 0)}}{{.ImportPath}}{{end}}' ./pkg/... | xargs); \
	$(GOTEST) -v -race -coverprofile=coverage.out $$TEST_PKGS

test-storage: dev-up ## Run storage tests with real PostgreSQL
	@echo "Running storage tests..."
	@CMDR_POSTGRES_URL=postgres://cmdr:cmdr_dev_password@localhost:5432/cmdr?sslmode=disable $(GOTEST) -v ./pkg/storage/...

test-integration: ## Run integration tests
	@echo "Running integration tests..."
	$(GOTEST) -v -tags=integration ./test/integration/...

test-e2e: ## Run end-to-end tests
	@echo "Running e2e tests..."
	$(GOTEST) -v -tags=e2e ./test/e2e/...

coverage: test ## Generate coverage report
	@echo "Generating coverage report..."
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# ============================================================================
# Code Quality
# ============================================================================

lint: ## Run linter
	@echo "Running linter..."
	@which golangci-lint > /dev/null || (echo "golangci-lint not installed. Run: curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $$(go env GOPATH)/bin" && exit 1)
	golangci-lint run ./...

fmt: ## Format code
	@echo "Formatting code..."
	$(GOFMT) ./...

# ============================================================================
# Maintenance
# ============================================================================

clean: ## Clean build artifacts
	@echo "Cleaning..."
	$(GOCLEAN)
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html

deps: ## Download dependencies
	@echo "Downloading dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy

# ============================================================================
# Docker
# ============================================================================

docker-build: ## Build Docker image
	@echo "Building Docker image..."
	docker build -t $(BINARY_NAME):$(VERSION) -t $(BINARY_NAME):latest .

docker-run: ## Run Docker container
	@echo "Running Docker container..."
	docker run -p 8080:8080 -p 4317:4317 -p 4318:4318 $(BINARY_NAME):latest

.DEFAULT_GOAL := help
