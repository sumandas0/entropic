package resilience

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/sony/gobreaker"
)

type CircuitBreakerConfig struct {
	Enabled          bool          `yaml:"enabled" mapstructure:"enabled"`
	MaxRequests      uint32        `yaml:"max_requests" mapstructure:"max_requests"`
	Interval         time.Duration `yaml:"interval" mapstructure:"interval"`
	Timeout          time.Duration `yaml:"timeout" mapstructure:"timeout"`
	FailureThreshold uint32        `yaml:"failure_threshold" mapstructure:"failure_threshold"`
	SuccessThreshold uint32        `yaml:"success_threshold" mapstructure:"success_threshold"`
}

type CircuitBreakerManager struct {
	config   CircuitBreakerConfig
	breakers map[string]*gobreaker.CircuitBreaker
	mutex    sync.RWMutex
}

func NewCircuitBreakerManager(config CircuitBreakerConfig) *CircuitBreakerManager {
	return &CircuitBreakerManager{
		config:   config,
		breakers: make(map[string]*gobreaker.CircuitBreaker),
	}
}

func (cbm *CircuitBreakerManager) GetBreaker(serviceName string) *gobreaker.CircuitBreaker {
	if !cbm.config.Enabled {
		return nil
	}

	cbm.mutex.RLock()
	breaker, exists := cbm.breakers[serviceName]
	cbm.mutex.RUnlock()

	if exists {
		return breaker
	}

	cbm.mutex.Lock()
	defer cbm.mutex.Unlock()

	if breaker, exists := cbm.breakers[serviceName]; exists {
		return breaker
	}

	settings := gobreaker.Settings{
		Name:        serviceName,
		MaxRequests: cbm.config.MaxRequests,
		Interval:    cbm.config.Interval,
		Timeout:     cbm.config.Timeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= cbm.config.FailureThreshold
		},
		OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {

			fmt.Printf("Circuit breaker %s changed from %s to %s\n", name, from, to)
		},
		IsSuccessful: func(err error) bool {

			return err == nil
		},
	}

	breaker = gobreaker.NewCircuitBreaker(settings)
	cbm.breakers[serviceName] = breaker

	return breaker
}

func (cbm *CircuitBreakerManager) Execute(serviceName string, fn func() (any, error)) (any, error) {
	if !cbm.config.Enabled {
		return fn()
	}

	breaker := cbm.GetBreaker(serviceName)
	if breaker == nil {
		return fn()
	}

	return breaker.Execute(fn)
}

