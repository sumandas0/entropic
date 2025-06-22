# OpenAPI Documentation for Entropic Storage Engine

This document explains how to work with the OpenAPI/Swagger documentation for the Entropic Storage Engine API.

## Overview

Entropic uses two approaches for API documentation:
1. **Swaggo** - Generates OpenAPI specs from Go code annotations
2. **Static OpenAPI Spec** - Hand-crafted comprehensive OpenAPI 3.0 specification

## Swaggo Documentation (Auto-generated)

### Installation

```bash
# Install swag CLI tool
go install github.com/swaggo/swag/cmd/swag@latest

# Install dependencies
go get -u github.com/swaggo/http-swagger
go get -u github.com/swaggo/files
```

### Generating Documentation

```bash
# Generate swagger docs from code annotations
swag init -g cmd/server/main.go -o docs --parseDependency --parseInternal

# Or use the Makefile
make openapi-gen
```

### Viewing Documentation

1. **Built-in Swagger UI** (when server is running):
   ```bash
   go run cmd/server/main.go
   # Open http://localhost:8080/swagger/index.html
   ```

2. **Standalone Swagger UI**:
   ```bash
   make openapi-serve
   # Open http://localhost:8081
   ```

### Adding Annotations

Add swaggo annotations to your handler functions:

```go
// CreateEntity creates a new entity
// @Summary Create a new entity
// @Description Creates a new entity of the specified type
// @Tags entities
// @Accept json
// @Produce json
// @Param entityType path string true "Entity Type"
// @Param entity body EntityRequest true "Entity data"
// @Success 201 {object} EntityResponse
// @Failure 400 {object} middleware.ErrorResponse
// @Router /api/v1/entities/{entityType} [post]
func (h *EntityHandler) CreateEntity(w http.ResponseWriter, r *http.Request) {
    // handler implementation
}
```

## Static OpenAPI Specification

### Location
- Main spec: `api/openapi.yaml`
- Generated code config: `api/codegen-config.yaml`

### Validating the Spec

```bash
# Validate OpenAPI specification
make openapi-validate

# Or directly
swagger-cli validate api/openapi.yaml
```

### Generating Code from Spec

```bash
# Generate server interfaces
make openapi-server-gen

# Generate client SDK
make openapi-client-gen

# Generate all artifacts
make openapi-all
```

## Available Make Targets

```bash
# OpenAPI related commands
make openapi-gen        # Generate docs from code annotations
make openapi-validate   # Validate OpenAPI specification
make openapi-server-gen # Generate server code from spec
make openapi-client-gen # Generate client code from spec
make openapi-all        # Generate all OpenAPI artifacts
make openapi-serve      # Serve docs with Swagger UI
make openapi-update     # Update spec from code annotations
```

## Development Workflow

### Using the Helper Script

```bash
# Install required tools
./scripts/openapi-dev.sh install

# Validate specification
./scripts/openapi-dev.sh validate

# Generate code
./scripts/openapi-dev.sh generate

# Watch for changes
./scripts/openapi-dev.sh watch

# Generate SDK for a language
./scripts/openapi-dev.sh sdk typescript
./scripts/openapi-dev.sh sdk python
./scripts/openapi-dev.sh sdk go
```

### Best Practices

1. **Keep Annotations Updated**: When modifying API endpoints, update swaggo annotations
2. **Validate Before Commit**: Run `make openapi-validate` before committing changes
3. **Use Consistent Models**: Ensure request/response models match between code and spec
4. **Document All Endpoints**: Every API endpoint should have complete documentation
5. **Include Examples**: Add example requests and responses in annotations

## API Documentation Structure

### Endpoints Organization

- **Health** (`/health`, `/ready`, `/metrics`) - System health and monitoring
- **Entities** (`/api/v1/entities`) - Entity CRUD operations
- **Relations** (`/api/v1/relations`) - Relationship management
- **Schemas** (`/api/v1/schemas`) - Schema definition and management
- **Search** (`/api/v1/search`) - Text and vector search operations

### Authentication

Currently, the API supports:
- API Key authentication (X-API-Key header)
- Basic authentication (for future implementation)

### Response Format

All API responses follow a consistent format:
- Success responses include the requested data
- Error responses include error message, code, and details
- All responses are in JSON format

## CI/CD Integration

The project includes GitHub Actions workflow (`.github/workflows/openapi.yml`) that:
- Validates OpenAPI specifications on PR
- Generates documentation from code
- Checks for breaking changes
- Generates client SDKs for multiple languages
- Publishes documentation to GitHub Pages

## Troubleshooting

### Common Issues

1. **Swag command not found**
   ```bash
   go install github.com/swaggo/swag/cmd/swag@latest
   export PATH=$PATH:$(go env GOPATH)/bin
   ```

2. **Import errors in generated docs**
   - Ensure all custom types are properly annotated
   - Use `--parseDependency` flag when generating

3. **Swagger UI not loading**
   - Check that docs are generated (`docs/` directory exists)
   - Verify import in main.go: `_ "github.com/sumandas0/entropic/docs"`

## Examples

### Example API Calls

```bash
# Create an entity
curl -X POST http://localhost:8080/api/v1/entities/user \
  -H "Content-Type: application/json" \
  -d '{"urn": "urn:entropic:user:john", "properties": {"name": "John Doe"}}'

# Search entities
curl -X POST http://localhost:8080/api/v1/search \
  -H "Content-Type: application/json" \
  -d '{"entity_types": ["user"], "query": "John", "limit": 10}'

# Get health status
curl http://localhost:8080/health
```

### Postman Collection

Generate a Postman collection from the OpenAPI spec:
```bash
make openapi-all
# Collection will be at api/postman-collection.json
```

## Contributing

When adding new endpoints:
1. Add handler implementation with swaggo annotations
2. Update static OpenAPI spec if needed
3. Generate documentation: `make openapi-gen`
4. Validate: `make openapi-validate`
5. Test the endpoint through Swagger UI
6. Submit PR with updated documentation