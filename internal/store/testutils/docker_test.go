package testutils

import (
	"database/sql"
	"net/http"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestSetupTestPostgres(t *testing.T) {
	container, err := SetupTestPostgres()
	if err != nil {
		t.Fatalf("Failed to setup postgres: %v", err)
	}
	defer container.Cleanup()

	db, err := sql.Open("pgx", container.URL)
	if err != nil {
		t.Fatalf("Failed to open connection: %v", err)
	}
	defer db.Close()

	var result int
	err = db.QueryRow("SELECT 1").Scan(&result)
	if err != nil {
		t.Fatalf("Failed to execute query: %v", err)
	}

	if result != 1 {
		t.Errorf("Expected 1, got %d", result)
	}

	_, err = db.Exec("CREATE EXTENSION IF NOT EXISTS vector")
	if err != nil {
		t.Logf("Warning: pgvector extension test failed: %v", err)
	} else {
		t.Log("pgvector extension created successfully")
	}
}

func TestSetupTestTypesense(t *testing.T) {
	container, err := SetupTestTypesense()
	if err != nil {
		t.Fatalf("Failed to setup typesense: %v", err)
	}
	defer container.Cleanup()

	// Test that the container was created with proper fields
	if container.URL == "" {
		t.Error("Typesense URL is empty")
	}
	if container.APIKey == "" {
		t.Error("Typesense API key is empty")
	}
	if container.Pool == nil {
		t.Error("Docker pool is nil")
	}
	if container.Resource == nil {
		t.Error("Docker resource is nil")
	}

	// Test health check using raw HTTP
	httpClient := &http.Client{}
	req, err := http.NewRequest("GET", container.URL+"/health", nil)
	if err != nil {
		t.Fatalf("Failed to create health request: %v", err)
	}
	req.Header.Set("X-TYPESENSE-API-KEY", container.APIKey)

	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to check Typesense health: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	t.Logf("Typesense container started successfully at %s", container.URL)
}
