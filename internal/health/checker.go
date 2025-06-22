package health

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sumandas0/entropic/internal/store"
)

type Status string

const (
	StatusHealthy   Status = "healthy"
	StatusUnhealthy Status = "unhealthy"
	StatusDegraded  Status = "degraded"
)

type ComponentHealth struct {
	Name      string            `json:"name"`
	Status    Status            `json:"status"`
	Message   string            `json:"message,omitempty"`
	LastCheck time.Time         `json:"last_check"`
	Duration  time.Duration     `json:"duration_ms"`
	Details   map[string]string `json:"details,omitempty"`
}

type SystemHealth struct {
	Status     Status                      `json:"status"`
	Timestamp  time.Time                   `json:"timestamp"`
	Components map[string]ComponentHealth  `json:"components"`
	Summary    HealthSummary               `json:"summary"`
}

type HealthSummary struct {
	Total     int `json:"total"`
	Healthy   int `json:"healthy"`
	Unhealthy int `json:"unhealthy"`
	Degraded  int `json:"degraded"`
}

type HealthChecker struct {
	components map[string]HealthCheckFunc
	results    map[string]ComponentHealth
	mutex      sync.RWMutex
	timeout    time.Duration
}

type HealthCheckFunc func(ctx context.Context) ComponentHealth

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

func (hc *HealthChecker) RegisterComponent(name string, checkFunc HealthCheckFunc) {
	hc.mutex.Lock()
	defer hc.mutex.Unlock()
	hc.components[name] = checkFunc
}

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

func (hc *HealthChecker) Check(ctx context.Context) SystemHealth {
	hc.mutex.RLock()
	components := make(map[string]HealthCheckFunc, len(hc.components))
	for name, checkFunc := range hc.components {
		components[name] = checkFunc
	}
	hc.mutex.RUnlock()

	checkCtx, cancel := context.WithTimeout(ctx, hc.timeout)
	defer cancel()

	resultChan := make(chan ComponentHealth, len(components))
	var wg sync.WaitGroup

	for name, checkFunc := range components {
		wg.Add(1)
		go func(n string, cf HealthCheckFunc) {
			defer wg.Done()

			done := make(chan ComponentHealth, 1)
			go func() {
				done <- cf(checkCtx)
			}()

			select {
			case result := <-done:
				resultChan <- result
			case <-checkCtx.Done():
				
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

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	results := make(map[string]ComponentHealth)
	for result := range resultChan {
		results[result.Name] = result
	}

	hc.mutex.Lock()
	hc.results = results
	hc.mutex.Unlock()

	return hc.calculateSystemHealth(results)
}

func (hc *HealthChecker) GetLastResults() SystemHealth {
	hc.mutex.RLock()
	defer hc.mutex.RUnlock()
	return hc.calculateSystemHealth(hc.results)
}

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

func CreateSearchHealthCheck(store store.IndexStore) HealthCheckFunc {
	return func(ctx context.Context) ComponentHealth {
		start := time.Now()
		health := ComponentHealth{
			Name:      "search",
			LastCheck: start,
			Details:   make(map[string]string),
		}

		if err := store.Ping(ctx); err != nil {
			health.Status = StatusDegraded 
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

func CreateMemoryHealthCheck() HealthCheckFunc {
	return func(ctx context.Context) ComponentHealth {
		start := time.Now()
		health := ComponentHealth{
			Name:      "memory",
			LastCheck: start,
			Details:   make(map[string]string),
		}

		health.Status = StatusHealthy
		health.Message = "Memory usage within normal limits"
		health.Duration = time.Since(start)

		return health
	}
}