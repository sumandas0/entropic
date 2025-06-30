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

### Dockertest Implementation

The project uses **dockertest** for integration testing with real databases instead of mocks. This ensures tests catch database-specific issues and verify actual behavior.

#### Test Utilities
Located in `internal/store/testutils/docker.go`:

```go
// Setup PostgreSQL with pgvector extension
container, err := testutils.SetupTestPostgres()
defer container.Cleanup()

// Setup Typesense for search/indexing
container, err := testutils.SetupTestTypesense()
defer container.Cleanup()
```

#### Benefits of Dockertest
- Tests run against real PostgreSQL with pgvector extension
- Each test gets an isolated, clean database instance
- Automatic cleanup after tests complete
- No external database setup required
- CI/CD friendly - works anywhere Docker is available
- Catches database-specific issues (constraints, triggers, etc.)
- Tests real transaction isolation and concurrency

### Unit Testing
```bash
# Run all unit tests (uses dockertest for store tests)
go test ./internal/...

# Run with coverage
go test -cover ./internal/...

# Run specific package tests
go test ./internal/store/postgres -count=1
go test ./internal/cache
```

### Integration Testing
```bash
# Run integration tests (uses dockertest)
go test ./tests/integration/... -count=1

# Run store adapter tests with real databases
go test ./internal/store/postgres -count=1
go test ./internal/store/typesense -count=1
```

### Common Test Issues

#### JSON Type Conversions
When testing with real databases, JSON marshalling converts types:
- Numbers become `float64` (not `int`)
- Arrays become `[]interface{}` (not `[]string`)

Handle this in tests by checking individual fields:
```go
// Instead of comparing entire properties map
assert.Equal(t, entity.Properties, retrieved.Properties) // May fail

// Compare individual fields with correct types
assert.Equal(t, float64(30), retrieved.Properties["age"])
assert.Equal(t, entity.Properties["name"], retrieved.Properties["name"])
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

```md
Add a new storage adapter for [Neo4j/MongoDB/etc] that implements the PrimaryStore interface. Include connection management, transaction support, and proper error handling.
```

### Implementing a New API Endpoint

```md
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

### Running Tests with Dockertest
```
Run the PostgreSQL adapter tests using dockertest to verify all database operations work correctly with a real database.
```

## Best Practices

1. **Error Handling**: Always wrap errors with context using `fmt.Errorf("context: %w", err)`
2. **Logging**: Use structured logging with correlation IDs
3. **Testing**: Maintain >80% test coverage
4. **Documentation**: Keep API documentation up-to-date
5. **Code Organization**: Follow standard Go project layout
6. **Concurrency**: Use goroutines judiciously, always with proper synchronization
7. **Resource Management**: Always close connections and release locks in defer statements
8. **Less noise**: Use `logrus` or `zerolog` for structured logging, avoid using `fmt.Println` in production code, and don't use unnecessary comments that state the obvious

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

## API Documentation

### Swagger/OpenAPI Integration

The Entropic API is fully documented using Swagger/OpenAPI annotations. All endpoints, request/response models, and error structures have comprehensive swagger annotations.

#### Key Features:
- **Automatic Documentation Generation**: Run `swag init -g cmd/server/main.go -o docs` to regenerate documentation
- **Interactive API Explorer**: Swagger UI available for testing endpoints
- **Type-Safe Client Generation**: Use the swagger spec to generate clients in any language
- **Comprehensive Examples**: All models include example values for better understanding

#### Swagger Annotations Added:
1. **Main API Info** (`cmd/server/main.go`):
   - Title, version, description
   - Contact information
   - License details
   - Host and base path configuration

2. **All API Endpoints** have:
   - `@Summary` - Brief description
   - `@Description` - Detailed explanation
   - `@Tags` - API grouping
   - `@Accept` / `@Produce` - Content types
   - `@Param` - Path, query, and body parameters
   - `@Success` / `@Failure` - Response codes and models
   - `@Router` - Endpoint path and method

3. **All Models** include:
   - Field-level examples
   - Validation constraints
   - Enum values where applicable
   - Proper type mappings (e.g., `swaggertype:"object"` for maps)

#### Accessing the Documentation:
- Swagger JSON: `/docs/swagger.json`
- Swagger YAML: `/docs/swagger.yaml`
- Generated Go code: `/docs/docs.go`

