package core

import (
	"context"
	"fmt"

	"github.com/sumandas0/entropic/internal/cache"
	"github.com/sumandas0/entropic/internal/models"
	"github.com/sumandas0/entropic/internal/store"
	"github.com/google/uuid"
)

// DenormalizationManager handles denormalization of relationship data into entities
type DenormalizationManager struct {
	primaryStore store.PrimaryStore
	cacheManager *cache.Manager
}

// NewDenormalizationManager creates a new denormalization manager
func NewDenormalizationManager(primaryStore store.PrimaryStore, cacheManager *cache.Manager) *DenormalizationManager {
	return &DenormalizationManager{
		primaryStore: primaryStore,
		cacheManager: cacheManager,
	}
}

// HandleRelationCreation handles denormalization when a relation is created
func (dm *DenormalizationManager) HandleRelationCreation(ctx context.Context, relation *models.Relation) error {
	// Get relationship schema
	schema, err := dm.cacheManager.GetRelationshipSchema(ctx, relation.RelationType)
	if err != nil {
		// If schema doesn't exist or doesn't require denormalization, skip
		return nil
	}
	
	if !schema.DenormalizationConfig.UpdateOnChange {
		return nil // Denormalization is disabled
	}
	
	// Denormalize from 'to' entity to 'from' entity
	if len(schema.DenormalizationConfig.DenormalizeToFrom) > 0 {
		if err := dm.denormalizeToFrom(ctx, relation, schema); err != nil {
			return fmt.Errorf("failed to denormalize to->from: %w", err)
		}
	}
	
	// Denormalize from 'from' entity to 'to' entity
	if len(schema.DenormalizationConfig.DenormalizeFromTo) > 0 {
		if err := dm.denormalizeFromTo(ctx, relation, schema); err != nil {
			return fmt.Errorf("failed to denormalize from->to: %w", err)
		}
	}
	
	return nil
}

