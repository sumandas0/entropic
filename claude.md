# Claude Code Implementation Guide: Entropic Storage Engine

## Project Overview

Entropic is a next-generation storage engine that provides a flexible entity-relationship model with dual-storage architecture. This guide will help you implement the entire system using Claude Code.

## Prerequisites

- Go 1.24+
- PostgreSQL 17.5 with pgvector extension
- Typesense Version 28.0
- Docker & Docker Compose

## Project Structure

```
entropic/
├── cmd/
│   └── server/
│       └── main.go
├── internal/
│   ├── api/
│   │   ├── handlers/
│   │   │   ├── entity.go
│   │   │   ├── relation.go
│   │   │   └── schema.go
│   │   ├── middleware/
│   │   │   └── logging.go
│   │   └── router.go
│   ├── core/
│   │   ├── engine.go
│   │   ├── validator.go
│   │   ├── transaction.go
│   │   ├── transaction_manager.go
│   │   └── denormalization_manager.go
│   ├── cache/
│   │   └── manager.go
│   ├── lock/
│   │   └── manager.go
│   ├── models/
│   │   ├── entity.go
│   │   ├── relation.go
│   │   ├── schema.go
│   │   └── search.go
│   └── store/
│       ├── interfaces.go
│       ├── postgres/
│       │   ├── adapter.go
│       │   ├── migrations/
│       │   └── transaction.go
│       └── typesense/
│           └── adapter.go
├── pkg/
│   └── utils/
│       └── errors.go
├── tests/
│   ├── integration/
│   │   └── full_workflow_test.go
│   ├── benchmark/
│   │   └── performance_test.go
│   └── testhelpers/
│       └── helpers.go
├── config/
│   └── config.go
├── docker-compose.yml
├── Dockerfile
├── go.mod
└── go.sum
```

## Implementation Phases

### Phase 1: Core Interfaces and Models

**Prompt for Claude Code:**
```
Create the storage interfaces and data models for the Entropic storage engine. 

Requirements:
1. Create internal/store/interfaces.go with PrimaryStore and IndexStore interfaces as defined in the TRD section 4.1
2. Create internal/models/ directory with:
   - entity.go: Entity struct with id, entity_type, urn, properties map, timestamps
   - relation.go: Relation struct with id, relation_type, from/to entity references
   - schema.go: EntitySchema and RelationshipSchema structs with denormalization config
   - search.go: SearchQuery, VectorQuery, and SearchResult structs
3. Use github.com/google/uuid for UUID types
4. Include proper JSON tags and validation tags (using github.com/go-playground/validator/v10)
```

### Phase 2: PostgreSQL Adapter

**Prompt for Claude Code:**
```
Implement the PostgreSQL adapter for the PrimaryStore interface.

Requirements:
1. Create internal/store/postgres/adapter.go implementing PrimaryStore interface
2. Create internal/store/postgres/transaction.go implementing Transaction interface
3. Include connection pooling using pgx/v5
4. Implement CheckURNExists method with proper error handling
5. Create SQL migration files in internal/store/postgres/migrations/:
   - 001_create_entities_table.sql
   - 002_create_relations_table.sql
   - 003_create_schemas_table.sql
6. Add pgvector support for vector properties
7. Handle JSONB operations for properties field
8. Implement proper transaction isolation levels
```

### Phase 3: Typesense Adapter

**Prompt for Claude Code:**
```
Implement the Typesense adapter for the IndexStore interface.

Requirements:
1. Create internal/store/typesense/adapter.go implementing IndexStore interface
2. Use the official Typesense Go client
3. Implement collection creation with proper field mappings
4. Handle document upsert with flattened properties
5. Implement Search and VectorSearch methods with faceting support
6. Add retry logic for network failures
7. Include proper error handling and logging
```

### Phase 4: Cache and Lock Managers

**Prompt for Claude Code:**
```
Implement thread-safe cache and lock managers for the system.

Requirements:
1. Create internal/cache/manager.go with:
   - Thread-safe schema caching using sync.Map
   - Lazy loading from PostgreSQL
   - Cache invalidation on schema updates
   - Interface for future Redis integration
   - CacheAwareManager wrapper for notifier integration
2. Create internal/lock/manager.go with:
   - In-memory mutex map for resource locking
   - Methods: Lock(resource string), Unlock(resource string)
   - Schema-level and entity-level locking
   - Deadlock prevention strategies
```

### Phase 5: Core Engine and Validation

**Prompt for Claude Code:**
```
Implement the core business logic engine and validation system.

Requirements:
1. Create internal/core/engine.go with:
   - Two-phase commit orchestration
   - Entity creation with relationship denormalization
   - Update and delete operations
   - Proper error handling and rollback
   - Search and VectorSearch methods
2. Create internal/core/validator.go with:
   - Schema-based validation for entities
   - Property type validation
   - URN uniqueness validation
   - Relationship cardinality validation
3. Create internal/core/transaction.go for transaction coordination
4. Create internal/core/transaction_manager.go for managing transactions with timeouts
5. Create internal/core/denormalization_manager.go for handling denormalization logic
```

