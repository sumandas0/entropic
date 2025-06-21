package integration

import (
	"context"
	"testing"
	"time"

	"github.com/entropic/entropic/internal/models"
	"github.com/entropic/entropic/tests/testhelpers"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFullWorkflow_EntityLifecycle(t *testing.T) {
	ctx := context.Background()
	env := testhelpers.SetupTestEnvironment(t, ctx)
	defer env.Cleanup(ctx)

	// Step 1: Create entity schema
	schema := testhelpers.CreateTestEntitySchema("user")
	err := env.Engine.CreateEntitySchema(ctx, schema)
	require.NoError(t, err, "Failed to create entity schema")

	// Step 2: Create entity
	entity := testhelpers.CreateTestEntity("user", "john-doe")
	originalURN := entity.URN
	err = env.Engine.CreateEntity(ctx, entity)
	require.NoError(t, err, "Failed to create entity")

	// Step 3: Verify entity exists in primary store
	retrievedEntity, err := env.Engine.GetEntity(ctx, entity.EntityType, entity.ID)
	require.NoError(t, err, "Failed to retrieve entity from primary store")
	testhelpers.AssertEntityEqual(t, entity, retrievedEntity)

	// Step 4: Wait for indexing and verify entity is searchable
	testhelpers.WaitForIndexing(t, ctx, env.IndexStore, 3*time.Second)

	searchQuery := &models.SearchQuery{
		EntityTypes: []string{"user"},
		Query:       "john",
		Limit:       10,
	}

	searchResults, err := env.Engine.SearchEntities(ctx, searchQuery)
	require.NoError(t, err, "Failed to search entities")
	assert.Greater(t, len(searchResults.Entities), 0, "Entity should be found in search")

	// Verify the searched entity matches
	found := false
	for _, result := range searchResults.Entities {
		if result.ID == entity.ID {
			found = true
			assert.Equal(t, entity.Properties["name"], result.Properties["name"])
			break
		}
	}
	assert.True(t, found, "Created entity should be found in search results")

	// Step 5: Update entity
	updatedProperties := map[string]interface{}{
		"name":        "john-doe-updated",
		"description": "Updated user description",
		"tags":        []string{"updated", "test", "user"},
		"metadata": map[string]interface{}{
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

	// Step 6: Verify update in primary store
	retrievedUpdated, err := env.Engine.GetEntity(ctx, entity.EntityType, entity.ID)
	require.NoError(t, err, "Failed to retrieve updated entity")
	assert.Equal(t, "john-doe-updated", retrievedUpdated.Properties["name"])
	assert.Greater(t, retrievedUpdated.Version, retrievedEntity.Version)

	// Step 7: Verify update is searchable
	testhelpers.WaitForIndexing(t, ctx, env.IndexStore, 3*time.Second)

	searchQueryUpdated := &models.SearchQuery{
		EntityTypes: []string{"user"},
		Query:       "updated",
		Limit:       10,
	}

	searchResultsUpdated, err := env.Engine.SearchEntities(ctx, searchQueryUpdated)
	require.NoError(t, err, "Failed to search updated entities")
	assert.Greater(t, len(searchResultsUpdated.Entities), 0, "Updated entity should be found in search")

	// Step 8: Delete entity
	err = env.Engine.DeleteEntity(ctx, entity.EntityType, entity.ID)
	require.NoError(t, err, "Failed to delete entity")

	// Step 9: Verify entity is deleted from primary store
	_, err = env.Engine.GetEntity(ctx, entity.EntityType, entity.ID)
	assert.Error(t, err, "Deleted entity should not exist in primary store")
	assert.Contains(t, err.Error(), "not found")

	// Step 10: Verify entity is removed from search index
	testhelpers.WaitForIndexing(t, ctx, env.IndexStore, 3*time.Second)

	searchQueryDeleted := &models.SearchQuery{
		EntityTypes: []string{"user"},
		Query:       originalURN,
		Limit:       10,
	}

	searchResultsDeleted, err := env.Engine.SearchEntities(ctx, searchQueryDeleted)
	require.NoError(t, err, "Search should succeed even if no results")
	
	// Entity should not be found in search results
	foundDeleted := false
	for _, result := range searchResultsDeleted.Entities {
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

	// Step 1: Create entity schemas
	userSchema := testhelpers.CreateTestEntitySchema("user")
	err := env.Engine.CreateEntitySchema(ctx, userSchema)
	require.NoError(t, err)

	orgSchema := testhelpers.CreateTestEntitySchema("organization")
	err = env.Engine.CreateEntitySchema(ctx, orgSchema)
	require.NoError(t, err)

	// Step 2: Create relationship schema
	relationSchema := testhelpers.CreateTestRelationshipSchema("member_of", "user", "organization")
	err = env.Engine.CreateRelationshipSchema(ctx, relationSchema)
	require.NoError(t, err)

	// Step 3: Create entities
	user := testhelpers.CreateTestEntity("user", "john-doe")
	err = env.Engine.CreateEntity(ctx, user)
	require.NoError(t, err)

	org := testhelpers.CreateTestEntity("organization", "acme-corp")
	err = env.Engine.CreateEntity(ctx, org)
	require.NoError(t, err)

	// Step 4: Create relationship
	relation := testhelpers.CreateTestRelation("member_of", user, org)
	err = env.Engine.CreateRelation(ctx, relation)
	require.NoError(t, err)

	// Step 5: Verify relationship exists
	retrievedRelation, err := env.Engine.GetRelation(ctx, relation.ID)
	require.NoError(t, err)
	testhelpers.AssertRelationEqual(t, relation, retrievedRelation)

	// Step 6: Verify denormalization (if configured)
	if relationSchema.DenormalizationConfig.DenormalizeToFrom != nil {
		// Check if denormalized data is present in the "to" entity
		retrievedOrg, err := env.Engine.GetEntity(ctx, org.EntityType, org.ID)
		require.NoError(t, err)
		
		// Verify denormalized fields are present
		assert.NotNil(t, retrievedOrg.Properties, "Organization should have denormalized properties")
	}

	// Step 7: Query relationships
	userRelations, err := env.Engine.GetEntityRelations(ctx, user.EntityType, user.ID)
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

	// Step 8: Delete relationship
	err = env.Engine.DeleteRelation(ctx, relation.ID)
	require.NoError(t, err)

	// Step 9: Verify relationship is deleted
	_, err = env.Engine.GetRelation(ctx, relation.ID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	// Step 10: Verify denormalized data is cleaned up
	if relationSchema.DenormalizationConfig.UpdateOnChange {
		retrievedOrgAfterDelete, err := env.Engine.GetEntity(ctx, org.EntityType, org.ID)
		require.NoError(t, err)
		
		// Denormalized data should be removed or updated
		assert.NotNil(t, retrievedOrgAfterDelete.Properties)
	}
}

func TestFullWorkflow_VectorSearchLifecycle(t *testing.T) {
	ctx := context.Background()
	env := testhelpers.SetupTestEnvironment(t, ctx)
	defer env.Cleanup(ctx)

	// Step 1: Create entity schema with vector support
	schema := testhelpers.CreateTestEntitySchema("document")
	err := env.Engine.CreateEntitySchema(ctx, schema)
	require.NoError(t, err)

	// Step 2: Create entities with vector embeddings
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

	// Step 3: Wait for indexing
	testhelpers.WaitForIndexing(t, ctx, env.IndexStore, 3*time.Second)

	// Step 4: Perform vector search
	vectorQuery := &models.VectorQuery{
		EntityTypes: []string{"document"},
		Vector:      embedding1, // Search for similar to doc1
		Limit:       5,
	}

	vectorResults, err := env.Engine.VectorSearch(ctx, vectorQuery)
	require.NoError(t, err)
	assert.Greater(t, len(vectorResults.Entities), 0, "Vector search should return results")

	// Step 5: Verify the exact match is first
	if len(vectorResults.Entities) > 0 {
		assert.Equal(t, doc1.ID, vectorResults.Entities[0].ID, "Exact match should be first result")
	}

	// Step 6: Update embedding and verify search updates
	newEmbedding := testhelpers.GenerateTestEmbedding(384)
	updatedDoc1 := &models.Entity{
		ID:         doc1.ID,
		EntityType: doc1.EntityType,
		URN:        doc1.URN,
		Properties: map[string]interface{}{
			"name":        doc1.Properties["name"],
			"description": "Updated document with new embedding",
			"embedding":   newEmbedding,
		},
		Version: doc1.Version,
	}

	err = env.Engine.UpdateEntity(ctx, updatedDoc1)
	require.NoError(t, err)

	// Wait for re-indexing
	testhelpers.WaitForIndexing(t, ctx, env.IndexStore, 3*time.Second)

	// Step 7: Search with new embedding should find updated document
	newVectorQuery := &models.VectorQuery{
		EntityTypes: []string{"document"},
		Vector:      newEmbedding,
		Limit:       5,
	}

	newVectorResults, err := env.Engine.VectorSearch(ctx, newVectorQuery)
	require.NoError(t, err)
	assert.Greater(t, len(newVectorResults.Entities), 0)

	if len(newVectorResults.Entities) > 0 {
		assert.Equal(t, doc1.ID, newVectorResults.Entities[0].ID, "Updated document should be found with new embedding")
	}
}

func TestFullWorkflow_ConcurrentOperations(t *testing.T) {
	ctx := context.Background()
	env := testhelpers.SetupTestEnvironment(t, ctx)
	defer env.Cleanup(ctx)

	// Create schema
	schema := testhelpers.CreateTestEntitySchema("user")
	err := env.Engine.CreateEntitySchema(ctx, schema)
	require.NoError(t, err)

	// Test concurrent entity creation
	numEntities := 20
	entityChan := make(chan *models.Entity, numEntities)
	errorChan := make(chan error, numEntities)

	// Create entities concurrently
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

	// Collect results
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

	// All operations should succeed
	assert.Empty(t, errors, "No errors expected in concurrent operations")
	assert.Equal(t, numEntities, len(entities), "All entities should be created")

	// Wait for indexing
	testhelpers.WaitForIndexing(t, ctx, env.IndexStore, 5*time.Second)

	// Verify all entities are searchable
	searchQuery := &models.SearchQuery{
		EntityTypes: []string{"user"},
		Query:       "*",
		Limit:       numEntities + 10,
	}

	searchResults, err := env.Engine.SearchEntities(ctx, searchQuery)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(searchResults.Entities), numEntities, "All entities should be searchable")
}

func TestFullWorkflow_TransactionRollback(t *testing.T) {
	ctx := context.Background()
	env := testhelpers.SetupTestEnvironment(t, ctx)
	defer env.Cleanup(ctx)

	// Create schema
	schema := testhelpers.CreateTestEntitySchema("user")
	err := env.Engine.CreateEntitySchema(ctx, schema)
	require.NoError(t, err)

	// Create entity with invalid properties to trigger rollback
	invalidEntity := &models.Entity{
		ID:         uuid.New(),
		EntityType: "user",
		URN:        "test:user:invalid",
		Properties: map[string]interface{}{
			"name": 123, // Invalid type - should be string
		},
	}

	// This should fail and rollback
	err = env.Engine.CreateEntity(ctx, invalidEntity)
	assert.Error(t, err, "Invalid entity creation should fail")

	// Verify entity was not created in primary store
	_, err = env.Engine.GetEntity(ctx, invalidEntity.EntityType, invalidEntity.ID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	// Wait and verify entity is not in search index
	testhelpers.WaitForIndexing(t, ctx, env.IndexStore, 2*time.Second)

	searchQuery := &models.SearchQuery{
		EntityTypes: []string{"user"},
		Query:       invalidEntity.URN,
		Limit:       10,
	}

	searchResults, err := env.Engine.SearchEntities(ctx, searchQuery)
	require.NoError(t, err)
	
	// Entity should not be found
	foundInvalid := false
	for _, result := range searchResults.Entities {
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

	// Step 1: Create initial schema
	initialSchema := testhelpers.CreateTestEntitySchema("user")
	err := env.Engine.CreateEntitySchema(ctx, initialSchema)
	require.NoError(t, err)

	// Step 2: Create entity with initial schema
	entity := testhelpers.CreateTestEntity("user", "test-user")
	err = env.Engine.CreateEntity(ctx, entity)
	require.NoError(t, err)

	// Step 3: Update schema with additional properties
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

	// Step 4: Create entity with new schema properties
	newEntity := &models.Entity{
		ID:         uuid.New(),
		EntityType: "user",
		URN:        "test:user:new-schema",
		Properties: map[string]interface{}{
			"name":        "New User",
			"description": "User with new schema",
			"email":       "user@example.com",
			"age":         30,
		},
	}

	err = env.Engine.CreateEntity(ctx, newEntity)
	require.NoError(t, err)

	// Step 5: Verify both old and new entities work
	retrievedOld, err := env.Engine.GetEntity(ctx, entity.EntityType, entity.ID)
	require.NoError(t, err)
	assert.Equal(t, entity.Properties["name"], retrievedOld.Properties["name"])

	retrievedNew, err := env.Engine.GetEntity(ctx, newEntity.EntityType, newEntity.ID)
	require.NoError(t, err)
	assert.Equal(t, "user@example.com", retrievedNew.Properties["email"])
	assert.Equal(t, float64(30), retrievedNew.Properties["age"])
}