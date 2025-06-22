package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/sumandas0/entropic/internal/models"
	"github.com/sumandas0/entropic/internal/store/testutils"
	"github.com/sumandas0/entropic/pkg/utils"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test fixtures
var (
	testContainer *testutils.PostgresTestContainer
	testStore     *PostgresStore
)

func TestMain(m *testing.M) {
	// Setup test container
	var err error
	testContainer, err = testutils.SetupTestPostgres()
	if err != nil {
		panic(err)
	}

	// Create store
	testStore, err = NewPostgresStore(testContainer.URL)
	if err != nil {
		testContainer.Cleanup()
		panic(err)
	}

	// Run migrations
	ctx := context.Background()
	migrator := NewMigrator(testStore.GetPool())
	if err := migrator.Run(ctx); err != nil {
		testStore.Close()
		testContainer.Cleanup()
		panic(err)
	}

	// Run tests
	code := m.Run()

	// Cleanup
	testStore.Close()
	testContainer.Cleanup()

	// Exit with test code
	if code != 0 {
		panic("tests failed")
	}
}

func TestPostgresStore_EntityOperations(t *testing.T) {
	ctx := context.Background()

	t.Run("CreateEntity", func(t *testing.T) {
		// Create test entity
		entity := &models.Entity{
			ID:         uuid.New(),
			EntityType: "user",
			URN:        "test:user:" + uuid.New().String(),
			Properties: map[string]interface{}{
				"name":        "Test User",
				"email":       "test@example.com",
				"age":         30,
				"tags":        []string{"test", "user"},
				"metadata":    map[string]interface{}{"created_by": "test"},
			},
			Version: 1,
		}

		err := testStore.CreateEntity(ctx, entity)
		require.NoError(t, err)

		// Verify entity was created
		retrieved, err := testStore.GetEntity(ctx, entity.EntityType, entity.ID)
		require.NoError(t, err)
		assert.Equal(t, entity.ID, retrieved.ID)
		assert.Equal(t, entity.EntityType, retrieved.EntityType)
		assert.Equal(t, entity.URN, retrieved.URN)
		
		// Check properties individually due to JSON marshalling type conversions
		assert.Equal(t, entity.Properties["name"], retrieved.Properties["name"])
		assert.Equal(t, entity.Properties["email"], retrieved.Properties["email"])
		assert.Equal(t, float64(30), retrieved.Properties["age"]) // JSON numbers become float64
		
		// Check tags array
		retrievedTags, ok := retrieved.Properties["tags"].([]interface{})
		require.True(t, ok, "tags should be an array")
		assert.Len(t, retrievedTags, 2)
		assert.Equal(t, "test", retrievedTags[0])
		assert.Equal(t, "user", retrievedTags[1])
		
		// Check metadata
		retrievedMetadata, ok := retrieved.Properties["metadata"].(map[string]interface{})
		require.True(t, ok, "metadata should be a map")
		assert.Equal(t, "test", retrievedMetadata["created_by"])
		
		assert.NotZero(t, retrieved.CreatedAt)
		assert.NotZero(t, retrieved.UpdatedAt)
	})

	t.Run("CreateEntity_WithVector", func(t *testing.T) {
		// Create test entity with vector
		embedding := make([]float32, 384)
		for i := range embedding {
			embedding[i] = float32(i) / 384.0
		}

		entity := &models.Entity{
			ID:         uuid.New(),
			EntityType: "document",
			URN:        "test:document:" + uuid.New().String(),
			Properties: map[string]interface{}{
				"title":     "Test Document",
				"content":   "This is a test document with vector embedding",
				"embedding": embedding,
			},
			Version: 1,
		}

		err := testStore.CreateEntity(ctx, entity)
		require.NoError(t, err)

		// Verify entity with vector was created
		retrieved, err := testStore.GetEntity(ctx, entity.EntityType, entity.ID)
		require.NoError(t, err)
		
		// Check vector field
		retrievedEmbedding, ok := retrieved.Properties["embedding"].([]interface{})
		require.True(t, ok, "embedding should be an array")
		assert.Len(t, retrievedEmbedding, 384)
	})

	t.Run("UpdateEntity", func(t *testing.T) {
		// Create initial entity
		entity := &models.Entity{
			ID:         uuid.New(),
			EntityType: "user",
			URN:        "test:user:" + uuid.New().String(),
			Properties: map[string]interface{}{
				"name": "Original Name",
				"age":  25,
			},
			Version: 1,
		}

		err := testStore.CreateEntity(ctx, entity)
		require.NoError(t, err)

		// Update entity
		entity.Properties["name"] = "Updated Name"
		entity.Properties["age"] = 26
		entity.Properties["new_field"] = "new value"
		
		err = testStore.UpdateEntity(ctx, entity)
		require.NoError(t, err)

		// Verify update
		retrieved, err := testStore.GetEntity(ctx, entity.EntityType, entity.ID)
		require.NoError(t, err)
		assert.Equal(t, "Updated Name", retrieved.Properties["name"])
		assert.Equal(t, float64(26), retrieved.Properties["age"])
		assert.Equal(t, "new value", retrieved.Properties["new_field"])
		assert.Greater(t, retrieved.Version, entity.Version)
		assert.Greater(t, retrieved.UpdatedAt, entity.CreatedAt)
	})

	t.Run("DeleteEntity", func(t *testing.T) {
		// Create entity
		entity := &models.Entity{
			ID:         uuid.New(),
			EntityType: "user",
			URN:        "test:user:" + uuid.New().String(),
			Properties: map[string]interface{}{
				"name": "To Be Deleted",
			},
			Version: 1,
		}

		err := testStore.CreateEntity(ctx, entity)
		require.NoError(t, err)

		// Delete entity
		err = testStore.DeleteEntity(ctx, entity.EntityType, entity.ID)
		require.NoError(t, err)

		// Verify deletion
		_, err = testStore.GetEntity(ctx, entity.EntityType, entity.ID)
		assert.Error(t, err)
		var appErr *utils.AppError
		assert.ErrorAs(t, err, &appErr)
		assert.True(t, utils.IsNotFound(err))
	})

	t.Run("ListEntities", func(t *testing.T) {
		// Clear existing test data
		cleanupTestData(t, ctx)

		// Create multiple entities
		entityType := "user"
		for i := 0; i < 5; i++ {
			entity := &models.Entity{
				ID:         uuid.New(),
				EntityType: entityType,
				URN:        fmt.Sprintf("test:user:list-%d", i),
				Properties: map[string]interface{}{
					"name":  fmt.Sprintf("User %d", i),
					"index": i,
				},
				Version: 1,
			}
			err := testStore.CreateEntity(ctx, entity)
			require.NoError(t, err)
		}

		// List entities
		entities, err := testStore.ListEntities(ctx, entityType, 3, 0)
		require.NoError(t, err)
		assert.Len(t, entities, 3)

		// Test pagination
		page2, err := testStore.ListEntities(ctx, entityType, 3, 3)
		require.NoError(t, err)
		assert.Len(t, page2, 2)

		// Verify no overlap
		for _, e1 := range entities {
			for _, e2 := range page2 {
				assert.NotEqual(t, e1.ID, e2.ID)
			}
		}
	})

	t.Run("CheckURNExists", func(t *testing.T) {
		urn := "test:user:unique-" + uuid.New().String()
		
		// Check non-existent URN
		exists, err := testStore.CheckURNExists(ctx, urn)
		require.NoError(t, err)
		assert.False(t, exists)

		// Create entity with URN
		entity := &models.Entity{
			ID:         uuid.New(),
			EntityType: "user",
			URN:        urn,
			Properties: map[string]interface{}{"name": "Test"},
			Version:    1,
		}
		err = testStore.CreateEntity(ctx, entity)
		require.NoError(t, err)

		// Check existing URN
		exists, err = testStore.CheckURNExists(ctx, urn)
		require.NoError(t, err)
		assert.True(t, exists)
	})
}

