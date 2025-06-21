package core

import (
	"context"
	"fmt"
	"time"

	"github.com/entropic/entropic/internal/cache"
	"github.com/entropic/entropic/internal/lock"
	"github.com/entropic/entropic/internal/models"
	"github.com/entropic/entropic/internal/store"
	"github.com/entropic/entropic/pkg/utils"
	"github.com/google/uuid"
)

// Engine is the core business logic engine that orchestrates all operations
type Engine struct {
	primaryStore        store.PrimaryStore
	indexStore          store.IndexStore
	cacheManager        *cache.CacheAwareManager
	lockManager         *lock.LockManager
	validator           *Validator
	txCoordinator       *TransactionCoordinator
	txManager           *TransactionManager
	denormalizationMgr  *DenormalizationManager
}

// NewEngine creates a new engine instance
func NewEngine(
	primaryStore store.PrimaryStore,
	indexStore store.IndexStore,
	cacheManager *cache.CacheAwareManager,
	lockManager *lock.LockManager,
) (*Engine, error) {
	
	validator := NewValidator(cacheManager.Manager, primaryStore)
	txCoordinator := NewTransactionCoordinator(primaryStore, indexStore, lockManager)
	txManager := NewTransactionManager(txCoordinator, 30*time.Second)
	denormalizationMgr := NewDenormalizationManager(primaryStore, cacheManager.Manager)
	
	engine := &Engine{
		primaryStore:        primaryStore,
		indexStore:          indexStore,
		cacheManager:        cacheManager,
		lockManager:         lockManager,
		validator:           validator,
		txCoordinator:       txCoordinator,
		txManager:           txManager,
		denormalizationMgr:  denormalizationMgr,
	}
	
	return engine, nil
}

// Entity Operations

// CreateEntity creates a new entity with validation and indexing
func (e *Engine) CreateEntity(ctx context.Context, entity *models.Entity) error {
	// Set timestamps and version if not set
	if entity.CreatedAt.IsZero() {
		entity.CreatedAt = time.Now()
	}
	if entity.UpdatedAt.IsZero() {
		entity.UpdatedAt = entity.CreatedAt
	}
	if entity.Version == 0 {
		entity.Version = 1
	}
	
	// Validate entity
	if err := e.validator.ValidateEntity(ctx, entity); err != nil {
		return err
	}
	
	// Execute in transaction
	return e.txManager.ExecuteWithTimeout(ctx, func(ctx context.Context, txCtx *TransactionContext) error {
		// Create entity
		if err := e.txCoordinator.CreateEntity(ctx, txCtx, entity); err != nil {
			return err
		}
		
		// Handle schema-based collection creation in index store
		if err := e.ensureIndexCollection(ctx, entity.EntityType); err != nil {
			// Log warning but don't fail the transaction
			fmt.Printf("Warning: failed to ensure index collection for %s: %v\n", entity.EntityType, err)
		}
		
		return nil
	})
}

// GetEntity retrieves an entity by type and ID
func (e *Engine) GetEntity(ctx context.Context, entityType string, id uuid.UUID) (*models.Entity, error) {
	return e.primaryStore.GetEntity(ctx, entityType, id)
}

// UpdateEntity updates an existing entity
func (e *Engine) UpdateEntity(ctx context.Context, entity *models.Entity) error {
	// Update timestamp and version
	entity.UpdatedAt = time.Now()
	entity.Version++
	
	// Validate entity
	if err := e.validator.ValidateEntity(ctx, entity); err != nil {
		return err
	}
	
	// Execute in transaction
	return e.txManager.ExecuteWithTimeout(ctx, func(ctx context.Context, txCtx *TransactionContext) error {
		// Update entity
		if err := e.txCoordinator.UpdateEntity(ctx, txCtx, entity); err != nil {
			return err
		}
		
		// Handle denormalization updates
		if err := e.denormalizationMgr.UpdateDenormalizedData(ctx, entity); err != nil {
			// Log warning but don't fail the transaction
			fmt.Printf("Warning: failed to update denormalized data for entity %s: %v\n", entity.ID, err)
		}
		
		return nil
	})
}