#### Regenerating Documentation:
```bash
# Install swag if not already installed
go install github.com/swaggo/swag/cmd/swag@latest

# Generate/update documentation
swag init -g cmd/server/main.go -o docs
```

## Go SDK

Entropic provides a comprehensive Go SDK for easy integration with Go applications. The SDK offers type-safe, idiomatic interfaces to all Entropic APIs.

### SDK Features

- **Full API Coverage**: Complete access to entities, relations, schemas, and search APIs
- **Type Safety**: Strongly typed requests/responses with compile-time validation
- **Builder Patterns**: Fluent interfaces for constructing complex requests
- **Error Handling**: Typed errors with helper methods for common scenarios
- **Zero Dependencies**: Uses only standard library and project dependencies
- **Context Support**: All operations support Go contexts for cancellation/timeouts

### Installation

```bash
go get github.com/sumandas0/entropic/pkg/sdk
```

### Quick Example

```go
import "github.com/sumandas0/entropic/pkg/sdk"

// Create client
client, err := sdk.NewClient("http://localhost:8080")

// Create entity
user := sdk.NewEntityBuilder().
    WithURN("urn:entropic:user:123").
    WithProperty("name", "John Doe").
    WithProperty("email", "john@example.com").
    Build()

created, err := client.Entities.Create(ctx, "user", user)

// Search entities
query := sdk.NewSearchQueryBuilder([]string{"user"}, "john").
    WithFilter("status", "active").
    WithLimit(20).
    Build()

results, err := client.Search.Search(ctx, query)

// Vector search
vectorQuery := sdk.NewVectorQueryBuilder(
    []string{"document"},
    embeddings,
    "content_embedding",
).WithTopK(10).Build()

results, err := client.Search.VectorSearch(ctx, vectorQuery)
```

### Detailed Examples

#### 1. Complete Entity Lifecycle with Schema

```go
// Define and create a schema for a product entity
productSchema := sdk.NewEntitySchemaBuilder("product").
    AddStringProperty("name", true, true, true).           // required, indexed, searchable
    AddStringProperty("description", true, false, true).    // required, not indexed, searchable
    AddNumberProperty("price", true, true, true).          // required, indexed, sortable
    AddStringProperty("category", true, true, true).       // for faceting
    AddNumberProperty("stock", true, true, false).         // inventory tracking
    AddVectorProperty("description_embedding", 768, false). // for semantic search
    AddProperty("metadata", sdk.PropertyDefinition{
        Type:     sdk.PropertyTypeObject,
        Required: false,
    }).
    AddIndex("category_price_idx", []string{"category", "price"}, false).
    Build()

_, err := client.Schemas.CreateEntitySchema(ctx, productSchema)

// Create a product entity
product := sdk.NewEntityBuilder().
    WithURN("urn:entropic:product:laptop-001").
    WithProperty("name", "ThinkPad X1 Carbon").
    WithProperty("description", "Premium business laptop with Intel Core i7").
    WithProperty("price", 1499.99).
    WithProperty("category", "Electronics").
    WithProperty("stock", 25).
    WithProperty("metadata", map[string]interface{}{
        "brand":        "Lenovo",
        "warranty":     "3 years",
        "weight":       "2.4 lbs",
        "release_year": 2024,
    }).
    Build()

created, err := client.Entities.Create(ctx, "product", product)

// Update stock after a sale
updateReq := &sdk.EntityRequest{
    URN: created.URN,
    Properties: map[string]interface{}{
        "name":        created.Properties["name"],
        "description": created.Properties["description"],
        "price":       created.Properties["price"],
        "category":    created.Properties["category"],
        "stock":       23, // Decreased by 2
        "metadata":    created.Properties["metadata"],
    },
}

updated, err := client.Entities.Update(ctx, "product", created.ID, updateReq)
```

#### 2. Building a Social Network with Relations