// HandleRelationDeletion handles denormalization cleanup when a relation is deleted
func (dm *DenormalizationManager) HandleRelationDeletion(ctx context.Context, relation *models.Relation) error {
	// Get relationship schema
	schema, err := dm.cacheManager.GetRelationshipSchema(ctx, relation.RelationType)
	if err != nil {
		// If schema doesn't exist, skip cleanup
		return nil
	}
	
	if !schema.DenormalizationConfig.UpdateOnChange {
		return nil // Denormalization is disabled
	}
	
	// Clean up denormalized data
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

// UpdateDenormalizedData updates denormalized data when an entity changes
func (dm *DenormalizationManager) UpdateDenormalizedData(ctx context.Context, entity *models.Entity) error {
	// Find all relationships involving this entity
	relations, err := dm.primaryStore.GetRelationsByEntity(ctx, entity.ID, nil)
	if err != nil {
		return fmt.Errorf("failed to get relations for entity: %w", err)
	}
	
	// Update denormalized data for each relation
	for _, relation := range relations {
		if err := dm.updateRelationDenormalization(ctx, relation, entity); err != nil {
			// Log error but continue with other relations
			fmt.Printf("Warning: failed to update denormalization for relation %s: %v\n", relation.ID, err)
		}
	}
	
	return nil
}

// denormalizeToFrom copies properties from 'to' entity to 'from' entity
func (dm *DenormalizationManager) denormalizeToFrom(ctx context.Context, relation *models.Relation, schema *models.RelationshipSchema) error {
	// Get the 'to' entity
	toEntity, err := dm.primaryStore.GetEntity(ctx, relation.ToEntityType, relation.ToEntityID)
	if err != nil {
		return fmt.Errorf("failed to get to entity: %w", err)
	}
	
	// Get the 'from' entity
	fromEntity, err := dm.primaryStore.GetEntity(ctx, relation.FromEntityType, relation.FromEntityID)
	if err != nil {
		return fmt.Errorf("failed to get from entity: %w", err)
	}
	
	// Copy specified properties
	updated := false
	for _, propName := range schema.DenormalizationConfig.DenormalizeToFrom {
		if value, exists := toEntity.Properties[propName]; exists {
			denormKey := dm.buildDenormalizationKey(relation.RelationType, propName)
			fromEntity.Properties[denormKey] = value
			updated = true
		}
	}
	
	// Include relation data if configured
	if schema.DenormalizationConfig.IncludeRelationData && relation.Properties != nil {
		relationKey := dm.buildRelationDataKey(relation.RelationType)
		fromEntity.Properties[relationKey] = relation.Properties
		updated = true
	}
	
	// Update the entity if changes were made
	if updated {
		return dm.primaryStore.UpdateEntity(ctx, fromEntity)
	}
	
	return nil
}

// denormalizeFromTo copies properties from 'from' entity to 'to' entity
func (dm *DenormalizationManager) denormalizeFromTo(ctx context.Context, relation *models.Relation, schema *models.RelationshipSchema) error {
	// Get the 'from' entity
	fromEntity, err := dm.primaryStore.GetEntity(ctx, relation.FromEntityType, relation.FromEntityID)
	if err != nil {
		return fmt.Errorf("failed to get from entity: %w", err)
	}
	
	// Get the 'to' entity
	toEntity, err := dm.primaryStore.GetEntity(ctx, relation.ToEntityType, relation.ToEntityID)
	if err != nil {
		return fmt.Errorf("failed to get to entity: %w", err)
	}
	
	// Copy specified properties
	updated := false
	for _, propName := range schema.DenormalizationConfig.DenormalizeFromTo {
		if value, exists := fromEntity.Properties[propName]; exists {
			denormKey := dm.buildDenormalizationKey(relation.RelationType, propName)
			toEntity.Properties[denormKey] = value
			updated = true
		}
	}
	
	// Include relation data if configured
	if schema.DenormalizationConfig.IncludeRelationData && relation.Properties != nil {
		relationKey := dm.buildRelationDataKey(relation.RelationType)
		toEntity.Properties[relationKey] = relation.Properties
		updated = true
	}
	
	// Update the entity if changes were made
	if updated {
		return dm.primaryStore.UpdateEntity(ctx, toEntity)
	}
	
	return nil
}

// updateRelationDenormalization updates denormalized data for a specific relation when an entity changes
func (dm *DenormalizationManager) updateRelationDenormalization(ctx context.Context, relation *models.Relation, changedEntity *models.Entity) error {
	// Get relationship schema
	schema, err := dm.cacheManager.GetRelationshipSchema(ctx, relation.RelationType)
	if err != nil {
		return nil // Skip if schema doesn't exist
	}
	
	if !schema.DenormalizationConfig.UpdateOnChange {
		return nil // Denormalization is disabled
	}
	
	// Determine which direction to update based on which entity changed
	if changedEntity.ID == relation.FromEntityID {
		// From entity changed, update denormalized data in to entity
		if len(schema.DenormalizationConfig.DenormalizeFromTo) > 0 {
			return dm.updateDenormalizedDataInTarget(ctx, relation.ToEntityType, relation.ToEntityID, changedEntity, relation.RelationType, schema.DenormalizationConfig.DenormalizeFromTo)
		}
	} else if changedEntity.ID == relation.ToEntityID {
		// To entity changed, update denormalized data in from entity
		if len(schema.DenormalizationConfig.DenormalizeToFrom) > 0 {
			return dm.updateDenormalizedDataInTarget(ctx, relation.FromEntityType, relation.FromEntityID, changedEntity, relation.RelationType, schema.DenormalizationConfig.DenormalizeToFrom)
		}
	}
	
	return nil
}

// updateDenormalizedDataInTarget updates denormalized data in the target entity
func (dm *DenormalizationManager) updateDenormalizedDataInTarget(ctx context.Context, targetEntityType string, targetEntityID uuid.UUID, sourceEntity *models.Entity, relationType string, propertiesToCopy []string) error {
	// Get the target entity
	targetEntity, err := dm.primaryStore.GetEntity(ctx, targetEntityType, targetEntityID)
	if err != nil {
		return fmt.Errorf("failed to get target entity: %w", err)
	}
	
	// Update denormalized properties
	updated := false
	for _, propName := range propertiesToCopy {
		denormKey := dm.buildDenormalizationKey(relationType, propName)
		if value, exists := sourceEntity.Properties[propName]; exists {
			targetEntity.Properties[denormKey] = value
			updated = true
		} else {
			// Remove the denormalized property if it no longer exists
			if _, exists := targetEntity.Properties[denormKey]; exists {
				delete(targetEntity.Properties, denormKey)
				updated = true
			}
		}
	}
	
	// Update the target entity if changes were made
	if updated {
		return dm.primaryStore.UpdateEntity(ctx, targetEntity)
	}
	
	return nil
}

// cleanupDenormalizedData removes denormalized data from an entity
func (dm *DenormalizationManager) cleanupDenormalizedData(ctx context.Context, entityType string, entityID uuid.UUID, propertiesToCleanup []string) error {
	// Get the entity
	entity, err := dm.primaryStore.GetEntity(ctx, entityType, entityID)
	if err != nil {
		return fmt.Errorf("failed to get entity for cleanup: %w", err)
	}
	
	// Remove denormalized properties
	updated := false
	for _, propName := range propertiesToCleanup {
		// Try different possible denormalization keys
		keysToTry := []string{
			dm.buildDenormalizationKey("", propName), // Generic key
			propName, // Direct property name
		}
		
		for _, key := range keysToTry {
			if _, exists := entity.Properties[key]; exists {
				delete(entity.Properties, key)
				updated = true
			}
		}
	}
	
	// Update the entity if changes were made
	if updated {
		return dm.primaryStore.UpdateEntity(ctx, entity)
	}
	
	return nil
}

// buildDenormalizationKey builds a key for denormalized properties
func (dm *DenormalizationManager) buildDenormalizationKey(relationType, propName string) string {
	if relationType == "" {
		return "_denorm_" + propName
	}
	return "_denorm_" + relationType + "_" + propName
}

// buildRelationDataKey builds a key for relation data storage
func (dm *DenormalizationManager) buildRelationDataKey(relationType string) string {
	return "_relation_" + relationType + "_data"
}

// RebuildDenormalization rebuilds all denormalized data for a specific relationship type
func (dm *DenormalizationManager) RebuildDenormalization(ctx context.Context, relationshipType string) error {
	// Get relationship schema
	schema, err := dm.cacheManager.GetRelationshipSchema(ctx, relationshipType)
	if err != nil {
		return fmt.Errorf("failed to get relationship schema: %w", err)
	}
	
	if !schema.DenormalizationConfig.UpdateOnChange {
		return nil // Denormalization is disabled
	}
	
	// This would require implementing a way to iterate through all relations of a type
	// For now, we'll return a placeholder error
	return fmt.Errorf("rebuild denormalization not yet implemented - would require relation iteration support")
}

// ValidateDenormalization validates that denormalized data is consistent
func (dm *DenormalizationManager) ValidateDenormalization(ctx context.Context, relationshipType string) error {
	// Get relationship schema
	schema, err := dm.cacheManager.GetRelationshipSchema(ctx, relationshipType)
	if err != nil {
		return fmt.Errorf("failed to get relationship schema: %w", err)
	}
	
	if !schema.DenormalizationConfig.UpdateOnChange {
		return nil // Denormalization is disabled
	}
	
	// This would require implementing validation logic to check consistency
	// For now, we'll return a placeholder error
	return fmt.Errorf("validate denormalization not yet implemented - would require relation iteration support")
}

// DenormalizationStats holds statistics about denormalization operations
type DenormalizationStats struct {
	TotalDenormalizations uint64
	FailedDenormalizations uint64
	AverageProcessingTime  float64
}

// GetDenormalizationStats returns denormalization statistics
func (dm *DenormalizationManager) GetDenormalizationStats() DenormalizationStats {
	// This would be implemented with actual tracking
	return DenormalizationStats{
		TotalDenormalizations:  0,
		FailedDenormalizations: 0,
		AverageProcessingTime:  0.0,
	}
}