### Phase 6: API Layer

**Prompt for Claude Code:**
```
Implement the RESTful API using Chi router.

Requirements:
1. Create internal/api/router.go with Chi router setup
2. Create handlers in internal/api/handlers/:
   - entity.go: POST /entities/{type}, GET /entities/{type}/{id}, PATCH, DELETE
   - relation.go: POST /relations, GET /relations/{id}, DELETE
   - schema.go: POST /schemas/entities, GET /schemas/entities/{type}, PUT, DELETE
3. Add request/response DTOs with proper validation
4. Implement error handling middleware
5. Add request logging middleware
6. Include CORS support
7. Add OpenAPI documentation comments
```

### Phase 7: Configuration and Main Server

**Prompt for Claude Code:**
```
Create the configuration system and main server entry point.

Requirements:
1. Create config/config.go with:
   - Environment-based configuration using viper
   - PostgreSQL connection settings
   - Typesense connection settings
   - Server port and timeout configurations
   - Log level configuration
2. Create cmd/server/main.go with:
   - Graceful shutdown handling
   - Health check endpoints
   - Metrics endpoint (Prometheus format)
   - Connection pool initialization
   - Migration runner
```

### Phase 8: Docker Setup

**Prompt for Claude Code:**
```
Create Docker configuration for local development and testing.

Requirements:
1. Create multi-stage Dockerfile for the Go application
2. Create docker-compose.yml with:
   - PostgreSQL service with pgvector extension
   - Typesense service
   - Entropic service with proper networking
   - Volume mounts for data persistence
3. Add docker-compose.test.yml for integration testing
4. Include environment variable templates
```

### Phase 9: Testing Suite

**Prompt for Claude Code:**
```
Implement comprehensive tests for the Entropic system.

Requirements:
1. Create unit tests for:
   - Storage adapters (using testcontainers)
   - Core engine logic
   - Validation logic
   - Cache and lock managers
2. Create integration tests for:
   - Full entity creation workflow
   - Two-phase commit scenarios
   - Concurrent operations
   - Schema updates with active data
3. Add benchmark tests for:
   - Entity creation throughput
   - Search performance
   - Cache hit rates
4. Use testify for assertions and mocks
```

### Phase 10: Advanced Features

**Prompt for Claude Code:**
```
Implement advanced features for production readiness.

Requirements:
1. Add observability:
   - OpenTelemetry integration for tracing
   - Structured logging with zerolog
   - Metrics collection with Prometheus
2. Add resilience:
   - Circuit breakers for external services
   - Retry mechanisms with exponential backoff
   - Graceful degradation when index store is unavailable
3. Add security:
   - Rate limiting per IP
   - Request size limits
   - SQL injection prevention
   - Input sanitization
```

## API Method Reference

### Core Engine Methods

The `Engine` struct provides the following methods:

#### Entity Operations
- `CreateEntity(ctx, entity) error` - Creates a new entity
- `GetEntity(ctx, entityType, id) (*Entity, error)` - Retrieves an entity
- `UpdateEntity(ctx, entity) error` - Updates an existing entity
- `DeleteEntity(ctx, entityType, id) error` - Deletes an entity
- `ListEntities(ctx, entityType, limit, offset) ([]*Entity, error)` - Lists entities with pagination

#### Relation Operations
- `CreateRelation(ctx, relation) error` - Creates a new relation
- `GetRelation(ctx, id) (*Relation, error)` - Retrieves a relation
- `DeleteRelation(ctx, id) error` - Deletes a relation
- `GetRelationsByEntity(ctx, entityID, relationTypes) ([]*Relation, error)` - Gets relations for an entity

#### Schema Operations
- `CreateEntitySchema(ctx, schema) error` - Creates an entity schema
- `GetEntitySchema(ctx, entityType) (*EntitySchema, error)` - Retrieves an entity schema
- `UpdateEntitySchema(ctx, schema) error` - Updates an entity schema
- `DeleteEntitySchema(ctx, entityType) error` - Deletes an entity schema
- `ListEntitySchemas(ctx) ([]*EntitySchema, error)` - Lists all entity schemas
- `CreateRelationshipSchema(ctx, schema) error` - Creates a relationship schema
- `GetRelationshipSchema(ctx, relationshipType) (*RelationshipSchema, error)` - Retrieves a relationship schema
- `UpdateRelationshipSchema(ctx, schema) error` - Updates a relationship schema
- `DeleteRelationshipSchema(ctx, relationshipType) error` - Deletes a relationship schema
- `ListRelationshipSchemas(ctx) ([]*RelationshipSchema, error)` - Lists all relationship schemas

#### Search Operations
- `Search(ctx, query) (*SearchResult, error)` - Performs text search
- `VectorSearch(ctx, query) (*SearchResult, error)` - Performs vector similarity search

### Model Structures

