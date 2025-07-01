package testutils

import (
	"bytes"
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

	resource.Expire(180) 

	pool.MaxWait = 180 * time.Second

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

	// Set the endpoint explicitly for Docker Desktop on macOS
	if err := pool.Client.Ping(); err != nil {
		// Try to connect with explicit endpoint
		pool, err = dockertest.NewPool("unix:///var/run/docker.sock")
		if err != nil {
			return nil, fmt.Errorf("could not construct pool with explicit endpoint: %w", err)
		}
		
		if err := pool.Client.Ping(); err != nil {
			return nil, fmt.Errorf("could not connect to Docker: %w", err)
		}
	}

	apiKey := "test-api-key-12345"

	runOpts := &dockertest.RunOptions{
		Repository: "typesense/typesense",
		Tag:        "28.0",
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
	}
	
	fmt.Printf("Creating Typesense container with options: %+v\n", runOpts)
	
	resource, err := pool.RunWithOptions(runOpts, func(config *docker.HostConfig) {
		config.AutoRemove = true
		config.RestartPolicy = docker.RestartPolicy{Name: "no"}
		// Create tmpfs mount for data directory
		config.Tmpfs = map[string]string{
			"/data": "size=100m",
		}
		// Ensure port mapping
		config.PortBindings = map[docker.Port][]docker.PortBinding{
			"8108/tcp": {{HostIP: "0.0.0.0", HostPort: ""}}, // Let Docker assign a random port
		}
		fmt.Printf("Host config: AutoRemove=%v, Tmpfs=%v\n", config.AutoRemove, config.Tmpfs)
	})
	if err != nil {
		return nil, fmt.Errorf("could not start resource: %w", err)
	}

	// Get the port mapping
	hostPort := resource.GetHostPort("8108/tcp")
	var url string
	
	if hostPort != "" {
		// Use the host port mapping
		url = fmt.Sprintf("http://%s", hostPort)
	} else {
		// Fall back to container IP if port mapping failed
		containerIP := resource.Container.NetworkSettings.IPAddress
		if containerIP == "" {
			return nil, fmt.Errorf("could not get container IP address")
		}
		url = fmt.Sprintf("http://%s:8108", containerIP)
	}
	
	fmt.Printf("Typesense container started, URL: %s\n", url)

	resource.Expire(300) // Increase to 5 minutes

	pool.MaxWait = 300 * time.Second

	// Initial delay to allow Typesense to start
	time.Sleep(3 * time.Second)

	// Retry with exponential backoff
	var lastErr error
	maxRetries := 30
	for i := 0; i < maxRetries; i++ {
		client := &http.Client{
			Timeout: 5 * time.Second,
		}

		req, err := http.NewRequest("GET", fmt.Sprintf("%s/health", url), nil)
		if err != nil {
			lastErr = err
			fmt.Printf("Attempt %d: Failed to create request: %v\n", i+1, err)
			time.Sleep(time.Duration(i+1) * time.Second)
			continue
		}
		req.Header.Set("X-TYPESENSE-API-KEY", apiKey)
		
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			fmt.Printf("Attempt %d: Failed to connect to Typesense at %s: %v\n", i+1, url, err)
			time.Sleep(time.Duration(i+1) * time.Second)
			continue
		}
		resp.Body.Close()
		
		if resp.StatusCode == http.StatusOK {
			fmt.Printf("Typesense ready after %d attempts\n", i+1)
			return &TypesenseTestContainer{
				Pool:     pool,
				Resource: resource,
				URL:      url,
				APIKey:   apiKey,
			}, nil
		}
		
		lastErr = fmt.Errorf("typesense not ready, status: %d", resp.StatusCode)
		fmt.Printf("Attempt %d: Typesense returned status %d\n", i+1, resp.StatusCode)
		time.Sleep(time.Duration(i+1) * time.Second)
	}
	
	// Get container logs for debugging
	logs, err := getContainerLogs(pool, resource)
	if err != nil {
		fmt.Printf("Failed to get container logs: %v\n", err)
	} else {
		fmt.Printf("Container logs:\n%s\n", logs)
	}
	
	// Cleanup on failure
	pool.Purge(resource)
	return nil, fmt.Errorf("could not connect to typesense after %d attempts: %w", maxRetries, lastErr)
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

// getContainerLogs retrieves the logs from a Docker container
func getContainerLogs(pool *dockertest.Pool, resource *dockertest.Resource) (string, error) {
	var buf bytes.Buffer
	opts := docker.LogsOptions{
		Container:    resource.Container.ID,
		OutputStream: &buf,
		ErrorStream:  &buf,
		Stdout:       true,
		Stderr:       true,
		Timestamps:   true,
		Tail:         "50", // Last 50 lines
	}
	
	err := pool.Client.Logs(opts)
	if err != nil {
		return "", err
	}
	
	return buf.String(), nil
}