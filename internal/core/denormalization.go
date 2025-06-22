package core

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/sumandas0/entropic/internal/cache"
	"github.com/sumandas0/entropic/internal/models"
	"github.com/sumandas0/entropic/internal/store"
)

type DenormalizationManager struct {
	primaryStore store.PrimaryStore
	cacheManager *cache.Manager
}

func NewDenormalizationManager(primaryStore store.PrimaryStore, cacheManager *cache.Manager) *DenormalizationManager {
	return &DenormalizationManager{
		primaryStore: primaryStore,
		cacheManager: cacheManager,
	}
}

func (dm *DenormalizationManager) HandleRelationCreation(ctx context.Context, relation *models.Relation) error {
	
	schema, err := dm.cacheManager.GetRelationshipSchema(ctx, relation.RelationType)
	if err != nil {
		
		return nil
	}

	if !schema.DenormalizationConfig.UpdateOnChange {
		return nil 
	}

	if len(schema.DenormalizationConfig.DenormalizeToFrom) > 0 {
		if err := dm.denormalizeToFrom(ctx, relation, schema); err != nil {
			return fmt.Errorf("failed to denormalize to->from: %w", err)
		}
	}

	if len(schema.DenormalizationConfig.DenormalizeFromTo) > 0 {
		if err := dm.denormalizeFromTo(ctx, relation, schema); err != nil {
			return fmt.Errorf("failed to denormalize from->to: %w", err)
		}
	}

	return nil
}

func (dm *DenormalizationManager) HandleRelationDeletion(ctx context.Context, relation *models.Relation) error {
	
	schema, err := dm.cacheManager.GetRelationshipSchema(ctx, relation.RelationType)
	if err != nil {
		
		return nil
	}

	if !schema.DenormalizationConfig.UpdateOnChange {
		return nil 
	}

	if len(schema.DenormalizationConfig.DenormalizeToFrom) > 0 {
		if err := dm.cleanupDenormalizedData(ctx, relation.FromEntityType, relation.FromEntityID, schema.DenormalizationConfig.DenormalizeToFrom); err != nil {
			return fmt.Errorf("failed to cleanup denormalized data in from entity: %w", err)
		}
	}

	if len(schema.DenormalizationConfig.DenormalizeFromTo) > 0 {
		if err := dm.cleanupDenormalizedData(ctx, relation.ToEntityType, relation.ToEntityID, schema.DenormalizationConfig.DenormalizeFromTo); err != nil {
			return fmt.Errorf("failed to cleanup denormalized data in to entity: %w", err)
		}
	}

	return nil
}

func (dm *DenormalizationManager) UpdateDenormalizedData(ctx context.Context, entity *models.Entity) error {
	
	relations, err := dm.primaryStore.GetRelationsByEntity(ctx, entity.ID, nil)
	if err != nil {
		return fmt.Errorf("failed to get relations for entity: %w", err)
	}

	for _, relation := range relations {
		if err := dm.updateRelationDenormalization(ctx, relation, entity); err != nil {
			
			fmt.Printf("Warning: failed to update denormalization for relation %s: %v\n", relation.ID, err)
		}
	}

	return nil
}

func (dm *DenormalizationManager) denormalizeToFrom(ctx context.Context, relation *models.Relation, schema *models.RelationshipSchema) error {
	
	toEntity, err := dm.primaryStore.GetEntity(ctx, relation.ToEntityType, relation.ToEntityID)
	if err != nil {
		return fmt.Errorf("failed to get to entity: %w", err)
	}

	fromEntity, err := dm.primaryStore.GetEntity(ctx, relation.FromEntityType, relation.FromEntityID)
	if err != nil {
		return fmt.Errorf("failed to get from entity: %w", err)
	}

	updated := false
	for _, propName := range schema.DenormalizationConfig.DenormalizeToFrom {
		if value, exists := toEntity.Properties[propName]; exists {
			denormKey := dm.buildDenormalizationKey(relation.RelationType, propName)
			fromEntity.Properties[denormKey] = value
			updated = true
		}
	}

	if schema.DenormalizationConfig.IncludeRelationData && relation.Properties != nil {
		relationKey := dm.buildRelationDataKey(relation.RelationType)
		fromEntity.Properties[relationKey] = relation.Properties
		updated = true
	}

	if updated {
		return dm.primaryStore.UpdateEntity(ctx, fromEntity)
	}

	return nil
}

func (dm *DenormalizationManager) denormalizeFromTo(ctx context.Context, relation *models.Relation, schema *models.RelationshipSchema) error {
	
	fromEntity, err := dm.primaryStore.GetEntity(ctx, relation.FromEntityType, relation.FromEntityID)
	if err != nil {
		return fmt.Errorf("failed to get from entity: %w", err)
	}

	toEntity, err := dm.primaryStore.GetEntity(ctx, relation.ToEntityType, relation.ToEntityID)
	if err != nil {
		return fmt.Errorf("failed to get to entity: %w", err)
	}

	updated := false
	for _, propName := range schema.DenormalizationConfig.DenormalizeFromTo {
		if value, exists := fromEntity.Properties[propName]; exists {
			denormKey := dm.buildDenormalizationKey(relation.RelationType, propName)
			toEntity.Properties[denormKey] = value
			updated = true
		}
	}

	if schema.DenormalizationConfig.IncludeRelationData && relation.Properties != nil {
		relationKey := dm.buildRelationDataKey(relation.RelationType)
		toEntity.Properties[relationKey] = relation.Properties
		updated = true
	}

	if updated {
		return dm.primaryStore.UpdateEntity(ctx, toEntity)
	}

	return nil
}

