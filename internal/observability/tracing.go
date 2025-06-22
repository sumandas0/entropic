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

type TracingConfig struct {
	Enabled     bool   `yaml:"enabled" mapstructure:"enabled"`
	JaegerURL   string `yaml:"jaeger_url" mapstructure:"jaeger_url"`
	ServiceName string `yaml:"service_name" mapstructure:"service_name"`
	Environment string `yaml:"environment" mapstructure:"environment"`
	SampleRate  float64 `yaml:"sample_rate" mapstructure:"sample_rate"`
}

type TracingManager struct {
	tracer   trace.Tracer
	provider *sdktrace.TracerProvider
	config   TracingConfig
}

func NewTracingManager(config TracingConfig) (*TracingManager, error) {
	if !config.Enabled {
		return &TracingManager{
			tracer: otel.Tracer(ServiceName),
			config: config,
		}, nil
	}

	exp, err := jaeger.New(jaeger.WithCollectorEndpoint(jaeger.WithEndpoint(config.JaegerURL)))
	if err != nil {
		return nil, fmt.Errorf("failed to create Jaeger exporter: %w", err)
	}

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

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.TraceIDRatioBased(config.SampleRate)),
	)

	otel.SetTracerProvider(tp)

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

func (tm *TracingManager) StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return tm.tracer.Start(ctx, name, opts...)
}

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

func (tm *TracingManager) AddSpanAttributes(span trace.Span, attrs ...attribute.KeyValue) {
	span.SetAttributes(attrs...)
}

func (tm *TracingManager) AddSpanEvent(span trace.Span, name string, attrs ...attribute.KeyValue) {
	span.AddEvent(name, trace.WithAttributes(attrs...))
}

func (tm *TracingManager) SetSpanError(span trace.Span, err error) {
	span.SetAttributes(
		attribute.Bool("error", true),
		attribute.String("error.message", err.Error()),
	)
	span.RecordError(err)
}

func (tm *TracingManager) Shutdown(ctx context.Context) error {
	if tm.provider != nil {
		return tm.provider.Shutdown(ctx)
	}
	return nil
}

func (tm *TracingManager) IsEnabled() bool {
	return tm.config.Enabled
}

func (tm *TracingManager) GetTracer() trace.Tracer {
	return tm.tracer
}

func getServiceName(config TracingConfig) string {
	if config.ServiceName != "" {
		return config.ServiceName
	}
	return ServiceName
}

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

			if span.SpanContext().HasTraceID() {
				w.Header().Set("X-Trace-ID", span.SpanContext().TraceID().String())
			}

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