# Entropic Observability Guide

This guide covers setting up and using observability features in Entropic Storage Engine using open-source tools including OpenTelemetry, Prometheus, Jaeger, and Loki.

## Table of Contents

- [Overview](#overview)
- [Architecture](#architecture)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [Metrics](#metrics)
- [Distributed Tracing](#distributed-tracing)
- [Logging](#logging)
- [Debugging Scenarios](#debugging-scenarios)
- [Best Practices](#best-practices)

## Overview

Entropic provides comprehensive observability through three pillars:

1. **Metrics** - Quantitative data about system performance using Prometheus
2. **Tracing** - Request flow tracking across services using OpenTelemetry and Jaeger
3. **Logging** - Structured logs with correlation IDs using Zerolog

## Architecture

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│                 │     │                 │     │                 │
│  Entropic API   │────▶│   Prometheus    │────▶│    Grafana     │
│                 │     │                 │     │                 │
└────────┬────────┘     └─────────────────┘     └─────────────────┘
         │
         │ OpenTelemetry
         │
         ▼
┌─────────────────┐     ┌─────────────────┐
│                 │     │                 │
│     Jaeger      │     │      Loki       │
│                 │     │                 │
└─────────────────┘     └─────────────────┘
```

## Quick Start

1. **Start the observability stack:**

```bash
# Start main services
docker-compose up -d

# Start observability services
docker-compose -f docker-compose.observability.yml up -d
```

2. **Access the UIs:**
   - Prometheus: http://localhost:9090
   - Jaeger: http://localhost:16686
   - Grafana: http://localhost:3000 (admin/admin)
   - Loki: http://localhost:3100/metrics

3. **Verify metrics are being collected:**

```bash
# Check Entropic metrics endpoint
curl http://localhost:8080/metrics

# Check Prometheus targets
curl http://localhost:9090/api/v1/targets
```

## Configuration

### Enabling Observability Features

Update your `entropic.yaml`:

```yaml
# Metrics Configuration
metrics:
  enabled: true
  path: "/metrics"
  port: 8080
  namespace: "entropic"
  subsystem: "storage"

# Tracing Configuration
tracing:
  enabled: true
  jaeger_url: "http://localhost:14268/api/traces"
  service_name: "entropic-storage-engine"
  environment: "development"
  sample_rate: 1.0  # 100% sampling for dev, reduce for production

# Logging Configuration
logging:
  level: "info"
  format: "json"  # Use "console" for development
  output: "stdout"
  time_format: "2006-01-02T15:04:05.000Z07:00"
  sampling:
    enabled: false  # Enable for high-volume production
    tick: "1s"
    first: 10
    thereafter: 100
```

### Environment Variables

You can override configuration using environment variables:

```bash
# Metrics
export ENTROPIC_METRICS_ENABLED=true
export ENTROPIC_METRICS_PATH=/metrics

# Tracing
export ENTROPIC_TRACING_ENABLED=true
export ENTROPIC_TRACING_JAEGER_URL=http://jaeger:14268/api/traces
export ENTROPIC_TRACING_SAMPLE_RATE=0.1  # 10% sampling

# Logging
export ENTROPIC_LOGGING_LEVEL=debug
export ENTROPIC_LOGGING_FORMAT=json
```

## Metrics

### Available Metrics

Entropic exposes the following key metrics:

#### HTTP Metrics
- `entropic_http_requests_total` - Total HTTP requests by method, path, and status
- `entropic_http_request_duration_seconds` - Request latency histogram
- `entropic_http_request_size_bytes` - Request body size
- `entropic_http_response_size_bytes` - Response body size

#### Entity Metrics
- `entropic_storage_entities_total` - Total entities by type
- `entropic_storage_entity_operations_total` - Entity operations counter
- `entropic_storage_entity_operation_duration_seconds` - Operation latency

#### Relation Metrics
- `entropic_storage_relations_total` - Total relations by type
- `entropic_storage_relation_operations_total` - Relation operations counter
- `entropic_storage_relation_operation_duration_seconds` - Operation latency

#### Search Metrics
- `entropic_storage_search_operations_total` - Search operations by type
- `entropic_storage_search_duration_seconds` - Search latency
- `entropic_storage_search_results_count` - Number of results returned

#### Cache Metrics
- `entropic_cache_operations_total` - Cache operations by type
- `entropic_cache_hits_total` - Cache hit counter
- `entropic_cache_misses_total` - Cache miss counter
- `entropic_cache_size_bytes` - Current cache size

#### Database Metrics
- `entropic_database_connections_active` - Active DB connections
- `entropic_database_connections_max` - Maximum DB connections
- `entropic_database_operations_total` - DB operations by type
- `entropic_database_operation_duration_seconds` - DB operation latency

#### System Metrics
- `entropic_system_uptime_seconds` - Service uptime
- `entropic_system_build_info` - Build information (version, commit, build time)

### Querying Metrics with PromQL

Common queries for monitoring Entropic:

```promql
# Request rate (requests per second)
rate(entropic_http_requests_total[5m])

# 95th percentile latency
histogram_quantile(0.95, rate(entropic_http_request_duration_seconds_bucket[5m]))

# Error rate
rate(entropic_http_requests_total{status_code=~"5.."}[5m])

# Entity creation rate by type
rate(entropic_storage_entity_operations_total{operation="create"}[5m])

# Cache hit ratio
rate(entropic_cache_hits_total[5m]) / 
(rate(entropic_cache_hits_total[5m]) + rate(entropic_cache_misses_total[5m]))

# Database connection utilization
entropic_database_connections_active / entropic_database_connections_max

# Search performance by type
histogram_quantile(0.99, rate(entropic_storage_search_duration_seconds_bucket[5m]))
```

### Creating Grafana Dashboards

1. **Access Grafana** at http://localhost:3000 (default: admin/admin)

2. **Add Prometheus data source:**
   - Configuration → Data Sources → Add data source
   - Select Prometheus
   - URL: http://prometheus:9090
   - Save & Test

3. **Create a new dashboard:**
   - Create → Dashboard → Add new panel
   - Use the PromQL queries above

Example panels to create:
- Request rate and error rate
- Latency percentiles (p50, p95, p99)
- Entity and relation operations
- Cache performance
- Database connection pool status

## Distributed Tracing

### Understanding Traces

Entropic automatically creates spans for:
- HTTP requests
- Entity operations
- Relation operations
- Search queries
- Database operations
- Cache operations
- Lock operations

### Viewing Traces in Jaeger

1. **Access Jaeger UI** at http://localhost:16686

2. **Search for traces:**
   - Service: `entropic-storage-engine`
   - Operation: Select specific operations or leave blank
   - Tags: Add filters like `entity.type=user` or `error=true`

3. **Trace anatomy:**
   ```
   HTTP POST /entities/user [300ms]
   ├── entity.validate [5ms]
   ├── lock.acquire [2ms]
   ├── db.CheckURNExists [15ms]
   ├── db.CreateEntity [50ms]
   ├── cache.invalidate [3ms]
   ├── search.index [200ms]
   └── lock.release [1ms]
   ```

### Trace Context Propagation

Entropic propagates trace context through HTTP headers:
- `traceparent` - W3C Trace Context
- `X-Trace-ID` - Returned in responses for correlation

Example using trace ID for debugging:
```bash
# Get trace ID from response header
TRACE_ID=$(curl -i http://localhost:8080/entities/user \
  -H "Content-Type: application/json" \
  -d '{"urn":"urn:user:123","properties":{"name":"John"}}' \
  | grep X-Trace-ID | cut -d' ' -f2)

# Search for the trace in Jaeger
open "http://localhost:16686/trace/$TRACE_ID"
```

### Custom Instrumentation

Add custom spans in your code:

```go
// Import the tracing manager
import "github.com/sumandas0/entropic/internal/observability"

// Start a custom span
ctx, span := tracingManager.StartSpan(ctx, "custom.operation",
    trace.WithAttributes(
        attribute.String("custom.field", "value"),
        attribute.Int("custom.count", 42),
    ),
)
defer span.End()

// Add events to the span
tracingManager.AddSpanEvent(span, "processing started")

// Record errors
if err != nil {
    tracingManager.SetSpanError(span, err)
}
```

## Logging

### Log Structure

Entropic uses structured JSON logging with these standard fields:
- `timestamp` - ISO 8601 timestamp
- `level` - Log level (debug, info, warn, error)
- `service` - Always "entropic"
- `trace_id` - OpenTelemetry trace ID
- `span_id` - OpenTelemetry span ID
- `caller` - Source file and line number
- `message` - Log message

Additional context fields:
- `entity_type`, `entity_id` - For entity operations
- `relation_type`, `relation_id` - For relation operations
- `operation` - Current operation name
- `duration` - Operation duration
- `error` - Error details with stack trace

### Viewing Logs

1. **Local development** - Console output:
   ```bash
   docker-compose logs -f entropic
   ```

2. **With Loki** - Query in Grafana:
   - Add Loki data source (http://loki:3100)
   - Use LogQL queries:
   ```logql
   {container="entropic-server"} |= "error"
   {container="entropic-server"} | json | trace_id="abc123"
   {container="entropic-server"} | json | entity_type="user" | operation="create"
   ```

### Correlating Logs with Traces

Find all logs for a specific trace:
```logql
{container="entropic-server"} | json | trace_id="your-trace-id-here"
```

## Debugging Scenarios

### 1. Slow Entity Creation

**Symptoms:** Entity creation takes longer than expected

**Investigation steps:**

1. **Check metrics:**
   ```promql
   # Entity creation latency
   histogram_quantile(0.99, rate(entropic_storage_entity_operation_duration_seconds_bucket{operation="create"}[5m]))
   
   # Database latency
   histogram_quantile(0.99, rate(entropic_database_operation_duration_seconds_bucket{operation="insert"}[5m]))
   ```

2. **Find slow traces:**
   - Jaeger: Search for `operation=entity.create` with min duration 1s
   - Look for slow spans (database, search indexing)

3. **Check logs:**
   ```logql
   {container="entropic-server"} | json | operation="create" | duration > 1000
   ```

### 2. High Error Rate

**Symptoms:** Increased 5xx errors

**Investigation steps:**

1. **Error metrics:**
   ```promql
   # Error rate by endpoint
   rate(entropic_http_requests_total{status_code=~"5.."}[5m])
   
   # Error rate by operation
   rate(entropic_storage_entity_operations_total{status="error"}[5m])
   ```

2. **Error traces:**
   - Jaeger: Search with tag `error=true`
   - Analyze error patterns

3. **Error logs:**
   ```logql
   {container="entropic-server"} | json | level="error"
   ```

### 3. Cache Performance Issues

**Symptoms:** High cache miss rate, slow response times

**Investigation:**

1. **Cache metrics:**
   ```promql
   # Cache hit ratio
   rate(entropic_cache_hits_total[5m]) / 
   (rate(entropic_cache_hits_total[5m]) + rate(entropic_cache_misses_total[5m]))
   
   # Cache operations
   rate(entropic_cache_operations_total[5m])
   ```

2. **Trace analysis:**
   - Look for repeated cache misses in traces
   - Check cache invalidation patterns

### 4. Database Connection Issues

**Symptoms:** Connection pool exhaustion, timeouts

**Investigation:**

1. **Connection metrics:**
   ```promql
   # Connection utilization
   entropic_database_connections_active / entropic_database_connections_max
   
   # Operation latency
   histogram_quantile(0.99, rate(entropic_database_operation_duration_seconds_bucket[5m]))
   ```

2. **Long-running queries:**
   - Check traces for slow database spans
   - Look for lock contention

### 5. Memory and Resource Issues

**Symptoms:** High memory usage, OOM errors

**Investigation:**

1. **System metrics:**
   ```promql
   # Go runtime metrics (if enabled)
   go_memstats_heap_inuse_bytes
   go_goroutines
   ```

2. **Check for leaks:**
   - Monitor cache size growth
   - Check active lock count

## Best Practices

### 1. Sampling Strategy

For production environments:

```yaml
tracing:
  sample_rate: 0.01  # 1% sampling for normal traffic
```

Use dynamic sampling based on:
- Error status (always sample errors)
- High latency (sample slow requests)
- Specific operations (higher sampling for critical paths)

### 2. Log Levels

Environment-specific recommendations:
- **Development:** `debug` with console format
- **Staging:** `info` with JSON format
- **Production:** `warn` with JSON format and sampling

### 3. Metric Cardinality

Avoid high-cardinality labels:
- ❌ `user_id` as a label
- ✅ `user_type` or `user_tier`
- ❌ Full URLs as labels
- ✅ URL patterns like `/entities/{type}`

### 4. Retention Policies

Recommended retention:
- **Metrics:** 15 days in Prometheus
- **Traces:** 7 days in Jaeger
- **Logs:** 30 days in Loki

### 5. Alerting

Essential alerts to configure:

```yaml
# High error rate
- alert: HighErrorRate
  expr: rate(entropic_http_requests_total{status_code=~"5.."}[5m]) > 0.05
  for: 5m
  
# High latency
- alert: HighLatency
  expr: histogram_quantile(0.95, rate(entropic_http_request_duration_seconds_bucket[5m])) > 1
  for: 5m
  
# Database connection exhaustion
- alert: DatabaseConnectionExhaustion
  expr: entropic_database_connections_active / entropic_database_connections_max > 0.9
  for: 5m
```

### 6. Performance Impact

Observability overhead:
- **Metrics:** < 1% CPU, minimal memory
- **Tracing:** ~2-5% overhead with 100% sampling
- **Logging:** Depends on volume, use sampling if needed

### 7. Security Considerations

- Never log sensitive data (passwords, API keys)
- Use separate networks for observability tools
- Secure endpoints with authentication
- Rotate Jaeger/Prometheus data regularly

## Troubleshooting

### Metrics Not Appearing

1. Check if metrics are enabled:
   ```bash
   curl http://localhost:8080/metrics
   ```

2. Verify Prometheus scraping:
   ```bash
   curl http://localhost:9090/api/v1/targets
   ```

3. Check container logs:
   ```bash
   docker-compose logs prometheus
   ```

### Traces Not Showing

1. Verify tracing is enabled in config
2. Check Jaeger agent is reachable:
   ```bash
   curl http://localhost:14268/
   ```
3. Look for trace export errors in logs

### High Memory Usage

1. Reduce metric cardinality
2. Enable log sampling
3. Lower trace sampling rate
4. Check for metric leaks

## Additional Resources

- [OpenTelemetry Documentation](https://opentelemetry.io/docs/)
- [Prometheus Best Practices](https://prometheus.io/docs/practices/)
- [Jaeger Documentation](https://www.jaegertracing.io/docs/)
- [Grafana Tutorials](https://grafana.com/tutorials/)
- [Loki Documentation](https://grafana.com/docs/loki/latest/)