// DeleteEntity deletes an entity
func (e *Engine) DeleteEntity(ctx context.Context, entityType string, id uuid.UUID) error {
	// Execute in transaction
	return e.txManager.ExecuteWithTimeout(ctx, func(ctx context.Context, txCtx *TransactionContext) error {
		// Check for existing relations
		relations, err := e.primaryStore.GetRelationsByEntity(ctx, id, nil)
		if err != nil {
			return err
		}
		
		// Delete all relations involving this entity
		for _, relation := range relations {
			if err := e.txCoordinator.DeleteRelation(ctx, txCtx, relation.ID); err != nil {
				return fmt.Errorf("failed to delete relation %s: %w", relation.ID, err)
			}
		}
		
		// Delete the entity
		return e.txCoordinator.DeleteEntity(ctx, txCtx, entityType, id)
	})
}

// ListEntities lists entities of a given type with pagination
func (e *Engine) ListEntities(ctx context.Context, entityType string, limit, offset int) ([]*models.Entity, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100 // Default limit
	}
	if offset < 0 {
		offset = 0
	}
	
	return e.primaryStore.ListEntities(ctx, entityType, limit, offset)
}

// Relation Operations

// CreateRelation creates a new relation with validation
func (e *Engine) CreateRelation(ctx context.Context, relation *models.Relation) error {
	// Set timestamps if not set
	if relation.CreatedAt.IsZero() {
		relation.CreatedAt = time.Now()
	}
	if relation.UpdatedAt.IsZero() {
		relation.UpdatedAt = relation.CreatedAt
	}
	
	// Validate relation
	if err := e.validator.ValidateRelation(ctx, relation); err != nil {
		return err
	}
	
	// Execute in transaction
	return e.txManager.ExecuteWithTimeout(ctx, func(ctx context.Context, txCtx *TransactionContext) error {
		// Create relation
		if err := e.txCoordinator.CreateRelation(ctx, txCtx, relation); err != nil {
			return err
		}
		
		// Handle denormalization
		if err := e.denormalizationMgr.HandleRelationCreation(ctx, relation); err != nil {
			// Log warning but don't fail the transaction
			fmt.Printf("Warning: failed to handle denormalization for relation %s: %v\n", relation.ID, err)
		}
		
		return nil
	})
}

// GetRelation retrieves a relation by ID
func (e *Engine) GetRelation(ctx context.Context, id uuid.UUID) (*models.Relation, error) {
	return e.primaryStore.GetRelation(ctx, id)
}

// DeleteRelation deletes a relation
func (e *Engine) DeleteRelation(ctx context.Context, id uuid.UUID) error {
	// Execute in transaction
	return e.txManager.ExecuteWithTimeout(ctx, func(ctx context.Context, txCtx *TransactionContext) error {
		// Get relation for denormalization cleanup
		relation, err := e.primaryStore.GetRelation(ctx, id)
		if err != nil {
			return err
		}
		
		// Delete relation
		if err := e.txCoordinator.DeleteRelation(ctx, txCtx, id); err != nil {
			return err
		}
		
		// Handle denormalization cleanup
		if err := e.denormalizationMgr.HandleRelationDeletion(ctx, relation); err != nil {
			// Log warning but don't fail the transaction
			fmt.Printf("Warning: failed to handle denormalization cleanup for relation %s: %v\n", id, err)
		}
		
		return nil
	})
}

// GetRelationsByEntity retrieves relations for an entity
func (e *Engine) GetRelationsByEntity(ctx context.Context, entityID uuid.UUID, relationTypes []string) ([]*models.Relation, error) {
	return e.primaryStore.GetRelationsByEntity(ctx, entityID, relationTypes)
}

// Schema Operations

// CreateEntitySchema creates a new entity schema
func (e *Engine) CreateEntitySchema(ctx context.Context, schema *models.EntitySchema) error {
	// Set timestamps and version if not set
	if schema.CreatedAt.IsZero() {
		schema.CreatedAt = time.Now()
	}
	if schema.UpdatedAt.IsZero() {
		schema.UpdatedAt = schema.CreatedAt
	}
	if schema.Version == 0 {
		schema.Version = 1
	}
	
	// Validate schema
	if err := e.validator.ValidateEntitySchema(schema); err != nil {
		return err
	}
	
	// Acquire schema lock
	return e.lockManager.WithSchemaLock(ctx, "entity", schema.EntityType, 30*time.Second, func() error {
		// Create schema in primary store
		if err := e.primaryStore.CreateEntitySchema(ctx, schema); err != nil {
			return err
		}
		
		// Create collection in index store
		if err := e.indexStore.CreateCollection(ctx, schema.EntityType, schema); err != nil {
			// Log warning but don't fail the operation
			fmt.Printf("Warning: failed to create index collection for %s: %v\n", schema.EntityType, err)
		}
		
		// Update cache
		e.cacheManager.SetEntitySchema(schema.EntityType, schema)
		
		// Notify listeners
		e.cacheManager.GetNotifier().NotifyEntitySchemaChange(schema.EntityType, "create")
		
		return nil
	})
}

