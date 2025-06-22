package store_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/sumandas0/entropic/internal/models"
	"github.com/sumandas0/entropic/internal/store/postgres"
	"github.com/sumandas0/entropic/internal/store/testutils"
	"github.com/sumandas0/entropic/internal/store/typesense"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStoreIntegration(t *testing.T) {
	ctx := context.Background()

	pgContainer, err := testutils.SetupTestPostgres()
	require.NoError(t, err)
	defer pgContainer.Cleanup()

	tsContainer, err := testutils.SetupTestTypesense()
	require.NoError(t, err)
	defer tsContainer.Cleanup()

	primaryStore, err := postgres.NewPostgresStore(pgContainer.URL)
	require.NoError(t, err)
	defer primaryStore.Close()

	migrator := postgres.NewMigrator(primaryStore.GetPool())
	err = migrator.Run(ctx)
	require.NoError(t, err)

	indexStore, err := typesense.NewTypesenseStore(tsContainer.URL, tsContainer.APIKey)
	require.NoError(t, err)
	defer indexStore.Close()

	t.Run("TwoPhaseCommit_Success", func(t *testing.T) {
		
		schema := &models.EntitySchema{
			EntityType: "product",
			Properties: models.PropertySchema{
				"name": models.PropertyDefinition{
					Type:     "string",
					Required: true,
				},
				"price": models.PropertyDefinition{
					Type:     "number",
					Required: true,
				},
				"description": models.PropertyDefinition{
					Type:     "string",
					Required: false,
				},
			},
		}
		err := primaryStore.CreateEntitySchema(ctx, schema)
		require.NoError(t, err)

		err = indexStore.CreateCollection(ctx, schema.EntityType, schema)
		require.NoError(t, err)

		entity := &models.Entity{
			ID:         uuid.New(),
			EntityType: "product",
			URN:        "test:product:" + uuid.New().String(),
			Properties: map[string]interface{}{
				"name":        "Test Product",
				"price":       99.99,
				"description": "A test product for integration testing",
			},
			Version:   1,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		tx, err := primaryStore.BeginTx(ctx)
		require.NoError(t, err)

		err = tx.CreateEntity(ctx, entity)
		require.NoError(t, err)

		err = indexStore.IndexEntity(ctx, entity)
		require.NoError(t, err)

		err = tx.Commit()
		require.NoError(t, err)

		retrieved, err := primaryStore.GetEntity(ctx, entity.EntityType, entity.ID)
		require.NoError(t, err)
		assert.Equal(t, entity.ID, retrieved.ID)

		time.Sleep(200 * time.Millisecond)

		searchQuery := &models.SearchQuery{
			EntityTypes: []string{"product"},
			Query:       "Test Product",
			Limit:       10,
		}

		results, err := indexStore.Search(ctx, searchQuery)
		require.NoError(t, err)
		assert.Greater(t, len(results.Hits), 0)
		
		found := false
		for _, hit := range results.Hits {
			if hit.ID == entity.ID {
				found = true
				break
			}
		}
		assert.True(t, found, "Entity should be found in search results")
	})

	t.Run("TwoPhaseCommit_Rollback", func(t *testing.T) {
		
		entity := &models.Entity{
			ID:         uuid.New(),
			EntityType: "product",
			URN:        "test:product:rollback-" + uuid.New().String(),
			Properties: map[string]interface{}{
				"name":  "Rollback Product",
				"price": 199.99,
			},
			Version:   1,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		tx, err := primaryStore.BeginTx(ctx)
		require.NoError(t, err)

		err = tx.CreateEntity(ctx, entity)
		require.NoError(t, err)

		err = tx.Rollback()
		require.NoError(t, err)

		_, err = primaryStore.GetEntity(ctx, entity.EntityType, entity.ID)
		assert.Error(t, err)

	})

	t.Run("ConcurrentEntityOperations", func(t *testing.T) {
		
		numEntities := 10
		entities := make([]*models.Entity, numEntities)

		for i := 0; i < numEntities; i++ {
			entities[i] = &models.Entity{
				ID:         uuid.New(),
				EntityType: "product",
				URN:        fmt.Sprintf("test:product:concurrent-%d", i),
				Properties: map[string]interface{}{
					"name":  fmt.Sprintf("Concurrent Product %d", i),
					"price": float64(i * 10),
				},
				Version:   1,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
		}

		errChan := make(chan error, numEntities)
		doneChan := make(chan bool, numEntities)

		for i := 0; i < numEntities; i++ {
			go func(idx int) {
				entity := entities[idx]

				err := primaryStore.CreateEntity(ctx, entity)
				if err != nil {
					errChan <- err
					return
				}

				err = indexStore.IndexEntity(ctx, entity)
				if err != nil {
					errChan <- err
					return
				}

				doneChan <- true
			}(i)
		}

		successCount := 0
		for i := 0; i < numEntities; i++ {
			select {
			case err := <-errChan:
				t.Errorf("Concurrent operation failed: %v", err)
			case <-doneChan:
				successCount++
			case <-time.After(10 * time.Second):
				t.Fatal("Timeout waiting for concurrent operations")
			}
		}

		assert.Equal(t, numEntities, successCount)

		for _, entity := range entities {
			retrieved, err := primaryStore.GetEntity(ctx, entity.EntityType, entity.ID)
			require.NoError(t, err)
			assert.Equal(t, entity.ID, retrieved.ID)
		}

		time.Sleep(500 * time.Millisecond)

		searchQuery := &models.SearchQuery{
			EntityTypes: []string{"product"},
			Query:       "Concurrent Product",
			Limit:       20,
		}

		results, err := indexStore.Search(ctx, searchQuery)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(results.Hits), numEntities)
	})

	t.Run("UpdateConsistency", func(t *testing.T) {
		
		entity := &models.Entity{
			ID:         uuid.New(),
			EntityType: "product",
			URN:        "test:product:update-consistency",
			Properties: map[string]interface{}{
				"name":  "Original Product",
				"price": 50.00,
			},
			Version:   1,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		err := primaryStore.CreateEntity(ctx, entity)
		require.NoError(t, err)
		err = indexStore.IndexEntity(ctx, entity)
		require.NoError(t, err)

		time.Sleep(200 * time.Millisecond)

		entity.Properties["name"] = "Updated Product"
		entity.Properties["price"] = 75.00
		entity.Version = 2
		entity.UpdatedAt = time.Now()

		err = primaryStore.UpdateEntity(ctx, entity)
		require.NoError(t, err)
		err = indexStore.UpdateEntityIndex(ctx, entity)
		require.NoError(t, err)

		retrieved, err := primaryStore.GetEntity(ctx, entity.EntityType, entity.ID)
		require.NoError(t, err)
		assert.Equal(t, "Updated Product", retrieved.Properties["name"])
		assert.Equal(t, 75.00, retrieved.Properties["price"])

		time.Sleep(200 * time.Millisecond)

		searchQuery := &models.SearchQuery{
			EntityTypes: []string{"product"},
			Query:       "Updated Product",
			Limit:       10,
		}

		results, err := indexStore.Search(ctx, searchQuery)
		require.NoError(t, err)
		
		found := false
		for _, hit := range results.Hits {
			if hit.ID == entity.ID {
				found = true
				assert.Equal(t, "Updated Product", hit.Properties["name"])
				assert.Equal(t, 75.00, hit.Properties["price"])
				break
			}
		}
		assert.True(t, found, "Updated entity should be found in search results")
	})

	t.Run("DeleteConsistency", func(t *testing.T) {
		
		entity := &models.Entity{
			ID:         uuid.New(),
			EntityType: "product",
			URN:        "test:product:delete-consistency",
			Properties: map[string]interface{}{
				"name":  "Product to Delete",
				"price": 25.00,
			},
			Version:   1,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		err := primaryStore.CreateEntity(ctx, entity)
		require.NoError(t, err)
		err = indexStore.IndexEntity(ctx, entity)
		require.NoError(t, err)

		time.Sleep(200 * time.Millisecond)

		err = primaryStore.DeleteEntity(ctx, entity.EntityType, entity.ID)
		require.NoError(t, err)
		err = indexStore.DeleteEntityIndex(ctx, entity.EntityType, entity.ID)
		require.NoError(t, err)

		_, err = primaryStore.GetEntity(ctx, entity.EntityType, entity.ID)
		assert.Error(t, err)

		time.Sleep(200 * time.Millisecond)

		searchQuery := &models.SearchQuery{
			EntityTypes: []string{"product"},
			Query:       entity.URN,
			Limit:       10,
		}

		results, err := indexStore.Search(ctx, searchQuery)
		require.NoError(t, err)
		
		found := false
		for _, hit := range results.Hits {
			if hit.ID == entity.ID {
				found = true
				break
			}
		}
		assert.False(t, found, "Deleted entity should not be found in search results")
	})

	t.Run("VectorSearchIntegration", func(t *testing.T) {
		
		schema := &models.EntitySchema{
			EntityType: "document",
			Properties: models.PropertySchema{
				"title": models.PropertyDefinition{
					Type:     "string",
					Required: true,
				},
				"content": models.PropertyDefinition{
					Type:     "string",
					Required: true,
				},
				"embedding": models.PropertyDefinition{
					Type:      "vector",
					VectorDim: 384,
					Required:  true,
				},
			},
		}
		
		err := primaryStore.CreateEntitySchema(ctx, schema)
		require.NoError(t, err)
		err = indexStore.CreateCollection(ctx, schema.EntityType, schema)
		require.NoError(t, err)

		embedding1 := make([]float32, 384)
		embedding2 := make([]float32, 384)
		for i := range embedding1 {
			embedding1[i] = float32(i) / 384.0
			embedding2[i] = float32(i) / 768.0 
		}

		doc1 := &models.Entity{
			ID:         uuid.New(),
			EntityType: "document",
			URN:        "test:document:vector-1",
			Properties: map[string]interface{}{
				"title":     "Document 1",
				"content":   "First test document",
				"embedding": embedding1,
			},
			Version:   1,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		doc2 := &models.Entity{
			ID:         uuid.New(),
			EntityType: "document",
			URN:        "test:document:vector-2",
			Properties: map[string]interface{}{
				"title":     "Document 2",
				"content":   "Second test document",
				"embedding": embedding2,
			},
			Version:   1,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		err = primaryStore.CreateEntity(ctx, doc1)
		require.NoError(t, err)
		err = primaryStore.CreateEntity(ctx, doc2)
		require.NoError(t, err)

		err = indexStore.IndexEntity(ctx, doc1)
		require.NoError(t, err)
		err = indexStore.IndexEntity(ctx, doc2)
		require.NoError(t, err)

		time.Sleep(300 * time.Millisecond)

		vectorQuery := &models.VectorQuery{
			EntityTypes: []string{"document"},
			Vector:      embedding1, 
			VectorField: "embedding",
			TopK:        5,
		}

		results, err := indexStore.VectorSearch(ctx, vectorQuery)
		require.NoError(t, err)
		assert.Greater(t, len(results.Hits), 0)

		if len(results.Hits) > 0 {
			assert.Equal(t, doc1.ID, results.Hits[0].ID)
			assert.Greater(t, results.Hits[0].Score, float32(0.9))
		}
	})
}

func TestSchemaEvolution(t *testing.T) {
	ctx := context.Background()

	pgContainer, err := testutils.SetupTestPostgres()
	require.NoError(t, err)
	defer pgContainer.Cleanup()

	tsContainer, err := testutils.SetupTestTypesense()
	require.NoError(t, err)
	defer tsContainer.Cleanup()

	primaryStore, err := postgres.NewPostgresStore(pgContainer.URL)
	require.NoError(t, err)
	defer primaryStore.Close()

	migrator := postgres.NewMigrator(primaryStore.GetPool())
	err = migrator.Run(ctx)
	require.NoError(t, err)

	indexStore, err := typesense.NewTypesenseStore(tsContainer.URL, tsContainer.APIKey)
	require.NoError(t, err)
	defer indexStore.Close()

	t.Run("AddNewField", func(t *testing.T) {
		
		schema := &models.EntitySchema{
			EntityType: "user",
			Properties: models.PropertySchema{
				"name": models.PropertyDefinition{
					Type:     "string",
					Required: true,
				},
				"email": models.PropertyDefinition{
					Type:     "string",
					Required: true,
				},
			},
		}

		err := primaryStore.CreateEntitySchema(ctx, schema)
		require.NoError(t, err)
		err = indexStore.CreateCollection(ctx, schema.EntityType, schema)
		require.NoError(t, err)

		entity := &models.Entity{
			ID:         uuid.New(),
			EntityType: "user",
			URN:        "test:user:schema-evolution",
			Properties: map[string]interface{}{
				"name":  "Test User",
				"email": "test@example.com",
			},
			Version:   1,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		err = primaryStore.CreateEntity(ctx, entity)
		require.NoError(t, err)
		err = indexStore.IndexEntity(ctx, entity)
		require.NoError(t, err)

		schema.Properties["age"] = models.PropertyDefinition{
			Type:     "number",
			Required: false,
		}

		err = primaryStore.UpdateEntitySchema(ctx, schema)
		require.NoError(t, err)
		err = indexStore.UpdateCollection(ctx, schema.EntityType, schema)
		require.NoError(t, err)

		newEntity := &models.Entity{
			ID:         uuid.New(),
			EntityType: "user",
			URN:        "test:user:with-age",
			Properties: map[string]interface{}{
				"name":  "New User",
				"email": "new@example.com",
				"age":   30,
			},
			Version:   1,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		err = primaryStore.CreateEntity(ctx, newEntity)
		require.NoError(t, err)
		err = indexStore.IndexEntity(ctx, newEntity)
		require.NoError(t, err)

		retrieved1, err := primaryStore.GetEntity(ctx, entity.EntityType, entity.ID)
		require.NoError(t, err)
		assert.Equal(t, entity.Properties["name"], retrieved1.Properties["name"])

		retrieved2, err := primaryStore.GetEntity(ctx, newEntity.EntityType, newEntity.ID)
		require.NoError(t, err)
		assert.Equal(t, float64(30), retrieved2.Properties["age"])
	})
}