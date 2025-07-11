package core_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/sumandas0/entropic/internal/models"
	"github.com/sumandas0/entropic/tests/testhelpers"
)

func TestEngine_CreateEntitySchema(t *testing.T) {
	ctx := context.Background()
	env := testhelpers.SetupTestEnvironment(t, ctx)
	defer env.Cleanup(ctx)

	tests := []struct {
		name      string
		schema    *models.EntitySchema
		wantError bool
	}{
		{
			name:      "valid schema",
			schema:    testhelpers.CreateTestEntitySchema("user"),
			wantError: false,
		},
		{
			name:      "duplicate schema",
			schema:    testhelpers.CreateTestEntitySchema("user"),
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := env.Engine.CreateEntitySchema(ctx, tt.schema)
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				retrieved, err := env.Engine.GetEntitySchema(ctx, tt.schema.EntityType)
				require.NoError(t, err)
				assert.Equal(t, tt.schema.EntityType, retrieved.EntityType)
				assert.Equal(t, len(tt.schema.Properties), len(retrieved.Properties))
			}
		})
	}
}

func TestEngine_CreateEntity(t *testing.T) {
	ctx := context.Background()
	env := testhelpers.SetupTestEnvironment(t, ctx)
	defer env.Cleanup(ctx)

	schema := testhelpers.CreateTestEntitySchema("user")
	err := env.Engine.CreateEntitySchema(ctx, schema)
	require.NoError(t, err)

	tests := []struct {
		name      string
		entity    *models.Entity
		wantError bool
	}{
		{
			name:      "valid entity",
			entity:    testhelpers.CreateTestEntity("user", "test-user"),
			wantError: false,
		},
		{
			name: "entity without schema",
			entity: &models.Entity{
				ID:         uuid.New(),
				EntityType: "unknown_type",
				URN:        "test:unknown:entity",
				Properties: map[string]any{"name": "test"},
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := env.Engine.CreateEntity(ctx, tt.entity)
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				retrieved, err := env.Engine.GetEntity(ctx, tt.entity.EntityType, tt.entity.ID)
				require.NoError(t, err)
				testhelpers.AssertEntityEqual(t, tt.entity, retrieved)

				testhelpers.WaitForIndexing(t, ctx, env.IndexStore, 2*time.Second)
			}
		})
	}
}