// GetEntitySchema retrieves an entity schema
func (e *Engine) GetEntitySchema(ctx context.Context, entityType string) (*models.EntitySchema, error) {
	return e.cacheManager.GetEntitySchema(ctx, entityType)
}

// UpdateEntitySchema updates an entity schema
func (e *Engine) UpdateEntitySchema(ctx context.Context, schema *models.EntitySchema) error {
	// Update timestamp and version
	schema.UpdatedAt = time.Now()
	schema.Version++
	
	// Validate schema
	if err := e.validator.ValidateEntitySchema(schema); err != nil {
		return err
	}
	
	// Acquire schema lock
	return e.lockManager.WithSchemaLock(ctx, "entity", schema.EntityType, 30*time.Second, func() error {
		// Update schema in primary store
		if err := e.primaryStore.UpdateEntitySchema(ctx, schema); err != nil {
			return err
		}
		
		// Update collection in index store
		if err := e.indexStore.UpdateCollection(ctx, schema.EntityType, schema); err != nil {
			// Log warning but don't fail the operation
			fmt.Printf("Warning: failed to update index collection for %s: %v\n", schema.EntityType, err)
		}
		
		// Update cache
		e.cacheManager.SetEntitySchema(schema.EntityType, schema)
		
		// Notify listeners
		e.cacheManager.GetNotifier().NotifyEntitySchemaChange(schema.EntityType, "update")
		
		return nil
	})
}

// DeleteEntitySchema deletes an entity schema
func (e *Engine) DeleteEntitySchema(ctx context.Context, entityType string) error {
	// Acquire schema lock
	return e.lockManager.WithSchemaLock(ctx, "entity", entityType, 30*time.Second, func() error {
		// Check if there are existing entities of this type
		entities, err := e.primaryStore.ListEntities(ctx, entityType, 1, 0)
		if err != nil {
			return err
		}
		
		if len(entities) > 0 {
			return utils.NewAppError(utils.CodeValidation, "cannot delete schema with existing entities", nil).
				WithDetail("entity_type", entityType).
				WithDetail("entity_count", len(entities))
		}
		
		// Delete schema from primary store
		if err := e.primaryStore.DeleteEntitySchema(ctx, entityType); err != nil {
			return err
		}
		
		// Delete collection from index store
		if err := e.indexStore.DeleteCollection(ctx, entityType); err != nil {
			// Log warning but don't fail the operation
			fmt.Printf("Warning: failed to delete index collection for %s: %v\n", entityType, err)
		}
		
		// Invalidate cache
		e.cacheManager.InvalidateEntitySchema(entityType)
		
		// Notify listeners
		e.cacheManager.GetNotifier().NotifyEntitySchemaChange(entityType, "delete")
		
		return nil
	})
}

// ListEntitySchemas lists all entity schemas
func (e *Engine) ListEntitySchemas(ctx context.Context) ([]*models.EntitySchema, error) {
	return e.primaryStore.ListEntitySchemas(ctx)
}

// CreateRelationshipSchema creates a new relationship schema
func (e *Engine) CreateRelationshipSchema(ctx context.Context, schema *models.RelationshipSchema) error {
	// Set timestamps and version if not set
	if schema.CreatedAt.IsZero() {
		schema.CreatedAt = time.Now()
	}
	if schema.UpdatedAt.IsZero() {
		schema.UpdatedAt = schema.CreatedAt
	}
	if schema.Version == 0 {
		schema.Version = 1
	}
	
	// Validate schema
	if err := e.validator.ValidateRelationshipSchema(schema); err != nil {
		return err
	}
	
	// Acquire schema lock
	return e.lockManager.WithSchemaLock(ctx, "relationship", schema.RelationshipType, 30*time.Second, func() error {
		// Create schema in primary store
		if err := e.primaryStore.CreateRelationshipSchema(ctx, schema); err != nil {
			return err
		}
		
		// Update cache
		e.cacheManager.SetRelationshipSchema(schema.RelationshipType, schema)
		
		// Notify listeners
		e.cacheManager.GetNotifier().NotifyRelationshipSchemaChange(schema.RelationshipType, "create")
		
		return nil
	})
}

// GetRelationshipSchema retrieves a relationship schema
func (e *Engine) GetRelationshipSchema(ctx context.Context, relationshipType string) (*models.RelationshipSchema, error) {
	return e.cacheManager.GetRelationshipSchema(ctx, relationshipType)
}

