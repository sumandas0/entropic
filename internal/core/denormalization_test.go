package core

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/sumandas0/entropic/internal/cache"
	"github.com/sumandas0/entropic/internal/models"
)

func TestDenormalizationManager_HandleRelationCreation(t *testing.T) {
	cleanupDatabase(t)
	ctx := context.Background()

	// Setup cache manager with real store
	cacheManager := cache.NewCacheAwareManager(primaryStore, 5*time.Minute)
	denormManager := NewDenormalizationManager(primaryStore, cacheManager.Manager)

	// Create entity schemas
	createTestEntitySchema(t, "user")
	createTestEntitySchema(t, "organization")

	// Create relationship schema with denormalization config
	relationSchema := &models.RelationshipSchema{
		ID:               uuid.New(),
		RelationshipType: "member_of",
		FromEntityType:   "user",
		ToEntityType:     "organization",
		Cardinality:      models.ManyToMany,
		Properties: map[string]models.PropertyDefinition{
			"role": {
				Type:     "string",
				Required: false,
			},
		},
		DenormalizationConfig: models.DenormalizationConfig{
			UpdateOnChange:    true,
			DenormalizeToFrom: []string{"name"}, // Copy org name to user
			DenormalizeFromTo: []string{"email"}, // Copy user email to org
			IncludeRelationData: true,
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := primaryStore.CreateRelationshipSchema(ctx, relationSchema)
	require.NoError(t, err)

	// Create entities
	user := &models.Entity{
		ID:         uuid.New(),
		EntityType: "user",
		URN:        "test:user:denorm1",
		Properties: map[string]any{
			"name":  "John Doe",
			"email": "john@example.com",
		},
		Version:   1,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = primaryStore.CreateEntity(ctx, user)
	require.NoError(t, err)

	org := &models.Entity{
		ID:         uuid.New(),
		EntityType: "organization",
		URN:        "test:org:denorm1",
		Properties: map[string]any{
			"name":        "Tech Corp",
			"description": "A technology company",
		},
		Version:   1,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = primaryStore.CreateEntity(ctx, org)
	require.NoError(t, err)

	// Create relation
	relation := &models.Relation{
		ID:             uuid.New(),
		RelationType:   "member_of",
		FromEntityID:   user.ID,
		FromEntityType: user.EntityType,
		ToEntityID:     org.ID,
		ToEntityType:   org.EntityType,
		Properties: map[string]any{
			"role": "developer",
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Handle relation creation
	err = denormManager.HandleRelationCreation(ctx, relation)
	require.NoError(t, err)

	// Verify denormalization to user (from org)
	updatedUser, err := primaryStore.GetEntity(ctx, user.EntityType, user.ID)
	require.NoError(t, err)
	assert.Equal(t, "Tech Corp", updatedUser.Properties["_denorm_member_of_name"])
	assert.NotNil(t, updatedUser.Properties["_relation_member_of_data"])

	// Verify denormalization to org (from user)
	updatedOrg, err := primaryStore.GetEntity(ctx, org.EntityType, org.ID)
	require.NoError(t, err)
	assert.Equal(t, "john@example.com", updatedOrg.Properties["_denorm_member_of_email"])
	assert.NotNil(t, updatedOrg.Properties["_relation_member_of_data"])
}

func TestDenormalizationManager_HandleRelationDeletion(t *testing.T) {
	cleanupDatabase(t)
	ctx := context.Background()

	// Setup
	cacheManager := cache.NewCacheAwareManager(primaryStore, 5*time.Minute)
	denormManager := NewDenormalizationManager(primaryStore, cacheManager.Manager)

	// Create schemas
	createTestEntitySchema(t, "user")
	createTestEntitySchema(t, "organization")

	relationSchema := &models.RelationshipSchema{
		ID:               uuid.New(),
		RelationshipType: "member_of",
		FromEntityType:   "user",
		ToEntityType:     "organization",
		Cardinality:      models.ManyToMany,
		DenormalizationConfig: models.DenormalizationConfig{
			UpdateOnChange:    true,
			DenormalizeToFrom: []string{"name"},
			DenormalizeFromTo: []string{"email"},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := primaryStore.CreateRelationshipSchema(ctx, relationSchema)
	require.NoError(t, err)

	// Create entities with denormalized data
	user := &models.Entity{
		ID:         uuid.New(),
		EntityType: "user",
		URN:        "test:user:denorm_del",
		Properties: map[string]any{
			"name":                   "Jane Doe",
			"email":                  "jane@example.com",
			"_denorm_member_of_name": "Old Corp", // Pre-existing denormalized data
		},
		Version:   1,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = primaryStore.CreateEntity(ctx, user)
	require.NoError(t, err)

	org := &models.Entity{
		ID:         uuid.New(),
		EntityType: "organization",
		URN:        "test:org:denorm_del",
		Properties: map[string]any{
			"name":                    "New Corp",
			"_denorm_member_of_email": "jane@example.com", // Pre-existing denormalized data
		},
		Version:   1,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = primaryStore.CreateEntity(ctx, org)
	require.NoError(t, err)

	// Create relation to delete
	relation := &models.Relation{
		ID:             uuid.New(),
		RelationType:   "member_of",
		FromEntityID:   user.ID,
		FromEntityType: user.EntityType,
		ToEntityID:     org.ID,
		ToEntityType:   org.EntityType,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	// Handle relation deletion
	err = denormManager.HandleRelationDeletion(ctx, relation)
	require.NoError(t, err)

	// Verify denormalized data was cleaned up from user
	updatedUser, err := primaryStore.GetEntity(ctx, user.EntityType, user.ID)
	require.NoError(t, err)
	assert.NotContains(t, updatedUser.Properties, "_denorm_member_of_name")
	assert.NotContains(t, updatedUser.Properties, "_denorm_name") // Also check alternate key format

	// Verify denormalized data was cleaned up from org
	updatedOrg, err := primaryStore.GetEntity(ctx, org.EntityType, org.ID)
	require.NoError(t, err)
	assert.NotContains(t, updatedOrg.Properties, "_denorm_member_of_email")
	assert.NotContains(t, updatedOrg.Properties, "_denorm_email") // Also check alternate key format
}

func TestDenormalizationManager_UpdateDenormalizedData(t *testing.T) {
	cleanupDatabase(t)
	ctx := context.Background()

	// Setup
	cacheManager := cache.NewCacheAwareManager(primaryStore, 5*time.Minute)
	denormManager := NewDenormalizationManager(primaryStore, cacheManager.Manager)

	// Create schemas
	createTestEntitySchema(t, "user")
	createTestEntitySchema(t, "organization")

	relationSchema := &models.RelationshipSchema{
		ID:               uuid.New(),
		RelationshipType: "member_of",
		FromEntityType:   "user",
		ToEntityType:     "organization",
		Cardinality:      models.ManyToMany,
		DenormalizationConfig: models.DenormalizationConfig{
			UpdateOnChange:    true,
			DenormalizeToFrom: []string{"name", "description"},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := primaryStore.CreateRelationshipSchema(ctx, relationSchema)
	require.NoError(t, err)

	// Create entities
	user := &models.Entity{
		ID:         uuid.New(),
		EntityType: "user",
		URN:        "test:user:update_denorm",
		Properties: map[string]any{
			"name":                        "Bob Smith",
			"email":                       "bob@example.com",
			"_denorm_member_of_name":      "Original Corp",
			"_denorm_member_of_description": "Original description",
		},
		Version:   1,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = primaryStore.CreateEntity(ctx, user)
	require.NoError(t, err)

	org := &models.Entity{
		ID:         uuid.New(),
		EntityType: "organization",
		URN:        "test:org:update_denorm",
		Properties: map[string]any{
			"name":        "Updated Corp",
			"description": "Updated description",
		},
		Version:   1,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = primaryStore.CreateEntity(ctx, org)
	require.NoError(t, err)

	// Create relation
	relation := &models.Relation{
		ID:             uuid.New(),
		RelationType:   "member_of",
		FromEntityID:   user.ID,
		FromEntityType: user.EntityType,
		ToEntityID:     org.ID,
		ToEntityType:   org.EntityType,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	err = primaryStore.CreateRelation(ctx, relation)
	require.NoError(t, err)

	// Update denormalized data
	err = denormManager.UpdateDenormalizedData(ctx, org)
	require.NoError(t, err)

	// Verify denormalized data was updated in user
	updatedUser, err := primaryStore.GetEntity(ctx, user.EntityType, user.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated Corp", updatedUser.Properties["_denorm_member_of_name"])
	assert.Equal(t, "Updated description", updatedUser.Properties["_denorm_member_of_description"])
}

func TestDenormalizationManager_BidirectionalDenormalization(t *testing.T) {
	cleanupDatabase(t)
	ctx := context.Background()

	// Setup
	cacheManager := cache.NewCacheAwareManager(primaryStore, 5*time.Minute)
	denormManager := NewDenormalizationManager(primaryStore, cacheManager.Manager)

	// Create schemas
	createTestEntitySchema(t, "user")
	createTestEntitySchema(t, "team")

	// Create bidirectional relationship schema
	relationSchema := &models.RelationshipSchema{
		ID:               uuid.New(),
		RelationshipType: "belongs_to",
		FromEntityType:   "user",
		ToEntityType:     "team",
		Cardinality:      models.ManyToMany,
		DenormalizationConfig: models.DenormalizationConfig{
			UpdateOnChange:      true,
			DenormalizeToFrom:   []string{"name", "score"}, // Team properties to user
			DenormalizeFromTo:   []string{"email", "name"}, // User properties to team
			IncludeRelationData: true,
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := primaryStore.CreateRelationshipSchema(ctx, relationSchema)
	require.NoError(t, err)

	// Create entities
	user := &models.Entity{
		ID:         uuid.New(),
		EntityType: "user",
		URN:        "test:user:bidir",
		Properties: map[string]any{
			"name":  "Alice Johnson",
			"email": "alice@example.com",
			"score": 95,
		},
		Version:   1,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = primaryStore.CreateEntity(ctx, user)
	require.NoError(t, err)

	team := &models.Entity{
		ID:         uuid.New(),
		EntityType: "team",
		URN:        "test:team:bidir",
		Properties: map[string]any{
			"name":  "Engineering Team",
			"score": 100,
		},
		Version:   1,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = primaryStore.CreateEntity(ctx, team)
	require.NoError(t, err)

	// Create relation with properties
	relation := &models.Relation{
		ID:             uuid.New(),
		RelationType:   "belongs_to",
		FromEntityID:   user.ID,
		FromEntityType: user.EntityType,
		ToEntityID:     team.ID,
		ToEntityType:   team.EntityType,
		Properties: map[string]any{
			"joined_at": time.Now().Format(time.RFC3339),
			"role":      "lead",
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Handle relation creation
	err = denormManager.HandleRelationCreation(ctx, relation)
	require.NoError(t, err)

	// Verify bidirectional denormalization
	updatedUser, err := primaryStore.GetEntity(ctx, user.EntityType, user.ID)
	require.NoError(t, err)
	assert.Equal(t, "Engineering Team", updatedUser.Properties["_denorm_belongs_to_name"])
	assert.Equal(t, float64(100), updatedUser.Properties["_denorm_belongs_to_score"])
	relationData := updatedUser.Properties["_relation_belongs_to_data"].(map[string]any)
	assert.Equal(t, "lead", relationData["role"])

	updatedTeam, err := primaryStore.GetEntity(ctx, team.EntityType, team.ID)
	require.NoError(t, err)
	assert.Equal(t, "alice@example.com", updatedTeam.Properties["_denorm_belongs_to_email"])
	assert.Equal(t, "Alice Johnson", updatedTeam.Properties["_denorm_belongs_to_name"])
	relationData = updatedTeam.Properties["_relation_belongs_to_data"].(map[string]any)
	assert.Equal(t, "lead", relationData["role"])
}

func TestDenormalizationManager_PropertyRemoval(t *testing.T) {
	cleanupDatabase(t)
	ctx := context.Background()

	// Setup
	cacheManager := cache.NewCacheAwareManager(primaryStore, 5*time.Minute)
	denormManager := NewDenormalizationManager(primaryStore, cacheManager.Manager)

	// Create schemas
	createTestEntitySchema(t, "user")
	createTestEntitySchema(t, "project")

	relationSchema := &models.RelationshipSchema{
		ID:               uuid.New(),
		RelationshipType: "works_on",
		FromEntityType:   "user",
		ToEntityType:     "project",
		Cardinality:      models.ManyToMany,
		DenormalizationConfig: models.DenormalizationConfig{
			UpdateOnChange:    true,
			DenormalizeToFrom: []string{"name", "status"},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := primaryStore.CreateRelationshipSchema(ctx, relationSchema)
	require.NoError(t, err)

	// Create entities
	user := &models.Entity{
		ID:         uuid.New(),
		EntityType: "user",
		URN:        "test:user:prop_removal",
		Properties: map[string]any{
			"name":                     "Charlie Brown",
			"_denorm_works_on_name":   "Old Project",
			"_denorm_works_on_status": "active",
		},
		Version:   1,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = primaryStore.CreateEntity(ctx, user)
	require.NoError(t, err)

	project := &models.Entity{
		ID:         uuid.New(),
		EntityType: "project",
		URN:        "test:project:prop_removal",
		Properties: map[string]any{
			"name": "New Project",
			// Note: status property is missing
		},
		Version:   1,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = primaryStore.CreateEntity(ctx, project)
	require.NoError(t, err)

	// Create relation
	relation := &models.Relation{
		ID:             uuid.New(),
		RelationType:   "works_on",
		FromEntityID:   user.ID,
		FromEntityType: user.EntityType,
		ToEntityID:     project.ID,
		ToEntityType:   project.EntityType,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	err = primaryStore.CreateRelation(ctx, relation)
	require.NoError(t, err)

	// Update denormalized data
	err = denormManager.UpdateDenormalizedData(ctx, project)
	require.NoError(t, err)

	// Verify property removal
	updatedUser, err := primaryStore.GetEntity(ctx, user.EntityType, user.ID)
	require.NoError(t, err)
	assert.Equal(t, "New Project", updatedUser.Properties["_denorm_works_on_name"])
	assert.NotContains(t, updatedUser.Properties, "_denorm_works_on_status") // Should be removed
}

func TestDenormalizationManager_NoUpdateOnChangeDisabled(t *testing.T) {
	cleanupDatabase(t)
	ctx := context.Background()

	// Setup
	cacheManager := cache.NewCacheAwareManager(primaryStore, 5*time.Minute)
	denormManager := NewDenormalizationManager(primaryStore, cacheManager.Manager)

	// Create schemas
	createTestEntitySchema(t, "user")
	createTestEntitySchema(t, "department")

	// Create relationship schema with UpdateOnChange disabled
	relationSchema := &models.RelationshipSchema{
		ID:               uuid.New(),
		RelationshipType: "reports_to",
		FromEntityType:   "user",
		ToEntityType:     "department",
		Cardinality:      models.ManyToMany,
		DenormalizationConfig: models.DenormalizationConfig{
			UpdateOnChange:    false, // Disabled
			DenormalizeToFrom: []string{"name"},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := primaryStore.CreateRelationshipSchema(ctx, relationSchema)
	require.NoError(t, err)

	// Create entities
	user := &models.Entity{
		ID:         uuid.New(),
		EntityType: "user",
		URN:        "test:user:no_update",
		Properties: map[string]any{
			"name": "David Wilson",
		},
		Version:   1,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = primaryStore.CreateEntity(ctx, user)
	require.NoError(t, err)

	dept := &models.Entity{
		ID:         uuid.New(),
		EntityType: "department",
		URN:        "test:dept:no_update",
		Properties: map[string]any{
			"name": "HR Department",
		},
		Version:   1,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = primaryStore.CreateEntity(ctx, dept)
	require.NoError(t, err)

	// Create relation
	relation := &models.Relation{
		ID:             uuid.New(),
		RelationType:   "reports_to",
		FromEntityID:   user.ID,
		FromEntityType: user.EntityType,
		ToEntityID:     dept.ID,
		ToEntityType:   dept.EntityType,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	// Handle relation creation - should do nothing
	err = denormManager.HandleRelationCreation(ctx, relation)
	require.NoError(t, err)

	// Verify no denormalization occurred
	updatedUser, err := primaryStore.GetEntity(ctx, user.EntityType, user.ID)
	require.NoError(t, err)
	assert.NotContains(t, updatedUser.Properties, "_denorm_reports_to_name")
}

func TestDenormalizationManager_MissingEntity(t *testing.T) {
	cleanupDatabase(t)
	ctx := context.Background()

	// Setup
	cacheManager := cache.NewCacheAwareManager(primaryStore, 5*time.Minute)
	denormManager := NewDenormalizationManager(primaryStore, cacheManager.Manager)

	// Create schemas
	createTestEntitySchema(t, "user")
	createTestEntitySchema(t, "company")

	relationSchema := &models.RelationshipSchema{
		ID:               uuid.New(),
		RelationshipType: "employed_by",
		FromEntityType:   "user",
		ToEntityType:     "company",
		Cardinality:      models.ManyToMany,
		DenormalizationConfig: models.DenormalizationConfig{
			UpdateOnChange:    true,
			DenormalizeToFrom: []string{"name"},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := primaryStore.CreateRelationshipSchema(ctx, relationSchema)
	require.NoError(t, err)

	// Create only the user entity
	user := &models.Entity{
		ID:         uuid.New(),
		EntityType: "user",
		URN:        "test:user:missing_entity",
		Properties: map[string]any{
			"name": "Eve Anderson",
		},
		Version:   1,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = primaryStore.CreateEntity(ctx, user)
	require.NoError(t, err)

	// Create relation with non-existent company
	relation := &models.Relation{
		ID:             uuid.New(),
		RelationType:   "employed_by",
		FromEntityID:   user.ID,
		FromEntityType: user.EntityType,
		ToEntityID:     uuid.New(), // Non-existent entity
		ToEntityType:   "company",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	// Handle relation creation - should fail gracefully
	err = denormManager.HandleRelationCreation(ctx, relation)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get to entity")
}

func TestDenormalizationManager_LargePropertySets(t *testing.T) {
	cleanupDatabase(t)
	ctx := context.Background()

	// Setup
	cacheManager := cache.NewCacheAwareManager(primaryStore, 5*time.Minute)
	denormManager := NewDenormalizationManager(primaryStore, cacheManager.Manager)

	// Create schemas
	createTestEntitySchema(t, "product")
	createTestEntitySchema(t, "category")

	// Create relationship with many properties to denormalize
	manyProps := []string{"name", "description", "price", "weight", "color", "brand", "model", "sku"}
	relationSchema := &models.RelationshipSchema{
		ID:               uuid.New(),
		RelationshipType: "belongs_to_category",
		FromEntityType:   "product",
		ToEntityType:     "category",
		Cardinality:      models.ManyToMany,
		DenormalizationConfig: models.DenormalizationConfig{
			UpdateOnChange:    true,
			DenormalizeToFrom: manyProps,
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := primaryStore.CreateRelationshipSchema(ctx, relationSchema)
	require.NoError(t, err)

	// Create entities with large property sets
	product := &models.Entity{
		ID:         uuid.New(),
		EntityType: "product",
		URN:        "test:product:large",
		Properties: map[string]any{
			"name": "Super Widget",
		},
		Version:   1,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = primaryStore.CreateEntity(ctx, product)
	require.NoError(t, err)

	categoryProps := make(map[string]any)
	for _, prop := range manyProps {
		categoryProps[prop] = "value_" + prop
	}
	category := &models.Entity{
		ID:         uuid.New(),
		EntityType: "category",
		URN:        "test:category:large",
		Properties: categoryProps,
		Version:    1,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	err = primaryStore.CreateEntity(ctx, category)
	require.NoError(t, err)

	// Create relation
	relation := &models.Relation{
		ID:             uuid.New(),
		RelationType:   "belongs_to_category",
		FromEntityID:   product.ID,
		FromEntityType: product.EntityType,
		ToEntityID:     category.ID,
		ToEntityType:   category.EntityType,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	// Handle relation creation
	err = denormManager.HandleRelationCreation(ctx, relation)
	require.NoError(t, err)

	// Verify all properties were denormalized
	updatedProduct, err := primaryStore.GetEntity(ctx, product.EntityType, product.ID)
	require.NoError(t, err)
	for _, prop := range manyProps {
		denormKey := "_denorm_belongs_to_category_" + prop
		assert.Equal(t, "value_"+prop, updatedProduct.Properties[denormKey])
	}
}

func BenchmarkDenormalizationManager_HandleRelationCreation(b *testing.B) {
	ctx := context.Background()
	cacheManager := cache.NewCacheAwareManager(primaryStore, 5*time.Minute)
	denormManager := NewDenormalizationManager(primaryStore, cacheManager.Manager)

	// Setup schemas once
	userSchema := &models.EntitySchema{
		ID:         uuid.New(),
		EntityType: "bench_user",
		Properties: map[string]models.PropertyDefinition{
			"name":  {Type: "string", Required: true},
			"email": {Type: "string", Required: false},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	primaryStore.CreateEntitySchema(ctx, userSchema)

	orgSchema := &models.EntitySchema{
		ID:         uuid.New(),
		EntityType: "bench_org",
		Properties: map[string]models.PropertyDefinition{
			"name": {Type: "string", Required: true},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	primaryStore.CreateEntitySchema(ctx, orgSchema)

	relationSchema := &models.RelationshipSchema{
		ID:               uuid.New(),
		RelationshipType: "bench_member_of",
		FromEntityType:   "bench_user",
		ToEntityType:     "bench_org",
		Cardinality:      models.ManyToMany,
		DenormalizationConfig: models.DenormalizationConfig{
			UpdateOnChange:    true,
			DenormalizeToFrom: []string{"name"},
			DenormalizeFromTo: []string{"email"},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	primaryStore.CreateRelationshipSchema(ctx, relationSchema)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Create entities
		user := &models.Entity{
			ID:         uuid.New(),
			EntityType: "bench_user",
			URN:        "test:bench_user:" + uuid.New().String(),
			Properties: map[string]any{
				"name":  "Bench User",
				"email": "bench@example.com",
			},
			Version: 1,
		}
		primaryStore.CreateEntity(ctx, user)

		org := &models.Entity{
			ID:         uuid.New(),
			EntityType: "bench_org",
			URN:        "test:bench_org:" + uuid.New().String(),
			Properties: map[string]any{
				"name": "Bench Org",
			},
			Version: 1,
		}
		primaryStore.CreateEntity(ctx, org)

		// Create relation
		relation := &models.Relation{
			ID:             uuid.New(),
			RelationType:   "bench_member_of",
			FromEntityID:   user.ID,
			FromEntityType: user.EntityType,
			ToEntityID:     org.ID,
			ToEntityType:   org.EntityType,
		}

		// Benchmark denormalization
		denormManager.HandleRelationCreation(ctx, relation)
	}
}