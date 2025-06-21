package integration

import (
	"context"
	"time"

	"github.com/entropic/entropic/internal/observability"
	"github.com/entropic/entropic/internal/resilience"
	"github.com/entropic/entropic/internal/security"
)

// ObservabilityManager integrates all observability components
type ObservabilityManager struct {
	tracing *observability.TracingManager
	logging *observability.Logger
	metrics *observability.MetricsManager
}

// NewObservabilityManager creates a new observability manager
func NewObservabilityManager(
	tracingConfig observability.TracingConfig,
	loggingConfig observability.LoggingConfig,
	metricsConfig observability.MetricsConfig,
) (*ObservabilityManager, error) {
	// Initialize tracing
	tracing, err := observability.NewTracingManager(tracingConfig)
	if err != nil {
		return nil, err
	}

	// Initialize logging
	logging, err := observability.NewLogger(loggingConfig)
	if err != nil {
		return nil, err
	}

	// Initialize metrics
	metrics := observability.NewMetricsManager(metricsConfig)

	// Set global logger
	observability.SetGlobalLogger(logging)

	return &ObservabilityManager{
		tracing: tracing,
		logging: logging,
		metrics: metrics,
	}, nil
}

// GetTracing returns the tracing manager
func (om *ObservabilityManager) GetTracing() *observability.TracingManager {
	return om.tracing
}

// GetLogging returns the logging manager
func (om *ObservabilityManager) GetLogging() *observability.Logger {
	return om.logging
}

// GetMetrics returns the metrics manager
func (om *ObservabilityManager) GetMetrics() *observability.MetricsManager {
	return om.metrics
}

// Shutdown gracefully shuts down all observability components
func (om *ObservabilityManager) Shutdown(ctx context.Context) error {
	return om.tracing.Shutdown(ctx)
}

// ResilienceManager integrates all resilience components
type ResilienceManager struct {
	circuitBreaker *resilience.CircuitBreakerManager
	retryManager   *resilience.RetryManager
}

// NewResilienceManager creates a new resilience manager
func NewResilienceManager(
	cbConfig resilience.CircuitBreakerConfig,
	retryConfig resilience.RetryConfig,
) *ResilienceManager {
	circuitBreaker := resilience.NewCircuitBreakerManager(cbConfig)
	retryManager := resilience.NewRetryManager(retryConfig, resilience.StrategyExponential)

	return &ResilienceManager{
		circuitBreaker: circuitBreaker,
		retryManager:   retryManager,
	}
}

// GetCircuitBreaker returns the circuit breaker manager
func (rm *ResilienceManager) GetCircuitBreaker() *resilience.CircuitBreakerManager {
	return rm.circuitBreaker
}

// GetRetryManager returns the retry manager
func (rm *ResilienceManager) GetRetryManager() *resilience.RetryManager {
	return rm.retryManager
}

// SecurityManager integrates all security components
type SecurityManager struct {
	rateLimiter *security.RateLimiter
	sanitizer   *security.InputSanitizer
}

// NewSecurityManager creates a new security manager
func NewSecurityManager(
	rateLimitConfig security.RateLimitConfig,
	sanitizerConfig security.SanitizerConfig,
) *SecurityManager {
	rateLimiter := security.NewRateLimiter(rateLimitConfig)
	sanitizer := security.NewInputSanitizer(sanitizerConfig)

	return &SecurityManager{
		rateLimiter: rateLimiter,
		sanitizer:   sanitizer,
	}
}

// GetRateLimiter returns the rate limiter
func (sm *SecurityManager) GetRateLimiter() *security.RateLimiter {
	return sm.rateLimiter
}

// GetSanitizer returns the input sanitizer
func (sm *SecurityManager) GetSanitizer() *security.InputSanitizer {
	return sm.sanitizer
}

// Stop stops all security components
func (sm *SecurityManager) Stop() {
	sm.rateLimiter.Stop()
}

// AdvancedFeaturesManager integrates all advanced features
type AdvancedFeaturesManager struct {
	observability *ObservabilityManager
	resilience    *ResilienceManager
	security      *SecurityManager
	startTime     time.Time
}

