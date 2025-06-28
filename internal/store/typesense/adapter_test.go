package typesense

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/sumandas0/entropic/internal/models"
	"github.com/sumandas0/entropic/internal/store/testutils"
)

var (
	testContainer *testutils.TypesenseTestContainer
	testStore     *TypesenseStore
)

func TestMain(m *testing.M) {

	var err error
	testContainer, err = testutils.SetupTestTypesense()
	if err != nil {
		panic(err)
	}

	testStore, err = NewTypesenseStore(testContainer.URL, testContainer.APIKey)
	if err != nil {
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

func TestTypesenseStore_IndexEntity(t *testing.T) {
	ctx := context.Background()

	t.Run("IndexEntity_Basic", func(t *testing.T) {

		entity := &models.Entity{
			ID:         uuid.New(),
			EntityType: "user",
			URN:        "test:user:" + uuid.New().String(),
			Properties: map[string]any{
				"name":   "Test User",
				"email":  "test@example.com",
				"age":    30,
				"tags":   []string{"test", "user"},
				"active": true,
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		err := testStore.IndexEntity(ctx, entity)
		require.NoError(t, err)

		time.Sleep(100 * time.Millisecond)

		query := &models.SearchQuery{
			EntityTypes: []string{"user"},
			Query:       "Test User",
			Limit:       10,
		}

		results, err := testStore.Search(ctx, query)
		require.NoError(t, err)
		assert.Greater(t, len(results.Hits), 0)

		found := false
		for _, hit := range results.Hits {
			if hit.ID == entity.ID {
				found = true
				assert.Equal(t, entity.EntityType, hit.EntityType)
				assert.Equal(t, entity.URN, hit.URN)
				assert.Equal(t, entity.Properties["name"], hit.Properties["name"])
				break
			}
		}
		assert.True(t, found, "Indexed entity should be found in search results")
	})

	t.Run("IndexEntity_WithVector", func(t *testing.T) {

		embedding := make([]float32, 384)
		for i := range embedding {
			embedding[i] = float32(i) / 384.0
		}

		entity := &models.Entity{
			ID:         uuid.New(),
			EntityType: "document",
			URN:        "test:document:" + uuid.New().String(),
			Properties: map[string]any{
				"title":     "Vector Document",
				"content":   "This is a document with vector embedding",
				"embedding": embedding,
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		err := testStore.IndexEntity(ctx, entity)
		require.NoError(t, err)

		time.Sleep(100 * time.Millisecond)

		vectorQuery := &models.VectorQuery{
			EntityTypes: []string{"document"},
			Vector:      embedding,
			VectorField: "embedding",
			TopK:        5,
		}

		results, err := testStore.VectorSearch(ctx, vectorQuery)
		require.NoError(t, err)
		assert.Greater(t, len(results.Hits), 0)

		if len(results.Hits) > 0 {

			assert.Equal(t, entity.ID, results.Hits[0].ID)
			assert.Greater(t, results.Hits[0].Score, float32(0.9))
		}
	})

	t.Run("IndexEntity_Update", func(t *testing.T) {

		entity := &models.Entity{
			ID:         uuid.New(),
			EntityType: "user",
			URN:        "test:user:update-" + uuid.New().String(),
			Properties: map[string]any{
				"name":  "Original Name",
				"email": "original@example.com",
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		err := testStore.IndexEntity(ctx, entity)
		require.NoError(t, err)
		time.Sleep(100 * time.Millisecond)

		entity.Properties["name"] = "Updated Name"
		entity.Properties["email"] = "updated@example.com"
		entity.UpdatedAt = time.Now()

		err = testStore.IndexEntity(ctx, entity)
		require.NoError(t, err)
		time.Sleep(100 * time.Millisecond)

		query := &models.SearchQuery{
			EntityTypes: []string{"user"},
			Query:       "Updated Name",
			Limit:       10,
		}

		results, err := testStore.Search(ctx, query)
		require.NoError(t, err)

		found := false
		for _, hit := range results.Hits {
			if hit.ID == entity.ID {
				found = true
				assert.Equal(t, "Updated Name", hit.Properties["name"])
				assert.Equal(t, "updated@example.com", hit.Properties["email"])
				break
			}
		}
		assert.True(t, found, "Updated entity should be found with new values")
	})

	t.Run("IndexEntity_ComplexProperties", func(t *testing.T) {

		entity := &models.Entity{
			ID:         uuid.New(),
			EntityType: "product",
			URN:        "test:product:" + uuid.New().String(),
			Properties: map[string]any{
				"name":        "Complex Product",
				"description": "A product with complex properties",
				"price":       99.99,
				"categories":  []string{"electronics", "computers", "laptops"},
				"specs": map[string]any{
					"cpu": "Intel i7",
					"ram": 16,
					"storage": map[string]any{
						"type":     "SSD",
						"capacity": "512GB",
					},
				},
				"reviews": []map[string]any{
					{
						"user":   "user1",
						"rating": 5,
						"text":   "Great product!",
					},
					{
						"user":   "user2",
						"rating": 4,
						"text":   "Good value",
					},
				},
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		err := testStore.IndexEntity(ctx, entity)
		require.NoError(t, err)
		time.Sleep(100 * time.Millisecond)

		query := &models.SearchQuery{
			EntityTypes: []string{"product"},
			Query:       "Complex Product",
			Limit:       10,
		}

		results, err := testStore.Search(ctx, query)
		require.NoError(t, err)
		assert.Greater(t, len(results.Hits), 0)
	})
}

func TestTypesenseStore_DeleteEntity(t *testing.T) {
	ctx := context.Background()

	t.Run("DeleteEntity_Success", func(t *testing.T) {

		entity := &models.Entity{
			ID:         uuid.New(),
			EntityType: "user",
			URN:        "test:user:delete-" + uuid.New().String(),
			Properties: map[string]any{
				"name": "To Be Deleted",
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		err := testStore.IndexEntity(ctx, entity)
		require.NoError(t, err)
		time.Sleep(100 * time.Millisecond)

		err = testStore.DeleteEntityIndex(ctx, entity.EntityType, entity.ID)
		require.NoError(t, err)
		time.Sleep(100 * time.Millisecond)

		query := &models.SearchQuery{
			EntityTypes: []string{"user"},
			Query:       entity.URN,
			Limit:       10,
		}

		results, err := testStore.Search(ctx, query)
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

	t.Run("DeleteEntity_NonExistent", func(t *testing.T) {

		err := testStore.DeleteEntityIndex(ctx, "user", uuid.New())

		assert.NoError(t, err)
	})
}

func TestTypesenseStore_Search(t *testing.T) {
	ctx := context.Background()

	setupSearchTestData(t, ctx)

	t.Run("Search_Basic", func(t *testing.T) {
		query := &models.SearchQuery{
			EntityTypes: []string{"user"},
			Query:       "John",
			Limit:       10,
		}

		results, err := testStore.Search(ctx, query)
		require.NoError(t, err)
		assert.Greater(t, len(results.Hits), 0)
		assert.Greater(t, results.TotalHits, int64(0))

		for _, hit := range results.Hits {
			assert.Equal(t, "user", hit.EntityType)
			assert.Contains(t, fmt.Sprintf("%v", hit.Properties["name"]), "John")
		}
	})

	t.Run("Search_WithFilters", func(t *testing.T) {
		query := &models.SearchQuery{
			EntityTypes: []string{"user"},
			Query:       "*",
			Filters: map[string]any{
				"age": map[string]any{
					">=": 25,
					"<=": 35,
				},
			},
			Limit: 20,
		}

		results, err := testStore.Search(ctx, query)
		require.NoError(t, err)

		for _, hit := range results.Hits {
			age, ok := hit.Properties["age"].(float64)
			assert.True(t, ok, "age should be a number")
			assert.GreaterOrEqual(t, age, float64(25))
			assert.LessOrEqual(t, age, float64(35))
		}
	})

	t.Run("Search_WithFacets", func(t *testing.T) {
		query := &models.SearchQuery{
			EntityTypes: []string{"user"},
			Query:       "*",
			Facets:      []string{"department", "active"},
			Limit:       10,
		}

		results, err := testStore.Search(ctx, query)
		require.NoError(t, err)
		assert.NotNil(t, results.Facets)

		if deptFacet, ok := results.Facets["department"]; ok {
			assert.Greater(t, len(deptFacet), 0)
			for _, facetValue := range deptFacet {
				assert.NotEmpty(t, facetValue.Value)
				assert.Greater(t, facetValue.Count, int64(0))
			}
		}
	})

	t.Run("Search_WithSort", func(t *testing.T) {
		query := &models.SearchQuery{
			EntityTypes: []string{"user"},
			Query:       "*",
			Sort: []models.SortOption{
				{Field: "age", Order: "desc"},
			},
			Limit: 10,
		}

		results, err := testStore.Search(ctx, query)
		require.NoError(t, err)
		assert.Greater(t, len(results.Hits), 1)

		for i := 1; i < len(results.Hits); i++ {
			prevAge, _ := results.Hits[i-1].Properties["age"].(float64)
			currAge, _ := results.Hits[i].Properties["age"].(float64)
			assert.GreaterOrEqual(t, prevAge, currAge, "Results should be sorted by age descending")
		}
	})

	t.Run("Search_Pagination", func(t *testing.T) {

		query1 := &models.SearchQuery{
			EntityTypes: []string{"user"},
			Query:       "*",
			Limit:       5,
			Offset:      0,
		}

		results1, err := testStore.Search(ctx, query1)
		require.NoError(t, err)
		assert.Len(t, results1.Hits, 5)

		query2 := &models.SearchQuery{
			EntityTypes: []string{"user"},
			Query:       "*",
			Limit:       5,
			Offset:      5,
		}

		results2, err := testStore.Search(ctx, query2)
		require.NoError(t, err)
		assert.Greater(t, len(results2.Hits), 0)

		page1IDs := make(map[uuid.UUID]bool)
		for _, hit := range results1.Hits {
			page1IDs[hit.ID] = true
		}

		for _, hit := range results2.Hits {
			assert.False(t, page1IDs[hit.ID], "Pages should not have overlapping results")
		}
	})

	t.Run("Search_MultipleEntityTypes", func(t *testing.T) {

		for i := 0; i < 3; i++ {
			product := &models.Entity{
				ID:         uuid.New(),
				EntityType: "product",
				URN:        fmt.Sprintf("test:product:search-%d", i),
				Properties: map[string]any{
					"name":  fmt.Sprintf("Product %d", i),
					"price": float64(i * 10),
				},
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
			err := testStore.IndexEntity(ctx, product)
			require.NoError(t, err)
		}
		time.Sleep(100 * time.Millisecond)

		query := &models.SearchQuery{
			EntityTypes: []string{"user", "product"},
			Query:       "*",
			Limit:       20,
		}

		results, err := testStore.Search(ctx, query)
		require.NoError(t, err)

		hasUser := false
		hasProduct := false
		for _, hit := range results.Hits {
			if hit.EntityType == "user" {
				hasUser = true
			}
			if hit.EntityType == "product" {
				hasProduct = true
			}
		}
		assert.True(t, hasUser, "Should have user entities")
		assert.True(t, hasProduct, "Should have product entities")
	})

	t.Run("Search_IncludeURN", func(t *testing.T) {
		query := &models.SearchQuery{
			EntityTypes: []string{"user"},
			Query:       "*",
			Limit:       5,
			IncludeURN:  true,
		}

		results, err := testStore.Search(ctx, query)
		require.NoError(t, err)

		for _, hit := range results.Hits {
			assert.NotEmpty(t, hit.URN, "URN should be included when requested")
		}
	})
}

func TestTypesenseStore_VectorSearch(t *testing.T) {
	ctx := context.Background()

	setupVectorTestData(t, ctx)

	t.Run("VectorSearch_Basic", func(t *testing.T) {

		queryVector := make([]float32, 384)
		for i := range queryVector {
			queryVector[i] = float32(i) / 384.0
		}

		query := &models.VectorQuery{
			EntityTypes: []string{"document"},
			Vector:      queryVector,
			VectorField: "embedding",
			TopK:        5,
		}

		results, err := testStore.VectorSearch(ctx, query)
		require.NoError(t, err)
		assert.Greater(t, len(results.Hits), 0)

		for i := 1; i < len(results.Hits); i++ {
			assert.GreaterOrEqual(t, results.Hits[i-1].Score, results.Hits[i].Score)
		}
	})

	t.Run("VectorSearch_WithFilters", func(t *testing.T) {
		queryVector := make([]float32, 384)
		for i := range queryVector {
			queryVector[i] = 0.5
		}

		query := &models.VectorQuery{
			EntityTypes: []string{"document"},
			Vector:      queryVector,
			VectorField: "embedding",
			TopK:        10,
			Filters: map[string]any{
				"category": "technical",
			},
		}

		results, err := testStore.VectorSearch(ctx, query)
		require.NoError(t, err)

		for _, hit := range results.Hits {
			assert.Equal(t, "technical", hit.Properties["category"])
		}
	})

	t.Run("VectorSearch_MinScore", func(t *testing.T) {
		queryVector := make([]float32, 384)
		for i := range queryVector {
			queryVector[i] = 0.1
		}

		query := &models.VectorQuery{
			EntityTypes: []string{"document"},
			Vector:      queryVector,
			VectorField: "embedding",
			TopK:        100,
			MinScore:    0.5,
		}

		results, err := testStore.VectorSearch(ctx, query)
		require.NoError(t, err)

		for _, hit := range results.Hits {
			assert.GreaterOrEqual(t, hit.Score, float32(0.5))
		}
	})

	t.Run("VectorSearch_IncludeVectors", func(t *testing.T) {
		queryVector := make([]float32, 384)
		for i := range queryVector {
			queryVector[i] = 0.5
		}

		query := &models.VectorQuery{
			EntityTypes:    []string{"document"},
			Vector:         queryVector,
			VectorField:    "embedding",
			TopK:           3,
			IncludeVectors: true,
		}

		results, err := testStore.VectorSearch(ctx, query)
		require.NoError(t, err)

		for _, hit := range results.Hits {
			assert.NotNil(t, hit.Vector)
			assert.Len(t, hit.Vector, 384)
		}
	})
}

func TestTypesenseStore_CollectionManagement(t *testing.T) {
	ctx := context.Background()

	t.Run("Collection_CreateIfNotExists", func(t *testing.T) {

		entity := &models.Entity{
			ID:         uuid.New(),
			EntityType: "test_collection_" + uuid.New().String()[:8],
			URN:        "test:collection:" + uuid.New().String(),
			Properties: map[string]any{
				"name": "Test Collection Entity",
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		err := testStore.IndexEntity(ctx, entity)
		require.NoError(t, err)

		time.Sleep(100 * time.Millisecond)

		query := &models.SearchQuery{
			EntityTypes: []string{entity.EntityType},
			Query:       "*",
			Limit:       10,
		}

		results, err := testStore.Search(ctx, query)
		require.NoError(t, err)
		assert.Equal(t, int64(1), results.TotalHits)
	})

	t.Run("Collection_DeleteByType", func(t *testing.T) {
		entityType := "temp_type_" + uuid.New().String()[:8]

		for i := 0; i < 5; i++ {
			entity := &models.Entity{
				ID:         uuid.New(),
				EntityType: entityType,
				URN:        fmt.Sprintf("test:temp:%d", i),
				Properties: map[string]any{
					"name":  fmt.Sprintf("Temp Entity %d", i),
					"index": i,
				},
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
			err := testStore.IndexEntity(ctx, entity)
			require.NoError(t, err)
		}

		time.Sleep(200 * time.Millisecond)

		err := testStore.DeleteCollection(ctx, entityType)
		require.NoError(t, err)

		time.Sleep(200 * time.Millisecond)

		query := &models.SearchQuery{
			EntityTypes: []string{entityType},
			Query:       "*",
			Limit:       10,
		}

		results, err := testStore.Search(ctx, query)
		require.NoError(t, err)
		assert.Equal(t, int64(0), results.TotalHits)
	})
}

func TestTypesenseStore_ErrorHandling(t *testing.T) {
	ctx := context.Background()

	t.Run("Search_InvalidEntityType", func(t *testing.T) {
		query := &models.SearchQuery{
			EntityTypes: []string{},
			Query:       "test",
			Limit:       10,
		}

		_, err := testStore.Search(ctx, query)
		assert.Error(t, err)
	})

	t.Run("VectorSearch_InvalidDimensions", func(t *testing.T) {

		embedding := make([]float32, 384)
		for i := range embedding {
			embedding[i] = float32(i) / 384.0
		}

		entity := &models.Entity{
			ID:         uuid.New(),
			EntityType: "document",
			URN:        "test:document:wrong-dim",
			Properties: map[string]any{
				"title":     "Test Doc",
				"embedding": embedding,
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		err := testStore.IndexEntity(ctx, entity)
		require.NoError(t, err)
		time.Sleep(100 * time.Millisecond)

		wrongVector := make([]float32, 128)
		query := &models.VectorQuery{
			EntityTypes: []string{"document"},
			Vector:      wrongVector,
			VectorField: "embedding",
			TopK:        5,
		}

		_, err = testStore.VectorSearch(ctx, query)
		assert.Error(t, err)
	})

	t.Run("Search_InvalidLimit", func(t *testing.T) {
		query := &models.SearchQuery{
			EntityTypes: []string{"user"},
			Query:       "*",
			Limit:       0,
		}

		_, err := testStore.Search(ctx, query)
		assert.Error(t, err)

		query.Limit = 1001
		_, err = testStore.Search(ctx, query)
		assert.Error(t, err)
	})
}

func TestTypesenseStore_ConcurrentOperations(t *testing.T) {
	ctx := context.Background()

	t.Run("Concurrent_IndexOperations", func(t *testing.T) {
		numGoroutines := 10
		errChan := make(chan error, numGoroutines)
		doneChan := make(chan bool, numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			go func(index int) {
				entity := &models.Entity{
					ID:         uuid.New(),
					EntityType: "user",
					URN:        fmt.Sprintf("test:concurrent:%d-%s", index, uuid.New().String()),
					Properties: map[string]any{
						"name":  fmt.Sprintf("Concurrent User %d", index),
						"index": index,
					},
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				}

				err := testStore.IndexEntity(ctx, entity)
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
				t.Errorf("Concurrent index failed: %v", err)
			case <-doneChan:
				successCount++
			case <-time.After(10 * time.Second):
				t.Fatal("Timeout waiting for concurrent operations")
			}
		}

		assert.Equal(t, numGoroutines, successCount)
	})

	t.Run("Concurrent_SearchOperations", func(t *testing.T) {

		entity := &models.Entity{
			ID:         uuid.New(),
			EntityType: "user",
			URN:        "test:concurrent:search",
			Properties: map[string]any{
				"name": "Search Target",
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		err := testStore.IndexEntity(ctx, entity)
		require.NoError(t, err)
		time.Sleep(200 * time.Millisecond)

		numGoroutines := 20
		errChan := make(chan error, numGoroutines)
		resultsChan := make(chan *models.SearchResult, numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			go func(index int) {
				query := &models.SearchQuery{
					EntityTypes: []string{"user"},
					Query:       "Search Target",
					Limit:       10,
				}

				results, err := testStore.Search(ctx, query)
				if err != nil {
					errChan <- err
				} else {
					resultsChan <- results
				}
			}(i)
		}

		successCount := 0
		for i := 0; i < numGoroutines; i++ {
			select {
			case err := <-errChan:
				t.Errorf("Concurrent search failed: %v", err)
			case results := <-resultsChan:
				successCount++
				assert.Greater(t, len(results.Hits), 0)
			case <-time.After(10 * time.Second):
				t.Fatal("Timeout waiting for concurrent searches")
			}
		}

		assert.Equal(t, numGoroutines, successCount)
	})
}

func setupSearchTestData(t *testing.T, ctx context.Context) {
	departments := []string{"engineering", "sales", "marketing", "hr"}

	for i := 0; i < 20; i++ {
		entity := &models.Entity{
			ID:         uuid.New(),
			EntityType: "user",
			URN:        fmt.Sprintf("test:user:search-%d", i),
			Properties: map[string]any{
				"name":       fmt.Sprintf("John Doe %d", i),
				"email":      fmt.Sprintf("john.doe%d@example.com", i),
				"age":        20 + (i % 30),
				"department": departments[i%len(departments)],
				"active":     i%2 == 0,
				"tags":       []string{"user", fmt.Sprintf("group%d", i%3)},
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		err := testStore.IndexEntity(ctx, entity)
		require.NoError(t, err)
	}

	time.Sleep(200 * time.Millisecond)
}

func setupVectorTestData(t *testing.T, ctx context.Context) {
	categories := []string{"technical", "business", "general"}

	for i := 0; i < 10; i++ {

		embedding := make([]float32, 384)
		for j := range embedding {

			embedding[j] = float32(j+i) / (384.0 + float32(i))
		}

		entity := &models.Entity{
			ID:         uuid.New(),
			EntityType: "document",
			URN:        fmt.Sprintf("test:document:vector-%d", i),
			Properties: map[string]any{
				"title":     fmt.Sprintf("Document %d", i),
				"content":   fmt.Sprintf("This is the content of document %d", i),
				"category":  categories[i%len(categories)],
				"embedding": embedding,
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		err := testStore.IndexEntity(ctx, entity)
		require.NoError(t, err)
	}

	time.Sleep(200 * time.Millisecond)
}