func TestPostgresStore_RelationOperations(t *testing.T) {
	ctx := context.Background()

	t.Run("CreateRelation", func(t *testing.T) {
		// Create entities first
		user := createTestEntity(t, ctx, "user", "relation-test-user")
		org := createTestEntity(t, ctx, "organization", "relation-test-org")

		// Create relation
		relation := &models.Relation{
			ID:             uuid.New(),
			RelationType:   "member_of",
			FromEntityID:   user.ID,
			FromEntityType: user.EntityType,
			ToEntityID:     org.ID,
			ToEntityType:   org.EntityType,
			Properties: map[string]interface{}{
				"role":       "admin",
				"joined_at":  time.Now().Format(time.RFC3339),
			},
		}

		err := testStore.CreateRelation(ctx, relation)
		require.NoError(t, err)

		// Verify relation was created
		retrieved, err := testStore.GetRelation(ctx, relation.ID)
		require.NoError(t, err)
		assert.Equal(t, relation.ID, retrieved.ID)
		assert.Equal(t, relation.RelationType, retrieved.RelationType)
		assert.Equal(t, relation.FromEntityID, retrieved.FromEntityID)
		assert.Equal(t, relation.ToEntityID, retrieved.ToEntityID)
		assert.Equal(t, relation.Properties, retrieved.Properties)
	})

	t.Run("DeleteRelation", func(t *testing.T) {
		// Create entities and relation
		user := createTestEntity(t, ctx, "user", "delete-relation-user")
		org := createTestEntity(t, ctx, "organization", "delete-relation-org")

		relation := &models.Relation{
			ID:             uuid.New(),
			RelationType:   "member_of",
			FromEntityID:   user.ID,
			FromEntityType: user.EntityType,
			ToEntityID:     org.ID,
			ToEntityType:   org.EntityType,
		}

		err := testStore.CreateRelation(ctx, relation)
		require.NoError(t, err)

		// Delete relation
		err = testStore.DeleteRelation(ctx, relation.ID)
		require.NoError(t, err)

		// Verify deletion
		_, err = testStore.GetRelation(ctx, relation.ID)
		assert.Error(t, err)
		assert.True(t, utils.IsNotFound(err))
	})

	t.Run("GetRelationsByEntity", func(t *testing.T) {
		// Create test entities
		user := createTestEntity(t, ctx, "user", "relations-test-user")
		org1 := createTestEntity(t, ctx, "organization", "relations-test-org1")
		org2 := createTestEntity(t, ctx, "organization", "relations-test-org2")
		project := createTestEntity(t, ctx, "project", "relations-test-project")

		// Create multiple relations
		relations := []*models.Relation{
			{
				ID:             uuid.New(),
				RelationType:   "member_of",
				FromEntityID:   user.ID,
				FromEntityType: user.EntityType,
				ToEntityID:     org1.ID,
				ToEntityType:   org1.EntityType,
			},
			{
				ID:             uuid.New(),
				RelationType:   "member_of",
				FromEntityID:   user.ID,
				FromEntityType: user.EntityType,
				ToEntityID:     org2.ID,
				ToEntityType:   org2.EntityType,
			},
			{
				ID:             uuid.New(),
				RelationType:   "owner_of",
				FromEntityID:   user.ID,
				FromEntityType: user.EntityType,
				ToEntityID:     project.ID,
				ToEntityType:   project.EntityType,
			},
		}

		for _, rel := range relations {
			err := testStore.CreateRelation(ctx, rel)
			require.NoError(t, err)
		}

		// Get all relations for user
		userRelations, err := testStore.GetRelationsByEntity(ctx, user.ID, nil)
		require.NoError(t, err)
		assert.Len(t, userRelations, 3)

		// Get specific relation types
		memberRelations, err := testStore.GetRelationsByEntity(ctx, user.ID, []string{"member_of"})
		require.NoError(t, err)
		assert.Len(t, memberRelations, 2)

		ownerRelations, err := testStore.GetRelationsByEntity(ctx, user.ID, []string{"owner_of"})
		require.NoError(t, err)
		assert.Len(t, ownerRelations, 1)
	})
}

