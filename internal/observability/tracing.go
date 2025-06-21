package observability

import (
	"context"
	"fmt"
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace"
)

const (
	ServiceName    = "entropic-storage-engine"
	ServiceVersion = "1.0.0"
)

// TracingConfig holds configuration for OpenTelemetry tracing
type TracingConfig struct {
	Enabled     bool   `yaml:"enabled" mapstructure:"enabled"`
	JaegerURL   string `yaml:"jaeger_url" mapstructure:"jaeger_url"`
	ServiceName string `yaml:"service_name" mapstructure:"service_name"`
	Environment string `yaml:"environment" mapstructure:"environment"`
	SampleRate  float64 `yaml:"sample_rate" mapstructure:"sample_rate"`
}

// TracingManager manages OpenTelemetry tracing setup and operations
type TracingManager struct {
	tracer   trace.Tracer
	provider *sdktrace.TracerProvider
	config   TracingConfig
}

// NewTracingManager creates a new tracing manager with the given configuration
func NewTracingManager(config TracingConfig) (*TracingManager, error) {
	if !config.Enabled {
		return &TracingManager{
			tracer: otel.Tracer(ServiceName),
			config: config,
		}, nil
	}

	// Create Jaeger exporter
	exp, err := jaeger.New(jaeger.WithCollectorEndpoint(jaeger.WithEndpoint(config.JaegerURL)))
	if err != nil {
		return nil, fmt.Errorf("failed to create Jaeger exporter: %w", err)
	}

	// Create resource with service information
	res, err := resource.New(context.Background(),
		resource.WithAttributes(
			semconv.ServiceName(getServiceName(config)),
			semconv.ServiceVersion(ServiceVersion),
			semconv.DeploymentEnvironment(config.Environment),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Create trace provider with sampling
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.TraceIDRatioBased(config.SampleRate)),
	)

	// Set global trace provider
	otel.SetTracerProvider(tp)

	// Set global propagator
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	tracer := tp.Tracer(getServiceName(config))

	return &TracingManager{
		tracer:   tracer,
		provider: tp,
		config:   config,
	}, nil
}

// StartSpan starts a new span with the given name and options
func (tm *TracingManager) StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return tm.tracer.Start(ctx, name, opts...)
}

// StartEntityOperation starts a span for entity operations
func (tm *TracingManager) StartEntityOperation(ctx context.Context, operation, entityType string, entityID string) (context.Context, trace.Span) {
	spanName := fmt.Sprintf("entity.%s", operation)
	ctx, span := tm.tracer.Start(ctx, spanName,
		trace.WithAttributes(
			attribute.String("entity.type", entityType),
			attribute.String("entity.id", entityID),
			attribute.String("operation", operation),
		),
	)
	return ctx, span
}

// StartRelationOperation starts a span for relation operations
func (tm *TracingManager) StartRelationOperation(ctx context.Context, operation, relationType string, relationID string) (context.Context, trace.Span) {
	spanName := fmt.Sprintf("relation.%s", operation)
	ctx, span := tm.tracer.Start(ctx, spanName,
		trace.WithAttributes(
			attribute.String("relation.type", relationType),
			attribute.String("relation.id", relationID),
			attribute.String("operation", operation),
		),
	)
	return ctx, span
}

// StartSearchOperation starts a span for search operations
func (tm *TracingManager) StartSearchOperation(ctx context.Context, searchType, query string, entityTypes []string) (context.Context, trace.Span) {
	spanName := fmt.Sprintf("search.%s", searchType)
	ctx, span := tm.tracer.Start(ctx, spanName,
		trace.WithAttributes(
			attribute.String("search.type", searchType),
			attribute.String("search.query", query),
			attribute.StringSlice("search.entity_types", entityTypes),
		),
	)
	return ctx, span
}

// StartDatabaseOperation starts a span for database operations
func (tm *TracingManager) StartDatabaseOperation(ctx context.Context, operation, table string) (context.Context, trace.Span) {
	spanName := fmt.Sprintf("db.%s", operation)
	ctx, span := tm.tracer.Start(ctx, spanName,
		trace.WithAttributes(
			attribute.String("db.operation", operation),
			attribute.String("db.table", table),
			attribute.String("db.system", "postgresql"),
		),
	)
	return ctx, span
}

