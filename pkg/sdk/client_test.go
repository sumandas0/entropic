package sdk_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/sumandas0/entropic/pkg/sdk"
)

func TestNewClient(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		wantErr bool
	}{
		{
			name:    "valid URL",
			baseURL: "http://localhost:8080",
			wantErr: false,
		},
		{
			name:    "invalid URL",
			baseURL: "://invalid-url",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := sdk.NewClient(tt.baseURL)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, client)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, client)
			}
		})
	}
}

func TestHealthCheck(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/health", r.URL.Path)
		assert.Equal(t, http.MethodGet, r.Method)
		
		response := map[string]interface{}{
			"status": "healthy",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client, err := sdk.NewClient(server.URL)
	require.NoError(t, err)

	err = client.HealthCheck(context.Background())
	assert.NoError(t, err)
}

func TestHealthCheckError(t *testing.T) {
	// Create test server that returns error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client, err := sdk.NewClient(server.URL)
	require.NoError(t, err)

	err = client.HealthCheck(context.Background())
	assert.Error(t, err)
}

func TestAPIError(t *testing.T) {
	apiErr := &sdk.APIError{
		Type:    sdk.ErrorTypeValidation,
		Message: "validation failed",
		Details: map[string]interface{}{
			"field": "email",
		},
		Code: http.StatusBadRequest,
	}

	// Test error string
	assert.Contains(t, apiErr.Error(), "validation")
	assert.Contains(t, apiErr.Error(), "validation failed")

	// Test type checking methods
	assert.True(t, apiErr.IsValidation())
	assert.False(t, apiErr.IsNotFound())
	assert.False(t, apiErr.IsAlreadyExists())
	assert.False(t, apiErr.IsInternal())

	// Test AsAPIError
	err := error(apiErr)
	retrievedErr, ok := sdk.AsAPIError(err)
	assert.True(t, ok)
	assert.Equal(t, apiErr, retrievedErr)
}

func TestParseUUID(t *testing.T) {
	validUUID := "550e8400-e29b-41d4-a716-446655440000"
	
	uuid, err := sdk.ParseUUID(validUUID)
	assert.NoError(t, err)
	assert.Equal(t, validUUID, uuid.String())

	_, err = sdk.ParseUUID("invalid-uuid")
	assert.Error(t, err)
}