func (cbm *CircuitBreakerManager) ExecuteWithContext(ctx context.Context, serviceName string, fn func(context.Context) (any, error)) (any, error) {
	if !cbm.config.Enabled {
		return fn(ctx)
	}

	breaker := cbm.GetBreaker(serviceName)
	if breaker == nil {
		return fn(ctx)
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	return breaker.Execute(func() (any, error) {
		return fn(ctx)
	})
}

func (cbm *CircuitBreakerManager) GetState(serviceName string) gobreaker.State {
	cbm.mutex.RLock()
	defer cbm.mutex.RUnlock()

	if breaker, exists := cbm.breakers[serviceName]; exists {
		return breaker.State()
	}

	return gobreaker.StateClosed
}

func (cbm *CircuitBreakerManager) GetCounts(serviceName string) gobreaker.Counts {
	cbm.mutex.RLock()
	defer cbm.mutex.RUnlock()

	if breaker, exists := cbm.breakers[serviceName]; exists {
		return breaker.Counts()
	}

	return gobreaker.Counts{}
}

func (cbm *CircuitBreakerManager) IsEnabled() bool {
	return cbm.config.Enabled
}

type DatabaseCircuitBreaker struct {
	manager *CircuitBreakerManager
}

func NewDatabaseCircuitBreaker(manager *CircuitBreakerManager) *DatabaseCircuitBreaker {
	return &DatabaseCircuitBreaker{
		manager: manager,
	}
}

func (dcb *DatabaseCircuitBreaker) Execute(ctx context.Context, operation string, fn func(context.Context) (any, error)) (any, error) {
	serviceName := fmt.Sprintf("database-%s", operation)
	return dcb.manager.ExecuteWithContext(ctx, serviceName, fn)
}

type SearchCircuitBreaker struct {
	manager *CircuitBreakerManager
}

func NewSearchCircuitBreaker(manager *CircuitBreakerManager) *SearchCircuitBreaker {
	return &SearchCircuitBreaker{
		manager: manager,
	}
}

func (scb *SearchCircuitBreaker) Execute(ctx context.Context, searchType string, fn func(context.Context) (any, error)) (any, error) {
	serviceName := fmt.Sprintf("search-%s", searchType)
	return scb.manager.ExecuteWithContext(ctx, serviceName, fn)
}

type CacheCircuitBreaker struct {
	manager *CircuitBreakerManager
}

func NewCacheCircuitBreaker(manager *CircuitBreakerManager) *CacheCircuitBreaker {
	return &CacheCircuitBreaker{
		manager: manager,
	}
}

func (ccb *CacheCircuitBreaker) Execute(ctx context.Context, operation string, fn func(context.Context) (any, error)) (any, error) {
	serviceName := fmt.Sprintf("cache-%s", operation)
	return ccb.manager.ExecuteWithContext(ctx, serviceName, fn)
}

type ExternalServiceCircuitBreaker struct {
	manager *CircuitBreakerManager
}

func NewExternalServiceCircuitBreaker(manager *CircuitBreakerManager) *ExternalServiceCircuitBreaker {
	return &ExternalServiceCircuitBreaker{
		manager: manager,
	}
}

func (escb *ExternalServiceCircuitBreaker) Execute(ctx context.Context, serviceName string, fn func(context.Context) (any, error)) (any, error) {
	return escb.manager.ExecuteWithContext(ctx, serviceName, fn)
}

var (
	ErrCircuitBreakerOpen     = errors.New("circuit breaker is open")
	ErrCircuitBreakerHalfOpen = errors.New("circuit breaker is half-open and rejecting requests")
	ErrServiceUnavailable     = errors.New("service temporarily unavailable")
)

func IsCircuitBreakerError(err error) bool {
	if err == nil {
		return false
	}

	switch err {
	case gobreaker.ErrOpenState, gobreaker.ErrTooManyRequests:
		return true
	default:
		return false
	}
}

func (cbm *CircuitBreakerManager) CircuitBreakerMiddleware(serviceName string) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !cbm.config.Enabled {
				next.ServeHTTP(w, r)
				return
			}

			_, err := cbm.ExecuteWithContext(r.Context(), serviceName, func(ctx context.Context) (any, error) {

				recorder := &responseRecorder{
					ResponseWriter: w,
					statusCode:     200,
				}

				next.ServeHTTP(recorder, r.WithContext(ctx))

				if recorder.statusCode >= 500 {
					return nil, fmt.Errorf("HTTP %d", recorder.statusCode)
				}

				return nil, nil
			})

			if err != nil && IsCircuitBreakerError(err) {
				http.Error(w, "Service temporarily unavailable", http.StatusServiceUnavailable)
				return
			}
		})
	}
}

type responseRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (rr *responseRecorder) WriteHeader(statusCode int) {
	rr.statusCode = statusCode
	rr.ResponseWriter.WriteHeader(statusCode)
}

type CircuitBreakerHealthCheck struct {
	manager *CircuitBreakerManager
}

func NewCircuitBreakerHealthCheck(manager *CircuitBreakerManager) *CircuitBreakerHealthCheck {
	return &CircuitBreakerHealthCheck{
		manager: manager,
	}
}

func (cbhc *CircuitBreakerHealthCheck) Check(ctx context.Context) map[string]any {
	cbhc.manager.mutex.RLock()
	defer cbhc.manager.mutex.RUnlock()

	status := make(map[string]any)

	for name, breaker := range cbhc.manager.breakers {
		state := breaker.State()
		counts := breaker.Counts()

		status[name] = map[string]any{
			"state":                 state.String(),
			"requests":              counts.Requests,
			"total_successes":       counts.TotalSuccesses,
			"total_failures":        counts.TotalFailures,
			"consecutive_successes": counts.ConsecutiveSuccesses,
			"consecutive_failures":  counts.ConsecutiveFailures,
		}
	}

	return map[string]any{
		"circuit_breakers": status,
		"enabled":          cbhc.manager.config.Enabled,
	}
}
