package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/sumandas0/entropic/internal/models"
)

func TestSchemaHandler_CreateEntitySchema(t *testing.T) {
	cleanupDatabase(t)
	
	handler := NewSchemaHandler(engine)
	
	tests := []struct {
		name           string
		requestBody    interface{}
		expectedStatus int
		checkResponse  func(t *testing.T, resp EntitySchemaResponse)
	}{
		{
			name: "valid entity schema creation",
			requestBody: EntitySchemaRequest{
				EntityType: "product",
				Properties: models.PropertySchema{
					"name": models.PropertyDefinition{
						Type:        "string",
						Required:    true,
						Description: "Product name",
					},
					"price": models.PropertyDefinition{
						Type:     "number",
						Required: true,
					},
					"tags": models.PropertyDefinition{
						Type:        "array",
						ElementType: "string",
						Required:    false,
					},
				},
				Indexes: []models.IndexConfig{
					{
						Name:   "idx_product_name",
						Fields: []string{"name"},
						Type:   "btree",
						Unique: true,
					},
				},
			},
			expectedStatus: http.StatusCreated,
			checkResponse: func(t *testing.T, resp EntitySchemaResponse) {
				assert.NotEqual(t, uuid.Nil, resp.ID)
				assert.Equal(t, "product", resp.EntityType)
				assert.Len(t, resp.Properties, 3)
				assert.Equal(t, "string", resp.Properties["name"].Type)
				assert.True(t, resp.Properties["name"].Required)
				assert.Len(t, resp.Indexes, 1)
				assert.Equal(t, 1, resp.Version)
			},
		},
		{
			name: "duplicate entity schema",
			requestBody: EntitySchemaRequest{
				EntityType: "user", // Already exists from setup
				Properties: models.PropertySchema{
					"name": models.PropertyDefinition{
						Type:     "string",
						Required: true,
					},
				},
			},
			expectedStatus: http.StatusConflict,
		},
		{
			name: "invalid property type",
			requestBody: EntitySchemaRequest{
				EntityType: "invalid_schema",
				Properties: models.PropertySchema{
					"data": models.PropertyDefinition{
						Type:     "invalid_type",
						Required: true,
					},
				},
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "empty entity type",
			requestBody: EntitySchemaRequest{
				EntityType: "",
				Properties: models.PropertySchema{
					"name": models.PropertyDefinition{
						Type: "string",
					},
				},
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "vector property",
			requestBody: EntitySchemaRequest{
				EntityType: "document",
				Properties: models.PropertySchema{
					"content": models.PropertyDefinition{
						Type:     "string",
						Required: true,
					},
					"embedding": models.PropertyDefinition{
						Type:      "vector",
						VectorDim: 1536,
						Required:  false,
					},
				},
			},
			expectedStatus: http.StatusCreated,
			checkResponse: func(t *testing.T, resp EntitySchemaResponse) {
				assert.Equal(t, "vector", resp.Properties["embedding"].Type)
				assert.Equal(t, 1536, resp.Properties["embedding"].VectorDim)
			},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := json.Marshal(tt.requestBody)
			require.NoError(t, err)
			
			req := httptest.NewRequest("POST", "/api/v1/schemas/entities", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			
			handler.CreateEntitySchema(w, req)
			
			assert.Equal(t, tt.expectedStatus, w.Code)
			
			if w.Code == http.StatusCreated && tt.checkResponse != nil {
				var resp EntitySchemaResponse
				err = json.NewDecoder(w.Body).Decode(&resp)
				require.NoError(t, err)
				tt.checkResponse(t, resp)
			}
		})
	}
}

func TestSchemaHandler_GetEntitySchema(t *testing.T) {
	cleanupDatabase(t)
	
	// Create test schema
	testSchema := createTestEntitySchema(t, "test_entity")
	handler := NewSchemaHandler(engine)
	
	tests := []struct {
		name           string
		entityType     string
		expectedStatus int
		checkResponse  func(t *testing.T, resp EntitySchemaResponse)
	}{
		{
			name:           "get existing schema",
			entityType:     "test_entity",
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp EntitySchemaResponse) {
				assert.Equal(t, testSchema.ID, resp.ID)
				assert.Equal(t, "test_entity", resp.EntityType)
				assert.NotEmpty(t, resp.Properties)
			},
		},
		{
			name:           "get non-existent schema",
			entityType:     "nonexistent",
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "empty entity type",
			entityType:     "",
			expectedStatus: http.StatusBadRequest,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/v1/schemas/entities/"+tt.entityType, nil)
			w := httptest.NewRecorder()
			
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("entityType", tt.entityType)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
			
			handler.GetEntitySchema(w, req)
			
			assert.Equal(t, tt.expectedStatus, w.Code)
			
			if w.Code == http.StatusOK && tt.checkResponse != nil {
				var resp EntitySchemaResponse
				err := json.NewDecoder(w.Body).Decode(&resp)
				require.NoError(t, err)
				tt.checkResponse(t, resp)
			}
		})
	}
}