// UpdateRelationshipSchema updates a relationship schema
func (e *Engine) UpdateRelationshipSchema(ctx context.Context, schema *models.RelationshipSchema) error {
	// Update timestamp and version
	schema.UpdatedAt = time.Now()
	schema.Version++
	
	// Validate schema
	if err := e.validator.ValidateRelationshipSchema(schema); err != nil {
		return err
	}
	
	// Acquire schema lock
	return e.lockManager.WithSchemaLock(ctx, "relationship", schema.RelationshipType, 30*time.Second, func() error {
		// Update schema in primary store
		if err := e.primaryStore.UpdateRelationshipSchema(ctx, schema); err != nil {
			return err
		}
		
		// Update cache
		e.cacheManager.SetRelationshipSchema(schema.RelationshipType, schema)
		
		// Notify listeners
		e.cacheManager.GetNotifier().NotifyRelationshipSchemaChange(schema.RelationshipType, "update")
		
		return nil
	})
}

// DeleteRelationshipSchema deletes a relationship schema
func (e *Engine) DeleteRelationshipSchema(ctx context.Context, relationshipType string) error {
	// Acquire schema lock
	return e.lockManager.WithSchemaLock(ctx, "relationship", relationshipType, 30*time.Second, func() error {
		// Delete schema from primary store
		if err := e.primaryStore.DeleteRelationshipSchema(ctx, relationshipType); err != nil {
			return err
		}
		
		// Invalidate cache
		e.cacheManager.InvalidateRelationshipSchema(relationshipType)
		
		// Notify listeners
		e.cacheManager.GetNotifier().NotifyRelationshipSchemaChange(relationshipType, "delete")
		
		return nil
	})
}

// ListRelationshipSchemas lists all relationship schemas
func (e *Engine) ListRelationshipSchemas(ctx context.Context) ([]*models.RelationshipSchema, error) {
	return e.primaryStore.ListRelationshipSchemas(ctx)
}

// Search Operations

// Search performs text search across entities
func (e *Engine) Search(ctx context.Context, query *models.SearchQuery) (*models.SearchResult, error) {
	return e.indexStore.Search(ctx, query)
}

// VectorSearch performs vector similarity search
func (e *Engine) VectorSearch(ctx context.Context, query *models.VectorQuery) (*models.SearchResult, error) {
	return e.indexStore.VectorSearch(ctx, query)
}

// Health and Statistics

// HealthCheck performs a comprehensive health check
func (e *Engine) HealthCheck(ctx context.Context) error {
	// Check primary store
	if err := e.primaryStore.Ping(ctx); err != nil {
		return fmt.Errorf("primary store health check failed: %w", err)
	}
	
	// Check index store
	if err := e.indexStore.Ping(ctx); err != nil {
		return fmt.Errorf("index store health check failed: %w", err)
	}
	
	return nil
}

// GetStats returns engine statistics
func (e *Engine) GetStats() EngineStats {
	cacheStats := e.cacheManager.Stats()
	txStats := e.txManager.GetStats()
	
	return EngineStats{
		CacheStats:      cacheStats,
		TransactionStats: txStats,
	}
}

// EngineStats holds engine statistics
type EngineStats struct {
	CacheStats       cache.CacheStats
	TransactionStats TransactionStats
}

// ensureIndexCollection ensures that an index collection exists for the entity type
func (e *Engine) ensureIndexCollection(ctx context.Context, entityType string) error {
	// Get entity schema
	schema, err := e.cacheManager.GetEntitySchema(ctx, entityType)
	if err != nil {
		// If schema doesn't exist, we can't create the collection
		return nil
	}
	
	// Create collection (this is idempotent in most implementations)
	return e.indexStore.CreateCollection(ctx, entityType, schema)
}

// Close gracefully shuts down the engine
func (e *Engine) Close() error {
	var errors []error
	
	if err := e.primaryStore.Close(); err != nil {
		errors = append(errors, fmt.Errorf("primary store close failed: %w", err))
	}
	
	if err := e.indexStore.Close(); err != nil {
		errors = append(errors, fmt.Errorf("index store close failed: %w", err))
	}
	
	if err := e.lockManager.Close(); err != nil {
		errors = append(errors, fmt.Errorf("lock manager close failed: %w", err))
	}
	
	if len(errors) > 0 {
		return fmt.Errorf("engine close failed with %d errors: %v", len(errors), errors[0])
	}
	
	return nil
}