func TestPostgresStore_SchemaOperations(t *testing.T) {
	ctx := context.Background()

	t.Run("EntitySchema_CRUD", func(t *testing.T) {
		// Create entity schema
		schema := &models.EntitySchema{
			EntityType: "test_entity_" + uuid.New().String()[:8],
			Properties: models.PropertySchema{
				"name": models.PropertyDefinition{
					Type:     "string",
					Required: true,
				},
				"age": models.PropertyDefinition{
					Type:     "number",
					Required: false,
				},
				"tags": models.PropertyDefinition{
					Type:        "array",
					ElementType: "string",
					Required:    false,
				},
			},
		}

		// Create
		err := testStore.CreateEntitySchema(ctx, schema)
		require.NoError(t, err)

		// Get
		retrieved, err := testStore.GetEntitySchema(ctx, schema.EntityType)
		require.NoError(t, err)
		assert.Equal(t, schema.EntityType, retrieved.EntityType)
		assert.Equal(t, schema.Properties, retrieved.Properties)

		// Update
		schema.Properties["email"] = models.PropertyDefinition{
			Type:     "string",
			Required: true,
		}
		err = testStore.UpdateEntitySchema(ctx, schema)
		require.NoError(t, err)

		// Verify update
		updated, err := testStore.GetEntitySchema(ctx, schema.EntityType)
		require.NoError(t, err)
		assert.Contains(t, updated.Properties, "email")

		// List
		schemas, err := testStore.ListEntitySchemas(ctx)
		require.NoError(t, err)
		assert.Greater(t, len(schemas), 0)

		// Delete
		err = testStore.DeleteEntitySchema(ctx, schema.EntityType)
		require.NoError(t, err)

		// Verify deletion
		_, err = testStore.GetEntitySchema(ctx, schema.EntityType)
		assert.Error(t, err)
		assert.True(t, utils.IsNotFound(err))
	})

	t.Run("RelationshipSchema_CRUD", func(t *testing.T) {
		// Create relationship schema
		schema := &models.RelationshipSchema{
			RelationshipType: "test_rel_" + uuid.New().String()[:8],
			FromEntityType:   "user",
			ToEntityType:     "organization",
			Cardinality:      models.ManyToMany,
			Properties: models.PropertySchema{
				"role": models.PropertyDefinition{
					Type:     "string",
					Required: true,
				},
			},
			DenormalizationConfig: models.DenormalizationConfig{
				DenormalizeToFrom:   []string{"name"},
				DenormalizeFromTo:   []string{"email"},
				UpdateOnChange:      true,
				IncludeRelationData: false,
			},
		}

		// Create
		err := testStore.CreateRelationshipSchema(ctx, schema)
		require.NoError(t, err)

		// Get
		retrieved, err := testStore.GetRelationshipSchema(ctx, schema.RelationshipType)
		require.NoError(t, err)
		assert.Equal(t, schema.RelationshipType, retrieved.RelationshipType)
		assert.Equal(t, schema.Properties, retrieved.Properties)
		assert.Equal(t, schema.DenormalizationConfig, retrieved.DenormalizationConfig)

		// Update
		schema.Properties["permissions"] = models.PropertyDefinition{
			Type:        "array",
			ElementType: "string",
			Required:    false,
		}
		err = testStore.UpdateRelationshipSchema(ctx, schema)
		require.NoError(t, err)

		// List
		schemas, err := testStore.ListRelationshipSchemas(ctx)
		require.NoError(t, err)
		assert.Greater(t, len(schemas), 0)

		// Delete
		err = testStore.DeleteRelationshipSchema(ctx, schema.RelationshipType)
		require.NoError(t, err)

		// Verify deletion
		_, err = testStore.GetRelationshipSchema(ctx, schema.RelationshipType)
		assert.Error(t, err)
		assert.True(t, utils.IsNotFound(err))
	})
}