// StartCacheOperation starts a span for cache operations
func (tm *TracingManager) StartCacheOperation(ctx context.Context, operation, key string) (context.Context, trace.Span) {
	spanName := fmt.Sprintf("cache.%s", operation)
	ctx, span := tm.tracer.Start(ctx, spanName,
		trace.WithAttributes(
			attribute.String("cache.operation", operation),
			attribute.String("cache.key", key),
		),
	)
	return ctx, span
}

// StartLockOperation starts a span for lock operations
func (tm *TracingManager) StartLockOperation(ctx context.Context, operation, resource string) (context.Context, trace.Span) {
	spanName := fmt.Sprintf("lock.%s", operation)
	ctx, span := tm.tracer.Start(ctx, spanName,
		trace.WithAttributes(
			attribute.String("lock.operation", operation),
			attribute.String("lock.resource", resource),
		),
	)
	return ctx, span
}

// AddSpanAttributes adds attributes to the current span
func (tm *TracingManager) AddSpanAttributes(span trace.Span, attrs ...attribute.KeyValue) {
	span.SetAttributes(attrs...)
}

// AddSpanEvent adds an event to the current span
func (tm *TracingManager) AddSpanEvent(span trace.Span, name string, attrs ...attribute.KeyValue) {
	span.AddEvent(name, trace.WithAttributes(attrs...))
}

// SetSpanError sets error information on the span
func (tm *TracingManager) SetSpanError(span trace.Span, err error) {
	span.SetAttributes(
		attribute.Bool("error", true),
		attribute.String("error.message", err.Error()),
	)
	span.RecordError(err)
}

// Shutdown gracefully shuts down the tracing provider
func (tm *TracingManager) Shutdown(ctx context.Context) error {
	if tm.provider != nil {
		return tm.provider.Shutdown(ctx)
	}
	return nil
}

// IsEnabled returns whether tracing is enabled
func (tm *TracingManager) IsEnabled() bool {
	return tm.config.Enabled
}

// GetTracer returns the OpenTelemetry tracer
func (tm *TracingManager) GetTracer() trace.Tracer {
	return tm.tracer
}

// Helper functions

func getServiceName(config TracingConfig) string {
	if config.ServiceName != "" {
		return config.ServiceName
	}
	return ServiceName
}

// TraceMiddleware creates middleware for HTTP request tracing
func (tm *TracingManager) TraceMiddleware() func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !tm.config.Enabled {
				next.ServeHTTP(w, r)
				return
			}

			ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
			
			spanName := fmt.Sprintf("%s %s", r.Method, r.URL.Path)
			ctx, span := tm.tracer.Start(ctx, spanName,
				trace.WithAttributes(
					semconv.HTTPMethod(r.Method),
					semconv.HTTPRoute(r.URL.Path),
					attribute.String("http.user_agent", r.UserAgent()),
					attribute.String("http.client_ip", r.RemoteAddr),
				),
			)
			defer span.End()

			// Add trace ID to response headers for debugging
			if span.SpanContext().HasTraceID() {
				w.Header().Set("X-Trace-ID", span.SpanContext().TraceID().String())
			}

			// Wrap response writer to capture status code
			wrapped := &responseWriter{ResponseWriter: w}
			
			next.ServeHTTP(wrapped, r.WithContext(ctx))

			span.SetAttributes(
				semconv.HTTPStatusCode(wrapped.statusCode),
			)

			if wrapped.statusCode >= 400 {
				span.SetAttributes(attribute.Bool("error", true))
			}
		})
	}
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(statusCode int) {
	rw.statusCode = statusCode
	rw.ResponseWriter.WriteHeader(statusCode)
}

// ExtractTraceInfo extracts trace information from context for logging
func ExtractTraceInfo(ctx context.Context) map[string]interface{} {
	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return nil
	}

	spanCtx := span.SpanContext()
	return map[string]interface{}{
		"trace_id": spanCtx.TraceID().String(),
		"span_id":  spanCtx.SpanID().String(),
	}
}