package integration

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

func TestFullWorkflow_EntityLifecycle(t *testing.T) {
	ctx := context.Background()
	env := testhelpers.SetupTestEnvironment(t, ctx)
	defer env.Cleanup(ctx)

	schema := testhelpers.CreateTestEntitySchema("user")
	err := env.Engine.CreateEntitySchema(ctx, schema)
	require.NoError(t, err, "Failed to create entity schema")

	entity := testhelpers.CreateTestEntity("user", "john-doe")
	originalURN := entity.URN
	err = env.Engine.CreateEntity(ctx, entity)
	require.NoError(t, err, "Failed to create entity")

	retrievedEntity, err := env.Engine.GetEntity(ctx, entity.EntityType, entity.ID)
	require.NoError(t, err, "Failed to retrieve entity from primary store")
	testhelpers.AssertEntityEqual(t, entity, retrievedEntity)

	testhelpers.WaitForIndexing(t, ctx, env.IndexStore, 3*time.Second)

	searchQuery := &models.SearchQuery{
		EntityTypes: []string{"user"},
		Query:       "john",
		Limit:       10,
	}

	searchResults, err := env.Engine.Search(ctx, searchQuery)
	require.NoError(t, err, "Failed to search entities")
	assert.Greater(t, len(searchResults.Hits), 0, "Entity should be found in search")

	found := false
	for _, result := range searchResults.Hits {
		if result.ID == entity.ID {
			found = true
			assert.Equal(t, entity.Properties["name"], result.Properties["name"])
			break
		}
	}
	assert.True(t, found, "Created entity should be found in search results")

	updatedProperties := map[string]any{
		"name":        "john-doe-updated",
		"description": "Updated user description",
		"tags":        []string{"updated", "test", "user"},
		"metadata": map[string]any{
			"updated_by": "test",
			"version":    2,
		},
	}

	updatedEntity := &models.Entity{
		ID:         entity.ID,
		EntityType: entity.EntityType,
		URN:        entity.URN,
		Properties: updatedProperties,
		Version:    retrievedEntity.Version,
	}

	err = env.Engine.UpdateEntity(ctx, updatedEntity)
	require.NoError(t, err, "Failed to update entity")

	retrievedUpdated, err := env.Engine.GetEntity(ctx, entity.EntityType, entity.ID)
	require.NoError(t, err, "Failed to retrieve updated entity")
	assert.Equal(t, "john-doe-updated", retrievedUpdated.Properties["name"])
	assert.Greater(t, retrievedUpdated.Version, retrievedEntity.Version)

	testhelpers.WaitForIndexing(t, ctx, env.IndexStore, 3*time.Second)

	searchQueryUpdated := &models.SearchQuery{
		EntityTypes: []string{"user"},
		Query:       "updated",
		Limit:       10,
	}

	searchResultsUpdated, err := env.Engine.Search(ctx, searchQueryUpdated)
	require.NoError(t, err, "Failed to search updated entities")
	assert.Greater(t, len(searchResultsUpdated.Hits), 0, "Updated entity should be found in search")

	err = env.Engine.DeleteEntity(ctx, entity.EntityType, entity.ID)
	require.NoError(t, err, "Failed to delete entity")

	_, err = env.Engine.GetEntity(ctx, entity.EntityType, entity.ID)
	assert.Error(t, err, "Deleted entity should not exist in primary store")
	assert.Contains(t, err.Error(), "not found")

	testhelpers.WaitForIndexing(t, ctx, env.IndexStore, 3*time.Second)

	searchQueryDeleted := &models.SearchQuery{
		EntityTypes: []string{"user"},
		Query:       originalURN,
		Limit:       10,
	}

	searchResultsDeleted, err := env.Engine.Search(ctx, searchQueryDeleted)
	require.NoError(t, err, "Search should succeed even if no results")

	foundDeleted := false
	for _, result := range searchResultsDeleted.Hits {
		if result.ID == entity.ID {
			foundDeleted = true
			break
		}
	}
	assert.False(t, foundDeleted, "Deleted entity should not be found in search results")
}

