package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/sumandas0/entropic/internal/cache"
	"github.com/sumandas0/entropic/internal/core"
	"github.com/sumandas0/entropic/internal/lock"
	"github.com/sumandas0/entropic/internal/models"
	"github.com/sumandas0/entropic/internal/store/postgres"
	"github.com/sumandas0/entropic/internal/store/testutils"
	"github.com/sumandas0/entropic/internal/store/typesense"
)

var (
	postgresContainer  *testutils.PostgresTestContainer
	typesenseContainer *testutils.TypesenseTestContainer
	primaryStore       *postgres.PostgresStore
	indexStore         *typesense.TypesenseStore
	engine             *core.Engine
)

func TestMain(m *testing.M) {
	var err error

	// Setup PostgreSQL
	postgresContainer, err = testutils.SetupTestPostgres()
	if err != nil {
		panic(err)
	}

	primaryStore, err = postgres.NewPostgresStore(postgresContainer.URL)
	if err != nil {
		postgresContainer.Cleanup()
		panic(err)
	}

	// Run migrations
	ctx := context.Background()
	migrator := postgres.NewMigrator(primaryStore.GetPool())
	if err := migrator.Run(ctx); err != nil {
		primaryStore.Close()
		postgresContainer.Cleanup()
		panic(err)
	}

	// Setup Typesense with simplified configuration
	typesenseContainer, err = testutils.SetupSimpleTypesense()
	if err != nil {
		primaryStore.Close()
		postgresContainer.Cleanup()
		panic(fmt.Sprintf("Failed to setup Typesense: %v", err))
	}

	indexStore, err = typesense.NewTypesenseStore(typesenseContainer.URL, typesenseContainer.APIKey)
	if err != nil {
		primaryStore.Close()
		postgresContainer.Cleanup()
		typesenseContainer.Cleanup()
		panic(err)
	}

	// Setup engine
	cacheManager := cache.NewCacheAwareManager(primaryStore, 5*time.Minute)
	lockManager := lock.NewLockManager(nil)
	var err2 error
	engine, err2 = core.NewEngine(primaryStore, indexStore, cacheManager, lockManager, nil)
	if err2 != nil {
		primaryStore.Close()
		indexStore.Close()
		postgresContainer.Cleanup()
		typesenseContainer.Cleanup()
		panic(err2)
	}

	// Run tests
	code := m.Run()

	// Cleanup
	primaryStore.Close()
	indexStore.Close()
	postgresContainer.Cleanup()
	typesenseContainer.Cleanup()

	if code != 0 {
		panic("tests failed")
	}
}

func cleanupDatabase(t *testing.T) {
	ctx := context.Background()
	
	// Clean PostgreSQL
	_, err := primaryStore.GetPool().Exec(ctx, "TRUNCATE entities, relations, entity_schemas, relationship_schemas CASCADE")
	require.NoError(t, err)
}