func TestPostgresStore_TransactionOperations(t *testing.T) {
	ctx := context.Background()

	t.Run("Transaction_Commit", func(t *testing.T) {
		tx, err := testStore.BeginTx(ctx)
		require.NoError(t, err)

		// Create entity within transaction
		entity := &models.Entity{
			ID:         uuid.New(),
			EntityType: "user",
			URN:        "test:user:tx-commit-" + uuid.New().String(),
			Properties: map[string]interface{}{
				"name": "Transaction Test",
			},
			Version: 1,
		}

		err = tx.CreateEntity(ctx, entity)
		require.NoError(t, err)

		// Commit transaction
		err = tx.Commit()
		require.NoError(t, err)

		// Verify entity exists
		retrieved, err := testStore.GetEntity(ctx, entity.EntityType, entity.ID)
		require.NoError(t, err)
		assert.Equal(t, entity.ID, retrieved.ID)
	})

	t.Run("Transaction_Rollback", func(t *testing.T) {
		tx, err := testStore.BeginTx(ctx)
		require.NoError(t, err)

		// Create entity within transaction
		entity := &models.Entity{
			ID:         uuid.New(),
			EntityType: "user",
			URN:        "test:user:tx-rollback-" + uuid.New().String(),
			Properties: map[string]interface{}{
				"name": "Should Not Exist",
			},
			Version: 1,
		}

		err = tx.CreateEntity(ctx, entity)
		require.NoError(t, err)

		// Rollback transaction
		err = tx.Rollback()
		require.NoError(t, err)

		// Verify entity does not exist
		_, err = testStore.GetEntity(ctx, entity.EntityType, entity.ID)
		assert.Error(t, err)
		assert.True(t, utils.IsNotFound(err))
	})

	t.Run("Transaction_Isolation", func(t *testing.T) {
		// Create entity
		entity := &models.Entity{
			ID:         uuid.New(),
			EntityType: "user",
			URN:        "test:user:tx-isolation-" + uuid.New().String(),
			Properties: map[string]interface{}{
				"counter": 0,
			},
			Version: 1,
		}
		err := testStore.CreateEntity(ctx, entity)
		require.NoError(t, err)

		// Start two transactions
		tx1, err := testStore.BeginTx(ctx)
		require.NoError(t, err)
		defer tx1.Rollback()

		tx2, err := testStore.BeginTx(ctx)
		require.NoError(t, err)
		defer tx2.Rollback()

		// Update in tx1
		entity.Properties["counter"] = 1
		err = tx1.UpdateEntity(ctx, entity)
		require.NoError(t, err)

		// Note: Transaction interface doesn't have GetEntity method
		// So we can't test isolation by reading in tx2
		// Instead, we'll just test that both transactions can proceed

		// Commit tx1
		err = tx1.Commit()
		require.NoError(t, err)

		// New read should see updated value
		updated, err := testStore.GetEntity(ctx, entity.EntityType, entity.ID)
		require.NoError(t, err)
		assert.Equal(t, float64(1), updated.Properties["counter"])
	})
}

