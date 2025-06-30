package sdk_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/sumandas0/entropic/pkg/sdk"
)

func TestEntityService_Create(t *testing.T) {
	entityID := uuid.New()
	entityType := "user"
	
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/entities/"+entityType, r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		
		// Verify request body
		var req sdk.EntityRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)
		assert.Equal(t, "urn:entropic:user:123", req.URN)
		assert.Equal(t, "John Doe", req.Properties["name"])
		
		// Send response
		response := sdk.Entity{
			ID:         entityID,
			EntityType: entityType,
			URN:        req.URN,
			Properties: req.Properties,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client, err := sdk.NewClient(server.URL)
	require.NoError(t, err)

	// Create entity
	req := sdk.NewEntityBuilder().
		WithURN("urn:entropic:user:123").
		WithProperty("name", "John Doe").
		Build()

	entity, err := client.Entities.Create(context.Background(), entityType, req)
	assert.NoError(t, err)
	assert.NotNil(t, entity)
	assert.Equal(t, entityID, entity.ID)
	assert.Equal(t, "John Doe", entity.Properties["name"])
}

func TestEntityService_Get(t *testing.T) {
	entityID := uuid.New()
	entityType := "user"
	
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expectedPath := "/api/v1/entities/" + entityType + "/" + entityID.String()
		assert.Equal(t, expectedPath, r.URL.Path)
		assert.Equal(t, http.MethodGet, r.Method)
		
		// Send response
		response := sdk.Entity{
			ID:         entityID,
			EntityType: entityType,
			URN:        "urn:entropic:user:123",
			Properties: map[string]interface{}{
				"name": "John Doe",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client, err := sdk.NewClient(server.URL)
	require.NoError(t, err)

	// Get entity
	entity, err := client.Entities.Get(context.Background(), entityType, entityID)
	assert.NoError(t, err)
	assert.NotNil(t, entity)
	assert.Equal(t, entityID, entity.ID)
	assert.Equal(t, "John Doe", entity.Properties["name"])
}

func TestEntityService_List(t *testing.T) {
	entityType := "user"
	
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/entities/"+entityType, r.URL.Path)
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "10", r.URL.Query().Get("limit"))
		assert.Equal(t, "0", r.URL.Query().Get("offset"))
		
		// Send response
		response := sdk.EntityListResponse{
			Entities: []sdk.Entity{
				{
					ID:         uuid.New(),
					EntityType: entityType,
					URN:        "urn:entropic:user:1",
					Properties: map[string]interface{}{"name": "User 1"},
				},
				{
					ID:         uuid.New(),
					EntityType: entityType,
					URN:        "urn:entropic:user:2",
					Properties: map[string]interface{}{"name": "User 2"},
				},
			},
			Total:  2,
			Limit:  10,
			Offset: 0,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client, err := sdk.NewClient(server.URL)
	require.NoError(t, err)

	// List entities
	result, err := client.Entities.List(context.Background(), entityType, &sdk.ListOptions{
		Limit:  10,
		Offset: 0,
	})
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result.Entities, 2)
	assert.Equal(t, 2, result.Total)
}

func TestEntityService_ValidationErrors(t *testing.T) {
	client, err := sdk.NewClient("http://localhost:8080")
	require.NoError(t, err)

	ctx := context.Background()

	// Test empty entity type
	_, err = client.Entities.Create(ctx, "", &sdk.EntityRequest{})
	assert.Error(t, err)
	apiErr, ok := sdk.AsAPIError(err)
	assert.True(t, ok)
	assert.True(t, apiErr.IsValidation())

	// Test nil request
	_, err = client.Entities.Create(ctx, "user", nil)
	assert.Error(t, err)
	apiErr, ok = sdk.AsAPIError(err)
	assert.True(t, ok)
	assert.True(t, apiErr.IsValidation())
}

func TestEntityBuilder(t *testing.T) {
	entity := sdk.NewEntityBuilder().
		WithURN("urn:test:123").
		WithProperty("name", "Test").
		WithProperty("age", 30).
		WithProperties(map[string]interface{}{
			"email": "test@example.com",
			"active": true,
		}).
		Build()

	assert.Equal(t, "urn:test:123", entity.URN)
	assert.Equal(t, "Test", entity.Properties["name"])
	assert.Equal(t, 30, entity.Properties["age"])
	assert.Equal(t, "test@example.com", entity.Properties["email"])
	assert.Equal(t, true, entity.Properties["active"])
}