package integration

import (
	"context"
	"time"

	"github.com/sumandas0/entropic/internal/observability"
	"github.com/sumandas0/entropic/internal/resilience"
	"github.com/sumandas0/entropic/internal/security"
)

type ObservabilityManager struct {
	tracing *observability.TracingManager
	logging *observability.Logger
	metrics *observability.MetricsManager
}

func NewObservabilityManager(
	tracingConfig observability.TracingConfig,
	loggingConfig observability.LoggingConfig,
	metricsConfig observability.MetricsConfig,
) (*ObservabilityManager, error) {
	
	tracing, err := observability.NewTracingManager(tracingConfig)
	if err != nil {
		return nil, err
	}

	logging, err := observability.NewLogger(loggingConfig)
	if err != nil {
		return nil, err
	}

	metrics := observability.NewMetricsManager(metricsConfig)

	observability.SetGlobalLogger(logging)

	return &ObservabilityManager{
		tracing: tracing,
		logging: logging,
		metrics: metrics,
	}, nil
}

func (om *ObservabilityManager) GetTracing() *observability.TracingManager {
	return om.tracing
}

func (om *ObservabilityManager) GetLogging() *observability.Logger {
	return om.logging
}

func (om *ObservabilityManager) GetMetrics() *observability.MetricsManager {
	return om.metrics
}

func (om *ObservabilityManager) Shutdown(ctx context.Context) error {
	return om.tracing.Shutdown(ctx)
}

type ResilienceManager struct {
	circuitBreaker *resilience.CircuitBreakerManager
	retryManager   *resilience.RetryManager
}

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

func (rm *ResilienceManager) GetCircuitBreaker() *resilience.CircuitBreakerManager {
	return rm.circuitBreaker
}

func (rm *ResilienceManager) GetRetryManager() *resilience.RetryManager {
	return rm.retryManager
}

type SecurityManager struct {
	rateLimiter *security.RateLimiter
	sanitizer   *security.InputSanitizer
}

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

func (sm *SecurityManager) GetRateLimiter() *security.RateLimiter {
	return sm.rateLimiter
}

func (sm *SecurityManager) GetSanitizer() *security.InputSanitizer {
	return sm.sanitizer
}

func (sm *SecurityManager) Stop() {
	sm.rateLimiter.Stop()
}

type AdvancedFeaturesManager struct {
	observability *ObservabilityManager
	resilience    *ResilienceManager
	security      *SecurityManager
	startTime     time.Time
}

type AdvancedFeaturesConfig struct {
	Tracing       observability.TracingConfig       `yaml:"tracing" mapstructure:"tracing"`
	Logging       observability.LoggingConfig       `yaml:"logging" mapstructure:"logging"`
	Metrics       observability.MetricsConfig       `yaml:"metrics" mapstructure:"metrics"`
	CircuitBreaker resilience.CircuitBreakerConfig   `yaml:"circuit_breaker" mapstructure:"circuit_breaker"`
	Retry         resilience.RetryConfig            `yaml:"retry" mapstructure:"retry"`
	RateLimit     security.RateLimitConfig          `yaml:"rate_limit" mapstructure:"rate_limit"`
	Sanitizer     security.SanitizerConfig          `yaml:"sanitizer" mapstructure:"sanitizer"`
}

func NewAdvancedFeaturesManager(config AdvancedFeaturesConfig) (*AdvancedFeaturesManager, error) {
	
	observability, err := NewObservabilityManager(config.Tracing, config.Logging, config.Metrics)
	if err != nil {
		return nil, err
	}

	resilience := NewResilienceManager(config.CircuitBreaker, config.Retry)

	security := NewSecurityManager(config.RateLimit, config.Sanitizer)

	afm := &AdvancedFeaturesManager{
		observability: observability,
		resilience:    resilience,
		security:      security,
		startTime:     time.Now(),
	}

	if config.Metrics.Enabled {
		observability.GetMetrics().StartUptimeTracker(context.Background(), afm.startTime)
		observability.GetMetrics().SetBuildInfo("1.0.0", "dev", time.Now().Format(time.RFC3339))
	}

	return afm, nil
}

func (afm *AdvancedFeaturesManager) GetObservability() *ObservabilityManager {
	return afm.observability
}

func (afm *AdvancedFeaturesManager) GetResilience() *ResilienceManager {
	return afm.resilience
}

func (afm *AdvancedFeaturesManager) GetSecurity() *SecurityManager {
	return afm.security
}

func (afm *AdvancedFeaturesManager) GetStartTime() time.Time {
	return afm.startTime
}

func (afm *AdvancedFeaturesManager) Shutdown(ctx context.Context) error {
	afm.security.Stop()
	return afm.observability.Shutdown(ctx)
}

func (afm *AdvancedFeaturesManager) HealthCheck(ctx context.Context) map[string]interface{} {
	health := make(map[string]interface{})

	health["observability"] = map[string]interface{}{
		"tracing_enabled": afm.observability.tracing.IsEnabled(),
		"metrics_enabled": afm.observability.metrics.IsEnabled(),
		"uptime_seconds":  time.Since(afm.startTime).Seconds(),
	}

	circuitBreakerHealth := resilience.NewCircuitBreakerHealthCheck(afm.resilience.circuitBreaker)
	health["resilience"] = map[string]interface{}{
		"circuit_breaker": circuitBreakerHealth.Check(ctx),
		"retry_enabled":   afm.resilience.retryManager.IsEnabled(),
	}

	health["security"] = map[string]interface{}{
		"rate_limiter": afm.security.rateLimiter.GetStats(),
		"sanitizer_enabled": afm.security.sanitizer.IsEnabled(),
	}

	return health
}

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