```go
// Create user schema with social properties
userSchema := sdk.NewEntitySchemaBuilder("user").
    AddStringProperty("username", true, true, false).  // unique username
    AddStringProperty("display_name", true, true, true).
    AddStringProperty("bio", false, false, true).
    AddStringProperty("location", false, true, true).
    AddProperty("interests", sdk.PropertyDefinition{
        Type:      sdk.PropertyTypeArray,
        Required:  false,
        Facetable: true,
    }).
    AddIndex("username_idx", []string{"username"}, true). // unique index
    Build()

_, _ = client.Schemas.CreateEntitySchema(ctx, userSchema)

// Create relationship schemas
followsSchema := &sdk.RelationshipSchemaRequest{
    RelationshipType: "follows",
    FromEntityType:   "user",
    ToEntityType:     "user",
    Cardinality:      sdk.CardinalityManyToMany,
    Properties: sdk.PropertySchema{
        "followed_at": sdk.PropertyDefinition{
            Type:     sdk.PropertyTypeDate,
            Required: true,
        },
        "notifications_enabled": sdk.PropertyDefinition{
            Type:     sdk.PropertyTypeBoolean,
            Required: false,
        },
    },
}

_, _ = client.Schemas.CreateRelationshipSchema(ctx, followsSchema)

// Create users
alice := sdk.NewEntityBuilder().
    WithURN("urn:entropic:user:alice").
    WithProperty("username", "alice_wonder").
    WithProperty("display_name", "Alice").
    WithProperty("bio", "Software engineer and coffee enthusiast").
    WithProperty("location", "San Francisco").
    WithProperty("interests", []string{"coding", "coffee", "hiking"}).
    Build()

bob := sdk.NewEntityBuilder().
    WithURN("urn:entropic:user:bob").
    WithProperty("username", "bob_builder").
    WithProperty("display_name", "Bob").
    WithProperty("bio", "Can we fix it? Yes we can!").
    WithProperty("location", "New York").
    WithProperty("interests", []string{"construction", "tools", "DIY"}).
    Build()

aliceEntity, _ := client.Entities.Create(ctx, "user", alice)
bobEntity, _ := client.Entities.Create(ctx, "user", bob)

// Create follow relationship
followRelation := sdk.NewRelationBuilder("follows").
    From("user", aliceEntity.ID).
    To("user", bobEntity.ID).
    WithProperty("followed_at", time.Now()).
    WithProperty("notifications_enabled", true).
    Build()

_, _ = client.Relations.Create(ctx, followRelation)

// Find all users that Alice follows
relations, _ := client.Entities.GetRelations(ctx, "user", aliceEntity.ID, []string{"follows"})

// Search for users interested in coding
searchQuery := sdk.NewSearchQueryBuilder([]string{"user"}, "").
    WithFilter("interests", "coding").
    WithFacets("location", "interests").
    WithLimit(10).
    Build()

results, _ := client.Search.Search(ctx, searchQuery)
```

#### 3. Document Management with Vector Search

```go
// Schema for documents with embeddings
docSchema := sdk.NewEntitySchemaBuilder("document").
    AddStringProperty("title", true, true, true).
    AddStringProperty("content", true, false, true).
    AddStringProperty("author", true, true, true).
    AddStringProperty("department", true, true, true).
    AddProperty("tags", sdk.PropertyDefinition{
        Type:      sdk.PropertyTypeArray,
        Required:  false,
        Facetable: true,
    }).
    AddVectorProperty("content_embedding", 1536, true). // OpenAI embeddings
    AddProperty("metadata", sdk.PropertyDefinition{
        Type:     sdk.PropertyTypeObject,
        Required: false,
    }).
    Build()

_, _ = client.Schemas.CreateEntitySchema(ctx, docSchema)

// Function to create document with embedding
func createDocument(client *sdk.Client, title, content, author, dept string, embedding []float32) (*sdk.Entity, error) {
    doc := sdk.NewEntityBuilder().
        WithURN(fmt.Sprintf("urn:entropic:document:%s", sanitizeTitle(title))).
        WithProperty("title", title).
        WithProperty("content", content).
        WithProperty("author", author).
        WithProperty("department", dept).
        WithProperty("tags", extractTags(content)).
        WithProperty("content_embedding", embedding).
        WithProperty("metadata", map[string]interface{}{
            "word_count":   len(strings.Fields(content)),
            "created_date": time.Now(),
            "version":      "1.0",
        }).
        Build()
    
    return client.Entities.Create(context.Background(), "document", doc)
}

// Semantic search for similar documents
queryEmbedding := getEmbedding("How to implement authentication in microservices")

vectorQuery := sdk.NewVectorQueryBuilder(
    []string{"document"},
    queryEmbedding,
    "content_embedding",
).
    WithTopK(5).
    WithMinScore(0.75).
    WithFilter("department", "Engineering").
    Build()

similarDocs, _ := client.Search.VectorSearch(ctx, vectorQuery)

// Hybrid search combining text and vector similarity
textResults, _ := client.Search.Search(ctx, 
    sdk.NewSearchQueryBuilder([]string{"document"}, "authentication microservices").
        WithFilter("department", "Engineering").
        WithLimit(20).
        Build())

// Process and rank results using both text relevance and semantic similarity
```

