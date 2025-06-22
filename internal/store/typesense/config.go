package typesense

import (
	"context"
	"fmt"
	"time"

	"github.com/typesense/typesense-go/typesense"
)

type Config struct {
	ServerURL           string
	APIKey              string
	ConnectionTimeout   time.Duration
	NumRetries          int
	RetryInterval       time.Duration
	HealthCheckInterval time.Duration
}

func DefaultConfig() *Config {
	return &Config{
		ConnectionTimeout:   5 * time.Second,
		NumRetries:          3,
		RetryInterval:       time.Second,
		HealthCheckInterval: 30 * time.Second,
	}
}

func (c *Config) Validate() error {
	if c.ServerURL == "" {
		return fmt.Errorf("server URL is required")
	}
	if c.APIKey == "" {
		return fmt.Errorf("API key is required")
	}
	if c.ConnectionTimeout <= 0 {
		c.ConnectionTimeout = 5 * time.Second
	}
	if c.NumRetries < 0 {
		c.NumRetries = 3
	}
	if c.RetryInterval <= 0 {
		c.RetryInterval = time.Second
	}
	return nil
}

func NewClient(config *Config) (*typesense.Client, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	client := typesense.NewClient(
		typesense.WithServer(config.ServerURL),
		typesense.WithAPIKey(config.APIKey),
		typesense.WithConnectionTimeout(config.ConnectionTimeout),
	)

	return client, nil
}

type HealthChecker struct {
	client   *typesense.Client
	interval time.Duration
	stopCh   chan struct{}
}

func NewHealthChecker(client *typesense.Client, interval time.Duration) *HealthChecker {
	return &HealthChecker{
		client:   client,
		interval: interval,
		stopCh:   make(chan struct{}),
	}
}

func (h *HealthChecker) Start(ctx context.Context) {
	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := h.check(ctx); err != nil {
				
				fmt.Printf("Typesense health check failed: %v\n", err)
			}
		case <-h.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

func (h *HealthChecker) Stop() {
	close(h.stopCh)
}

func (h *HealthChecker) check(ctx context.Context) error {
	healthy, err := h.client.Health(ctx, 5*time.Second)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}

	if !healthy {
		return fmt.Errorf("typesense is not healthy")
	}

	return nil
}

type CollectionManager struct {
	client *typesense.Client
}

func NewCollectionManager(client *typesense.Client) *CollectionManager {
	return &CollectionManager{client: client}
}

func (m *CollectionManager) ListCollections(ctx context.Context) ([]string, error) {
	collections, err := m.client.Collections().Retrieve(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list collections: %w", err)
	}

	names := make([]string, len(collections))
	for i, col := range collections {
		names[i] = col.Name
	}

	return names, nil
}

func (m *CollectionManager) CollectionExists(ctx context.Context, name string) (bool, error) {
	_, err := m.client.Collection(name).Retrieve(ctx)
	if err != nil {
		if isNotFoundError(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to check collection existence: %w", err)
	}
	return true, nil
}

func (m *CollectionManager) DropAllCollections(ctx context.Context) error {
	collections, err := m.ListCollections(ctx)
	if err != nil {
		return err
	}

	for _, name := range collections {
		if _, err := m.client.Collection(name).Delete(ctx); err != nil {
			return fmt.Errorf("failed to delete collection %s: %w", name, err)
		}
	}

	return nil
}

func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	
	return contains(err.Error(), "not found") || contains(err.Error(), "404")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && 
		(s == substr || 
		 len(s) > len(substr) && 
		 (contains(s[1:], substr) || contains(s[:len(s)-1], substr)))
}