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

type PostgresTestContainer struct {
	Pool     *dockertest.Pool
	Resource *dockertest.Resource
	URL      string
}

type TypesenseTestContainer struct {
	Pool     *dockertest.Pool
	Resource *dockertest.Resource
	URL      string
	APIKey   string
}

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

	resource.Expire(120) 

	pool.MaxWait = 120 * time.Second

	if err = pool.Retry(func() error {
		db, err := sql.Open("pgx", databaseUrl)
		if err != nil {
			return err
		}
		defer db.Close()
		return db.Ping()
	}); err != nil {
		
		pool.Purge(resource)
		return nil, fmt.Errorf("could not connect to database: %w", err)
	}

	db, err := sql.Open("pgx", databaseUrl)
	if err != nil {
		pool.Purge(resource)
		return nil, fmt.Errorf("could not open database: %w", err)
	}
	defer db.Close()

	_, err = db.Exec("CREATE EXTENSION IF NOT EXISTS vector")
	if err != nil {

		fmt.Printf("Warning: could not create vector extension: %v\n", err)
	}

	return &PostgresTestContainer{
		Pool:     pool,
		Resource: resource,
		URL:      databaseUrl,
	}, nil
}

func (c *PostgresTestContainer) Cleanup() error {
	if err := c.Pool.Purge(c.Resource); err != nil {
		return fmt.Errorf("could not purge resource: %w", err)
	}
	return nil
}

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

	resource.Expire(120) 

	pool.MaxWait = 120 * time.Second

	if err = pool.Retry(func() error {
		
		client := &http.Client{
			Timeout: 5 * time.Second,
		}

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

func (c *TypesenseTestContainer) Cleanup() error {
	if err := c.Pool.Purge(c.Resource); err != nil {
		return fmt.Errorf("could not purge resource: %w", err)
	}
	return nil
}

func MustSetupTestPostgres() *PostgresTestContainer {
	container, err := SetupTestPostgres()
	if err != nil {
		panic(fmt.Sprintf("failed to setup test postgres: %v", err))
	}
	return container
}

func MustSetupTestTypesense() *TypesenseTestContainer {
	container, err := SetupTestTypesense()
	if err != nil {
		panic(fmt.Sprintf("failed to setup test typesense: %v", err))
	}
	return container
}