#### 4. Error Handling and Retry Logic

```go
// Robust entity creation with retry
func createEntityWithRetry(client *sdk.Client, entityType string, req *sdk.EntityRequest, maxRetries int) (*sdk.Entity, error) {
    var lastErr error
    
    for i := 0; i < maxRetries; i++ {
        entity, err := client.Entities.Create(context.Background(), entityType, req)
        if err == nil {
            return entity, nil
        }
        
        // Check error type and decide whether to retry
        if apiErr, ok := sdk.AsAPIError(err); ok {
            switch {
            case apiErr.IsAlreadyExists():
                // Entity already exists, try to get it instead
                // Extract ID from URN or error details
                if existingID, ok := apiErr.Details["entity_id"].(string); ok {
                    if uid, err := sdk.ParseUUID(existingID); err == nil {
                        return client.Entities.Get(context.Background(), entityType, uid)
                    }
                }
                return nil, err // Don't retry
                
            case apiErr.IsValidation():
                // Validation errors won't be fixed by retry
                return nil, err
                
            case apiErr.IsNotFound():
                // Schema might not exist, try creating it
                if i == 0 && entityType == "custom_type" {
                    // Create a basic schema and retry
                    createDefaultSchema(client, entityType)
                    continue
                }
                return nil, err
                
            case apiErr.IsInternal():
                // Internal errors might be transient, retry with backoff
                lastErr = err
                time.Sleep(time.Duration(i+1) * time.Second)
                continue
            }
        }
        
        // Network or other errors, retry with backoff
        lastErr = err
        time.Sleep(time.Duration(i+1) * time.Second)
    }
    
    return nil, fmt.Errorf("failed after %d retries: %w", maxRetries, lastErr)
}

// Usage
entity, err := createEntityWithRetry(client, "user", userRequest, 3)
if err != nil {
    log.Printf("Failed to create entity: %v", err)
}
```

#### 5. Batch Operations and Performance

```go
// Batch entity creation with goroutines
func batchCreateEntities(client *sdk.Client, entityType string, requests []*sdk.EntityRequest) ([]*sdk.Entity, []error) {
    results := make([]*sdk.Entity, len(requests))
    errors := make([]error, len(requests))
    
    var wg sync.WaitGroup
    semaphore := make(chan struct{}, 10) // Limit concurrent requests
    
    for i, req := range requests {
        wg.Add(1)
        go func(index int, request *sdk.EntityRequest) {
            defer wg.Done()
            
            semaphore <- struct{}{}        // Acquire
            defer func() { <-semaphore }() // Release
            
            ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
            defer cancel()
            
            entity, err := client.Entities.Create(ctx, entityType, request)
            results[index] = entity
            errors[index] = err
        }(i, req)
    }
    
    wg.Wait()
    return results, errors
}

// Streaming search results with pagination
func streamSearchResults(client *sdk.Client, query *sdk.SearchQuery, processFunc func(*sdk.SearchHit) error) error {
    iterator := client.Search.NewIterator(query)
    
    for iterator.HasMore() {
        results, err := iterator.Next(context.Background())
        if err != nil {
            return fmt.Errorf("search failed: %w", err)
        }
        
        for _, hit := range results.Hits {
            if err := processFunc(&hit); err != nil {
                return fmt.Errorf("processing failed: %w", err)
            }
        }
    }
    
    return nil
}

// Usage
query := sdk.NewSearchQueryBuilder([]string{"product"}, "").
    WithFilter("category", "Electronics").
    WithSort("price", sdk.SortAsc).
    WithLimit(100).
    Build()

err := streamSearchResults(client, query, func(hit *sdk.SearchHit) error {
    fmt.Printf("Product: %s - Price: $%.2f\n", 
        hit.Properties["name"], 
        hit.Properties["price"])
    return nil
})
```

