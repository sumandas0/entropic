# Entropic Storage Engine

A next-generation storage engine that provides a flexible entity-relationship model with dual-storage architecture, combining PostgreSQL for primary storage and Typesense for search/indexing capabilities.

## Features

- **Dual Storage Architecture**: PostgreSQL for ACID compliance and Typesense for fast search
- **Entity-Relationship Model**: Flexible schema with support for complex relationships
- **Vector Search**: Native pgvector support for AI/ML applications
- **Two-Phase Commit**: Ensures consistency between primary and index stores
- **Schema Evolution**: Support for adding/modifying fields without downtime
- **High Performance**: Optimized for both transactional and analytical workloads

## Architecture

```
┌─────────────────┐     ┌─────────────────┐
│   API Layer     │     │   CLI Tools     │
└────────┬────────┘     └────────┬────────┘
         │                       │
         └───────────┬───────────┘
                     │
         ┌───────────▼───────────┐
         │    Core Engine        │
         │  - Validation         │
         │  - Transaction Mgmt   │
         │  - Denormalization    │
         └───────────┬───────────┘
                     │
      ┌──────────────┴──────────────┐
      │                             │
┌─────▼─────┐              ┌───────▼────────┐
│PostgreSQL │              │   Typesense    │
│- Entities │              │- Full-text     │
│- Relations│              │- Vector search │
│- Schemas  │              │- Faceting      │
└───────────┘              └────────────────┘
```

## Prerequisites

- Go 1.24+
- PostgreSQL 17.5 with pgvector extension
- Typesense 28.0
- Docker & Docker Compose (for development and testing)

## Quick Start

### Using Docker Compose

```bash
# Clone the repository
git clone https://github.com/sumandas0/entropic.git
cd entropic

# Start all services
docker-compose up -d

# Run migrations
go run cmd/server/main.go migrate

# Start the server
go run cmd/server/main.go serve
```

### Manual Setup

1. **Install PostgreSQL with pgvector**:
```bash
# macOS
brew install postgresql@17
brew install pgvector

# Ubuntu/Debian
sudo apt-get install postgresql-17 postgresql-17-pgvector
```

2. **Install Typesense**:
```bash
# Using Docker
docker run -p 8108:8108 -v/tmp/typesense-data:/data \
  typesense/typesense:28.0 \
  --data-dir /data --api-key=your-api-key
```

3. **Configure the application**:
```bash
cp config/config.example.yaml config/config.yaml
# Edit config.yaml with your database and Typesense settings
```

4. **Run migrations and start**:
```bash
go run cmd/server/main.go migrate
go run cmd/server/main.go serve
```

## Testing

The project uses **dockertest** for integration testing with real databases, ensuring tests catch database-specific issues and verify actual behavior.

### Running Tests

```bash
# Run all tests (includes dockertest integration tests)
go test ./...

# Run specific package tests
go test ./internal/store/postgres -v

# Run with coverage
go test -cover ./...

# Run integration tests only
go test ./tests/integration/... -count=1
```

### Dockertest Benefits

- Tests run against real PostgreSQL with pgvector extension
- Each test gets an isolated, clean database instance
- Automatic cleanup after tests complete
- No external database setup required
- CI/CD friendly - works anywhere Docker is available

### Test Utilities

The project includes dockertest utilities in `internal/store/testutils/docker.go`:

```go
// Setup PostgreSQL for testing
container, err := testutils.SetupTestPostgres()
defer container.Cleanup()

// Setup Typesense for testing
container, err := testutils.SetupTestTypesense()
defer container.Cleanup()
```

## API Examples

### Create Entity Schema

```bash
curl -X POST http://localhost:8080/schemas/entities \
  -H "Content-Type: application/json" \
  -d '{
    "entity_type": "user",
    "properties": {
      "name": {"type": "string", "required": true},
      "email": {"type": "string", "required": true},
      "age": {"type": "number", "required": false}
    }
  }'
```

### Create Entity

```bash
curl -X POST http://localhost:8080/entities/user \
  -H "Content-Type: application/json" \
  -d '{
    "urn": "user:john-doe",
    "properties": {
      "name": "John Doe",
      "email": "john@example.com",
      "age": 30
    }
  }'
```

### Search Entities

```bash
curl -X POST http://localhost:8080/search \
  -H "Content-Type: application/json" \
  -d '{
    "entity_types": ["user"],
    "query": "John",
    "limit": 10
  }'
```

### Vector Search

```bash
curl -X POST http://localhost:8080/search/vector \
  -H "Content-Type: application/json" \
  -d '{
    "entity_types": ["document"],
    "vector": [0.1, 0.2, 0.3, ...],
    "vector_field": "embedding",
    "top_k": 5
  }'
```

## Development

### Project Structure

```
entropic/
├── cmd/              # Application entry points
├── internal/         # Private application code
│   ├── api/         # REST API handlers
│   ├── core/        # Business logic
│   ├── store/       # Storage adapters
│   └── models/      # Data models
├── pkg/             # Public packages
├── tests/           # Integration tests
└── config/          # Configuration files
```

### Building

```bash
# Build the server
go build -o entropic cmd/server/main.go

# Build with optimizations
go build -ldflags="-w -s" -o entropic cmd/server/main.go
```

### Code Style

The project follows standard Go conventions:
- Run `go fmt` before committing
- Use `golangci-lint` for linting
- Write tests for new functionality
- Document exported functions

## Monitoring

### Health Check

```bash
curl http://localhost:8080/health
```

### Metrics

Prometheus metrics are exposed at:
```bash
curl http://localhost:8080/metrics
```

## Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

### Testing Requirements

- All new features must include tests
- Tests must pass with dockertest (real database testing)
- Maintain >80% code coverage
- Run `go test ./...` before submitting PR

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Acknowledgments

- PostgreSQL team for the excellent database
- pgvector team for vector similarity search
- Typesense team for the fast search engine
- Docker and dockertest teams for testing infrastructure