func TestSchemaHandler_UpdateEntitySchema(t *testing.T) {
	cleanupDatabase(t)
	
	// Create initial schema
	createTestEntitySchema(t, "update_test")
	handler := NewSchemaHandler(engine)
	
	tests := []struct {
		name           string
		entityType     string
		requestBody    interface{}
		expectedStatus int
		checkResponse  func(t *testing.T, resp EntitySchemaResponse)
	}{
		{
			name:       "add new property",
			entityType: "update_test",
			requestBody: EntitySchemaRequest{
				EntityType: "update_test",
				Properties: models.PropertySchema{
					"name": models.PropertyDefinition{
						Type:     "string",
						Required: true,
					},
					"email": models.PropertyDefinition{
						Type:     "string",
						Required: false,
					},
					"score": models.PropertyDefinition{
						Type:     "number",
						Required: false,
					},
					"age": models.PropertyDefinition{ // New property
						Type:     "number",
						Required: false,
					},
				},
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp EntitySchemaResponse) {
				assert.Equal(t, "update_test", resp.EntityType)
				assert.Len(t, resp.Properties, 4)
				assert.Contains(t, resp.Properties, "age")
				assert.Equal(t, 2, resp.Version) // Version should increment
			},
		},
		{
			name:       "remove required property",
			entityType: "update_test",
			requestBody: EntitySchemaRequest{
				EntityType: "update_test",
				Properties: models.PropertySchema{
					// name is missing - this should fail
					"email": models.PropertyDefinition{
						Type:     "string",
						Required: false,
					},
				},
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:       "update non-existent schema",
			entityType: "nonexistent",
			requestBody: EntitySchemaRequest{
				EntityType: "nonexistent",
				Properties: models.PropertySchema{
					"data": models.PropertyDefinition{
						Type: "string",
					},
				},
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name:       "type mismatch",
			entityType: "update_test",
			requestBody: EntitySchemaRequest{
				EntityType: "different_type", // Mismatch with URL param
				Properties: models.PropertySchema{
					"name": models.PropertyDefinition{
						Type: "string",
					},
				},
			},
			expectedStatus: http.StatusBadRequest,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := json.Marshal(tt.requestBody)
			require.NoError(t, err)
			
			req := httptest.NewRequest("PUT", "/api/v1/schemas/entities/"+tt.entityType, bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("entityType", tt.entityType)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
			
			handler.UpdateEntitySchema(w, req)
			
			assert.Equal(t, tt.expectedStatus, w.Code)
			
			if w.Code == http.StatusOK && tt.checkResponse != nil {
				var resp EntitySchemaResponse
				err = json.NewDecoder(w.Body).Decode(&resp)
				require.NoError(t, err)
				tt.checkResponse(t, resp)
			}
		})
	}
}

func TestSchemaHandler_DeleteEntitySchema(t *testing.T) {
	cleanupDatabase(t)
	
	// Create schemas to delete
	createTestEntitySchema(t, "delete_test1")
	createTestEntitySchema(t, "delete_test2")
	
	// Create entity with delete_test2 schema
	ctx := context.Background()
	entity := &models.Entity{
		ID:         uuid.New(),
		EntityType: "delete_test2",
		URN:        "test:delete:1",
		Properties: map[string]any{
			"name": "Test Entity",
		},
		Version: 1,
	}
	err := engine.CreateEntity(ctx, entity)
	require.NoError(t, err)
	
	handler := NewSchemaHandler(engine)
	
	tests := []struct {
		name           string
		entityType     string
		expectedStatus int
	}{
		{
			name:           "delete unused schema",
			entityType:     "delete_test1",
			expectedStatus: http.StatusNoContent,
		},
		{
			name:           "delete schema with entities",
			entityType:     "delete_test2",
			expectedStatus: http.StatusConflict, // Should fail due to existing entities
		},
		{
			name:           "delete non-existent schema",
			entityType:     "nonexistent",
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "empty entity type",
			entityType:     "",
			expectedStatus: http.StatusBadRequest,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("DELETE", "/api/v1/schemas/entities/"+tt.entityType, nil)
			w := httptest.NewRecorder()
			
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("entityType", tt.entityType)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
			
			handler.DeleteEntitySchema(w, req)
			
			assert.Equal(t, tt.expectedStatus, w.Code)
			
			// Verify deletion
			if tt.expectedStatus == http.StatusNoContent {
				_, err := engine.GetEntitySchema(ctx, tt.entityType)
				assert.Error(t, err)
			}
		})
	}
}

func TestSchemaHandler_CreateRelationshipSchema(t *testing.T) {
	cleanupDatabase(t)
	
	// Create entity schemas first
	createTestEntitySchema(t, "user")
	createTestEntitySchema(t, "organization")
	
	handler := NewSchemaHandler(engine)
	
	tests := []struct {
		name           string
		requestBody    interface{}
		expectedStatus int
		checkResponse  func(t *testing.T, resp RelationshipSchemaResponse)
	}{
		{
			name: "valid relationship schema",
			requestBody: RelationshipSchemaRequest{
				RelationshipType: "member_of",
				FromEntityType:   "user",
				ToEntityType:     "organization",
				Properties: models.PropertySchema{
					"role": models.PropertyDefinition{
						Type:     "string",
						Required: true,
					},
					"since": models.PropertyDefinition{
						Type:     "datetime",
						Required: false,
					},
				},
				Cardinality: models.ManyToMany,
				DenormalizationConfig: models.DenormalizationConfig{
					DenormalizeToFrom: []string{"name"},
					UpdateOnChange:    true,
				},
			},
			expectedStatus: http.StatusCreated,
			checkResponse: func(t *testing.T, resp RelationshipSchemaResponse) {
				assert.NotEqual(t, uuid.Nil, resp.ID)
				assert.Equal(t, "member_of", resp.RelationshipType)
				assert.Equal(t, "user", resp.FromEntityType)
				assert.Equal(t, "organization", resp.ToEntityType)
				assert.Equal(t, models.ManyToMany, resp.Cardinality)
				assert.True(t, resp.DenormalizationConfig.UpdateOnChange)
			},
		},
		{
			name: "duplicate relationship schema",
			requestBody: RelationshipSchemaRequest{
				RelationshipType: "member_of", // Try to create again
				FromEntityType:   "user",
				ToEntityType:     "organization",
				Cardinality:      models.OneToMany,
			},
			expectedStatus: http.StatusConflict,
		},
		{
			name: "non-existent entity types",
			requestBody: RelationshipSchemaRequest{
				RelationshipType: "invalid_rel",
				FromEntityType:   "nonexistent1",
				ToEntityType:     "nonexistent2",
				Cardinality:      models.OneToOne,
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "self-referential relationship",
			requestBody: RelationshipSchemaRequest{
				RelationshipType: "reports_to",
				FromEntityType:   "user",
				ToEntityType:     "user",
				Cardinality:      models.ManyToOne,
			},
			expectedStatus: http.StatusCreated,
			checkResponse: func(t *testing.T, resp RelationshipSchemaResponse) {
				assert.Equal(t, "user", resp.FromEntityType)
				assert.Equal(t, "user", resp.ToEntityType)
			},
		},
		{
			name: "empty relationship type",
			requestBody: RelationshipSchemaRequest{
				RelationshipType: "",
				FromEntityType:   "user",
				ToEntityType:     "organization",
				Cardinality:      models.OneToOne,
			},
			expectedStatus: http.StatusBadRequest,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := json.Marshal(tt.requestBody)
			require.NoError(t, err)
			
			req := httptest.NewRequest("POST", "/api/v1/schemas/relationships", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			
			handler.CreateRelationshipSchema(w, req)
			
			assert.Equal(t, tt.expectedStatus, w.Code)
			
			if w.Code == http.StatusCreated && tt.checkResponse != nil {
				var resp RelationshipSchemaResponse
				err = json.NewDecoder(w.Body).Decode(&resp)
				require.NoError(t, err)
				tt.checkResponse(t, resp)
			}
		})
	}
}

func TestSchemaHandler_GetRelationshipSchema(t *testing.T) {
	cleanupDatabase(t)
	
	// Create test schemas
	createTestEntitySchema(t, "user")
	createTestEntitySchema(t, "project")
	testRelSchema := createTestRelationshipSchema(t, "works_on", "user", "project")
	
	handler := NewSchemaHandler(engine)
	
	tests := []struct {
		name             string
		relationshipType string
		expectedStatus   int
		checkResponse    func(t *testing.T, resp RelationshipSchemaResponse)
	}{
		{
			name:             "get existing schema",
			relationshipType: "works_on",
			expectedStatus:   http.StatusOK,
			checkResponse: func(t *testing.T, resp RelationshipSchemaResponse) {
				assert.Equal(t, testRelSchema.ID, resp.ID)
				assert.Equal(t, "works_on", resp.RelationshipType)
				assert.Equal(t, "user", resp.FromEntityType)
				assert.Equal(t, "project", resp.ToEntityType)
			},
		},
		{
			name:             "get non-existent schema",
			relationshipType: "nonexistent",
			expectedStatus:   http.StatusNotFound,
		},
		{
			name:             "empty relationship type",
			relationshipType: "",
			expectedStatus:   http.StatusBadRequest,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/v1/schemas/relationships/"+tt.relationshipType, nil)
			w := httptest.NewRecorder()
			
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("relationshipType", tt.relationshipType)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
			
			handler.GetRelationshipSchema(w, req)
			
			assert.Equal(t, tt.expectedStatus, w.Code)
			
			if w.Code == http.StatusOK && tt.checkResponse != nil {
				var resp RelationshipSchemaResponse
				err := json.NewDecoder(w.Body).Decode(&resp)
				require.NoError(t, err)
				tt.checkResponse(t, resp)
			}
		})
	}
}

func TestSchemaHandler_ListEntitySchemas(t *testing.T) {
	cleanupDatabase(t)
	
	// Create multiple schemas
	for i := 0; i < 5; i++ {
		createTestEntitySchema(t, fmt.Sprintf("entity_%d", i))
	}
	
	handler := NewSchemaHandler(engine)
	
	req := httptest.NewRequest("GET", "/api/v1/schemas/entities", nil)
	w := httptest.NewRecorder()
	
	handler.ListEntitySchemas(w, req)
	
	assert.Equal(t, http.StatusOK, w.Code)
	
	var resp struct {
		Schemas []EntitySchemaResponse `json:"schemas"`
		Total   int                    `json:"total"`
	}
	err := json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)
	
	assert.GreaterOrEqual(t, resp.Total, 5)
	assert.GreaterOrEqual(t, len(resp.Schemas), 5)
}