// AdvancedFeaturesConfig holds configuration for all advanced features
type AdvancedFeaturesConfig struct {
	Tracing       observability.TracingConfig       `yaml:"tracing" mapstructure:"tracing"`
	Logging       observability.LoggingConfig       `yaml:"logging" mapstructure:"logging"`
	Metrics       observability.MetricsConfig       `yaml:"metrics" mapstructure:"metrics"`
	CircuitBreaker resilience.CircuitBreakerConfig   `yaml:"circuit_breaker" mapstructure:"circuit_breaker"`
	Retry         resilience.RetryConfig            `yaml:"retry" mapstructure:"retry"`
	RateLimit     security.RateLimitConfig          `yaml:"rate_limit" mapstructure:"rate_limit"`
	Sanitizer     security.SanitizerConfig          `yaml:"sanitizer" mapstructure:"sanitizer"`
}

// NewAdvancedFeaturesManager creates a new advanced features manager
func NewAdvancedFeaturesManager(config AdvancedFeaturesConfig) (*AdvancedFeaturesManager, error) {
	// Initialize observability
	observability, err := NewObservabilityManager(config.Tracing, config.Logging, config.Metrics)
	if err != nil {
		return nil, err
	}

	// Initialize resilience
	resilience := NewResilienceManager(config.CircuitBreaker, config.Retry)

	// Initialize security
	security := NewSecurityManager(config.RateLimit, config.Sanitizer)

	afm := &AdvancedFeaturesManager{
		observability: observability,
		resilience:    resilience,
		security:      security,
		startTime:     time.Now(),
	}

	// Start metrics tracking
	if config.Metrics.Enabled {
		observability.GetMetrics().StartUptimeTracker(context.Background(), afm.startTime)
		observability.GetMetrics().SetBuildInfo("1.0.0", "dev", time.Now().Format(time.RFC3339))
	}

	return afm, nil
}

// GetObservability returns the observability manager
func (afm *AdvancedFeaturesManager) GetObservability() *ObservabilityManager {
	return afm.observability
}

// GetResilience returns the resilience manager
func (afm *AdvancedFeaturesManager) GetResilience() *ResilienceManager {
	return afm.resilience
}

// GetSecurity returns the security manager
func (afm *AdvancedFeaturesManager) GetSecurity() *SecurityManager {
	return afm.security
}

// GetStartTime returns the application start time
func (afm *AdvancedFeaturesManager) GetStartTime() time.Time {
	return afm.startTime
}

// Shutdown gracefully shuts down all advanced features
func (afm *AdvancedFeaturesManager) Shutdown(ctx context.Context) error {
	afm.security.Stop()
	return afm.observability.Shutdown(ctx)
}

// HealthCheck provides health information for all advanced features
func (afm *AdvancedFeaturesManager) HealthCheck(ctx context.Context) map[string]interface{} {
	health := make(map[string]interface{})

	// Observability health
	health["observability"] = map[string]interface{}{
		"tracing_enabled": afm.observability.tracing.IsEnabled(),
		"metrics_enabled": afm.observability.metrics.IsEnabled(),
		"uptime_seconds":  time.Since(afm.startTime).Seconds(),
	}

	// Resilience health
	circuitBreakerHealth := resilience.NewCircuitBreakerHealthCheck(afm.resilience.circuitBreaker)
	health["resilience"] = map[string]interface{}{
		"circuit_breaker": circuitBreakerHealth.Check(ctx),
		"retry_enabled":   afm.resilience.retryManager.IsEnabled(),
	}

	// Security health
	health["security"] = map[string]interface{}{
		"rate_limiter": afm.security.rateLimiter.GetStats(),
		"sanitizer_enabled": afm.security.sanitizer.IsEnabled(),
	}

	return health
}

// GetMetricsStats returns comprehensive metrics statistics
func (afm *AdvancedFeaturesManager) GetMetricsStats() map[string]interface{} {
	if !afm.observability.metrics.IsEnabled() {
		return map[string]interface{}{
			"enabled": false,
		}
	}

	return map[string]interface{}{
		"enabled":      true,
		"uptime":       time.Since(afm.startTime).String(),
		"rate_limiter": afm.security.rateLimiter.GetStats(),
	}
}