package store

import (
	"context"
	"time"

	"github.com/entropic/entropic/internal/models"
	"github.com/google/uuid"
)

// PrimaryStore defines the interface for the primary storage backend
type PrimaryStore interface {
	// Entity operations
	CreateEntity(ctx context.Context, entity *models.Entity) error
	GetEntity(ctx context.Context, entityType string, id uuid.UUID) (*models.Entity, error)
	UpdateEntity(ctx context.Context, entity *models.Entity) error
	DeleteEntity(ctx context.Context, entityType string, id uuid.UUID) error
	CheckURNExists(ctx context.Context, urn string) (bool, error)
	ListEntities(ctx context.Context, entityType string, limit, offset int) ([]*models.Entity, error)

	// Relation operations
	CreateRelation(ctx context.Context, relation *models.Relation) error
	GetRelation(ctx context.Context, id uuid.UUID) (*models.Relation, error)
	DeleteRelation(ctx context.Context, id uuid.UUID) error
	GetRelationsByEntity(ctx context.Context, entityID uuid.UUID, relationTypes []string) ([]*models.Relation, error)

	// Schema operations
	CreateEntitySchema(ctx context.Context, schema *models.EntitySchema) error
	GetEntitySchema(ctx context.Context, entityType string) (*models.EntitySchema, error)
	UpdateEntitySchema(ctx context.Context, schema *models.EntitySchema) error
	DeleteEntitySchema(ctx context.Context, entityType string) error
	ListEntitySchemas(ctx context.Context) ([]*models.EntitySchema, error)

	CreateRelationshipSchema(ctx context.Context, schema *models.RelationshipSchema) error
	GetRelationshipSchema(ctx context.Context, relationshipType string) (*models.RelationshipSchema, error)
	UpdateRelationshipSchema(ctx context.Context, schema *models.RelationshipSchema) error
	DeleteRelationshipSchema(ctx context.Context, relationshipType string) error
	ListRelationshipSchemas(ctx context.Context) ([]*models.RelationshipSchema, error)

	// Transaction support
	BeginTx(ctx context.Context) (Transaction, error)

	// Health check
	Ping(ctx context.Context) error
	Close() error
}

// IndexStore defines the interface for the search and indexing backend
type IndexStore interface {
	// Document operations
	IndexEntity(ctx context.Context, entity *models.Entity) error
	UpdateEntityIndex(ctx context.Context, entity *models.Entity) error
	DeleteEntityIndex(ctx context.Context, entityType string, id uuid.UUID) error
	
	// Search operations
	Search(ctx context.Context, query *models.SearchQuery) (*models.SearchResult, error)
	VectorSearch(ctx context.Context, query *models.VectorQuery) (*models.SearchResult, error)
	
	// Collection management
	CreateCollection(ctx context.Context, entityType string, schema *models.EntitySchema) error
	UpdateCollection(ctx context.Context, entityType string, schema *models.EntitySchema) error
	DeleteCollection(ctx context.Context, entityType string) error
	
	// Health check
	Ping(ctx context.Context) error
	Close() error
}

// Transaction defines the interface for database transactions
type Transaction interface {
	// Entity operations within transaction
	CreateEntity(ctx context.Context, entity *models.Entity) error
	UpdateEntity(ctx context.Context, entity *models.Entity) error
	DeleteEntity(ctx context.Context, entityType string, id uuid.UUID) error
	
	// Relation operations within transaction
	CreateRelation(ctx context.Context, relation *models.Relation) error
	DeleteRelation(ctx context.Context, id uuid.UUID) error
	
	// Transaction control
	Commit() error
	Rollback() error
}

// Cache defines the interface for caching
type Cache interface {
	Get(key string) (interface{}, bool)
	Set(key string, value interface{}, ttl time.Duration)
	Delete(key string)
	Clear()
}

// Lock defines the interface for distributed locking
type Lock interface {
	Lock(resource string) error
	Unlock(resource string) error
	TryLock(resource string, timeout time.Duration) error
}