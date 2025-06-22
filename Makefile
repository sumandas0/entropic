# Entropic Storage Engine Makefile

# Variables
GOPATH ?= $(shell go env GOPATH)
GOBIN ?= $(GOPATH)/bin
GOLANGCI_LINT_VERSION := v1.54.2
SWAG_VERSION := v1.16.2
OAPI_CODEGEN_VERSION := v2.3.0

# Default target
.PHONY: all
all: build

# Build the application
.PHONY: build
build:
	go build -o bin/entropic cmd/server/main.go

# Run the application
.PHONY: run
run:
	go run cmd/server/main.go

# Run tests
.PHONY: test
test:
	go test -v -race -coverprofile=coverage.out ./...

# Run unit tests only
.PHONY: test-unit
test-unit:
	go test -v -race -short ./...

# Run integration tests
.PHONY: test-integration
test-integration:
	go test -v -race -run Integration ./tests/integration/...

# Run benchmarks
.PHONY: bench
bench:
	go test -bench=. -benchmem ./tests/benchmark/...

# Coverage report
.PHONY: coverage
coverage: test
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Lint the code
.PHONY: lint
lint: install-lint
	$(GOBIN)/golangci-lint run

# Install linter
.PHONY: install-lint
install-lint:
	@if ! command -v $(GOBIN)/golangci-lint &> /dev/null; then \
		echo "Installing golangci-lint..."; \
		curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(GOBIN) $(GOLANGCI_LINT_VERSION); \
	fi

# Format code
.PHONY: fmt
fmt:
	go fmt ./...
	gofmt -s -w .

# Clean build artifacts
.PHONY: clean
clean:
	rm -rf bin/
	rm -f coverage.out coverage.html
	rm -rf docs/swagger/

# Docker commands
.PHONY: docker-build
docker-build:
	docker build -t entropic:latest .

.PHONY: docker-up
docker-up:
	docker-compose up -d

.PHONY: docker-down
docker-down:
	docker-compose down

.PHONY: docker-logs
docker-logs:
	docker-compose logs -f

# OpenAPI/Swagger generation
.PHONY: install-openapi-tools
install-openapi-tools:
	@echo "Installing OpenAPI tools..."
	@if ! command -v $(GOBIN)/swag &> /dev/null; then \
		go install github.com/swaggo/swag/cmd/swag@$(SWAG_VERSION); \
	fi
	@if ! command -v $(GOBIN)/oapi-codegen &> /dev/null; then \
		go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@$(OAPI_CODEGEN_VERSION); \
	fi
	@if ! command -v swagger-cli &> /dev/null; then \
		npm install -g @apidevtools/swagger-cli; \
	fi

# Generate OpenAPI documentation from code annotations
.PHONY: openapi-gen
openapi-gen: install-openapi-tools
	@echo "Generating OpenAPI documentation from code..."
	$(GOBIN)/swag init -g cmd/server/main.go -o docs/swagger --parseDependency --parseInternal

# Validate OpenAPI spec
.PHONY: openapi-validate
openapi-validate:
	@echo "Validating OpenAPI specification..."
	swagger-cli validate api/openapi.yaml

# Generate Go server code from OpenAPI spec
.PHONY: openapi-server-gen
openapi-server-gen: install-openapi-tools openapi-validate
	@echo "Generating server interfaces from OpenAPI spec..."
	$(GOBIN)/oapi-codegen -generate types,server,spec -package generated -o internal/generated/openapi_types.gen.go api/openapi.yaml

# Generate Go client code from OpenAPI spec
.PHONY: openapi-client-gen
openapi-client-gen: install-openapi-tools openapi-validate
	@echo "Generating client code from OpenAPI spec..."
	$(GOBIN)/oapi-codegen -generate types,client -package client -o pkg/client/openapi_client.gen.go api/openapi.yaml

# Generate all OpenAPI artifacts
.PHONY: openapi-all
openapi-all: openapi-validate openapi-gen openapi-server-gen openapi-client-gen
	@echo "All OpenAPI artifacts generated successfully!"

# Serve OpenAPI documentation locally
.PHONY: openapi-serve
openapi-serve:
	@echo "Starting Swagger UI at http://localhost:8081"
	docker run -p 8081:8080 -e SWAGGER_JSON=/api/openapi.yaml -v $(PWD)/api:/api swaggerapi/swagger-ui

# Generate and update OpenAPI spec from code
.PHONY: openapi-update
openapi-update: openapi-gen
	@echo "OpenAPI spec updated from code annotations"

# Development setup
.PHONY: dev-setup
dev-setup: install-lint install-openapi-tools
	@echo "Installing dependencies..."
	go mod download
	go mod tidy
	@echo "Development environment ready!"

# Run with hot reload (requires air)
.PHONY: dev
dev:
	@if ! command -v air &> /dev/null; then \
		echo "Installing air for hot reload..."; \
		go install github.com/air-verse/air@latest; \
	fi
	air

# Database migrations
.PHONY: migrate-up
migrate-up:
	@echo "Running database migrations..."
	go run cmd/migrate/main.go up

.PHONY: migrate-down
migrate-down:
	@echo "Rolling back database migrations..."
	go run cmd/migrate/main.go down

# Help target
.PHONY: help
help:
	@echo "Available targets:"
	@echo "  all                 - Build the application (default)"
	@echo "  build              - Build the application binary"
	@echo "  run                - Run the application"
	@echo "  test               - Run all tests with coverage"
	@echo "  test-unit          - Run unit tests only"
	@echo "  test-integration   - Run integration tests"
	@echo "  bench              - Run benchmarks"
	@echo "  coverage           - Generate coverage report"
	@echo "  lint               - Run linter"
	@echo "  fmt                - Format code"
	@echo "  clean              - Clean build artifacts"
	@echo ""
	@echo "Docker targets:"
	@echo "  docker-build       - Build Docker image"
	@echo "  docker-up          - Start services with docker-compose"
	@echo "  docker-down        - Stop services"
	@echo "  docker-logs        - View container logs"
	@echo ""
	@echo "OpenAPI targets:"
	@echo "  openapi-gen        - Generate OpenAPI docs from code"
	@echo "  openapi-validate   - Validate OpenAPI specification"
	@echo "  openapi-server-gen - Generate server code from OpenAPI"
	@echo "  openapi-client-gen - Generate client code from OpenAPI"
	@echo "  openapi-all        - Generate all OpenAPI artifacts"
	@echo "  openapi-serve      - Serve OpenAPI docs with Swagger UI"
	@echo "  openapi-update     - Update OpenAPI spec from code"
	@echo ""
	@echo "Development targets:"
	@echo "  dev-setup          - Setup development environment"
	@echo "  dev                - Run with hot reload"
	@echo "  migrate-up         - Run database migrations"
	@echo "  migrate-down       - Rollback migrations"