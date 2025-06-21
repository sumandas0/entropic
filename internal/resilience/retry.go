package resilience

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"time"
)

// RetryConfig holds configuration for retry mechanisms
type RetryConfig struct {
	Enabled           bool          `yaml:"enabled" mapstructure:"enabled"`
	MaxAttempts       int           `yaml:"max_attempts" mapstructure:"max_attempts"`
	InitialDelay      time.Duration `yaml:"initial_delay" mapstructure:"initial_delay"`
	MaxDelay          time.Duration `yaml:"max_delay" mapstructure:"max_delay"`
	BackoffMultiplier float64       `yaml:"backoff_multiplier" mapstructure:"backoff_multiplier"`
	JitterEnabled     bool          `yaml:"jitter_enabled" mapstructure:"jitter_enabled"`
	JitterFactor      float64       `yaml:"jitter_factor" mapstructure:"jitter_factor"`
}

// RetryStrategy defines different retry strategies
type RetryStrategy string

const (
	StrategyExponential RetryStrategy = "exponential"
	StrategyLinear      RetryStrategy = "linear"
	StrategyFixed       RetryStrategy = "fixed"
)

// RetryManager manages retry operations with different strategies
type RetryManager struct {
	config   RetryConfig
	strategy RetryStrategy
}

// NewRetryManager creates a new retry manager with the given configuration
func NewRetryManager(config RetryConfig, strategy RetryStrategy) *RetryManager {
	return &RetryManager{
		config:   config,
		strategy: strategy,
	}
}

// IsRetryableError defines which errors should trigger a retry
type IsRetryableError func(error) bool

// DefaultRetryableErrors defines common retryable error conditions
func DefaultRetryableErrors(err error) bool {
	if err == nil {
		return false
	}

	// Add common retryable error patterns
	errorStr := err.Error()
	
	// Network-related errors
	if contains(errorStr, "connection refused") ||
		contains(errorStr, "connection reset") ||
		contains(errorStr, "timeout") ||
		contains(errorStr, "temporary failure") ||
		contains(errorStr, "service unavailable") {
		return true
	}

	// Database-related errors
	if contains(errorStr, "connection lost") ||
		contains(errorStr, "deadlock") ||
		contains(errorStr, "lock timeout") {
		return true
	}

	return false
}

// DatabaseRetryableErrors defines database-specific retryable errors
func DatabaseRetryableErrors(err error) bool {
	if err == nil {
		return false
	}

	errorStr := err.Error()
	
	// PostgreSQL specific errors
	if contains(errorStr, "connection lost") ||
		contains(errorStr, "connection reset") ||
		contains(errorStr, "server closed the connection") ||
		contains(errorStr, "deadlock detected") ||
		contains(errorStr, "lock timeout") ||
		contains(errorStr, "serialization failure") {
		return true
	}

	return DefaultRetryableErrors(err)
}

// SearchRetryableErrors defines search-specific retryable errors
func SearchRetryableErrors(err error) bool {
	if err == nil {
		return false
	}

	errorStr := err.Error()
	
	// Typesense/Elasticsearch specific errors
	if contains(errorStr, "search request failed") ||
		contains(errorStr, "cluster not ready") ||
		contains(errorStr, "too many requests") ||
		contains(errorStr, "index not available") {
		return true
	}

	return DefaultRetryableErrors(err)
}

// Execute executes a function with retry logic
func (rm *RetryManager) Execute(ctx context.Context, fn func() error, isRetryable IsRetryableError) error {
	if !rm.config.Enabled {
		return fn()
	}

	var lastErr error
	
	for attempt := 1; attempt <= rm.config.MaxAttempts; attempt++ {
		// Check context cancellation before each attempt
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		err := fn()
		if err == nil {
			return nil
		}

		lastErr = err

		// Check if error is retryable
		if !isRetryable(err) {
			return err
		}

		// Don't delay after the last attempt
		if attempt == rm.config.MaxAttempts {
			break
		}

		// Calculate delay for next attempt
		delay := rm.calculateDelay(attempt)
		
		// Wait before next attempt, respecting context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
			continue
		}
	}

	return fmt.Errorf("operation failed after %d attempts: %w", rm.config.MaxAttempts, lastErr)
}

