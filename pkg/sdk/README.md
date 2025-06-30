# Entropic Go SDK

The official Go SDK for the Entropic storage engine, providing a type-safe and idiomatic interface to all Entropic APIs.

## Installation

```bash
go get github.com/sumandas0/entropic/pkg/sdk
```

## Quick Start

```go
package main

import (
    "context"
    "log"
    
    "github.com/sumandas0/entropic/pkg/sdk"
)

func main() {
    // Create client
    client, err := sdk.NewClient("http://localhost:8080")
    if err != nil {
        log.Fatal(err)
    }
    
    ctx := context.Background()
    
    // Create an entity
    user := sdk.NewEntityBuilder().
        WithURN("urn:entropic:user:john").
        WithProperty("name", "John Doe").
        WithProperty("email", "john@example.com").
        Build()
    
    created, err := client.Entities.Create(ctx, "user", user)
    if err != nil {
        log.Fatal(err)
    }
    
    log.Printf("Created user with ID: %s", created.ID)
}
```

## Features

- **Full API Coverage**: Access to all Entropic APIs including entities, relations, schemas, and search
- **Type Safety**: Strongly typed requests and responses with compile-time validation
- **Builder Patterns**: Fluent interfaces for constructing complex requests
- **Error Handling**: Typed errors with helper methods for common error scenarios
- **Context Support**: All operations support Go contexts for cancellation and timeouts
- **Zero Dependencies**: Only uses standard library and existing project dependencies

## Services

### Entity Service

Manage entities with full CRUD operations:

```go
// Create entity
entity, err := client.Entities.Create(ctx, "user", entityRequest)

// Get entity
entity, err := client.Entities.Get(ctx, "user", entityID)

// Update entity
entity, err := client.Entities.Update(ctx, "user", entityID, updateRequest)

// Delete entity
err := client.Entities.Delete(ctx, "user", entityID)

// List entities
list, err := client.Entities.List(ctx, "user", &sdk.ListOptions{
    Limit:  20,
    Offset: 0,
})

// Get entity relations
relations, err := client.Entities.GetRelations(ctx, "user", entityID, []string{"owns"})
```

### Relation Service

Create and manage relationships between entities:

```go
// Create relation using builder
relation := sdk.NewRelationBuilder("owns").
    From("user", userID).
    To("document", docID).
    WithProperty("access_level", "owner").
    Build()

created, err := client.Relations.Create(ctx, relation)

// Get relation
relation, err := client.Relations.Get(ctx, relationID)

// Delete relation
err := client.Relations.Delete(ctx, relationID)
```

### Schema Service

Define and manage entity and relationship schemas:

```go
// Create entity schema
schema := sdk.NewEntitySchemaBuilder("user").
    AddStringProperty("name", true, true, true).      // required, indexed, searchable
    AddStringProperty("email", true, true, false).    // required, indexed, not searchable
    AddNumberProperty("age", false, true, true).      // optional, indexed, sortable
    AddVectorProperty("embedding", 384, false).       // 384-dimensional vector
    AddIndex("email_idx", []string{"email"}, true).   // unique index on email
    Build()

created, err := client.Schemas.CreateEntitySchema(ctx, schema)

// Create relationship schema
relSchema := &sdk.RelationshipSchemaRequest{
    RelationshipType: "owns",
    FromEntityType:   "user",
    ToEntityType:     "document",
    Cardinality:      sdk.CardinalityOneToMany,
}

created, err := client.Schemas.CreateRelationshipSchema(ctx, relSchema)
```

### Search Service

Perform text and vector similarity searches:

```go
// Text search with builder
query := sdk.NewSearchQueryBuilder([]string{"user", "document"}, "john").
    WithFilter("status", "active").
    WithFacets("department", "role").
    WithSort("created_at", sdk.SortDesc).
    WithLimit(20).
    Build()

results, err := client.Search.Search(ctx, query)

// Vector similarity search
vectorQuery := sdk.NewVectorQueryBuilder(
    []string{"document"},
    embeddings,           // []float32 vector
    "content_embedding",  // field name
).
    WithTopK(10).
    WithMinScore(0.7).
    WithFilter("category", "technical").
    Build()

results, err := client.Search.VectorSearch(ctx, vectorQuery)

// Pagination with iterator
iterator := client.Search.NewIterator(searchQuery)
for iterator.HasMore() {
    page, err := iterator.Next(ctx)
    if err != nil {
        break
    }
    // Process results
}
```

## Error Handling

The SDK provides typed errors with helper methods:

```go
entity, err := client.Entities.Get(ctx, "user", id)
if err != nil {
    if apiErr, ok := sdk.AsAPIError(err); ok {
        switch {
        case apiErr.IsNotFound():
            // Handle not found
        case apiErr.IsValidation():
            // Handle validation error
            log.Printf("Validation error: %s", apiErr.Message)
            log.Printf("Details: %v", apiErr.Details)
        case apiErr.IsAlreadyExists():
            // Handle already exists
        case apiErr.IsInternal():
            // Handle server error
        }
    }
    return err
}
```

## Advanced Usage

### Custom HTTP Client

```go
httpClient := &http.Client{
    Timeout: 60 * time.Second,
    Transport: &http.Transport{
        MaxIdleConns:    100,
        IdleConnTimeout: 90 * time.Second,
    },
}

client, err := sdk.NewClient("http://localhost:8080", 
    sdk.WithHTTPClient(httpClient),
)
```

### Request Timeout

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

entity, err := client.Entities.Get(ctx, "user", id)
```

### Complex Property Schemas

```go
schema := sdk.NewEntitySchemaBuilder("product").
    AddProperty("metadata", sdk.PropertyDefinition{
        Type:        sdk.PropertyTypeObject,
        Required:    true,
        Description: "Product metadata",
    }).
    AddProperty("tags", sdk.PropertyDefinition{
        Type:       sdk.PropertyTypeArray,
        Required:   false,
        Facetable:  true,
    }).
    AddProperty("location", sdk.PropertyDefinition{
        Type:     sdk.PropertyTypeGeoPoint,
        Required: false,
        Indexed:  true,
    }).
    Build()
```

## Examples

See the [examples directory](../../examples/sdk/) for complete working examples:

- [Basic Usage](../../examples/sdk/basic_usage.go) - CRUD operations and basic workflows
- [Vector Search](../../examples/sdk/vector_search.go) - Vector similarity search and hybrid search

## Testing

The SDK includes comprehensive unit tests. To run tests:

```bash
go test ./pkg/sdk/...
```

For integration tests against a real Entropic instance:

```bash
# Start Entropic services
docker-compose up -d

# Run integration tests
go test ./pkg/sdk/... -tags=integration
```

## Contributing

Contributions are welcome! Please ensure:

1. All tests pass
2. Code follows Go conventions
3. New features include tests and documentation
4. Builder patterns are used for complex types

## License

Same as the Entropic project.