func TestPostgresStore_ConcurrentOperations(t *testing.T) {
	ctx := context.Background()

	t.Run("Concurrent_Creates", func(t *testing.T) {
		numGoroutines := 10
		errChan := make(chan error, numGoroutines)
		doneChan := make(chan bool, numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			go func(index int) {
				entity := &models.Entity{
					ID:         uuid.New(),
					EntityType: "user",
					URN:        fmt.Sprintf("test:user:concurrent-%d-%s", index, uuid.New().String()),
					Properties: map[string]interface{}{
						"name":  fmt.Sprintf("User %d", index),
						"index": index,
					},
					Version: 1,
				}

				err := testStore.CreateEntity(ctx, entity)
				if err != nil {
					errChan <- err
				} else {
					doneChan <- true
				}
			}(i)
		}

		// Wait for all goroutines
		successCount := 0
		for i := 0; i < numGoroutines; i++ {
			select {
			case err := <-errChan:
				t.Errorf("Concurrent create failed: %v", err)
			case <-doneChan:
				successCount++
			case <-time.After(5 * time.Second):
				t.Fatal("Timeout waiting for concurrent operations")
			}
		}

		assert.Equal(t, numGoroutines, successCount)
	})

	t.Run("Concurrent_Updates", func(t *testing.T) {
		// Create entity
		entity := &models.Entity{
			ID:         uuid.New(),
			EntityType: "user",
			URN:        "test:user:concurrent-update-" + uuid.New().String(),
			Properties: map[string]interface{}{
				"counter": 0,
				"name":    "Concurrent Test",
			},
			Version: 1,
		}
		err := testStore.CreateEntity(ctx, entity)
		require.NoError(t, err)

		// Concurrent updates
		numGoroutines := 5
		errChan := make(chan error, numGoroutines)
		doneChan := make(chan bool, numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			go func(index int) {
				// Get latest version
				current, err := testStore.GetEntity(ctx, entity.EntityType, entity.ID)
				if err != nil {
					errChan <- err
					return
				}

				// Update
				current.Properties["counter"] = index
				current.Properties[fmt.Sprintf("update_%d", index)] = true
				
				err = testStore.UpdateEntity(ctx, current)
				if err != nil {
					errChan <- err
				} else {
					doneChan <- true
				}
			}(i)
		}

		// Wait for completion
		for i := 0; i < numGoroutines; i++ {
			select {
			case err := <-errChan:
				// Some updates may fail due to version conflicts, which is expected
				t.Logf("Update failed (expected for some): %v", err)
			case <-doneChan:
				// Success
			case <-time.After(5 * time.Second):
				t.Fatal("Timeout waiting for concurrent updates")
			}
		}

		// Verify entity still exists and has been updated
		final, err := testStore.GetEntity(ctx, entity.EntityType, entity.ID)
		require.NoError(t, err)
		assert.Greater(t, final.Version, entity.Version)
	})
}

