# Entropic Makefile
# Provides common development tasks for the Entropic storage engine

.PHONY: help build test test-unit test-integration test-benchmark clean deps fmt lint vet security run dev docker-build docker-up docker-down migrate backup restore

# Default target
.DEFAULT_GOAL := help

# Colors for output
RED=\033[0;31m
GREEN=\033[0;32m
YELLOW=\033[1;33m
BLUE=\033[0;34m
NC=\033[0m # No Color

# Variables
BINARY_NAME=entropic-server
BUILD_DIR=bin
DOCKER_TAG=entropic/server:latest
TEST_TIMEOUT=30m

# Help target
help: ## Show this help message
	@echo "$(BLUE)Entropic Storage Engine - Development Commands$(NC)"
	@echo ""
	@awk 'BEGIN {FS = ":.*##"; printf "Usage:\n  make $(YELLOW)<target>$(NC)\n\nTargets:\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  $(GREEN)%-15s$(NC) %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

# Build targets
build: ## Build the application binary
	@echo "$(BLUE)Building Entropic server...$(NC)"
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/server
	@echo "$(GREEN)Build completed: $(BUILD_DIR)/$(BINARY_NAME)$(NC)"

build-race: ## Build with race detection enabled
	@echo "$(BLUE)Building Entropic server with race detection...$(NC)"
	@mkdir -p $(BUILD_DIR)
	go build -race -o $(BUILD_DIR)/$(BINARY_NAME)-race ./cmd/server
	@echo "$(GREEN)Race detection build completed: $(BUILD_DIR)/$(BINARY_NAME)-race$(NC)"

# Dependency management
deps: ## Download and verify dependencies
	@echo "$(BLUE)Downloading dependencies...$(NC)"
	go mod download
	go mod verify
	@echo "$(GREEN)Dependencies updated$(NC)"

deps-update: ## Update all dependencies
	@echo "$(BLUE)Updating dependencies...$(NC)"
	go get -u ./...
	go mod tidy
	@echo "$(GREEN)Dependencies updated$(NC)"

# Testing targets
test: test-unit test-integration ## Run all tests
	@echo "$(GREEN)All tests completed$(NC)"

test-unit: ## Run unit tests
	@echo "$(BLUE)Running unit tests...$(NC)"
	go test -v -timeout=$(TEST_TIMEOUT) -race ./internal/...

test-integration: ## Run integration tests
	@echo "$(BLUE)Running integration tests...$(NC)"
	go test -v -timeout=$(TEST_TIMEOUT) -tags=integration ./tests/integration/...

test-benchmark: ## Run benchmark tests
	@echo "$(BLUE)Running benchmark tests...$(NC)"
	go test -v -timeout=$(TEST_TIMEOUT) -bench=. -benchmem ./tests/benchmark/...

test-coverage: ## Run tests with coverage report
	@echo "$(BLUE)Running tests with coverage...$(NC)"
	go test -v -timeout=$(TEST_TIMEOUT) -race -coverprofile=coverage.out ./internal/...
	go tool cover -html=coverage.out -o coverage.html
	@echo "$(GREEN)Coverage report generated: coverage.html$(NC)"

# Code quality targets
fmt: ## Format Go code
	@echo "$(BLUE)Formatting code...$(NC)"
	go fmt ./...
	@echo "$(GREEN)Code formatted$(NC)"

lint: ## Run linter
	@echo "$(BLUE)Running linter...$(NC)"
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "$(YELLOW)golangci-lint not found, using go vet instead$(NC)"; \
		go vet ./...; \
	fi
	@echo "$(GREEN)Linting completed$(NC)"

vet: ## Run go vet
	@echo "$(BLUE)Running go vet...$(NC)"
	go vet ./...
	@echo "$(GREEN)Vet completed$(NC)"

security: ## Run security checks
	@echo "$(BLUE)Running security checks...$(NC)"
	@if command -v gosec >/dev/null 2>&1; then \
		gosec ./...; \
	else \
		echo "$(YELLOW)gosec not found, skipping security checks$(NC)"; \
		echo "$(YELLOW)Install with: go install github.com/securecodewarrior/gosec/v2/cmd/gosec@latest$(NC)"; \
	fi

# Development targets
run: build ## Run the application
	@echo "$(BLUE)Starting Entropic server...$(NC)"
	./$(BUILD_DIR)/$(BINARY_NAME)

dev: ## Start development environment
	@echo "$(BLUE)Starting development environment...$(NC)"
	./scripts/deploy.sh dev

dev-reset: ## Reset development environment
	@echo "$(BLUE)Resetting development environment...$(NC)"
	./scripts/deploy.sh clean -f
	./scripts/deploy.sh dev

# Docker targets
docker-build: ## Build Docker image
	@echo "$(BLUE)Building Docker image...$(NC)"
	docker build -t $(DOCKER_TAG) .
	@echo "$(GREEN)Docker image built: $(DOCKER_TAG)$(NC)"

docker-up: ## Start Docker services
	@echo "$(BLUE)Starting Docker services...$(NC)"
	docker-compose up -d
	@echo "$(GREEN)Docker services started$(NC)"

docker-down: ## Stop Docker services
	@echo "$(BLUE)Stopping Docker services...$(NC)"
	docker-compose down
	@echo "$(GREEN)Docker services stopped$(NC)"

docker-logs: ## Show Docker service logs
	@echo "$(BLUE)Showing Docker logs...$(NC)"
	docker-compose logs -f

docker-test: ## Run tests in Docker
	@echo "$(BLUE)Running tests in Docker...$(NC)"
	docker-compose -f docker-compose.test.yml up --build --abort-on-container-exit
	docker-compose -f docker-compose.test.yml down -v

# Database targets
migrate: ## Run database migrations
	@echo "$(BLUE)Running database migrations...$(NC)"
	@if [ -f "./$(BUILD_DIR)/$(BINARY_NAME)" ]; then \
		./$(BUILD_DIR)/$(BINARY_NAME) migrate; \
	else \
		echo "$(RED)Binary not found. Run 'make build' first.$(NC)"; \
		exit 1; \
	fi

migrate-status: ## Show migration status
	@echo "$(BLUE)Checking migration status...$(NC)"
	@if [ -f "./$(BUILD_DIR)/$(BINARY_NAME)" ]; then \
		./$(BUILD_DIR)/$(BINARY_NAME) migrate --status; \
	else \
		echo "$(RED)Binary not found. Run 'make build' first.$(NC)"; \
		exit 1; \
	fi

# Backup and restore targets
backup: ## Create data backup
	@echo "$(BLUE)Creating backup...$(NC)"
	./scripts/deploy.sh backup
	@echo "$(GREEN)Backup completed$(NC)"

restore: ## Restore from backup (requires BACKUP_DIR variable)
	@echo "$(BLUE)Restoring from backup...$(NC)"
	@if [ -z "$(BACKUP_DIR)" ]; then \
		echo "$(RED)BACKUP_DIR variable is required$(NC)"; \
		echo "$(YELLOW)Usage: make restore BACKUP_DIR=/path/to/backup$(NC)"; \
		exit 1; \
	fi
	./scripts/deploy.sh restore $(BACKUP_DIR)
	@echo "$(GREEN)Restore completed$(NC)"

# Utility targets
clean: ## Clean build artifacts and temporary files
	@echo "$(BLUE)Cleaning build artifacts...$(NC)"
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html
	go clean ./...
	@echo "$(GREEN)Clean completed$(NC)"

clean-docker: ## Clean Docker resources
	@echo "$(BLUE)Cleaning Docker resources...$(NC)"
	docker-compose down -v --remove-orphans
	docker system prune -f
	@echo "$(GREEN)Docker cleanup completed$(NC)"

install-tools: ## Install development tools
	@echo "$(BLUE)Installing development tools...$(NC)"
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install github.com/securecodewarrior/gosec/v2/cmd/gosec@latest
	go install golang.org/x/tools/cmd/goimports@latest
	@echo "$(GREEN)Development tools installed$(NC)"

version: ## Show version information
	@echo "$(BLUE)Entropic Storage Engine$(NC)"
	@echo "Go version: $(shell go version)"
	@echo "Build time: $(shell date)"
	@if [ -f "./$(BUILD_DIR)/$(BINARY_NAME)" ]; then \
		echo "Binary version: $(shell ./$(BUILD_DIR)/$(BINARY_NAME) version)"; \
	else \
		echo "Binary: Not built"; \
	fi

# Performance targets
profile-cpu: ## Run CPU profiling
	@echo "$(BLUE)Running CPU profiling...$(NC)"
	go test -v -timeout=$(TEST_TIMEOUT) -cpuprofile=cpu.prof -bench=. ./tests/benchmark/...
	@echo "$(GREEN)CPU profile saved to cpu.prof$(NC)"
	@echo "$(YELLOW)View with: go tool pprof cpu.prof$(NC)"

profile-mem: ## Run memory profiling
	@echo "$(BLUE)Running memory profiling...$(NC)"
	go test -v -timeout=$(TEST_TIMEOUT) -memprofile=mem.prof -bench=. ./tests/benchmark/...
	@echo "$(GREEN)Memory profile saved to mem.prof$(NC)"
	@echo "$(YELLOW)View with: go tool pprof mem.prof$(NC)"

# CI targets
ci: deps fmt vet lint test-unit test-integration ## Run CI pipeline
	@echo "$(GREEN)CI pipeline completed successfully$(NC)"

ci-full: ci test-benchmark security ## Run full CI pipeline including benchmarks and security
	@echo "$(GREEN)Full CI pipeline completed successfully$(NC)"

# Load testing targets
load-test: ## Run load tests (requires k6)
	@echo "$(BLUE)Running load tests...$(NC)"
	@if command -v k6 >/dev/null 2>&1; then \
		k6 run tests/load/entity_creation.js; \
	else \
		echo "$(YELLOW)k6 not found. Install from https://k6.io/docs/getting-started/installation/$(NC)"; \
	fi

# Documentation targets
docs: ## Generate documentation
	@echo "$(BLUE)Generating documentation...$(NC)"
	@if command -v godoc >/dev/null 2>&1; then \
		echo "$(GREEN)Starting documentation server at http://localhost:6060$(NC)"; \
		godoc -http=:6060; \
	else \
		echo "$(YELLOW)godoc not found. Install with: go install golang.org/x/tools/cmd/godoc@latest$(NC)"; \
	fi

# Status targets
status: ## Show service status
	@echo "$(BLUE)Checking service status...$(NC)"
	./scripts/deploy.sh status

logs: ## Show application logs
	@echo "$(BLUE)Showing application logs...$(NC)"
	./scripts/deploy.sh logs entropic

# Quick development commands
quick-test: ## Quick test run (unit tests only, no race detection)
	@echo "$(BLUE)Running quick tests...$(NC)"
	go test -timeout=5m ./internal/...

quick-build: ## Quick build (no race detection)
	@echo "$(BLUE)Quick build...$(NC)"
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/server

# Environment setup
setup: install-tools deps ## Setup development environment
	@echo "$(BLUE)Setting up development environment...$(NC)"
	@if [ ! -f "entropic.yaml" ]; then \
		cp entropic.example.yaml entropic.yaml; \
		echo "$(GREEN)Created entropic.yaml from example$(NC)"; \
	fi
	@echo "$(GREEN)Development environment setup completed$(NC)"
	@echo "$(YELLOW)Next steps:$(NC)"
	@echo "  1. Review and modify entropic.yaml as needed"
	@echo "  2. Run 'make dev' to start development environment"
	@echo "  3. Run 'make test' to verify everything works"