func TestEngine_UpdateEntity(t *testing.T) {
	ctx := context.Background()
	env := testhelpers.SetupTestEnvironment(t, ctx)
	defer env.Cleanup(ctx)

	schema := testhelpers.CreateTestEntitySchema("user")
	err := env.Engine.CreateEntitySchema(ctx, schema)
	require.NoError(t, err)

	original := testhelpers.CreateTestEntity("user", "test-user")
	err = env.Engine.CreateEntity(ctx, original)
	require.NoError(t, err)

	updated := &models.Entity{
		ID:         original.ID,
		EntityType: original.EntityType,
		URN:        original.URN,
		Properties: map[string]any{
			"name":        "Updated Name",
			"description": "Updated description",
			"tags":        []string{"updated", "test"},
		},
		Version: original.Version,
	}

	err = env.Engine.UpdateEntity(ctx, updated)
	require.NoError(t, err)

	retrieved, err := env.Engine.GetEntity(ctx, updated.EntityType, updated.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated Name", retrieved.Properties["name"])
	assert.Equal(t, "Updated description", retrieved.Properties["description"])
	assert.Greater(t, retrieved.Version, original.Version)
}

func TestEngine_DeleteEntity(t *testing.T) {
	ctx := context.Background()
	env := testhelpers.SetupTestEnvironment(t, ctx)
	defer env.Cleanup(ctx)

	schema := testhelpers.CreateTestEntitySchema("user")
	err := env.Engine.CreateEntitySchema(ctx, schema)
	require.NoError(t, err)

	entity := testhelpers.CreateTestEntity("user", "test-user")
	err = env.Engine.CreateEntity(ctx, entity)
	require.NoError(t, err)

	err = env.Engine.DeleteEntity(ctx, entity.EntityType, entity.ID)
	require.NoError(t, err)

	_, err = env.Engine.GetEntity(ctx, entity.EntityType, entity.ID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestEngine_CreateRelationshipSchema(t *testing.T) {
	ctx := context.Background()
	env := testhelpers.SetupTestEnvironment(t, ctx)
	defer env.Cleanup(ctx)

	userSchema := testhelpers.CreateTestEntitySchema("user")
	err := env.Engine.CreateEntitySchema(ctx, userSchema)
	require.NoError(t, err)

	orgSchema := testhelpers.CreateTestEntitySchema("organization")
	err = env.Engine.CreateEntitySchema(ctx, orgSchema)
	require.NoError(t, err)

	relationSchema := testhelpers.CreateTestRelationshipSchema("member_of", "user", "organization")
	err = env.Engine.CreateRelationshipSchema(ctx, relationSchema)
	require.NoError(t, err)

	retrieved, err := env.Engine.GetRelationshipSchema(ctx, relationSchema.RelationshipType)
	require.NoError(t, err)
	assert.Equal(t, relationSchema.RelationshipType, retrieved.RelationshipType)
	assert.Equal(t, relationSchema.FromEntityType, retrieved.FromEntityType)
	assert.Equal(t, relationSchema.ToEntityType, retrieved.ToEntityType)
}

func TestEngine_CreateRelation(t *testing.T) {
	ctx := context.Background()
	env := testhelpers.SetupTestEnvironment(t, ctx)
	defer env.Cleanup(ctx)

	userSchema := testhelpers.CreateTestEntitySchema("user")
	err := env.Engine.CreateEntitySchema(ctx, userSchema)
	require.NoError(t, err)

	orgSchema := testhelpers.CreateTestEntitySchema("organization")
	err = env.Engine.CreateEntitySchema(ctx, orgSchema)
	require.NoError(t, err)

	relationSchema := testhelpers.CreateTestRelationshipSchema("member_of", "user", "organization")
	err = env.Engine.CreateRelationshipSchema(ctx, relationSchema)
	require.NoError(t, err)

	user := testhelpers.CreateTestEntity("user", "test-user")
	err = env.Engine.CreateEntity(ctx, user)
	require.NoError(t, err)

	org := testhelpers.CreateTestEntity("organization", "test-org")
	err = env.Engine.CreateEntity(ctx, org)
	require.NoError(t, err)

	relation := testhelpers.CreateTestRelation("member_of", user, org)
	err = env.Engine.CreateRelation(ctx, relation)
	require.NoError(t, err)

	retrieved, err := env.Engine.GetRelation(ctx, relation.ID)
	require.NoError(t, err)
	testhelpers.AssertRelationEqual(t, relation, retrieved)
}

func TestEngine_SearchEntities(t *testing.T) {
	ctx := context.Background()
	env := testhelpers.SetupTestEnvironment(t, ctx)
	defer env.Cleanup(ctx)

	schema := testhelpers.CreateTestEntitySchema("user")
	err := env.Engine.CreateEntitySchema(ctx, schema)
	require.NoError(t, err)

	entities := []*models.Entity{
		testhelpers.CreateTestEntity("user", "john-doe"),
		testhelpers.CreateTestEntity("user", "jane-smith"),
		testhelpers.CreateTestEntity("user", "bob-wilson"),
	}

	for _, entity := range entities {
		err = env.Engine.CreateEntity(ctx, entity)
		require.NoError(t, err)
	}

	testhelpers.WaitForIndexing(t, ctx, env.IndexStore, 2*time.Second)

	query := &models.SearchQuery{
		EntityTypes: []string{"user"},
		Query:       "john",
		Limit:       10,
	}

	results, err := env.Engine.Search(ctx, query)
	require.NoError(t, err)
	assert.Greater(t, len(results.Hits), 0)

	found := false
	for _, hit := range results.Hits {
		if hit.Properties["name"] == "john-doe" {
			found = true
			break
		}
	}
	assert.True(t, found, "Expected entity not found in search results")
}

func TestEngine_VectorSearch(t *testing.T) {
	ctx := context.Background()
	env := testhelpers.SetupTestEnvironment(t, ctx)
	defer env.Cleanup(ctx)

	schema := testhelpers.CreateTestEntitySchema("user")
	err := env.Engine.CreateEntitySchema(ctx, schema)
	require.NoError(t, err)

	embedding1 := testhelpers.GenerateTestEmbedding(384)
	embedding2 := testhelpers.GenerateTestEmbedding(384)

	entity1 := testhelpers.CreateTestEntityWithEmbedding("user", "user-1", embedding1)
	err = env.Engine.CreateEntity(ctx, entity1)
	require.NoError(t, err)

	entity2 := testhelpers.CreateTestEntityWithEmbedding("user", "user-2", embedding2)
	err = env.Engine.CreateEntity(ctx, entity2)
	require.NoError(t, err)

	testhelpers.WaitForIndexing(t, ctx, env.IndexStore, 2*time.Second)

	query := &models.VectorQuery{
		EntityTypes: []string{"user"},
		Vector:      embedding1,
		TopK:        5,
	}

	results, err := env.Engine.VectorSearch(ctx, query)
	require.NoError(t, err)
	assert.Greater(t, len(results.Hits), 0)

	if len(results.Hits) > 0 {
		assert.Equal(t, entity1.ID, results.Hits[0].ID)
	}
}

func TestEngine_TwoPhaseCommit(t *testing.T) {
	ctx := context.Background()
	env := testhelpers.SetupTestEnvironment(t, ctx)
	defer env.Cleanup(ctx)

	schema := testhelpers.CreateTestEntitySchema("user")
	err := env.Engine.CreateEntitySchema(ctx, schema)
	require.NoError(t, err)

	entity := testhelpers.CreateTestEntity("user", "test-user")

	err = env.Engine.CreateEntity(ctx, entity)
	require.NoError(t, err)

	primaryEntity, err := env.PrimaryStore.GetEntity(ctx, entity.EntityType, entity.ID)
	require.NoError(t, err)
	assert.Equal(t, entity.URN, primaryEntity.URN)

	testhelpers.WaitForIndexing(t, ctx, env.IndexStore, 2*time.Second)

	query := &models.SearchQuery{
		EntityTypes: []string{"user"},
		Query:       entity.Properties["name"].(string),
		Limit:       1,
	}

	results, err := env.Engine.Search(ctx, query)
	require.NoError(t, err)
	assert.Equal(t, 1, len(results.Hits))
}

func TestEngine_ConcurrentOperations(t *testing.T) {
	ctx := context.Background()
	env := testhelpers.SetupTestEnvironment(t, ctx)
	defer env.Cleanup(ctx)

	schema := testhelpers.CreateTestEntitySchema("user")
	err := env.Engine.CreateEntitySchema(ctx, schema)
	require.NoError(t, err)

	numEntities := 10
	results := make(chan error, numEntities)

	for i := 0; i < numEntities; i++ {
		go func(index int) {
			entity := testhelpers.CreateTestEntity("user", testhelpers.RandomString(8))
			entity.URN = testhelpers.RandomString(16)
			results <- env.Engine.CreateEntity(ctx, entity)
		}(i)
	}

	var errors []error
	for i := 0; i < numEntities; i++ {
		if err := <-results; err != nil {
			errors = append(errors, err)
		}
	}

	assert.Empty(t, errors, "Expected no errors in concurrent operations")
}

func BenchmarkEngine_CreateEntity(b *testing.B) {
	ctx := context.Background()
	env := testhelpers.SetupTestEnvironment(&testing.T{}, ctx)
	defer env.Cleanup(ctx)

	schema := testhelpers.CreateTestEntitySchema("user")
	env.Engine.CreateEntitySchema(ctx, schema)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		entity := testhelpers.CreateTestEntity("user", "benchmark-user")
		entity.URN = testhelpers.RandomString(32)
		env.Engine.CreateEntity(ctx, entity)
	}
}

func BenchmarkEngine_SearchEntities(b *testing.B) {
	ctx := context.Background()
	env := testhelpers.SetupTestEnvironment(&testing.T{}, ctx)
	defer env.Cleanup(ctx)

	schema := testhelpers.CreateTestEntitySchema("user")
	env.Engine.CreateEntitySchema(ctx, schema)

	for i := 0; i < 100; i++ {
		entity := testhelpers.CreateTestEntity("user", testhelpers.RandomString(8))
		entity.URN = testhelpers.RandomString(32)
		env.Engine.CreateEntity(ctx, entity)
	}

	query := &models.SearchQuery{
		EntityTypes: []string{"user"},
		Query:       "test",
		Limit:       10,
	}

	time.Sleep(500 * time.Millisecond)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		env.Engine.Search(ctx, query)
	}
}