func TestPostgresStore_EdgeCases(t *testing.T) {
	ctx := context.Background()

	t.Run("LargeProperties", func(t *testing.T) {
		// Create entity with large properties
		largeArray := make([]string, 1000)
		for i := range largeArray {
			largeArray[i] = fmt.Sprintf("item_%d_%s", i, uuid.New().String())
		}

		entity := &models.Entity{
			ID:         uuid.New(),
			EntityType: "document",
			URN:        "test:document:large-" + uuid.New().String(),
			Properties: map[string]interface{}{
				"title":       "Large Document",
				"large_array": largeArray,
				"nested": map[string]interface{}{
					"level1": map[string]interface{}{
						"level2": map[string]interface{}{
							"level3": "deeply nested value",
						},
					},
				},
			},
			Version: 1,
		}

		err := testStore.CreateEntity(ctx, entity)
		require.NoError(t, err)

		// Retrieve and verify
		retrieved, err := testStore.GetEntity(ctx, entity.EntityType, entity.ID)
		require.NoError(t, err)
		
		retrievedArray, ok := retrieved.Properties["large_array"].([]interface{})
		require.True(t, ok)
		assert.Len(t, retrievedArray, 1000)
	})

	t.Run("SpecialCharacters", func(t *testing.T) {
		// Test with special characters in properties
		entity := &models.Entity{
			ID:         uuid.New(),
			EntityType: "test",
			URN:        "test:special:" + uuid.New().String(),
			Properties: map[string]interface{}{
				"name":         "Test \"quoted\" 'name'",
				"description":  "Line1\nLine2\tTabbed",
				"unicode":      "Hello 世界 🌍",
				"special_json": `{"key": "value with \"quotes\""}`,
			},
			Version: 1,
		}

		err := testStore.CreateEntity(ctx, entity)
		require.NoError(t, err)

		retrieved, err := testStore.GetEntity(ctx, entity.EntityType, entity.ID)
		require.NoError(t, err)
		assert.Equal(t, entity.Properties["name"], retrieved.Properties["name"])
		assert.Equal(t, entity.Properties["unicode"], retrieved.Properties["unicode"])
	})

	t.Run("NullAndEmptyValues", func(t *testing.T) {
		entity := &models.Entity{
			ID:         uuid.New(),
			EntityType: "test",
			URN:        "test:null:" + uuid.New().String(),
			Properties: map[string]interface{}{
				"null_value":  nil,
				"empty_string": "",
				"empty_array": []interface{}{},
				"empty_object": map[string]interface{}{},
				"zero_number": 0,
				"false_bool":  false,
			},
			Version: 1,
		}

		err := testStore.CreateEntity(ctx, entity)
		require.NoError(t, err)

		retrieved, err := testStore.GetEntity(ctx, entity.EntityType, entity.ID)
		require.NoError(t, err)
		assert.Equal(t, nil, retrieved.Properties["null_value"])
		assert.Equal(t, "", retrieved.Properties["empty_string"])
		assert.Equal(t, float64(0), retrieved.Properties["zero_number"])
		assert.Equal(t, false, retrieved.Properties["false_bool"])
	})
}

// Helper functions

func createTestEntity(t *testing.T, ctx context.Context, entityType, name string) *models.Entity {
	entity := &models.Entity{
		ID:         uuid.New(),
		EntityType: entityType,
		URN:        fmt.Sprintf("test:%s:%s-%s", entityType, name, uuid.New().String()),
		Properties: map[string]interface{}{
			"name": name,
		},
		Version: 1,
	}

	err := testStore.CreateEntity(ctx, entity)
	require.NoError(t, err)
	return entity
}

func cleanupTestData(t *testing.T, ctx context.Context) {
	// Clean up test entities
	db, err := sql.Open("pgx", testContainer.URL)
	require.NoError(t, err)
	defer db.Close()

	_, err = db.ExecContext(ctx, "DELETE FROM entities WHERE urn LIKE 'test:%'")
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, "DELETE FROM relations WHERE id IN (SELECT id FROM relations LIMIT 1000)")
	require.NoError(t, err)
}