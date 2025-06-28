package testhelpers

import (
	"context"
	"testing"

	"github.com/rs/zerolog"
	"github.com/sumandas0/entropic/internal/core"
	"github.com/sumandas0/entropic/internal/integration"
	"github.com/sumandas0/entropic/internal/observability"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

// NewTestObservabilityManager creates an observability manager with no-op implementations for testing
func NewTestObservabilityManager(t *testing.T) *integration.ObservabilityManager {
	tracingConfig := observability.TracingConfig{
		Enabled:     false,
		ServiceName: "test-service",
		Environment: "test",
	}
	
	loggingConfig := observability.LoggingConfig{
		Level:  observability.LogLevelError,
		Format: observability.LogFormatJSON,
		Output: "discard",
	}
	
	metricsConfig := observability.MetricsConfig{
		Enabled: false,
	}
	
	obsManager, err := integration.NewObservabilityManager(
		tracingConfig,
		loggingConfig,
		metricsConfig,
	)
	if err != nil {
		t.Fatalf("Failed to create test observability manager: %v", err)
	}
	
	return obsManager
}

// NewTestLogger creates a no-op logger for testing
func NewTestLogger() zerolog.Logger {
	return zerolog.Nop()
}

// NewTestTracer creates a no-op tracer for testing
func NewTestTracer() trace.Tracer {
	return otel.GetTracerProvider().Tracer("test")
}

// NewTestObservabilityManagerWithLogging creates an observability manager with logging enabled for test debugging
func NewTestObservabilityManagerWithLogging(t *testing.T) *integration.ObservabilityManager {
	tracingConfig := observability.TracingConfig{
		Enabled:     false,
		ServiceName: "test-service",
		Environment: "test",
	}
	
	loggingConfig := observability.LoggingConfig{
		Level:  observability.LogLevelDebug,
		Format: observability.LogFormatJSON,
		Output: "stdout",
	}
	
	metricsConfig := observability.MetricsConfig{
		Enabled: false,
	}
	
	obsManager, err := integration.NewObservabilityManager(
		tracingConfig,
		loggingConfig,
		metricsConfig,
	)
	if err != nil {
		t.Fatalf("Failed to create test observability manager with logging: %v", err)
	}
	
	return obsManager
}

// NewTestObservabilityManagerWithTracing creates an observability manager with tracing enabled for test debugging
func NewTestObservabilityManagerWithTracing(t *testing.T) *integration.ObservabilityManager {
	tracingConfig := observability.TracingConfig{
		Enabled:     true,
		ServiceName: "test-service",
		Environment: "test",
		// Use stdout exporter for testing
		JaegerURL:  "", // Empty URL will use stdout exporter
		SampleRate: 1.0,
	}
	
	loggingConfig := observability.LoggingConfig{
		Level:  observability.LogLevelError,
		Format: observability.LogFormatJSON,
		Output: "discard",
	}
	
	metricsConfig := observability.MetricsConfig{
		Enabled: false,
	}
	
	obsManager, err := integration.NewObservabilityManager(
		tracingConfig,
		loggingConfig,
		metricsConfig,
	)
	if err != nil {
		t.Fatalf("Failed to create test observability manager with tracing: %v", err)
	}
	
	return obsManager
}

// SetupTestEnvironmentWithObservability extends the test environment setup to include observability
func SetupTestEnvironmentWithObservability(t *testing.T) *TestEnvironmentWithObservability {
	env := SetupTestEnvironment(t, context.Background())
	obsManager := NewTestObservabilityManager(t)
	
	// Set observability on all components
	env.PrimaryStore.SetObservability(obsManager)
	env.IndexStore.SetObservability(obsManager)
	env.CacheManager.SetObservability(obsManager)
	env.LockManager.SetObservability(obsManager)
	
	// Recreate engine with observability
	engine, err := core.NewEngine(
		env.PrimaryStore,
		env.IndexStore,
		env.CacheManager,
		env.LockManager,
		obsManager,
	)
	if err != nil {
		t.Fatalf("Failed to create engine with observability: %v", err)
	}
	env.Engine = engine
	
	return &TestEnvironmentWithObservability{
		TestEnvironment: env,
		ObsManager:      obsManager,
	}
}

// TestEnvironmentWithObservability extends TestEnvironment with observability manager
type TestEnvironmentWithObservability struct {
	*TestEnvironment
	ObsManager *integration.ObservabilityManager
}

// Cleanup properly shuts down the test environment including observability
func (te *TestEnvironmentWithObservability) Cleanup(ctx context.Context) error {
	// First cleanup the base test environment
	if err := te.TestEnvironment.Cleanup(ctx); err != nil {
		return err
	}
	
	// Then shutdown observability
	if te.ObsManager != nil {
		return te.ObsManager.Shutdown(ctx)
	}
	
	return nil
}