func createTestEntitySchema(t *testing.T, entityType string) *models.EntitySchema {
	schema := &models.EntitySchema{
		ID:         uuid.New(),
		EntityType: entityType,
		Properties: map[string]models.PropertyDefinition{
			"name": {
				Type:     "string",
				Required: true,
			},
			"email": {
				Type:     "string",
				Required: false,
			},
			"score": {
				Type:     "number",
				Required: false,
			},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	
	ctx := context.Background()
	err := engine.CreateEntitySchema(ctx, schema)
	require.NoError(t, err)
	
	return schema
}

func TestEntityHandler_CreateEntity(t *testing.T) {
	cleanupDatabase(t)
	
	// Setup
	createTestEntitySchema(t, "user")
	handler := NewEntityHandler(engine)
	
	tests := []struct {
		name           string
		entityType     string
		requestBody    interface{}
		expectedStatus int
		checkResponse  func(t *testing.T, resp EntityResponse)
	}{
		{
			name:       "valid entity creation",
			entityType: "user",
			requestBody: EntityRequest{
				URN: "test:user:create1",
				Properties: map[string]any{
					"name":  "John Doe",
					"email": "john@example.com",
					"score": 85,
				},
			},
			expectedStatus: http.StatusCreated,
			checkResponse: func(t *testing.T, resp EntityResponse) {
				assert.NotEqual(t, uuid.Nil, resp.ID)
				assert.Equal(t, "user", resp.EntityType)
				assert.Equal(t, "test:user:create1", resp.URN)
				assert.Equal(t, "John Doe", resp.Properties["name"])
				assert.Equal(t, "john@example.com", resp.Properties["email"])
				assert.Equal(t, float64(85), resp.Properties["score"])
				assert.Equal(t, 1, resp.Version)
			},
		},
		{
			name:       "missing required property",
			entityType: "user",
			requestBody: EntityRequest{
				URN: "test:user:missing_required",
				Properties: map[string]any{
					"email": "missing@example.com",
					// name is missing
				},
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:       "duplicate URN",
			entityType: "user",
			requestBody: EntityRequest{
				URN: "test:user:duplicate",
				Properties: map[string]any{
					"name": "First User",
				},
			},
			expectedStatus: http.StatusCreated,
			checkResponse: func(t *testing.T, resp EntityResponse) {
				// Create another entity with the same URN
				handler := NewEntityHandler(engine)
				req := EntityRequest{
					URN: "test:user:duplicate",
					Properties: map[string]any{
						"name": "Second User",
					},
				}
				body, _ := json.Marshal(req)
				r := httptest.NewRequest("POST", "/api/v1/entities/user", bytes.NewReader(body))
				w := httptest.NewRecorder()
				
				rctx := chi.NewRouteContext()
				rctx.URLParams.Add("entityType", "user")
				r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
				
				handler.CreateEntity(w, r)
				assert.Equal(t, http.StatusConflict, w.Code)
			},
		},
		{
			name:       "invalid entity type",
			entityType: "nonexistent",
			requestBody: EntityRequest{
				URN: "test:nonexistent:1",
				Properties: map[string]any{
					"data": "value",
				},
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "empty entity type",
			entityType:     "",
			requestBody:    EntityRequest{},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid request body",
			entityType: "user",
			requestBody: "invalid json",
			expectedStatus: http.StatusBadRequest,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Prepare request
			body, err := json.Marshal(tt.requestBody)
			require.NoError(t, err)
			
			req := httptest.NewRequest("POST", "/api/v1/entities/"+tt.entityType, bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			
			// Add chi context
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("entityType", tt.entityType)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
			
			// Execute
			handler.CreateEntity(w, req)
			
			// Verify status
			assert.Equal(t, tt.expectedStatus, w.Code)
			
			// Check response if successful
			if w.Code == http.StatusCreated {
				var resp EntityResponse
				err = json.NewDecoder(w.Body).Decode(&resp)
				require.NoError(t, err)
				
				if tt.checkResponse != nil {
					tt.checkResponse(t, resp)
				}
			}
		})
	}
}

func TestEntityHandler_GetEntity(t *testing.T) {
	cleanupDatabase(t)
	
	// Setup
	createTestEntitySchema(t, "user")
	handler := NewEntityHandler(engine)
	
	// Create test entity
	testEntity := &models.Entity{
		ID:         uuid.New(),
		EntityType: "user",
		URN:        "test:user:get1",
		Properties: map[string]any{
			"name":  "Get Test User",
			"email": "get@example.com",
		},
		Version: 1,
	}
	ctx := context.Background()
	err := engine.CreateEntity(ctx, testEntity)
	require.NoError(t, err)
	
	tests := []struct {
		name           string
		entityType     string
		entityID       string
		expectedStatus int
		checkResponse  func(t *testing.T, resp EntityResponse)
	}{
		{
			name:           "get existing entity",
			entityType:     "user",
			entityID:       testEntity.ID.String(),
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp EntityResponse) {
				assert.Equal(t, testEntity.ID, resp.ID)
				assert.Equal(t, "user", resp.EntityType)
				assert.Equal(t, "test:user:get1", resp.URN)
				assert.Equal(t, "Get Test User", resp.Properties["name"])
			},
		},
		{
			name:           "get non-existent entity",
			entityType:     "user",
			entityID:       uuid.New().String(),
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "invalid entity ID",
			entityType:     "user",
			entityID:       "invalid-uuid",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "empty entity type",
			entityType:     "",
			entityID:       testEntity.ID.String(),
			expectedStatus: http.StatusBadRequest,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/v1/entities/"+tt.entityType+"/"+tt.entityID, nil)
			w := httptest.NewRecorder()
			
			// Add chi context
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("entityType", tt.entityType)
			rctx.URLParams.Add("entityID", tt.entityID)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
			
			// Execute
			handler.GetEntity(w, req)
			
			// Verify
			assert.Equal(t, tt.expectedStatus, w.Code)
			
			if w.Code == http.StatusOK && tt.checkResponse != nil {
				var resp EntityResponse
				err = json.NewDecoder(w.Body).Decode(&resp)
				require.NoError(t, err)
				tt.checkResponse(t, resp)
			}
		})
	}
}

func TestEntityHandler_UpdateEntity(t *testing.T) {
	cleanupDatabase(t)
	
	// Setup
	createTestEntitySchema(t, "user")
	handler := NewEntityHandler(engine)
	
	// Create test entity
	testEntity := &models.Entity{
		ID:         uuid.New(),
		EntityType: "user",
		URN:        "test:user:update1",
		Properties: map[string]any{
			"name":  "Original Name",
			"email": "original@example.com",
			"score": 50,
		},
		Version: 1,
	}
	ctx := context.Background()
	err := engine.CreateEntity(ctx, testEntity)
	require.NoError(t, err)
	
	tests := []struct {
		name           string
		entityType     string
		entityID       string
		requestBody    interface{}
		expectedStatus int
		checkResponse  func(t *testing.T, resp EntityResponse)
	}{
		{
			name:       "valid update",
			entityType: "user",
			entityID:   testEntity.ID.String(),
			requestBody: EntityRequest{
				URN: testEntity.URN,
				Properties: map[string]any{
					"name":  "Updated Name",
					"email": "updated@example.com",
					"score": 75,
				},
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp EntityResponse) {
				assert.Equal(t, testEntity.ID, resp.ID)
				assert.Equal(t, "Updated Name", resp.Properties["name"])
				assert.Equal(t, "updated@example.com", resp.Properties["email"])
				assert.Equal(t, float64(75), resp.Properties["score"])
				assert.Equal(t, 2, resp.Version)
			},
		},
		{
			name:       "update non-existent entity",
			entityType: "user",
			entityID:   uuid.New().String(),
			requestBody: EntityRequest{
				URN: "test:user:nonexistent",
				Properties: map[string]any{
					"name": "Should Fail",
				},
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name:       "update with missing required field",
			entityType: "user",
			entityID:   testEntity.ID.String(),
			requestBody: EntityRequest{
				URN: testEntity.URN,
				Properties: map[string]any{
					// name is missing
					"email": "missing_required@example.com",
				},
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid entity ID",
			entityType: "user",
			entityID:   "invalid-uuid",
			requestBody: EntityRequest{
				URN: "test:user:invalid",
				Properties: map[string]any{
					"name": "Invalid",
				},
			},
			expectedStatus: http.StatusBadRequest,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := json.Marshal(tt.requestBody)
			require.NoError(t, err)
			
			req := httptest.NewRequest("PATCH", "/api/v1/entities/"+tt.entityType+"/"+tt.entityID, bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			
			// Add chi context
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("entityType", tt.entityType)
			rctx.URLParams.Add("entityID", tt.entityID)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
			
			// Execute
			handler.UpdateEntity(w, req)
			
			// Verify
			assert.Equal(t, tt.expectedStatus, w.Code)
			
			if w.Code == http.StatusOK && tt.checkResponse != nil {
				var resp EntityResponse
				err = json.NewDecoder(w.Body).Decode(&resp)
				require.NoError(t, err)
				tt.checkResponse(t, resp)
			}
		})
	}
}

func TestEntityHandler_DeleteEntity(t *testing.T) {
	cleanupDatabase(t)
	
	// Setup
	createTestEntitySchema(t, "user")
	handler := NewEntityHandler(engine)
	
	// Create test entities
	entity1 := &models.Entity{
		ID:         uuid.New(),
		EntityType: "user",
		URN:        "test:user:delete1",
		Properties: map[string]any{
			"name": "Delete Test 1",
		},
		Version: 1,
	}
	entity2 := &models.Entity{
		ID:         uuid.New(),
		EntityType: "user",
		URN:        "test:user:delete2",
		Properties: map[string]any{
			"name": "Delete Test 2",
		},
		Version: 1,
	}
	
	ctx := context.Background()
	err := engine.CreateEntity(ctx, entity1)
	require.NoError(t, err)
	err = engine.CreateEntity(ctx, entity2)
	require.NoError(t, err)
	
	tests := []struct {
		name           string
		entityType     string
		entityID       string
		expectedStatus int
		verifyDeleted  bool
	}{
		{
			name:           "delete existing entity",
			entityType:     "user",
			entityID:       entity1.ID.String(),
			expectedStatus: http.StatusNoContent,
			verifyDeleted:  true,
		},
		{
			name:           "delete non-existent entity",
			entityType:     "user",
			entityID:       uuid.New().String(),
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "invalid entity ID",
			entityType:     "user",
			entityID:       "invalid-uuid",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "empty entity type",
			entityType:     "",
			entityID:       entity2.ID.String(),
			expectedStatus: http.StatusBadRequest,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("DELETE", "/api/v1/entities/"+tt.entityType+"/"+tt.entityID, nil)
			w := httptest.NewRecorder()
			
			// Add chi context
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("entityType", tt.entityType)
			rctx.URLParams.Add("entityID", tt.entityID)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
			
			// Execute
			handler.DeleteEntity(w, req)
			
			// Verify
			assert.Equal(t, tt.expectedStatus, w.Code)
			
			// Verify entity is actually deleted
			if tt.verifyDeleted && w.Code == http.StatusNoContent {
				entityID, _ := uuid.Parse(tt.entityID)
				_, err := engine.GetEntity(ctx, tt.entityType, entityID)
				assert.Error(t, err)
			}
		})
	}
}

func TestEntityHandler_ListEntities(t *testing.T) {
	cleanupDatabase(t)
	
	// Setup
	createTestEntitySchema(t, "user")
	handler := NewEntityHandler(engine)
	
	// Create multiple test entities
	ctx := context.Background()
	for i := 0; i < 15; i++ {
		entity := &models.Entity{
			ID:         uuid.New(),
			EntityType: "user",
			URN:        fmt.Sprintf("test:user:list%d", i),
			Properties: map[string]any{
				"name":  fmt.Sprintf("List User %d", i),
				"score": i * 10,
			},
			Version: 1,
		}
		err := engine.CreateEntity(ctx, entity)
		require.NoError(t, err)
	}
	
	tests := []struct {
		name           string
		entityType     string
		queryParams    map[string]string
		expectedStatus int
		checkResponse  func(t *testing.T, resp EntityListResponse)
	}{
		{
			name:           "list with default pagination",
			entityType:     "user",
			queryParams:    map[string]string{},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp EntityListResponse) {
				assert.Len(t, resp.Entities, 10) // Default limit
				assert.Equal(t, 15, resp.Total)
				assert.Equal(t, 10, resp.Limit)
				assert.Equal(t, 0, resp.Offset)
			},
		},
		{
			name:       "list with custom pagination",
			entityType: "user",
			queryParams: map[string]string{
				"limit":  "5",
				"offset": "5",
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp EntityListResponse) {
				assert.Len(t, resp.Entities, 5)
				assert.Equal(t, 15, resp.Total)
				assert.Equal(t, 5, resp.Limit)
				assert.Equal(t, 5, resp.Offset)
			},
		},
		{
			name:       "list with limit exceeding max",
			entityType: "user",
			queryParams: map[string]string{
				"limit": "200", // Should be capped at 100
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp EntityListResponse) {
				assert.Len(t, resp.Entities, 15) // All entities
				assert.Equal(t, 100, resp.Limit) // Capped at max
			},
		},
		{
			name:       "list with invalid limit",
			entityType: "user",
			queryParams: map[string]string{
				"limit": "invalid",
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "list non-existent entity type",
			entityType:     "nonexistent",
			queryParams:    map[string]string{},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp EntityListResponse) {
				assert.Len(t, resp.Entities, 0)
				assert.Equal(t, 0, resp.Total)
			},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/v1/entities/"+tt.entityType, nil)
			
			// Add query parameters
			q := req.URL.Query()
			for k, v := range tt.queryParams {
				q.Add(k, v)
			}
			req.URL.RawQuery = q.Encode()
			
			w := httptest.NewRecorder()
			
			// Add chi context
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("entityType", tt.entityType)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
			
			// Execute
			handler.ListEntities(w, req)
			
			// Verify
			assert.Equal(t, tt.expectedStatus, w.Code)
			
			if w.Code == http.StatusOK && tt.checkResponse != nil {
				var resp EntityListResponse
				err := json.NewDecoder(w.Body).Decode(&resp)
				require.NoError(t, err)
				tt.checkResponse(t, resp)
			}
		})
	}
}

func TestEntityHandler_ConcurrentOperations(t *testing.T) {
	cleanupDatabase(t)
	
	// Setup
	createTestEntitySchema(t, "user")
	handler := NewEntityHandler(engine)
	
	// Test concurrent creates
	t.Run("concurrent creates", func(t *testing.T) {
		numGoroutines := 10
		errors := make(chan error, numGoroutines)
		
		for i := 0; i < numGoroutines; i++ {
			go func(idx int) {
				req := EntityRequest{
					URN: fmt.Sprintf("test:user:concurrent%d", idx),
					Properties: map[string]any{
						"name": fmt.Sprintf("Concurrent User %d", idx),
					},
				}
				
				body, _ := json.Marshal(req)
				r := httptest.NewRequest("POST", "/api/v1/entities/user", bytes.NewReader(body))
				r.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				
				rctx := chi.NewRouteContext()
				rctx.URLParams.Add("entityType", "user")
				r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
				
				handler.CreateEntity(w, r)
				
				if w.Code != http.StatusCreated {
					errors <- fmt.Errorf("unexpected status: %d", w.Code)
				} else {
					errors <- nil
				}
			}(i)
		}
		
		// Collect results
		for i := 0; i < numGoroutines; i++ {
			err := <-errors
			assert.NoError(t, err)
		}
	})
	
	// Test concurrent updates to same entity
	t.Run("concurrent updates", func(t *testing.T) {
		// Create entity
		ctx := context.Background()
		entity := &models.Entity{
			ID:         uuid.New(),
			EntityType: "user",
			URN:        "test:user:concurrent_update",
			Properties: map[string]any{
				"name":  "Initial Name",
				"score": 0,
			},
			Version: 1,
		}
		err := engine.CreateEntity(ctx, entity)
		require.NoError(t, err)
		
		// Concurrent updates
		numGoroutines := 5
		results := make(chan int, numGoroutines)
		
		for i := 0; i < numGoroutines; i++ {
			go func(idx int) {
				req := EntityRequest{
					URN: entity.URN,
					Properties: map[string]any{
						"name":  fmt.Sprintf("Updated Name %d", idx),
						"score": idx * 10,
					},
				}
				
				body, _ := json.Marshal(req)
				r := httptest.NewRequest("PATCH", "/api/v1/entities/user/"+entity.ID.String(), bytes.NewReader(body))
				r.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				
				rctx := chi.NewRouteContext()
				rctx.URLParams.Add("entityType", "user")
				rctx.URLParams.Add("entityID", entity.ID.String())
				r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
				
				handler.UpdateEntity(w, r)
				results <- w.Code
			}(i)
		}
		
		// Collect results - at least one should succeed
		successCount := 0
		for i := 0; i < numGoroutines; i++ {
			status := <-results
			if status == http.StatusOK {
				successCount++
			}
		}
		assert.GreaterOrEqual(t, successCount, 1)
	})
}

func BenchmarkEntityHandler_CreateEntity(b *testing.B) {
	// Setup
	ctx := context.Background()
	schema := &models.EntitySchema{
		ID:         uuid.New(),
		EntityType: "bench_user",
		Properties: map[string]models.PropertyDefinition{
			"name":  {Type: "string", Required: true},
			"email": {Type: "string", Required: false},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	engine.CreateEntitySchema(ctx, schema)
	
	handler := NewEntityHandler(engine)
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		req := EntityRequest{
			URN: fmt.Sprintf("test:bench_user:%d", i),
			Properties: map[string]any{
				"name":  fmt.Sprintf("Bench User %d", i),
				"email": fmt.Sprintf("bench%d@example.com", i),
			},
		}
		
		body, _ := json.Marshal(req)
		r := httptest.NewRequest("POST", "/api/v1/entities/bench_user", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("entityType", "bench_user")
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
		
		handler.CreateEntity(w, r)
	}
}