package testutils

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/ory/dockertest/v3"
)

func TestSetupTestPostgres(t *testing.T) {
	container, err := SetupTestPostgres()
	if err != nil {
		t.Fatalf("Failed to setup postgres: %v", err)
	}
	defer container.Cleanup()

	// Test connection
	db, err := sql.Open("pgx", container.URL)
	if err != nil {
		t.Fatalf("Failed to open connection: %v", err)
	}
	defer db.Close()

	// Test query
	var result int
	err = db.QueryRow("SELECT 1").Scan(&result)
	if err != nil {
		t.Fatalf("Failed to execute query: %v", err)
	}

	if result != 1 {
		t.Errorf("Expected 1, got %d", result)
	}

	// Test pgvector extension
	_, err = db.Exec("CREATE EXTENSION IF NOT EXISTS vector")
	if err != nil {
		t.Logf("Warning: pgvector extension test failed: %v", err)
	} else {
		t.Log("pgvector extension created successfully")
	}
}

func TestSetupTestTypesense(t *testing.T) {
	// Try setting up with manual retry to debug
	pool, err := dockertest.NewPool("")
	if err != nil {
		t.Fatalf("Could not construct pool: %v", err)
	}

	apiKey := "test-api-key-12345"
	resource, err := pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "typesense/typesense",
		Tag:        "28.0",
		Env: []string{
			"TYPESENSE_DATA_DIR=/data",
			fmt.Sprintf("TYPESENSE_API_KEY=%s", apiKey),
			"TYPESENSE_ENABLE_CORS=true",
		},
	})
	if err != nil {
		t.Fatalf("Could not start resource: %v", err)
	}
	defer pool.Purge(resource)

	// Get container logs
	time.Sleep(5 * time.Second) // Let it start

	hostAndPort := resource.GetHostPort("8108/tcp")
	url := fmt.Sprintf("http://%s", hostAndPort)
	t.Logf("Typesense URL: %s", url)
	t.Logf("Typesense API Key: %s", apiKey)
}
