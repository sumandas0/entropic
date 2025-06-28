package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/sumandas0/entropic/internal/models"
)

type PrimaryStore interface {
	CreateEntity(ctx context.Context, entity *models.Entity) error
	GetEntity(ctx context.Context, entityType string, id uuid.UUID) (*models.Entity, error)
	UpdateEntity(ctx context.Context, entity *models.Entity) error
	DeleteEntity(ctx context.Context, entityType string, id uuid.UUID) error
	CheckURNExists(ctx context.Context, urn string) (bool, error)
	ListEntities(ctx context.Context, entityType string, limit, offset int) ([]*models.Entity, error)

	CreateRelation(ctx context.Context, relation *models.Relation) error
	GetRelation(ctx context.Context, id uuid.UUID) (*models.Relation, error)
	DeleteRelation(ctx context.Context, id uuid.UUID) error
	GetRelationsByEntity(ctx context.Context, entityID uuid.UUID, relationTypes []string) ([]*models.Relation, error)

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

	BeginTx(ctx context.Context) (Transaction, error)

	Ping(ctx context.Context) error
	Close() error
}

type IndexStore interface {
	IndexEntity(ctx context.Context, entity *models.Entity) error
	UpdateEntityIndex(ctx context.Context, entity *models.Entity) error
	DeleteEntityIndex(ctx context.Context, entityType string, id uuid.UUID) error

	Search(ctx context.Context, query *models.SearchQuery) (*models.SearchResult, error)
	VectorSearch(ctx context.Context, query *models.VectorQuery) (*models.SearchResult, error)

	CreateCollection(ctx context.Context, entityType string, schema *models.EntitySchema) error
	UpdateCollection(ctx context.Context, entityType string, schema *models.EntitySchema) error
	DeleteCollection(ctx context.Context, entityType string) error

	Ping(ctx context.Context) error
	Close() error
}

type Transaction interface {
	CreateEntity(ctx context.Context, entity *models.Entity) error
	UpdateEntity(ctx context.Context, entity *models.Entity) error
	DeleteEntity(ctx context.Context, entityType string, id uuid.UUID) error

	CreateRelation(ctx context.Context, relation *models.Relation) error
	DeleteRelation(ctx context.Context, id uuid.UUID) error

	Commit() error
	Rollback() error
}

type Cache interface {
	Get(key string) (any, bool)
	Set(key string, value any, ttl time.Duration)
	Delete(key string)
	Clear()
}

type Lock interface {
	Lock(resource string) error
	Unlock(resource string) error
	TryLock(resource string, timeout time.Duration) error
}
