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

// TestStoreIntegration tests the interaction between primary and index stores
func TestStoreIntegration(t *testing.T) {
	ctx := context.Background()

	// Setup containers
	pgContainer, err := testutils.SetupTestPostgres()
	require.NoError(t, err)
	defer pgContainer.Cleanup()

	tsContainer, err := testutils.SetupTestTypesense()
	require.NoError(t, err)
	defer tsContainer.Cleanup()

	// Initialize stores
	primaryStore, err := postgres.NewPostgresStore(pgContainer.URL)
	require.NoError(t, err)
	defer primaryStore.Close()

	// Run migrations
	migrator := postgres.NewMigrator(primaryStore.GetPool())
	err = migrator.Run(ctx)
	require.NoError(t, err)

	indexStore, err := typesense.NewTypesenseStore(tsContainer.URL, tsContainer.APIKey)
	require.NoError(t, err)
	defer indexStore.Close()

	t.Run("TwoPhaseCommit_Success", func(t *testing.T) {
		// Create entity schema first
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

		// Create collection in index store
		err = indexStore.CreateCollection(ctx, schema.EntityType, schema)
		require.NoError(t, err)

		// Simulate two-phase commit
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

		// Phase 1: Begin transaction and insert into primary store
		tx, err := primaryStore.BeginTx(ctx)
		require.NoError(t, err)

		err = tx.CreateEntity(ctx, entity)
		require.NoError(t, err)

		// Phase 2: Index entity
		err = indexStore.IndexEntity(ctx, entity)
		require.NoError(t, err)

		// Commit transaction
		err = tx.Commit()
		require.NoError(t, err)

		// Verify entity exists in both stores
		retrieved, err := primaryStore.GetEntity(ctx, entity.EntityType, entity.ID)
		require.NoError(t, err)
		assert.Equal(t, entity.ID, retrieved.ID)

		// Wait for indexing
		time.Sleep(200 * time.Millisecond)

		// Search for entity
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
		// Create entity for rollback test
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

		// Begin transaction
		tx, err := primaryStore.BeginTx(ctx)
		require.NoError(t, err)

		// Insert into primary store
		err = tx.CreateEntity(ctx, entity)
		require.NoError(t, err)

		// Simulate failure before indexing - rollback transaction
		err = tx.Rollback()
		require.NoError(t, err)

		// Verify entity does not exist in primary store
		_, err = primaryStore.GetEntity(ctx, entity.EntityType, entity.ID)
		assert.Error(t, err)

		// Since we rolled back before indexing, entity should not be indexed
		// This maintains consistency between stores
	})

	t.Run("ConcurrentEntityOperations", func(t *testing.T) {
		// Test concurrent operations between stores
		numEntities := 10
		entities := make([]*models.Entity, numEntities)

		// Create entities
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

		// Create entities concurrently
		errChan := make(chan error, numEntities)
		doneChan := make(chan bool, numEntities)

		for i := 0; i < numEntities; i++ {
			go func(idx int) {
				entity := entities[idx]

				// Create in primary store
				err := primaryStore.CreateEntity(ctx, entity)
				if err != nil {
					errChan <- err
					return
				}

				// Index entity
				err = indexStore.IndexEntity(ctx, entity)
				if err != nil {
					errChan <- err
					return
				}

				doneChan <- true
			}(i)
		}

		// Wait for all operations
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

		// Verify all entities exist in primary store
		for _, entity := range entities {
			retrieved, err := primaryStore.GetEntity(ctx, entity.EntityType, entity.ID)
			require.NoError(t, err)
			assert.Equal(t, entity.ID, retrieved.ID)
		}

		// Wait for indexing
		time.Sleep(500 * time.Millisecond)

		// Verify all entities are searchable
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
		// Create entity
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

		// Create in both stores
		err := primaryStore.CreateEntity(ctx, entity)
		require.NoError(t, err)
		err = indexStore.IndexEntity(ctx, entity)
		require.NoError(t, err)

		// Wait for indexing
		time.Sleep(200 * time.Millisecond)

		// Update entity
		entity.Properties["name"] = "Updated Product"
		entity.Properties["price"] = 75.00
		entity.Version = 2
		entity.UpdatedAt = time.Now()

		// Update in both stores
		err = primaryStore.UpdateEntity(ctx, entity)
		require.NoError(t, err)
		err = indexStore.UpdateEntityIndex(ctx, entity)
		require.NoError(t, err)

		// Verify update in primary store
		retrieved, err := primaryStore.GetEntity(ctx, entity.EntityType, entity.ID)
		require.NoError(t, err)
		assert.Equal(t, "Updated Product", retrieved.Properties["name"])
		assert.Equal(t, 75.00, retrieved.Properties["price"])

		// Wait for index update
		time.Sleep(200 * time.Millisecond)

		// Verify update in search
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
		// Create entity
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

		// Create in both stores
		err := primaryStore.CreateEntity(ctx, entity)
		require.NoError(t, err)
		err = indexStore.IndexEntity(ctx, entity)
		require.NoError(t, err)

		// Wait for indexing
		time.Sleep(200 * time.Millisecond)

		// Delete from both stores
		err = primaryStore.DeleteEntity(ctx, entity.EntityType, entity.ID)
		require.NoError(t, err)
		err = indexStore.DeleteEntityIndex(ctx, entity.EntityType, entity.ID)
		require.NoError(t, err)

		// Verify deletion from primary store
		_, err = primaryStore.GetEntity(ctx, entity.EntityType, entity.ID)
		assert.Error(t, err)

		// Wait for index deletion
		time.Sleep(200 * time.Millisecond)

		// Verify deletion from search
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
		// Create entity schema with vector field
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

		// Create documents with embeddings
		embedding1 := make([]float32, 384)
		embedding2 := make([]float32, 384)
		for i := range embedding1 {
			embedding1[i] = float32(i) / 384.0
			embedding2[i] = float32(i) / 768.0 // Different pattern
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

		// Create documents
		err = primaryStore.CreateEntity(ctx, doc1)
		require.NoError(t, err)
		err = primaryStore.CreateEntity(ctx, doc2)
		require.NoError(t, err)

		err = indexStore.IndexEntity(ctx, doc1)
		require.NoError(t, err)
		err = indexStore.IndexEntity(ctx, doc2)
		require.NoError(t, err)

		// Wait for indexing
		time.Sleep(300 * time.Millisecond)

		// Perform vector search
		vectorQuery := &models.VectorQuery{
			EntityTypes: []string{"document"},
			Vector:      embedding1, // Search for similar to doc1
			VectorField: "embedding",
			TopK:        5,
		}

		results, err := indexStore.VectorSearch(ctx, vectorQuery)
		require.NoError(t, err)
		assert.Greater(t, len(results.Hits), 0)

		// The most similar should be doc1 itself
		if len(results.Hits) > 0 {
			assert.Equal(t, doc1.ID, results.Hits[0].ID)
			assert.Greater(t, results.Hits[0].Score, float32(0.9))
		}
	})
}

// TestSchemaEvolution tests schema changes between stores
func TestSchemaEvolution(t *testing.T) {
	ctx := context.Background()

	// Setup containers
	pgContainer, err := testutils.SetupTestPostgres()
	require.NoError(t, err)
	defer pgContainer.Cleanup()

	tsContainer, err := testutils.SetupTestTypesense()
	require.NoError(t, err)
	defer tsContainer.Cleanup()

	// Initialize stores
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
		// Create initial schema
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

		// Create entity with initial schema
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

		// Update schema to add new field
		schema.Properties["age"] = models.PropertyDefinition{
			Type:     "number",
			Required: false,
		}

		err = primaryStore.UpdateEntitySchema(ctx, schema)
		require.NoError(t, err)
		err = indexStore.UpdateCollection(ctx, schema.EntityType, schema)
		require.NoError(t, err)

		// Create new entity with new field
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

		// Both entities should still work
		retrieved1, err := primaryStore.GetEntity(ctx, entity.EntityType, entity.ID)
		require.NoError(t, err)
		assert.Equal(t, entity.Properties["name"], retrieved1.Properties["name"])

		retrieved2, err := primaryStore.GetEntity(ctx, newEntity.EntityType, newEntity.ID)
		require.NoError(t, err)
		assert.Equal(t, float64(30), retrieved2.Properties["age"])
	})
}