#### SearchQuery
```go
type SearchQuery struct {
    EntityTypes []string               // Required: entity types to search
    Query       string                 // Search query string
    Filters     map[string]interface{} // Optional filters
    Facets      []string               // Fields to facet on
    Sort        []SortOption           // Sort options
    Limit       int                    // Max results (1-1000)
    Offset      int                    // Pagination offset
    IncludeURN  bool                   // Include URN in results
}
```

#### VectorQuery
```go
type VectorQuery struct {
    EntityTypes    []string               // Required: entity types to search
    Vector         []float32              // Required: query vector
    VectorField    string                 // Required: field containing vectors
    TopK           int                    // Required: number of results (1-1000)
    Filters        map[string]interface{} // Optional filters
    MinScore       float32                // Minimum similarity score
    IncludeVectors bool                   // Include vectors in results
}
```

#### SearchResult
```go
type SearchResult struct {
    Hits       []SearchHit              // Search results
    TotalHits  int64                    // Total number of matches
    Facets     map[string][]FacetValue  // Facet results
    SearchTime time.Duration            // Search execution time
    Query      interface{}              // Original query
}
```

#### SearchHit
```go
type SearchHit struct {
    ID         uuid.UUID              // Entity ID
    EntityType string                 // Entity type
    URN        string                 // Entity URN (optional)
    Score      float32                // Relevance/similarity score
    Properties map[string]interface{} // Entity properties
    Highlights map[string][]string    // Search highlights
    Vector     []float32              // Vector (if requested)
}
```

## Testing Strategy

### Unit Testing
```bash
# Run all unit tests
go test ./internal/...

# Run with coverage
go test -cover ./internal/...

# Run specific package tests
go test ./internal/store/postgres
go test ./internal/cache
```

### Integration Testing
```bash
# Start test environment
docker-compose -f docker-compose.test.yml up -d

# Run integration tests
go test ./tests/integration/... -tags=integration

# Clean up
docker-compose -f docker-compose.test.yml down -v
```

### Load Testing
```bash
# Use k6 for load testing
k6 run tests/load/entity_creation.js
```

## Common Issues and Solutions

### Test Compilation Errors

When working with tests, ensure:
1. Use `Search()` instead of `SearchEntities()` on the Engine
2. Use `GetRelationsByEntity()` instead of `GetEntityRelations()`
3. Access search results via `SearchResult.Hits` not `SearchResult.Entities`
4. Use `TopK` field instead of `Limit` in VectorQuery
5. Always include `VectorField` when creating a VectorQuery

### Error Handling

The `pkg/utils/errors.go` package provides:
- `AppError` type for structured errors
- Helper functions: `IsNotFound()`, `IsAlreadyExists()`, `IsValidation()`
- Proper usage of `errors.As()` with pointer variables

Example:
```go
var appErr *AppError
if errors.As(err, &appErr) {
    // Handle AppError
}
```

## Deployment Considerations

### Production Configuration
- Use connection pooling with appropriate limits
- Configure PostgreSQL with proper indexes
- Set up Typesense clustering for high availability
- Use environment-specific configuration files
- Enable TLS for all external connections

### Monitoring
- Set up Grafana dashboards for metrics
- Configure alerts for:
  - High error rates
  - Slow query performance
  - Cache miss rates
  - Lock contention

### Backup Strategy
- PostgreSQL continuous archiving
- Regular schema backups
- Typesense snapshot scheduling

## Common Claude Code Prompts

### Adding a New Storage Backend
```
Add a new storage adapter for [Neo4j/MongoDB/etc] that implements the PrimaryStore interface. Include connection management, transaction support, and proper error handling.
```

### Implementing a New API Endpoint
```
Add a new API endpoint for [bulk operations/export/import] with proper validation, error handling, and documentation.
```

### Performance Optimization
```
Analyze and optimize the entity creation workflow for better performance. Consider batch operations, connection pooling optimization, and caching strategies.
```

### Debugging Issues
```
Add comprehensive debugging for the two-phase commit process, including detailed logging at each step and rollback scenarios.
```

## Best Practices

1. **Error Handling**: Always wrap errors with context using `fmt.Errorf("context: %w", err)`
2. **Logging**: Use structured logging with correlation IDs
3. **Testing**: Maintain >80% test coverage
4. **Documentation**: Keep API documentation up-to-date
5. **Code Organization**: Follow standard Go project layout
6. **Concurrency**: Use goroutines judiciously, always with proper synchronization
7. **Resource Management**: Always close connections and release locks in defer statements

## Troubleshooting

### Common Issues

1. **Connection Pool Exhaustion**
   - Check max connection settings
   - Look for connection leaks
   - Monitor slow queries

2. **Lock Contention**
   - Review locking granularity
   - Check for deadlocks
   - Consider optimistic locking

3. **Cache Inconsistency**
   - Verify invalidation logic
   - Check for race conditions
   - Monitor cache hit rates

4. **Transaction Rollback Failures**
   - Check for partial commits
   - Verify rollback handlers
   - Review error propagation

## Next Steps

After implementing the base system:
1. Add authentication and authorization
2. Implement a gRPC interface
3. Add support for additional storage backends
4. Implement data migration tools
5. Add GraphQL API support
6. Build admin UI for schema management