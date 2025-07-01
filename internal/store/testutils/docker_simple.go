package testutils

import (
	"fmt"
	"net/http"
	"time"

	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
)

// SetupSimpleTypesense creates a Typesense container with simplified configuration
func SetupSimpleTypesense() (*TypesenseTestContainer, error) {
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

	// Set a longer timeout for CI/slow systems
	pool.MaxWait = 300 * time.Second

	apiKey := "test-api-key-12345"

	// Run with minimal configuration
	resource, err := pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "typesense/typesense",
		Tag:        "28.0",
		Env: []string{
			fmt.Sprintf("TYPESENSE_API_KEY=%s", apiKey),
		},
	}, func(config *docker.HostConfig) {
		config.AutoRemove = true
		config.RestartPolicy = docker.RestartPolicy{Name: "no"}
		// Ensure port mapping
		config.PortBindings = map[docker.Port][]docker.PortBinding{
			"8108/tcp": {{HostIP: "0.0.0.0", HostPort: ""}},
		}
	})
	if err != nil {
		return nil, fmt.Errorf("could not start resource: %w", err)
	}

	// Set container to expire after 5 minutes
	resource.Expire(300)

	// Get the mapped port
	hostPort := resource.GetPort("8108/tcp")
	if hostPort == "" {
		pool.Purge(resource)
		return nil, fmt.Errorf("could not get host port")
	}

	url := fmt.Sprintf("http://localhost:%s", hostPort)
	
	// Give Typesense time to start up
	time.Sleep(2 * time.Second)

	// Retry connecting with exponential backoff
	var lastErr error
	maxRetries := 30
	for i := 0; i < maxRetries; i++ {
		client := &http.Client{
			Timeout: 5 * time.Second,
		}

		req, err := http.NewRequest("GET", fmt.Sprintf("%s/health", url), nil)
		if err != nil {
			lastErr = err
			time.Sleep(time.Duration(i+1) * time.Second)
			continue
		}
		req.Header.Set("X-TYPESENSE-API-KEY", apiKey)
		
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			fmt.Printf("Typesense health check attempt %d failed: %v\n", i+1, err)
			time.Sleep(time.Duration(i+1) * time.Second)
			continue
		}
		resp.Body.Close()
		
		if resp.StatusCode == http.StatusOK {
			fmt.Printf("Typesense ready after %d attempts at %s\n", i+1, url)
			return &TypesenseTestContainer{
				Pool:     pool,
				Resource: resource,
				URL:      url,
				APIKey:   apiKey,
			}, nil
		}
		
		lastErr = fmt.Errorf("typesense not ready, status: %d", resp.StatusCode)
		time.Sleep(time.Duration(i+1) * time.Second)
	}

	// Cleanup on failure
	pool.Purge(resource)
	return nil, fmt.Errorf("could not connect to typesense after %d attempts: %w", maxRetries, lastErr)
}