func TestSchemaHandler_ListRelationshipSchemas(t *testing.T) {
	cleanupDatabase(t)
	
	// Create entity schemas
	createTestEntitySchema(t, "user")
	createTestEntitySchema(t, "team")
	createTestEntitySchema(t, "project")
	
	// Create relationship schemas
	createTestRelationshipSchema(t, "member_of", "user", "team")
	createTestRelationshipSchema(t, "works_on", "user", "project")
	createTestRelationshipSchema(t, "manages", "user", "team")
	
	handler := NewSchemaHandler(engine)
	
	req := httptest.NewRequest("GET", "/api/v1/schemas/relationships", nil)
	w := httptest.NewRecorder()
	
	handler.ListRelationshipSchemas(w, req)
	
	assert.Equal(t, http.StatusOK, w.Code)
	
	var resp struct {
		Schemas []RelationshipSchemaResponse `json:"schemas"`
		Total   int                          `json:"total"`
	}
	err := json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)
	
	assert.Equal(t, 3, resp.Total)
	assert.Len(t, resp.Schemas, 3)
	
	// Verify all schemas are present
	schemaTypes := make(map[string]bool)
	for _, schema := range resp.Schemas {
		schemaTypes[schema.RelationshipType] = true
	}
	assert.True(t, schemaTypes["member_of"])
	assert.True(t, schemaTypes["works_on"])
	assert.True(t, schemaTypes["manages"])
}

