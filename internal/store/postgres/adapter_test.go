package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/sumandas0/entropic/internal/models"
	"github.com/sumandas0/entropic/internal/store/testutils"
	"github.com/sumandas0/entropic/pkg/utils"
)

var (
	testContainer *testutils.PostgresTestContainer
	testStore     *PostgresStore
)

func TestMain(m *testing.M) {

	var err error
	testContainer, err = testutils.SetupTestPostgres()
	if err != nil {
		panic(err)
	}

	testStore, err = NewPostgresStore(testContainer.URL)
	if err != nil {
		testContainer.Cleanup()
		panic(err)
	}

	ctx := context.Background()
	migrator := NewMigrator(testStore.GetPool())
	if err := migrator.Run(ctx); err != nil {
		testStore.Close()
		testContainer.Cleanup()
		panic(err)
	}

	code := m.Run()

	testStore.Close()
	testContainer.Cleanup()

	if code != 0 {
		panic("tests failed")
	}
}

func TestPostgresStore_EntityOperations(t *testing.T) {
	ctx := context.Background()

	t.Run("CreateEntity", func(t *testing.T) {

		entity := &models.Entity{
			ID:         uuid.New(),
			EntityType: "user",
			URN:        "test:user:" + uuid.New().String(),
			Properties: map[string]any{
				"name":     "Test User",
				"email":    "test@example.com",
				"age":      30,
				"tags":     []string{"test", "user"},
				"metadata": map[string]any{"created_by": "test"},
			},
			Version: 1,
		}

		err := testStore.CreateEntity(ctx, entity)
		require.NoError(t, err)

		retrieved, err := testStore.GetEntity(ctx, entity.EntityType, entity.ID)
		require.NoError(t, err)
		assert.Equal(t, entity.ID, retrieved.ID)
		assert.Equal(t, entity.EntityType, retrieved.EntityType)
		assert.Equal(t, entity.URN, retrieved.URN)

		assert.Equal(t, entity.Properties["name"], retrieved.Properties["name"])
		assert.Equal(t, entity.Properties["email"], retrieved.Properties["email"])
		assert.Equal(t, float64(30), retrieved.Properties["age"])

		retrievedTags, ok := retrieved.Properties["tags"].([]any)
		require.True(t, ok, "tags should be an array")
		assert.Len(t, retrievedTags, 2)
		assert.Equal(t, "test", retrievedTags[0])
		assert.Equal(t, "user", retrievedTags[1])

		retrievedMetadata, ok := retrieved.Properties["metadata"].(map[string]any)
		require.True(t, ok, "metadata should be a map")
		assert.Equal(t, "test", retrievedMetadata["created_by"])

		assert.NotZero(t, retrieved.CreatedAt)
		assert.NotZero(t, retrieved.UpdatedAt)
	})

	t.Run("CreateEntity_WithVector", func(t *testing.T) {

		embedding := make([]float32, 384)
		for i := range embedding {
			embedding[i] = float32(i) / 384.0
		}

		entity := &models.Entity{
			ID:         uuid.New(),
			EntityType: "document",
			URN:        "test:document:" + uuid.New().String(),
			Properties: map[string]any{
				"title":     "Test Document",
				"content":   "This is a test document with vector embedding",
				"embedding": embedding,
			},
			Version: 1,
		}

		err := testStore.CreateEntity(ctx, entity)
		require.NoError(t, err)

		retrieved, err := testStore.GetEntity(ctx, entity.EntityType, entity.ID)
		require.NoError(t, err)

		retrievedEmbedding, ok := retrieved.Properties["embedding"].([]any)
		require.True(t, ok, "embedding should be an array")
		assert.Len(t, retrievedEmbedding, 384)
	})

	t.Run("UpdateEntity", func(t *testing.T) {

		entity := &models.Entity{
			ID:         uuid.New(),
			EntityType: "user",
			URN:        "test:user:" + uuid.New().String(),
			Properties: map[string]any{
				"name": "Original Name",
				"age":  25,
			},
			Version: 1,
		}

		err := testStore.CreateEntity(ctx, entity)
		require.NoError(t, err)

		entity.Properties["name"] = "Updated Name"
		entity.Properties["age"] = 26
		entity.Properties["new_field"] = "new value"

		err = testStore.UpdateEntity(ctx, entity)
		require.NoError(t, err)

		retrieved, err := testStore.GetEntity(ctx, entity.EntityType, entity.ID)
		require.NoError(t, err)
		assert.Equal(t, "Updated Name", retrieved.Properties["name"])
		assert.Equal(t, float64(26), retrieved.Properties["age"])
		assert.Equal(t, "new value", retrieved.Properties["new_field"])
		assert.Greater(t, retrieved.Version, entity.Version)
		assert.Greater(t, retrieved.UpdatedAt, entity.CreatedAt)
	})

	t.Run("DeleteEntity", func(t *testing.T) {

		entity := &models.Entity{
			ID:         uuid.New(),
			EntityType: "user",
			URN:        "test:user:" + uuid.New().String(),
			Properties: map[string]any{
				"name": "To Be Deleted",
			},
			Version: 1,
		}

		err := testStore.CreateEntity(ctx, entity)
		require.NoError(t, err)

		err = testStore.DeleteEntity(ctx, entity.EntityType, entity.ID)
		require.NoError(t, err)

		_, err = testStore.GetEntity(ctx, entity.EntityType, entity.ID)
		assert.Error(t, err)
		var appErr *utils.AppError
		assert.ErrorAs(t, err, &appErr)
		assert.True(t, utils.IsNotFound(err))
	})

	t.Run("ListEntities", func(t *testing.T) {

		cleanupTestData(t, ctx)

		entityType := "user"
		for i := 0; i < 5; i++ {
			entity := &models.Entity{
				ID:         uuid.New(),
				EntityType: entityType,
				URN:        fmt.Sprintf("test:user:list-%d", i),
				Properties: map[string]any{
					"name":  fmt.Sprintf("User %d", i),
					"index": i,
				},
				Version: 1,
			}
			err := testStore.CreateEntity(ctx, entity)
			require.NoError(t, err)
		}

		entities, err := testStore.ListEntities(ctx, entityType, 3, 0)
		require.NoError(t, err)
		assert.Len(t, entities, 3)

		page2, err := testStore.ListEntities(ctx, entityType, 3, 3)
		require.NoError(t, err)
		assert.Len(t, page2, 2)

		for _, e1 := range entities {
			for _, e2 := range page2 {
				assert.NotEqual(t, e1.ID, e2.ID)
			}
		}
	})

	t.Run("CheckURNExists", func(t *testing.T) {
		urn := "test:user:unique-" + uuid.New().String()

		exists, err := testStore.CheckURNExists(ctx, urn)
		require.NoError(t, err)
		assert.False(t, exists)

		entity := &models.Entity{
			ID:         uuid.New(),
			EntityType: "user",
			URN:        urn,
			Properties: map[string]any{"name": "Test"},
			Version:    1,
		}
		err = testStore.CreateEntity(ctx, entity)
		require.NoError(t, err)

		exists, err = testStore.CheckURNExists(ctx, urn)
		require.NoError(t, err)
		assert.True(t, exists)
	})
}