func TestFullWorkflow_RelationshipLifecycle(t *testing.T) {
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

	user := testhelpers.CreateTestEntity("user", "john-doe")
	err = env.Engine.CreateEntity(ctx, user)
	require.NoError(t, err)

	org := testhelpers.CreateTestEntity("organization", "acme-corp")
	err = env.Engine.CreateEntity(ctx, org)
	require.NoError(t, err)

	relation := testhelpers.CreateTestRelation("member_of", user, org)
	err = env.Engine.CreateRelation(ctx, relation)
	require.NoError(t, err)

	retrievedRelation, err := env.Engine.GetRelation(ctx, relation.ID)
	require.NoError(t, err)
	testhelpers.AssertRelationEqual(t, relation, retrievedRelation)

	if relationSchema.DenormalizationConfig.DenormalizeToFrom != nil {

		retrievedOrg, err := env.Engine.GetEntity(ctx, org.EntityType, org.ID)
		require.NoError(t, err)

		assert.NotNil(t, retrievedOrg.Properties, "Organization should have denormalized properties")
	}

	userRelations, err := env.Engine.GetRelationsByEntity(ctx, user.ID, nil)
	require.NoError(t, err)
	assert.Greater(t, len(userRelations), 0, "User should have relations")

	foundRelation := false
	for _, rel := range userRelations {
		if rel.ID == relation.ID {
			foundRelation = true
			break
		}
	}
	assert.True(t, foundRelation, "Created relation should be found in user relations")

	err = env.Engine.DeleteRelation(ctx, relation.ID)
	require.NoError(t, err)

	_, err = env.Engine.GetRelation(ctx, relation.ID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	if relationSchema.DenormalizationConfig.UpdateOnChange {
		retrievedOrgAfterDelete, err := env.Engine.GetEntity(ctx, org.EntityType, org.ID)
		require.NoError(t, err)

		assert.NotNil(t, retrievedOrgAfterDelete.Properties)
	}
}

func TestFullWorkflow_VectorSearchLifecycle(t *testing.T) {
	ctx := context.Background()
	env := testhelpers.SetupTestEnvironment(t, ctx)
	defer env.Cleanup(ctx)

	schema := testhelpers.CreateTestEntitySchema("document")
	err := env.Engine.CreateEntitySchema(ctx, schema)
	require.NoError(t, err)

	embedding1 := testhelpers.GenerateTestEmbedding(384)
	embedding2 := testhelpers.GenerateTestEmbedding(384)
	embedding3 := testhelpers.GenerateTestEmbedding(384)

	doc1 := testhelpers.CreateTestEntityWithEmbedding("document", "doc-1", embedding1)
	err = env.Engine.CreateEntity(ctx, doc1)
	require.NoError(t, err)

	doc2 := testhelpers.CreateTestEntityWithEmbedding("document", "doc-2", embedding2)
	err = env.Engine.CreateEntity(ctx, doc2)
	require.NoError(t, err)

	doc3 := testhelpers.CreateTestEntityWithEmbedding("document", "doc-3", embedding3)
	err = env.Engine.CreateEntity(ctx, doc3)
	require.NoError(t, err)

	testhelpers.WaitForIndexing(t, ctx, env.IndexStore, 3*time.Second)

	vectorQuery := &models.VectorQuery{
		EntityTypes: []string{"document"},
		Vector:      embedding1,
		VectorField: "embedding",
		TopK:        5,
	}

	vectorResults, err := env.Engine.VectorSearch(ctx, vectorQuery)
	require.NoError(t, err)
	assert.Greater(t, len(vectorResults.Hits), 0, "Vector search should return results")

	if len(vectorResults.Hits) > 0 {
		assert.Equal(t, doc1.ID, vectorResults.Hits[0].ID, "Exact match should be first result")
	}

	newEmbedding := testhelpers.GenerateTestEmbedding(384)
	updatedDoc1 := &models.Entity{
		ID:         doc1.ID,
		EntityType: doc1.EntityType,
		URN:        doc1.URN,
		Properties: map[string]any{
			"name":        doc1.Properties["name"],
			"description": "Updated document with new embedding",
			"embedding":   newEmbedding,
		},
		Version: doc1.Version,
	}

	err = env.Engine.UpdateEntity(ctx, updatedDoc1)
	require.NoError(t, err)

	testhelpers.WaitForIndexing(t, ctx, env.IndexStore, 3*time.Second)

	newVectorQuery := &models.VectorQuery{
		EntityTypes: []string{"document"},
		Vector:      newEmbedding,
		VectorField: "embedding",
		TopK:        5,
	}

	newVectorResults, err := env.Engine.VectorSearch(ctx, newVectorQuery)
	require.NoError(t, err)
	assert.Greater(t, len(newVectorResults.Hits), 0)

	if len(newVectorResults.Hits) > 0 {
		assert.Equal(t, doc1.ID, newVectorResults.Hits[0].ID, "Updated document should be found with new embedding")
	}
}

func TestFullWorkflow_ConcurrentOperations(t *testing.T) {
	ctx := context.Background()
	env := testhelpers.SetupTestEnvironment(t, ctx)
	defer env.Cleanup(ctx)

	schema := testhelpers.CreateTestEntitySchema("user")
	err := env.Engine.CreateEntitySchema(ctx, schema)
	require.NoError(t, err)

	numEntities := 20
	entityChan := make(chan *models.Entity, numEntities)
	errorChan := make(chan error, numEntities)

	for i := 0; i < numEntities; i++ {
		go func(index int) {
			entity := testhelpers.CreateTestEntity("user", testhelpers.RandomString(8))
			entity.URN = testhelpers.RandomString(32)

			err := env.Engine.CreateEntity(ctx, entity)
			if err != nil {
				errorChan <- err
			} else {
				entityChan <- entity
			}
		}(i)
	}

	var entities []*models.Entity
	var errors []error

	for i := 0; i < numEntities; i++ {
		select {
		case entity := <-entityChan:
			entities = append(entities, entity)
		case err := <-errorChan:
			errors = append(errors, err)
		case <-time.After(10 * time.Second):
			t.Fatal("Timeout waiting for concurrent operations")
		}
	}

	assert.Empty(t, errors, "No errors expected in concurrent operations")
	assert.Equal(t, numEntities, len(entities), "All entities should be created")

	testhelpers.WaitForIndexing(t, ctx, env.IndexStore, 5*time.Second)

	searchQuery := &models.SearchQuery{
		EntityTypes: []string{"user"},
		Query:       "*",
		Limit:       numEntities + 10,
	}

	searchResults, err := env.Engine.Search(ctx, searchQuery)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(searchResults.Hits), numEntities, "All entities should be searchable")
}