func TestSchemaHandler_SchemaEvolution(t *testing.T) {
	cleanupDatabase(t)
	
	// Test schema evolution with existing data
	createTestEntitySchema(t, "evolving_entity")
	
	// Create entities with current schema
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		entity := &models.Entity{
			ID:         uuid.New(),
			EntityType: "evolving_entity",
			URN:        fmt.Sprintf("test:evolve:%d", i),
			Properties: map[string]any{
				"name":  fmt.Sprintf("Entity %d", i),
				"email": fmt.Sprintf("entity%d@example.com", i),
			},
			Version: 1,
		}
		err := engine.CreateEntity(ctx, entity)
		require.NoError(t, err)
	}
	
	handler := NewSchemaHandler(engine)
	
	// Update schema to add new optional property
	updateReq := EntitySchemaRequest{
		EntityType: "evolving_entity",
		Properties: models.PropertySchema{
			"name": models.PropertyDefinition{
				Type:     "string",
				Required: true,
			},
			"email": models.PropertyDefinition{
				Type:     "string",
				Required: false,
			},
			"score": models.PropertyDefinition{
				Type:     "number",
				Required: false,
			},
			"status": models.PropertyDefinition{ // New optional property
				Type:     "string",
				Required: false,
				Default:  "active",
			},
		},
	}
	
	body, _ := json.Marshal(updateReq)
	req := httptest.NewRequest("PUT", "/api/v1/schemas/entities/evolving_entity", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("entityType", "evolving_entity")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	
	handler.UpdateEntitySchema(w, req)
	
	assert.Equal(t, http.StatusOK, w.Code)
	
	// Verify existing entities still work
	entities, err := engine.ListEntities(ctx, "evolving_entity", 10, 0)
	require.NoError(t, err)
	assert.Len(t, entities, 3)
	
	// Create new entity with new schema
	newEntity := &models.Entity{
		ID:         uuid.New(),
		EntityType: "evolving_entity",
		URN:        "test:evolve:new",
		Properties: map[string]any{
			"name":   "New Entity",
			"email":  "new@example.com",
			"status": "pending", // Using new property
		},
		Version: 1,
	}
	err = engine.CreateEntity(ctx, newEntity)
	require.NoError(t, err)
}

func BenchmarkSchemaHandler_CreateEntitySchema(b *testing.B) {
	handler := NewSchemaHandler(engine)
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		req := EntitySchemaRequest{
			EntityType: fmt.Sprintf("bench_entity_%d", i),
			Properties: models.PropertySchema{
				"name": models.PropertyDefinition{
					Type:     "string",
					Required: true,
				},
				"value": models.PropertyDefinition{
					Type:     "number",
					Required: false,
				},
			},
		}
		
		body, _ := json.Marshal(req)
		r := httptest.NewRequest("POST", "/api/v1/schemas/entities", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		
		handler.CreateEntitySchema(w, r)
	}
}