func TestPostgresStore_RelationOperations(t *testing.T) {
	ctx := context.Background()

	t.Run("CreateRelation", func(t *testing.T) {

		user := createTestEntity(t, ctx, "user", "relation-test-user")
		org := createTestEntity(t, ctx, "organization", "relation-test-org")

		relation := &models.Relation{
			ID:             uuid.New(),
			RelationType:   "member_of",
			FromEntityID:   user.ID,
			FromEntityType: user.EntityType,
			ToEntityID:     org.ID,
			ToEntityType:   org.EntityType,
			Properties: map[string]any{
				"role":      "admin",
				"joined_at": time.Now().Format(time.RFC3339),
			},
		}

		err := testStore.CreateRelation(ctx, relation)
		require.NoError(t, err)

		retrieved, err := testStore.GetRelation(ctx, relation.ID)
		require.NoError(t, err)
		assert.Equal(t, relation.ID, retrieved.ID)
		assert.Equal(t, relation.RelationType, retrieved.RelationType)
		assert.Equal(t, relation.FromEntityID, retrieved.FromEntityID)
		assert.Equal(t, relation.ToEntityID, retrieved.ToEntityID)
		assert.Equal(t, relation.Properties, retrieved.Properties)
	})

	t.Run("DeleteRelation", func(t *testing.T) {

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

		err = testStore.DeleteRelation(ctx, relation.ID)
		require.NoError(t, err)

		_, err = testStore.GetRelation(ctx, relation.ID)
		assert.Error(t, err)
		assert.True(t, utils.IsNotFound(err))
	})

	t.Run("GetRelationsByEntity", func(t *testing.T) {

		user := createTestEntity(t, ctx, "user", "relations-test-user")
		org1 := createTestEntity(t, ctx, "organization", "relations-test-org1")
		org2 := createTestEntity(t, ctx, "organization", "relations-test-org2")
		project := createTestEntity(t, ctx, "project", "relations-test-project")

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

		userRelations, err := testStore.GetRelationsByEntity(ctx, user.ID, nil)
		require.NoError(t, err)
		assert.Len(t, userRelations, 3)

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

		err := testStore.CreateEntitySchema(ctx, schema)
		require.NoError(t, err)

		retrieved, err := testStore.GetEntitySchema(ctx, schema.EntityType)
		require.NoError(t, err)
		assert.Equal(t, schema.EntityType, retrieved.EntityType)
		assert.Equal(t, schema.Properties, retrieved.Properties)

		schema.Properties["email"] = models.PropertyDefinition{
			Type:     "string",
			Required: true,
		}
		err = testStore.UpdateEntitySchema(ctx, schema)
		require.NoError(t, err)

		updated, err := testStore.GetEntitySchema(ctx, schema.EntityType)
		require.NoError(t, err)
		assert.Contains(t, updated.Properties, "email")

		schemas, err := testStore.ListEntitySchemas(ctx)
		require.NoError(t, err)
		assert.Greater(t, len(schemas), 0)

		err = testStore.DeleteEntitySchema(ctx, schema.EntityType)
		require.NoError(t, err)

		_, err = testStore.GetEntitySchema(ctx, schema.EntityType)
		assert.Error(t, err)
		assert.True(t, utils.IsNotFound(err))
	})

	t.Run("RelationshipSchema_CRUD", func(t *testing.T) {

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

		err := testStore.CreateRelationshipSchema(ctx, schema)
		require.NoError(t, err)

		retrieved, err := testStore.GetRelationshipSchema(ctx, schema.RelationshipType)
		require.NoError(t, err)
		assert.Equal(t, schema.RelationshipType, retrieved.RelationshipType)
		assert.Equal(t, schema.Properties, retrieved.Properties)
		assert.Equal(t, schema.DenormalizationConfig, retrieved.DenormalizationConfig)

		schema.Properties["permissions"] = models.PropertyDefinition{
			Type:        "array",
			ElementType: "string",
			Required:    false,
		}
		err = testStore.UpdateRelationshipSchema(ctx, schema)
		require.NoError(t, err)

		schemas, err := testStore.ListRelationshipSchemas(ctx)
		require.NoError(t, err)
		assert.Greater(t, len(schemas), 0)

		err = testStore.DeleteRelationshipSchema(ctx, schema.RelationshipType)
		require.NoError(t, err)

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

		entity := &models.Entity{
			ID:         uuid.New(),
			EntityType: "user",
			URN:        "test:user:tx-commit-" + uuid.New().String(),
			Properties: map[string]any{
				"name": "Transaction Test",
			},
			Version: 1,
		}

		err = tx.CreateEntity(ctx, entity)
		require.NoError(t, err)

		err = tx.Commit()
		require.NoError(t, err)

		retrieved, err := testStore.GetEntity(ctx, entity.EntityType, entity.ID)
		require.NoError(t, err)
		assert.Equal(t, entity.ID, retrieved.ID)
	})

	t.Run("Transaction_Rollback", func(t *testing.T) {
		tx, err := testStore.BeginTx(ctx)
		require.NoError(t, err)

		entity := &models.Entity{
			ID:         uuid.New(),
			EntityType: "user",
			URN:        "test:user:tx-rollback-" + uuid.New().String(),
			Properties: map[string]any{
				"name": "Should Not Exist",
			},
			Version: 1,
		}

		err = tx.CreateEntity(ctx, entity)
		require.NoError(t, err)

		err = tx.Rollback()
		require.NoError(t, err)

		_, err = testStore.GetEntity(ctx, entity.EntityType, entity.ID)
		assert.Error(t, err)
		assert.True(t, utils.IsNotFound(err))
	})

	t.Run("Transaction_Isolation", func(t *testing.T) {

		entity := &models.Entity{
			ID:         uuid.New(),
			EntityType: "user",
			URN:        "test:user:tx-isolation-" + uuid.New().String(),
			Properties: map[string]any{
				"counter": 0,
			},
			Version: 1,
		}
		err := testStore.CreateEntity(ctx, entity)
		require.NoError(t, err)

		tx1, err := testStore.BeginTx(ctx)
		require.NoError(t, err)
		defer tx1.Rollback()

		tx2, err := testStore.BeginTx(ctx)
		require.NoError(t, err)
		defer tx2.Rollback()

		entity.Properties["counter"] = 1
		err = tx1.UpdateEntity(ctx, entity)
		require.NoError(t, err)

		err = tx1.Commit()
		require.NoError(t, err)

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
					Properties: map[string]any{
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

		entity := &models.Entity{
			ID:         uuid.New(),
			EntityType: "user",
			URN:        "test:user:concurrent-update-" + uuid.New().String(),
			Properties: map[string]any{
				"counter": 0,
				"name":    "Concurrent Test",
			},
			Version: 1,
		}
		err := testStore.CreateEntity(ctx, entity)
		require.NoError(t, err)

		numGoroutines := 5
		errChan := make(chan error, numGoroutines)
		doneChan := make(chan bool, numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			go func(index int) {

				current, err := testStore.GetEntity(ctx, entity.EntityType, entity.ID)
				if err != nil {
					errChan <- err
					return
				}

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

		for i := 0; i < numGoroutines; i++ {
			select {
			case err := <-errChan:

				t.Logf("Update failed (expected for some): %v", err)
			case <-doneChan:

			case <-time.After(5 * time.Second):
				t.Fatal("Timeout waiting for concurrent updates")
			}
		}

		final, err := testStore.GetEntity(ctx, entity.EntityType, entity.ID)
		require.NoError(t, err)
		assert.Greater(t, final.Version, entity.Version)
	})
}

func TestPostgresStore_EdgeCases(t *testing.T) {
	ctx := context.Background()

	t.Run("LargeProperties", func(t *testing.T) {

		largeArray := make([]string, 1000)
		for i := range largeArray {
			largeArray[i] = fmt.Sprintf("item_%d_%s", i, uuid.New().String())
		}

		entity := &models.Entity{
			ID:         uuid.New(),
			EntityType: "document",
			URN:        "test:document:large-" + uuid.New().String(),
			Properties: map[string]any{
				"title":       "Large Document",
				"large_array": largeArray,
				"nested": map[string]any{
					"level1": map[string]any{
						"level2": map[string]any{
							"level3": "deeply nested value",
						},
					},
				},
			},
			Version: 1,
		}

		err := testStore.CreateEntity(ctx, entity)
		require.NoError(t, err)

		retrieved, err := testStore.GetEntity(ctx, entity.EntityType, entity.ID)
		require.NoError(t, err)

		retrievedArray, ok := retrieved.Properties["large_array"].([]any)
		require.True(t, ok)
		assert.Len(t, retrievedArray, 1000)
	})

	t.Run("SpecialCharacters", func(t *testing.T) {

		entity := &models.Entity{
			ID:         uuid.New(),
			EntityType: "test",
			URN:        "test:special:" + uuid.New().String(),
			Properties: map[string]any{
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
			Properties: map[string]any{
				"null_value":   nil,
				"empty_string": "",
				"empty_array":  []any{},
				"empty_object": map[string]any{},
				"zero_number":  0,
				"false_bool":   false,
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

func createTestEntity(t *testing.T, ctx context.Context, entityType, name string) *models.Entity {
	entity := &models.Entity{
		ID:         uuid.New(),
		EntityType: entityType,
		URN:        fmt.Sprintf("test:%s:%s-%s", entityType, name, uuid.New().String()),
		Properties: map[string]any{
			"name": name,
		},
		Version: 1,
	}

	err := testStore.CreateEntity(ctx, entity)
	require.NoError(t, err)
	return entity
}

func cleanupTestData(t *testing.T, ctx context.Context) {

	db, err := sql.Open("pgx", testContainer.URL)
	require.NoError(t, err)
	defer db.Close()

	_, err = db.ExecContext(ctx, "DELETE FROM entities WHERE urn LIKE 'test:%'")
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, "DELETE FROM relations WHERE id IN (SELECT id FROM relations LIMIT 1000)")
	require.NoError(t, err)
}
