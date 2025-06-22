package core

import (
	"context"
	"fmt"
	"time"

	"github.com/sumandas0/entropic/internal/cache"
	"github.com/sumandas0/entropic/internal/lock"
	"github.com/sumandas0/entropic/internal/models"
	"github.com/sumandas0/entropic/internal/store"
	"github.com/sumandas0/entropic/pkg/utils"
	"github.com/google/uuid"
)

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

func (e *Engine) CreateEntity(ctx context.Context, entity *models.Entity) error {
	if entity.CreatedAt.IsZero() {
		entity.CreatedAt = time.Now()
	}
	if entity.UpdatedAt.IsZero() {
		entity.UpdatedAt = entity.CreatedAt
	}
	if entity.Version == 0 {
		entity.Version = 1
	}
	
	if err := e.validator.ValidateEntity(ctx, entity); err != nil {
		return err
	}
	
	return e.txManager.ExecuteWithTimeout(ctx, func(ctx context.Context, txCtx *TransactionContext) error {
		if err := e.txCoordinator.CreateEntity(ctx, txCtx, entity); err != nil {
			return err
		}
		
		if err := e.ensureIndexCollection(ctx, entity.EntityType); err != nil {
			fmt.Printf("Warning: failed to ensure index collection for %s: %v\n", entity.EntityType, err)
		}
		
		return nil
	})
}

func (e *Engine) GetEntity(ctx context.Context, entityType string, id uuid.UUID) (*models.Entity, error) {
	return e.primaryStore.GetEntity(ctx, entityType, id)
}

func (e *Engine) UpdateEntity(ctx context.Context, entity *models.Entity) error {
	entity.UpdatedAt = time.Now()
	entity.Version++
	
	if err := e.validator.ValidateEntity(ctx, entity); err != nil {
		return err
	}
	
	return e.txManager.ExecuteWithTimeout(ctx, func(ctx context.Context, txCtx *TransactionContext) error {
		if err := e.txCoordinator.UpdateEntity(ctx, txCtx, entity); err != nil {
			return err
		}
		
		if err := e.denormalizationMgr.UpdateDenormalizedData(ctx, entity); err != nil {
			
			fmt.Printf("Warning: failed to update denormalized data for entity %s: %v\n", entity.ID, err)
		}
		
		return nil
	})
}

func (e *Engine) DeleteEntity(ctx context.Context, entityType string, id uuid.UUID) error {
	return e.txManager.ExecuteWithTimeout(ctx, func(ctx context.Context, txCtx *TransactionContext) error {
		relations, err := e.primaryStore.GetRelationsByEntity(ctx, id, nil)
		if err != nil {
			return err
		}
		
		for _, relation := range relations {
			if err := e.txCoordinator.DeleteRelation(ctx, txCtx, relation.ID); err != nil {
				return fmt.Errorf("failed to delete relation %s: %w", relation.ID, err)
			}
		}
		
		return e.txCoordinator.DeleteEntity(ctx, txCtx, entityType, id)
	})
}

func (e *Engine) ListEntities(ctx context.Context, entityType string, limit, offset int) ([]*models.Entity, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	
	return e.primaryStore.ListEntities(ctx, entityType, limit, offset)
}

func (e *Engine) CreateRelation(ctx context.Context, relation *models.Relation) error {
	if relation.CreatedAt.IsZero() {
		relation.CreatedAt = time.Now()
	}
	if relation.UpdatedAt.IsZero() {
		relation.UpdatedAt = relation.CreatedAt
	}
	
	if err := e.validator.ValidateRelation(ctx, relation); err != nil {
		return err
	}
	
	return e.txManager.ExecuteWithTimeout(ctx, func(ctx context.Context, txCtx *TransactionContext) error {
		if err := e.txCoordinator.CreateRelation(ctx, txCtx, relation); err != nil {
			return err
		}
		
		if err := e.denormalizationMgr.HandleRelationCreation(ctx, relation); err != nil {
			
			fmt.Printf("Warning: failed to handle denormalization for relation %s: %v\n", relation.ID, err)
		}
		
		return nil
	})
}

func (e *Engine) GetRelation(ctx context.Context, id uuid.UUID) (*models.Relation, error) {
	return e.primaryStore.GetRelation(ctx, id)
}