func (dm *DenormalizationManager) updateRelationDenormalization(ctx context.Context, relation *models.Relation, changedEntity *models.Entity) error {
	
	schema, err := dm.cacheManager.GetRelationshipSchema(ctx, relation.RelationType)
	if err != nil {
		return nil 
	}

	if !schema.DenormalizationConfig.UpdateOnChange {
		return nil 
	}

	switch changedEntity.ID {
	case relation.FromEntityID:
		
		if len(schema.DenormalizationConfig.DenormalizeFromTo) > 0 {
			return dm.updateDenormalizedDataInTarget(ctx, relation.ToEntityType, relation.ToEntityID, changedEntity, relation.RelationType, schema.DenormalizationConfig.DenormalizeFromTo)
		}
	case relation.ToEntityID:
		
		if len(schema.DenormalizationConfig.DenormalizeToFrom) > 0 {
			return dm.updateDenormalizedDataInTarget(ctx, relation.FromEntityType, relation.FromEntityID, changedEntity, relation.RelationType, schema.DenormalizationConfig.DenormalizeToFrom)
		}
	}

	return nil
}

func (dm *DenormalizationManager) updateDenormalizedDataInTarget(ctx context.Context, targetEntityType string, targetEntityID uuid.UUID, sourceEntity *models.Entity, relationType string, propertiesToCopy []string) error {
	
	targetEntity, err := dm.primaryStore.GetEntity(ctx, targetEntityType, targetEntityID)
	if err != nil {
		return fmt.Errorf("failed to get target entity: %w", err)
	}

	updated := false
	for _, propName := range propertiesToCopy {
		denormKey := dm.buildDenormalizationKey(relationType, propName)
		if value, exists := sourceEntity.Properties[propName]; exists {
			targetEntity.Properties[denormKey] = value
			updated = true
		} else {
			
			if _, exists := targetEntity.Properties[denormKey]; exists {
				delete(targetEntity.Properties, denormKey)
				updated = true
			}
		}
	}

	if updated {
		return dm.primaryStore.UpdateEntity(ctx, targetEntity)
	}

	return nil
}

func (dm *DenormalizationManager) cleanupDenormalizedData(ctx context.Context, entityType string, entityID uuid.UUID, propertiesToCleanup []string) error {
	
	entity, err := dm.primaryStore.GetEntity(ctx, entityType, entityID)
	if err != nil {
		return fmt.Errorf("failed to get entity for cleanup: %w", err)
	}

	updated := false
	for _, propName := range propertiesToCleanup {
		
		keysToTry := []string{
			dm.buildDenormalizationKey("", propName), 
			propName,                                 
		}

		for _, key := range keysToTry {
			if _, exists := entity.Properties[key]; exists {
				delete(entity.Properties, key)
				updated = true
			}
		}
	}

	if updated {
		return dm.primaryStore.UpdateEntity(ctx, entity)
	}

	return nil
}

func (dm *DenormalizationManager) buildDenormalizationKey(relationType, propName string) string {
	if relationType == "" {
		return "_denorm_" + propName
	}
	return "_denorm_" + relationType + "_" + propName
}

func (dm *DenormalizationManager) buildRelationDataKey(relationType string) string {
	return "_relation_" + relationType + "_data"
}

func (dm *DenormalizationManager) RebuildDenormalization(ctx context.Context, relationshipType string) error {
	
	schema, err := dm.cacheManager.GetRelationshipSchema(ctx, relationshipType)
	if err != nil {
		return fmt.Errorf("failed to get relationship schema: %w", err)
	}

	if !schema.DenormalizationConfig.UpdateOnChange {
		return nil 
	}

	return fmt.Errorf("rebuild denormalization not yet implemented - would require relation iteration support")
}

func (dm *DenormalizationManager) ValidateDenormalization(ctx context.Context, relationshipType string) error {
	
	schema, err := dm.cacheManager.GetRelationshipSchema(ctx, relationshipType)
	if err != nil {
		return fmt.Errorf("failed to get relationship schema: %w", err)
	}

	if !schema.DenormalizationConfig.UpdateOnChange {
		return nil 
	}

	return fmt.Errorf("validate denormalization not yet implemented - would require relation iteration support")
}

type DenormalizationStats struct {
	TotalDenormalizations  uint64
	FailedDenormalizations uint64
	AverageProcessingTime  float64
}

func (dm *DenormalizationManager) GetDenormalizationStats() DenormalizationStats {
	
	return DenormalizationStats{
		TotalDenormalizations:  0,
		FailedDenormalizations: 0,
		AverageProcessingTime:  0.0,
	}
}
