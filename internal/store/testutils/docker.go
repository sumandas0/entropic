package testutils

import (
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// PostgresTestContainer holds PostgreSQL test container resources
type PostgresTestContainer struct {
	Pool     *dockertest.Pool
	Resource *dockertest.Resource
	URL      string
}

// TypesenseTestContainer holds Typesense test container resources
type TypesenseTestContainer struct {
	Pool     *dockertest.Pool
	Resource *dockertest.Resource
	URL      string
	APIKey   string
}

// SetupTestPostgres creates a PostgreSQL test container with pgvector
func SetupTestPostgres() (*PostgresTestContainer, error) {
	pool, err := dockertest.NewPool("")
	if err != nil {
		return nil, fmt.Errorf("could not construct pool: %w", err)
	}

	err = pool.Client.Ping()
	if err != nil {
		return nil, fmt.Errorf("could not connect to Docker: %w", err)
	}

	resource, err := pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "pgvector/pgvector",
		Tag:        "pg17",
		Env: []string{
			"POSTGRES_PASSWORD=postgres",
			"POSTGRES_USER=postgres",
			"POSTGRES_DB=entropic_test",
			"listen_addresses = '*'",
		},
		Cmd: []string{
			"postgres",
			"-c", "shared_preload_libraries=vector",
		},
	}, func(config *docker.HostConfig) {
		config.AutoRemove = true
		config.RestartPolicy = docker.RestartPolicy{Name: "no"}
	})
	if err != nil {
		return nil, fmt.Errorf("could not start resource: %w", err)
	}

	hostAndPort := resource.GetHostPort("5432/tcp")
	databaseUrl := fmt.Sprintf("postgres://postgres:postgres@%s/entropic_test?sslmode=disable", hostAndPort)

	resource.Expire(120) // Tell Docker to hard kill the container in 120 seconds

	// Set max wait time to 120 seconds
	pool.MaxWait = 120 * time.Second

	// Wait for the database to be ready
	if err = pool.Retry(func() error {
		db, err := sql.Open("pgx", databaseUrl)
		if err != nil {
			return err
		}
		defer db.Close()
		return db.Ping()
	}); err != nil {
		// Clean up on failure
		pool.Purge(resource)
		return nil, fmt.Errorf("could not connect to database: %w", err)
	}

	// Create pgvector extension with superuser
	db, err := sql.Open("pgx", databaseUrl)
	if err != nil {
		pool.Purge(resource)
		return nil, fmt.Errorf("could not open database: %w", err)
	}
	defer db.Close()

	// First create the extension in template1 to make it available
	_, err = db.Exec("CREATE EXTENSION IF NOT EXISTS vector")
	if err != nil {
		// If it fails, try without the extension (some tests might not need it)
		// Log the error but don't fail
		fmt.Printf("Warning: could not create vector extension: %v\n", err)
	}

	return &PostgresTestContainer{
		Pool:     pool,
		Resource: resource,
		URL:      databaseUrl,
	}, nil
}

// Cleanup purges the PostgreSQL test container
func (c *PostgresTestContainer) Cleanup() error {
	if err := c.Pool.Purge(c.Resource); err != nil {
		return fmt.Errorf("could not purge resource: %w", err)
	}
	return nil
}

// SetupTestTypesense creates a Typesense test container
func SetupTestTypesense() (*TypesenseTestContainer, error) {
	pool, err := dockertest.NewPool("")
	if err != nil {
		return nil, fmt.Errorf("could not construct pool: %w", err)
	}

	err = pool.Client.Ping()
	if err != nil {
		return nil, fmt.Errorf("could not connect to Docker: %w", err)
	}

	apiKey := "test-api-key-12345"

	resource, err := pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "typesense/typesense",
		Tag:        "26.0",
		Env: []string{
			"TYPESENSE_DATA_DIR=/data",
			fmt.Sprintf("TYPESENSE_API_KEY=%s", apiKey),
			"TYPESENSE_ENABLE_CORS=true",
		},
		Cmd: []string{
			"--data-dir=/data",
			"--api-key=" + apiKey,
			"--enable-cors",
		},
	}, func(config *docker.HostConfig) {
		config.AutoRemove = true
		config.RestartPolicy = docker.RestartPolicy{Name: "no"}
	})
	if err != nil {
		return nil, fmt.Errorf("could not start resource: %w", err)
	}

	hostAndPort := resource.GetHostPort("8108/tcp")
	url := fmt.Sprintf("http://%s", hostAndPort)
	
	fmt.Printf("Typesense container started, URL: %s\n", url)

	resource.Expire(120) // Tell Docker to hard kill the container in 120 seconds

	// Set max wait time to 120 seconds
	pool.MaxWait = 120 * time.Second

	// Wait for Typesense to be ready
	if err = pool.Retry(func() error {
		// Create a custom HTTP client with timeout
		client := &http.Client{
			Timeout: 5 * time.Second,
		}
		
		// Check if Typesense is ready by hitting the health endpoint
		req, err := http.NewRequest("GET", fmt.Sprintf("%s/health", url), nil)
		if err != nil {
			return err
		}
		req.Header.Set("X-TYPESENSE-API-KEY", apiKey)
		
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("typesense not ready, status: %d", resp.StatusCode)
		}
		return nil
	}); err != nil {
		// Clean up on failure
		pool.Purge(resource)
		return nil, fmt.Errorf("could not connect to typesense: %w", err)
	}

	return &TypesenseTestContainer{
		Pool:     pool,
		Resource: resource,
		URL:      url,
		APIKey:   apiKey,
	}, nil
}

// Cleanup purges the Typesense test container
func (c *TypesenseTestContainer) Cleanup() error {
	if err := c.Pool.Purge(c.Resource); err != nil {
		return fmt.Errorf("could not purge resource: %w", err)
	}
	return nil
}

// MustSetupTestPostgres creates a PostgreSQL test container and panics on error
func MustSetupTestPostgres() *PostgresTestContainer {
	container, err := SetupTestPostgres()
	if err != nil {
		panic(fmt.Sprintf("failed to setup test postgres: %v", err))
	}
	return container
}

// MustSetupTestTypesense creates a Typesense test container and panics on error
func MustSetupTestTypesense() *TypesenseTestContainer {
	container, err := SetupTestTypesense()
	if err != nil {
		panic(fmt.Sprintf("failed to setup test typesense: %v", err))
	}
	return container
}