// ExecuteWithResult executes a function with retry logic and returns a result
func (rm *RetryManager) ExecuteWithResult(ctx context.Context, fn func() (interface{}, error), isRetryable IsRetryableError) (interface{}, error) {
	if !rm.config.Enabled {
		return fn()
	}

	var lastErr error
	var result interface{}
	
	for attempt := 1; attempt <= rm.config.MaxAttempts; attempt++ {
		// Check context cancellation before each attempt
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		result, err := fn()
		if err == nil {
			return result, nil
		}

		lastErr = err

		// Check if error is retryable
		if !isRetryable(err) {
			return nil, err
		}

		// Don't delay after the last attempt
		if attempt == rm.config.MaxAttempts {
			break
		}

		// Calculate delay for next attempt
		delay := rm.calculateDelay(attempt)
		
		// Wait before next attempt, respecting context cancellation
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
			continue
		}
	}

	return nil, fmt.Errorf("operation failed after %d attempts: %w", rm.config.MaxAttempts, lastErr)
}

// calculateDelay calculates the delay for the next retry attempt
func (rm *RetryManager) calculateDelay(attempt int) time.Duration {
	var delay time.Duration

	switch rm.strategy {
	case StrategyExponential:
		delay = rm.calculateExponentialDelay(attempt)
	case StrategyLinear:
		delay = rm.calculateLinearDelay(attempt)
	case StrategyFixed:
		delay = rm.config.InitialDelay
	default:
		delay = rm.calculateExponentialDelay(attempt)
	}

	// Apply jitter if enabled
	if rm.config.JitterEnabled {
		delay = rm.applyJitter(delay)
	}

	// Ensure delay doesn't exceed max delay
	if delay > rm.config.MaxDelay {
		delay = rm.config.MaxDelay
	}

	return delay
}

// calculateExponentialDelay calculates exponential backoff delay
func (rm *RetryManager) calculateExponentialDelay(attempt int) time.Duration {
	multiplier := math.Pow(rm.config.BackoffMultiplier, float64(attempt-1))
	delay := time.Duration(float64(rm.config.InitialDelay) * multiplier)
	return delay
}

// calculateLinearDelay calculates linear backoff delay
func (rm *RetryManager) calculateLinearDelay(attempt int) time.Duration {
	delay := time.Duration(int64(rm.config.InitialDelay) * int64(attempt))
	return delay
}

// applyJitter applies jitter to reduce thundering herd effect
func (rm *RetryManager) applyJitter(delay time.Duration) time.Duration {
	if rm.config.JitterFactor <= 0 || rm.config.JitterFactor >= 1 {
		return delay
	}

	jitter := rm.config.JitterFactor * float64(delay)
	randomJitter := (rand.Float64() * 2 - 1) * jitter // Random value between -jitter and +jitter
	
	finalDelay := time.Duration(float64(delay) + randomJitter)
	if finalDelay < 0 {
		finalDelay = time.Duration(float64(delay) * 0.1) // Minimum delay
	}

	return finalDelay
}

// IsEnabled returns whether retry is enabled
func (rm *RetryManager) IsEnabled() bool {
	return rm.config.Enabled
}

// Service-specific retry wrappers

// DatabaseRetryWrapper wraps database operations with retry logic
type DatabaseRetryWrapper struct {
	manager *RetryManager
}

// NewDatabaseRetryWrapper creates a new database retry wrapper
func NewDatabaseRetryWrapper(manager *RetryManager) *DatabaseRetryWrapper {
	return &DatabaseRetryWrapper{
		manager: manager,
	}
}

// Execute executes a database operation with retry logic
func (drw *DatabaseRetryWrapper) Execute(ctx context.Context, fn func() error) error {
	return drw.manager.Execute(ctx, fn, DatabaseRetryableErrors)
}

// ExecuteWithResult executes a database operation with retry logic and returns a result
func (drw *DatabaseRetryWrapper) ExecuteWithResult(ctx context.Context, fn func() (interface{}, error)) (interface{}, error) {
	return drw.manager.ExecuteWithResult(ctx, fn, DatabaseRetryableErrors)
}

// SearchRetryWrapper wraps search operations with retry logic
type SearchRetryWrapper struct {
	manager *RetryManager
}

// NewSearchRetryWrapper creates a new search retry wrapper
func NewSearchRetryWrapper(manager *RetryManager) *SearchRetryWrapper {
	return &SearchRetryWrapper{
		manager: manager,
	}
}