func (e *Engine) DeleteRelation(ctx context.Context, id uuid.UUID) error {
	return e.txManager.ExecuteWithTimeout(ctx, func(ctx context.Context, txCtx *TransactionContext) error {
		relation, err := e.primaryStore.GetRelation(ctx, id)
		if err != nil {
			return err
		}
		
		if err := e.txCoordinator.DeleteRelation(ctx, txCtx, id); err != nil {
			return err
		}
		
		if err := e.denormalizationMgr.HandleRelationDeletion(ctx, relation); err != nil {
			fmt.Printf("Warning: failed to handle denormalization cleanup for relation %s: %v\n", id, err)
		}
		
		return nil
	})
}

func (e *Engine) GetRelationsByEntity(ctx context.Context, entityID uuid.UUID, relationTypes []string) ([]*models.Relation, error) {
	return e.primaryStore.GetRelationsByEntity(ctx, entityID, relationTypes)
}

func (e *Engine) CreateEntitySchema(ctx context.Context, schema *models.EntitySchema) error {
	if schema.CreatedAt.IsZero() {
		schema.CreatedAt = time.Now()
	}
	if schema.UpdatedAt.IsZero() {
		schema.UpdatedAt = schema.CreatedAt
	}
	if schema.Version == 0 {
		schema.Version = 1
	}
	
	if err := e.validator.ValidateEntitySchema(schema); err != nil {
		return err
	}
	
	return e.lockManager.WithSchemaLock(ctx, "entity", schema.EntityType, 30*time.Second, func() error {
		if err := e.primaryStore.CreateEntitySchema(ctx, schema); err != nil {
			return err
		}
		
		if err := e.indexStore.CreateCollection(ctx, schema.EntityType, schema); err != nil {
			fmt.Printf("Warning: failed to create index collection for %s: %v\n", schema.EntityType, err)
		}
		
		e.cacheManager.SetEntitySchema(schema.EntityType, schema)
		
		e.cacheManager.GetNotifier().NotifyEntitySchemaChange(schema.EntityType, "create")
		
		return nil
	})
}

func (e *Engine) GetEntitySchema(ctx context.Context, entityType string) (*models.EntitySchema, error) {
	return e.cacheManager.GetEntitySchema(ctx, entityType)
}

func (e *Engine) UpdateEntitySchema(ctx context.Context, schema *models.EntitySchema) error {
	schema.UpdatedAt = time.Now()
	schema.Version++
	
	if err := e.validator.ValidateEntitySchema(schema); err != nil {
		return err
	}
	
	return e.lockManager.WithSchemaLock(ctx, "entity", schema.EntityType, 30*time.Second, func() error {
		if err := e.primaryStore.UpdateEntitySchema(ctx, schema); err != nil {
			return err
		}
		
		if err := e.indexStore.UpdateCollection(ctx, schema.EntityType, schema); err != nil {
			
			fmt.Printf("Warning: failed to update index collection for %s: %v\n", schema.EntityType, err)
		}
		
		e.cacheManager.SetEntitySchema(schema.EntityType, schema)
		
		e.cacheManager.GetNotifier().NotifyEntitySchemaChange(schema.EntityType, "update")
		
		return nil
	})
}

func (e *Engine) DeleteEntitySchema(ctx context.Context, entityType string) error {
	return e.lockManager.WithSchemaLock(ctx, "entity", entityType, 30*time.Second, func() error {
		entities, err := e.primaryStore.ListEntities(ctx, entityType, 1, 0)
		if err != nil {
			return err
		}
		
		if len(entities) > 0 {
			return utils.NewAppError(utils.CodeValidation, "cannot delete schema with existing entities", nil).
				WithDetail("entity_type", entityType).
				WithDetail("entity_count", len(entities))
		}
		
		if err := e.primaryStore.DeleteEntitySchema(ctx, entityType); err != nil {
			return err
		}
		
		if err := e.indexStore.DeleteCollection(ctx, entityType); err != nil {
			
			fmt.Printf("Warning: failed to delete index collection for %s: %v\n", entityType, err)
		}
		
		e.cacheManager.InvalidateEntitySchema(entityType)
		
		e.cacheManager.GetNotifier().NotifyEntitySchemaChange(entityType, "delete")
		
		return nil
	})
}