func TestFullWorkflow_TransactionRollback(t *testing.T) {
	ctx := context.Background()
	env := testhelpers.SetupTestEnvironment(t, ctx)
	defer env.Cleanup(ctx)

	schema := testhelpers.CreateTestEntitySchema("user")
	err := env.Engine.CreateEntitySchema(ctx, schema)
	require.NoError(t, err)

	invalidEntity := &models.Entity{
		ID:         uuid.New(),
		EntityType: "user",
		URN:        "test:user:invalid",
		Properties: map[string]any{
			"name": 123,
		},
	}

	err = env.Engine.CreateEntity(ctx, invalidEntity)
	assert.Error(t, err, "Invalid entity creation should fail")

	_, err = env.Engine.GetEntity(ctx, invalidEntity.EntityType, invalidEntity.ID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	testhelpers.WaitForIndexing(t, ctx, env.IndexStore, 2*time.Second)

	searchQuery := &models.SearchQuery{
		EntityTypes: []string{"user"},
		Query:       invalidEntity.URN,
		Limit:       10,
	}

	searchResults, err := env.Engine.Search(ctx, searchQuery)
	require.NoError(t, err)

	foundInvalid := false
	for _, result := range searchResults.Hits {
		if result.ID == invalidEntity.ID {
			foundInvalid = true
			break
		}
	}
	assert.False(t, foundInvalid, "Invalid entity should not be found in search")
}

func TestFullWorkflow_SchemaEvolution(t *testing.T) {
	ctx := context.Background()
	env := testhelpers.SetupTestEnvironment(t, ctx)
	defer env.Cleanup(ctx)

	initialSchema := testhelpers.CreateTestEntitySchema("user")
	err := env.Engine.CreateEntitySchema(ctx, initialSchema)
	require.NoError(t, err)

	entity := testhelpers.CreateTestEntity("user", "test-user")
	err = env.Engine.CreateEntity(ctx, entity)
	require.NoError(t, err)

	updatedSchema := testhelpers.CreateTestEntitySchema("user")
	updatedSchema.Properties["email"] = models.PropertyDefinition{
		Type:     "string",
		Required: false,
	}
	updatedSchema.Properties["age"] = models.PropertyDefinition{
		Type:     "number",
		Required: false,
	}

	err = env.Engine.UpdateEntitySchema(ctx, updatedSchema)
	require.NoError(t, err)

	newEntity := &models.Entity{
		ID:         uuid.New(),
		EntityType: "user",
		URN:        "test:user:new-schema",
		Properties: map[string]any{
			"name":        "New User",
			"description": "User with new schema",
			"email":       "user@example.com",
			"age":         30,
		},
	}

	err = env.Engine.CreateEntity(ctx, newEntity)
	require.NoError(t, err)

	retrievedOld, err := env.Engine.GetEntity(ctx, entity.EntityType, entity.ID)
	require.NoError(t, err)
	assert.Equal(t, entity.Properties["name"], retrievedOld.Properties["name"])

	retrievedNew, err := env.Engine.GetEntity(ctx, newEntity.EntityType, newEntity.ID)
	require.NoError(t, err)
	assert.Equal(t, "user@example.com", retrievedNew.Properties["email"])
	assert.Equal(t, float64(30), retrievedNew.Properties["age"])
}