// Execute executes a search operation with retry logic
func (srw *SearchRetryWrapper) Execute(ctx context.Context, fn func() error) error {
	return srw.manager.Execute(ctx, fn, SearchRetryableErrors)
}

// ExecuteWithResult executes a search operation with retry logic and returns a result
func (srw *SearchRetryWrapper) ExecuteWithResult(ctx context.Context, fn func() (interface{}, error)) (interface{}, error) {
	return srw.manager.ExecuteWithResult(ctx, fn, SearchRetryableErrors)
}

// ExternalServiceRetryWrapper wraps external service calls with retry logic
type ExternalServiceRetryWrapper struct {
	manager *RetryManager
}

// NewExternalServiceRetryWrapper creates a new external service retry wrapper
func NewExternalServiceRetryWrapper(manager *RetryManager) *ExternalServiceRetryWrapper {
	return &ExternalServiceRetryWrapper{
		manager: manager,
	}
}

// Execute executes an external service call with retry logic
func (esrw *ExternalServiceRetryWrapper) Execute(ctx context.Context, fn func() error) error {
	return esrw.manager.Execute(ctx, fn, DefaultRetryableErrors)
}

// ExecuteWithResult executes an external service call with retry logic and returns a result
func (esrw *ExternalServiceRetryWrapper) ExecuteWithResult(ctx context.Context, fn func() (interface{}, error)) (interface{}, error) {
	return esrw.manager.ExecuteWithResult(ctx, fn, DefaultRetryableErrors)
}

// RetryMetrics holds metrics for retry operations
type RetryMetrics struct {
	TotalAttempts    int64         `json:"total_attempts"`
	SuccessfulRetries int64         `json:"successful_retries"`
	FailedRetries    int64         `json:"failed_retries"`
	AverageDelay     time.Duration `json:"average_delay"`
	MaxDelay         time.Duration `json:"max_delay"`
}

// RetryObserver observes retry operations for metrics collection
type RetryObserver struct {
	metrics RetryMetrics
}

// NewRetryObserver creates a new retry observer
func NewRetryObserver() *RetryObserver {
	return &RetryObserver{}
}

// ObserveAttempt records a retry attempt
func (ro *RetryObserver) ObserveAttempt(attempt int, delay time.Duration, success bool) {
	ro.metrics.TotalAttempts++
	
	if success && attempt > 1 {
		ro.metrics.SuccessfulRetries++
	} else if !success {
		ro.metrics.FailedRetries++
	}

	if delay > ro.metrics.MaxDelay {
		ro.metrics.MaxDelay = delay
	}

	// Update average delay (simplified calculation)
	if ro.metrics.TotalAttempts > 0 {
		ro.metrics.AverageDelay = time.Duration(
			(int64(ro.metrics.AverageDelay)*ro.metrics.TotalAttempts + int64(delay)) / (ro.metrics.TotalAttempts + 1),
		)
	}
}

// GetMetrics returns current retry metrics
func (ro *RetryObserver) GetMetrics() RetryMetrics {
	return ro.metrics
}

// Helper functions

func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[len(s)-len(substr):] == substr || 
		   len(s) >= len(substr) && s[:len(substr)] == substr ||
		   (len(s) > len(substr) && func() bool {
			   for i := 0; i <= len(s)-len(substr); i++ {
				   if s[i:i+len(substr)] == substr {
					   return true
				   }
			   }
			   return false
		   }())
}

// BackoffStrategies provides common backoff strategy configurations
var BackoffStrategies = map[string]RetryConfig{
	"fast": {
		Enabled:           true,
		MaxAttempts:       3,
		InitialDelay:      100 * time.Millisecond,
		MaxDelay:          1 * time.Second,
		BackoffMultiplier: 2.0,
		JitterEnabled:     true,
		JitterFactor:      0.1,
	},
	"standard": {
		Enabled:           true,
		MaxAttempts:       5,
		InitialDelay:      500 * time.Millisecond,
		MaxDelay:          30 * time.Second,
		BackoffMultiplier: 2.0,
		JitterEnabled:     true,
		JitterFactor:      0.2,
	},
	"aggressive": {
		Enabled:           true,
		MaxAttempts:       10,
		InitialDelay:      1 * time.Second,
		MaxDelay:          5 * time.Minute,
		BackoffMultiplier: 1.5,
		JitterEnabled:     true,
		JitterFactor:      0.3,
	},
}