func (e *Engine) ListEntitySchemas(ctx context.Context) ([]*models.EntitySchema, error) {
	return e.primaryStore.ListEntitySchemas(ctx)
}

func (e *Engine) CreateRelationshipSchema(ctx context.Context, schema *models.RelationshipSchema) error {
	if schema.CreatedAt.IsZero() {
		schema.CreatedAt = time.Now()
	}
	if schema.UpdatedAt.IsZero() {
		schema.UpdatedAt = schema.CreatedAt
	}
	if schema.Version == 0 {
		schema.Version = 1
	}
	
	if err := e.validator.ValidateRelationshipSchema(schema); err != nil {
		return err
	}
	
	return e.lockManager.WithSchemaLock(ctx, "relationship", schema.RelationshipType, 30*time.Second, func() error {
		if err := e.primaryStore.CreateRelationshipSchema(ctx, schema); err != nil {
			return err
		}
		
		e.cacheManager.SetRelationshipSchema(schema.RelationshipType, schema)
		
		e.cacheManager.GetNotifier().NotifyRelationshipSchemaChange(schema.RelationshipType, "create")
		
		return nil
	})
}

func (e *Engine) GetRelationshipSchema(ctx context.Context, relationshipType string) (*models.RelationshipSchema, error) {
	return e.cacheManager.GetRelationshipSchema(ctx, relationshipType)
}

func (e *Engine) UpdateRelationshipSchema(ctx context.Context, schema *models.RelationshipSchema) error {
	schema.UpdatedAt = time.Now()
	schema.Version++
	
	if err := e.validator.ValidateRelationshipSchema(schema); err != nil {
		return err
	}
	
	return e.lockManager.WithSchemaLock(ctx, "relationship", schema.RelationshipType, 30*time.Second, func() error {
		if err := e.primaryStore.UpdateRelationshipSchema(ctx, schema); err != nil {
			return err
		}
		
		e.cacheManager.SetRelationshipSchema(schema.RelationshipType, schema)
		
		e.cacheManager.GetNotifier().NotifyRelationshipSchemaChange(schema.RelationshipType, "update")
		
		return nil
	})
}

func (e *Engine) DeleteRelationshipSchema(ctx context.Context, relationshipType string) error {
	return e.lockManager.WithSchemaLock(ctx, "relationship", relationshipType, 30*time.Second, func() error {
		if err := e.primaryStore.DeleteRelationshipSchema(ctx, relationshipType); err != nil {
			return err
		}
		
		e.cacheManager.InvalidateRelationshipSchema(relationshipType)
		
		e.cacheManager.GetNotifier().NotifyRelationshipSchemaChange(relationshipType, "delete")
		
		return nil
	})
}

func (e *Engine) ListRelationshipSchemas(ctx context.Context) ([]*models.RelationshipSchema, error) {
	return e.primaryStore.ListRelationshipSchemas(ctx)
}

func (e *Engine) Search(ctx context.Context, query *models.SearchQuery) (*models.SearchResult, error) {
	return e.indexStore.Search(ctx, query)
}

func (e *Engine) VectorSearch(ctx context.Context, query *models.VectorQuery) (*models.SearchResult, error) {
	return e.indexStore.VectorSearch(ctx, query)
}

func (e *Engine) HealthCheck(ctx context.Context) error {
	if err := e.primaryStore.Ping(ctx); err != nil {
		return fmt.Errorf("primary store health check failed: %w", err)
	}
	
	if err := e.indexStore.Ping(ctx); err != nil {
		return fmt.Errorf("index store health check failed: %w", err)
	}
	
	return nil
}

func (e *Engine) GetStats() EngineStats {
	cacheStats := e.cacheManager.Stats()
	txStats := e.txManager.GetStats()
	
	return EngineStats{
		CacheStats:      cacheStats,
		TransactionStats: txStats,
	}
}

type EngineStats struct {
	CacheStats       cache.CacheStats
	TransactionStats TransactionStats
}

func (e *Engine) ensureIndexCollection(ctx context.Context, entityType string) error {
	schema, err := e.cacheManager.GetEntitySchema(ctx, entityType)
	if err != nil {
		return nil
	}
	
	return e.indexStore.CreateCollection(ctx, entityType, schema)
}

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