#### 6. Advanced Schema Management

```go
// Create a complex e-commerce schema with denormalization
func setupEcommerceSchema(client *sdk.Client) error {
    // Customer schema
    customerSchema := sdk.NewEntitySchemaBuilder("customer").
        AddStringProperty("email", true, true, false).
        AddStringProperty("name", true, true, true).
        AddProperty("address", sdk.PropertyDefinition{
            Type:     sdk.PropertyTypeObject,
            Required: true,
        }).
        AddProperty("preferences", sdk.PropertyDefinition{
            Type:     sdk.PropertyTypeObject,
            Required: false,
        }).
        Build()
    
    // Order schema with denormalized customer info
    orderSchema := sdk.NewEntitySchemaBuilder("order").
        AddStringProperty("order_number", true, true, false).
        AddNumberProperty("total_amount", true, true, true).
        AddStringProperty("status", true, true, true).
        AddProperty("customer_info", sdk.PropertyDefinition{
            Type:        sdk.PropertyTypeObject,
            Required:    true,
            Description: "Denormalized customer data",
        }).
        AddProperty("items", sdk.PropertyDefinition{
            Type:     sdk.PropertyTypeArray,
            Required: true,
        }).
        Build()
    
    // Create schemas
    _, err := client.Schemas.CreateEntitySchema(context.Background(), customerSchema)
    if err != nil && !isAlreadyExists(err) {
        return err
    }
    
    _, err = client.Schemas.CreateEntitySchema(context.Background(), orderSchema)
    if err != nil && !isAlreadyExists(err) {
        return err
    }
    
    // Create relationship with denormalization
    placedBySchema := &sdk.RelationshipSchemaRequest{
        RelationshipType: "placed_by",
        FromEntityType:   "order",
        ToEntityType:     "customer",
        Cardinality:      sdk.CardinalityManyToOne,
        DenormalizationConfig: sdk.DenormalizationConfig{
            DenormalizeToSource: true,
            TargetFields:        []string{"name", "email"}, // Denormalize to order
        },
    }
    
    _, err = client.Schemas.CreateRelationshipSchema(context.Background(), placedBySchema)
    return err
}
```

### SDK Structure

Located in `pkg/sdk/`:
- `client.go` - Main client with configuration options
- `entity_service.go` - Entity CRUD operations
- `relation_service.go` - Relation management
- `schema_service.go` - Schema operations
- `search_service.go` - Text and vector search
- `types.go` - Request/response types with builders
- `errors.go` - Typed error handling

### Common SDK Prompts

**Creating entities with the SDK:**
```
Show me how to use the Entropic Go SDK to create a user entity with custom properties and then establish relationships with other entities.
```

**Implementing vector search:**
```
Demonstrate using the SDK to perform vector similarity search with filters and show how to handle pagination for large result sets.
```

**Error handling patterns:**
```
Show best practices for error handling with the SDK, including checking for specific error types like NotFound or AlreadyExists.
```

**Building a social network:**
```
Help me implement a social network using the SDK with users, posts, and follow relationships. Include search functionality.
```

**E-commerce schema setup:**
```
Show how to create an e-commerce system with the SDK including products, customers, orders with proper denormalization.
```

**Batch operations:**
```
Demonstrate efficient batch creation of entities using the SDK with concurrent requests and proper error handling.
```

**Document management system:**
```
Implement a document management system with the SDK including vector embeddings for semantic search and metadata.
```

**Schema migration:**
```
Show how to safely update existing schemas using the SDK while maintaining backward compatibility.
```

**Performance optimization:**
```
Demonstrate SDK best practices for high-performance operations including connection pooling and request batching.
```

**Complex queries:**
```
Show advanced search queries using the SDK with multiple filters, facets, sorting, and hybrid text/vector search.
```

See `pkg/sdk/README.md` for complete documentation and `examples/sdk/` for working examples.

## Next Steps

After implementing the base system:
1. Add authentication and authorization
2. Implement a gRPC interface
3. Add support for additional storage backends
4. Implement data migration tools
5. Add GraphQL API support
6. Build admin UI for schema management