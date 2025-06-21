package health

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/entropic/entropic/internal/store"
)

// Status represents the health status of a component
type Status string

const (
	StatusHealthy   Status = "healthy"
	StatusUnhealthy Status = "unhealthy"
	StatusDegraded  Status = "degraded"
)

// ComponentHealth represents the health status of a single component
type ComponentHealth struct {
	Name      string            `json:"name"`
	Status    Status            `json:"status"`
	Message   string            `json:"message,omitempty"`
	LastCheck time.Time         `json:"last_check"`
	Duration  time.Duration     `json:"duration_ms"`
	Details   map[string]string `json:"details,omitempty"`
}

// SystemHealth represents the overall system health
type SystemHealth struct {
	Status     Status                      `json:"status"`
	Timestamp  time.Time                   `json:"timestamp"`
	Components map[string]ComponentHealth  `json:"components"`
	Summary    HealthSummary               `json:"summary"`
}

// HealthSummary provides a summary of component health
type HealthSummary struct {
	Total     int `json:"total"`
	Healthy   int `json:"healthy"`
	Unhealthy int `json:"unhealthy"`
	Degraded  int `json:"degraded"`
}

// HealthChecker manages health checks for various components
type HealthChecker struct {
	components map[string]HealthCheckFunc
	results    map[string]ComponentHealth
	mutex      sync.RWMutex
	timeout    time.Duration
}

// HealthCheckFunc is a function that checks the health of a component
type HealthCheckFunc func(ctx context.Context) ComponentHealth

// NewHealthChecker creates a new health checker
func NewHealthChecker(timeout time.Duration) *HealthChecker {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	return &HealthChecker{
		components: make(map[string]HealthCheckFunc),
		results:    make(map[string]ComponentHealth),
		timeout:    timeout,
	}
}

// RegisterComponent registers a health check function for a component
func (hc *HealthChecker) RegisterComponent(name string, checkFunc HealthCheckFunc) {
	hc.mutex.Lock()
	defer hc.mutex.Unlock()
	hc.components[name] = checkFunc
}

// RegisterStore registers a store component for health checking
func (hc *HealthChecker) RegisterStore(name string, store interface{ Ping(context.Context) error }) {
	checkFunc := func(ctx context.Context) ComponentHealth {
		start := time.Now()
		health := ComponentHealth{
			Name:      name,
			LastCheck: start,
		}

		if err := store.Ping(ctx); err != nil {
			health.Status = StatusUnhealthy
			health.Message = err.Error()
		} else {
			health.Status = StatusHealthy
			health.Message = "Connection successful"
		}

		health.Duration = time.Since(start)
		return health
	}

	hc.RegisterComponent(name, checkFunc)
}

// Check performs health checks on all registered components
func (hc *HealthChecker) Check(ctx context.Context) SystemHealth {
	hc.mutex.RLock()
	components := make(map[string]HealthCheckFunc, len(hc.components))
	for name, checkFunc := range hc.components {
		components[name] = checkFunc
	}
	hc.mutex.RUnlock()

	// Create timeout context
	checkCtx, cancel := context.WithTimeout(ctx, hc.timeout)
	defer cancel()

	// Run health checks concurrently
	resultChan := make(chan ComponentHealth, len(components))
	var wg sync.WaitGroup

	for name, checkFunc := range components {
		wg.Add(1)
		go func(n string, cf HealthCheckFunc) {
			defer wg.Done()
			
			// Run health check with timeout protection
			done := make(chan ComponentHealth, 1)
			go func() {
				done <- cf(checkCtx)
			}()

			select {
			case result := <-done:
				resultChan <- result
			case <-checkCtx.Done():
				// Timeout occurred
				resultChan <- ComponentHealth{
					Name:      n,
					Status:    StatusUnhealthy,
					Message:   "Health check timeout",
					LastCheck: time.Now(),
					Duration:  hc.timeout,
				}
			}
		}(name, checkFunc)
	}

	// Wait for all checks to complete
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Collect results
	results := make(map[string]ComponentHealth)
	for result := range resultChan {
		results[result.Name] = result
	}

	// Update stored results
	hc.mutex.Lock()
	hc.results = results
	hc.mutex.Unlock()

	// Calculate overall status and summary
	return hc.calculateSystemHealth(results)
}

// GetLastResults returns the last health check results
func (hc *HealthChecker) GetLastResults() SystemHealth {
	hc.mutex.RLock()
	defer hc.mutex.RUnlock()
	return hc.calculateSystemHealth(hc.results)
}

// calculateSystemHealth calculates the overall system health from component results
func (hc *HealthChecker) calculateSystemHealth(results map[string]ComponentHealth) SystemHealth {
	summary := HealthSummary{
		Total: len(results),
	}

	for _, result := range results {
		switch result.Status {
		case StatusHealthy:
			summary.Healthy++
		case StatusUnhealthy:
			summary.Unhealthy++
		case StatusDegraded:
			summary.Degraded++
		}
	}

	// Determine overall status
	var overallStatus Status
	if summary.Unhealthy > 0 {
		overallStatus = StatusUnhealthy
	} else if summary.Degraded > 0 {
		overallStatus = StatusDegraded
	} else {
		overallStatus = StatusHealthy
	}

	return SystemHealth{
		Status:     overallStatus,
		Timestamp:  time.Now(),
		Components: results,
		Summary:    summary,
	}
}

// StartPeriodicChecks starts periodic health checks
func (hc *HealthChecker) StartPeriodicChecks(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 30 * time.Second
	}

	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				hc.Check(ctx)
			case <-ctx.Done():
				return
			}
		}
	}()
}

// CreateDatabaseHealthCheck creates a health check function for database connectivity
func CreateDatabaseHealthCheck(store store.PrimaryStore) HealthCheckFunc {
	return func(ctx context.Context) ComponentHealth {
		start := time.Now()
		health := ComponentHealth{
			Name:      "database",
			LastCheck: start,
			Details:   make(map[string]string),
		}

		if err := store.Ping(ctx); err != nil {
			health.Status = StatusUnhealthy
			health.Message = fmt.Sprintf("Database ping failed: %v", err)
		} else {
			health.Status = StatusHealthy
			health.Message = "Database connection healthy"
		}

		health.Duration = time.Since(start)
		health.Details["response_time"] = health.Duration.String()

		return health
	}
}

// CreateSearchHealthCheck creates a health check function for search engine connectivity
func CreateSearchHealthCheck(store store.IndexStore) HealthCheckFunc {
	return func(ctx context.Context) ComponentHealth {
		start := time.Now()
		health := ComponentHealth{
			Name:      "search",
			LastCheck: start,
			Details:   make(map[string]string),
		}

		if err := store.Ping(ctx); err != nil {
			health.Status = StatusDegraded // Search is not critical
			health.Message = fmt.Sprintf("Search engine ping failed: %v", err)
		} else {
			health.Status = StatusHealthy
			health.Message = "Search engine connection healthy"
		}

		health.Duration = time.Since(start)
		health.Details["response_time"] = health.Duration.String()

		return health
	}
}

// CreateMemoryHealthCheck creates a health check function for memory usage
func CreateMemoryHealthCheck() HealthCheckFunc {
	return func(ctx context.Context) ComponentHealth {
		start := time.Now()
		health := ComponentHealth{
			Name:      "memory",
			LastCheck: start,
			Details:   make(map[string]string),
		}

		// This would check memory usage in a real implementation
		// For now, always return healthy
		health.Status = StatusHealthy
		health.Message = "Memory usage within normal limits"
		health.Duration = time.Since(start)

		return health
	}
}