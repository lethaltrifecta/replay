.PHONY: help build test test-integration test-e2e test-e2e-replay-freeze test-e2e-freeze-contract test-agent-loop-freeze test-migration-demo-full-loop lint fmt clean run docker-build docker-run setup-dev dev-up dev-down dev-reset generate

# Binary name
BINARY_NAME=cmdr
VERSION?=0.1.0
BUILD_DIR=bin

# Tools
OAPI_CODEGEN_VERSION=v2.6.0
OAPI_CODEGEN=go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@$(OAPI_CODEGEN_VERSION)
OPENAPI_TS_CODEGEN_VERSION=0.29.0
OPENAPI_TS_CODEGEN=npx -y openapi-typescript-codegen@$(OPENAPI_TS_CODEGEN_VERSION)

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
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-30s\033[0m %s\n", $$1, $$2}'

# ============================================================================
# Development
# ============================================================================

setup-dev: ## Set up local development environment
	@echo "Setting up development environment..."
	@chmod +x scripts/setup-dev.sh
	@./scripts/setup-dev.sh

dev-up: ## Start development services (PostgreSQL, Jaeger)
	@echo "Starting development services with $(DOCKER_COMPOSE)..."
	$(DOCKER_COMPOSE) up -d db
	@echo "Waiting for services to be ready..."
	@sleep 3
	@echo "✅ Development services started"
	@echo "   - PostgreSQL: localhost:5432"

dev-down: ## Stop development services
	@echo "Stopping development services..."
	$(DOCKER_COMPOSE) down

dev-reset: ## Reset development database
	@echo "Resetting development database..."
	$(DOCKER_COMPOSE) down -v
	$(DOCKER_COMPOSE) up -d db
	@echo "Waiting for PostgreSQL to be ready..."
	@sleep 3
	@echo "✅ Database reset complete"

generate: ## Generate code from OpenAPI spec
	@echo "Generating OpenAPI code..."
	@mkdir -p pkg/api
	@mkdir -p ui/packages/api
	$(OAPI_CODEGEN) -package api -generate types,std-http api/openapi.yaml > pkg/api/openapi_generated.gen.go
	$(OPENAPI_TS_CODEGEN) --input api/openapi.yaml --output ui/packages/api --client fetch
	@echo "✅ Generated pkg/api/openapi_generated.gen.go"
	@echo "✅ Generated ui/packages/api"

# ============================================================================
# Build
# ============================================================================

build: generate ## Build the binary
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/cmdr/main.go

run: build ## Build and run the service
	@echo "Running $(BINARY_NAME)..."
	@if [ ! -f .env ]; then echo "⚠️  Warning: .env file not found. Copy .env.example to .env"; fi
	@set -a; [ -f .env ] && . ./.env; set +a; $(BUILD_DIR)/$(BINARY_NAME) serve

# ============================================================================
# Testing
# ============================================================================

test: generate ## Run unit tests
	@echo "Running unit tests..."
	@$(GOTEST) -v ./pkg/... ./cmd/...

test-storage: dev-up ## Run storage tests with real PostgreSQL
	@echo "Running storage tests..."
	@CMDR_POSTGRES_URL=postgres://cmdr:cmdr_dev_password@localhost:5432/cmdr?sslmode=disable $(GOTEST) -v -tags=integration ./pkg/storage/...

test-integration: ## Run integration tests
	$(GOTEST) -v -tags=integration ./test/integration/...

test-e2e: ## Run end-to-end tests
	$(MAKE) test-e2e-freeze-contract
	$(MAKE) test-e2e-replay-freeze

test-e2e-replay-freeze: ## Run replay->freeze ingest/replay e2e flow
	@echo "Running replay+freeze integration e2e..."
	E2E_REPLAY_FREEZE=1 $(GOTEST) -v -tags=e2e ./test/e2e/... -run TestReplayFreezeIngestAndReplayIntegration -count=1

test-e2e-freeze-contract: ## Run freeze MCP contract e2e suite
	@echo "Running freeze MCP contract e2e..."
	E2E_FREEZE_CONTRACT=1 $(GOTEST) -v -tags=e2e ./test/e2e/... -run TestFreezeMCPContract -count=1

test-agent-loop-freeze: ## Run the local full-loop freeze-mcp smoke harness
	@echo "Running local freeze-mcp agent loop smoke harness..."
	./scripts/test-agent-loop-freeze.sh

test-migration-demo-full-loop: ## Run the database migration full-loop demo harness
	@echo "Running database migration full-loop demo..."
	./scripts/test-migration-demo-full-loop.sh

# ============================================================================
# Code Quality
# ============================================================================

lint: ## Run linter
	@echo "Running linter..."
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

# ============================================================================
# Docker
# ============================================================================

docker-build: ## Build Docker image
	@echo "Building Docker image..."
	docker build -t $(BINARY_NAME):$(VERSION) -t $(BINARY_NAME):latest .

docker-run: ## Run Docker container
	@echo "Running Docker container..."
	docker run -p 8080:8080 -p 4317:4317 -p 4318:4318 $(BINARY_NAME):latest

# ============================================================================
# Demo
# ============================================================================

.PHONY: demo
demo: build ## Run the hackathon demo (requires PostgreSQL: make dev-up)
	@echo "Starting demo (requires PostgreSQL: make dev-up)..."
	@bash scripts/demo.sh

.DEFAULT_GOAL := help
