package observability

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// MetricsConfig holds configuration for Prometheus metrics
type MetricsConfig struct {
	Enabled    bool   `yaml:"enabled" mapstructure:"enabled"`
	Path       string `yaml:"path" mapstructure:"path"`
	Port       int    `yaml:"port" mapstructure:"port"`
	Namespace  string `yaml:"namespace" mapstructure:"namespace"`
	Subsystem  string `yaml:"subsystem" mapstructure:"subsystem"`
}

// MetricsManager manages Prometheus metrics collection and exposition
type MetricsManager struct {
	config    MetricsConfig
	registry  *prometheus.Registry
	
	// HTTP metrics
	httpRequestsTotal     *prometheus.CounterVec
	httpRequestDuration   *prometheus.HistogramVec
	httpRequestSize       *prometheus.HistogramVec
	httpResponseSize      *prometheus.HistogramVec
	
	// Entity metrics
	entitiesTotal         *prometheus.CounterVec
	entityOperations      *prometheus.CounterVec
	entityOperationDuration *prometheus.HistogramVec
	
	// Relation metrics
	relationsTotal        *prometheus.CounterVec
	relationOperations    *prometheus.CounterVec
	relationOperationDuration *prometheus.HistogramVec
	
	// Search metrics
	searchOperations      *prometheus.CounterVec
	searchDuration        *prometheus.HistogramVec
	searchResults         *prometheus.HistogramVec
	
	// Cache metrics
	cacheOperations       *prometheus.CounterVec
	cacheHits             *prometheus.CounterVec
	cacheMisses           *prometheus.CounterVec
	cacheSize             prometheus.Gauge
	
	// Database metrics
	dbConnections         prometheus.Gauge
	dbConnectionsMax      prometheus.Gauge
	dbOperations          *prometheus.CounterVec
	dbOperationDuration   *prometheus.HistogramVec
	
	// Lock metrics
	lockOperations        *prometheus.CounterVec
	lockWaitDuration      *prometheus.HistogramVec
	activeLocks           prometheus.Gauge
	
	// System metrics
	uptimeSeconds         prometheus.Gauge
	buildInfo             *prometheus.GaugeVec
}

// NewMetricsManager creates a new metrics manager with the given configuration
func NewMetricsManager(config MetricsConfig) *MetricsManager {
	if !config.Enabled {
		return &MetricsManager{config: config}
	}

	registry := prometheus.NewRegistry()
	
	// Set default values
	namespace := config.Namespace
	if namespace == "" {
		namespace = "entropic"
	}
	subsystem := config.Subsystem
	if subsystem == "" {
		subsystem = "storage"
	}

	mm := &MetricsManager{
		config:   config,
		registry: registry,
	}

	// HTTP metrics
	mm.httpRequestsTotal = promauto.With(registry).NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "http",
			Name:      "requests_total",
			Help:      "Total number of HTTP requests",
		},
		[]string{"method", "path", "status_code"},
	)

	mm.httpRequestDuration = promauto.With(registry).NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "http",
			Name:      "request_duration_seconds",
			Help:      "HTTP request duration in seconds",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)

	mm.httpRequestSize = promauto.With(registry).NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "http",
			Name:      "request_size_bytes",
			Help:      "HTTP request size in bytes",
			Buckets:   prometheus.ExponentialBuckets(1024, 2, 10),
		},
		[]string{"method", "path"},
	)

	mm.httpResponseSize = promauto.With(registry).NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "http",
			Name:      "response_size_bytes",
			Help:      "HTTP response size in bytes",
			Buckets:   prometheus.ExponentialBuckets(1024, 2, 10),
		},
		[]string{"method", "path"},
	)

	// Entity metrics
	mm.entitiesTotal = promauto.With(registry).NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "entities_total",
			Help:      "Total number of entities by type",
		},
		[]string{"entity_type"},
	)

	mm.entityOperations = promauto.With(registry).NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "entity_operations_total",
			Help:      "Total number of entity operations",
		},
		[]string{"operation", "entity_type", "status"},
	)

	mm.entityOperationDuration = promauto.With(registry).NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "entity_operation_duration_seconds",
			Help:      "Entity operation duration in seconds",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"operation", "entity_type"},
	)

	// Relation metrics
	mm.relationsTotal = promauto.With(registry).NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "relations_total",
			Help:      "Total number of relations by type",
		},
		[]string{"relation_type"},
	)

	mm.relationOperations = promauto.With(registry).NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "relation_operations_total",
			Help:      "Total number of relation operations",
		},
		[]string{"operation", "relation_type", "status"},
	)

	mm.relationOperationDuration = promauto.With(registry).NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "relation_operation_duration_seconds",
			Help:      "Relation operation duration in seconds",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"operation", "relation_type"},
	)

	// Search metrics
	mm.searchOperations = promauto.With(registry).NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "search_operations_total",
			Help:      "Total number of search operations",
		},
		[]string{"search_type", "status"},
	)

	mm.searchDuration = promauto.With(registry).NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "search_duration_seconds",
			Help:      "Search operation duration in seconds",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"search_type"},
	)

	mm.searchResults = promauto.With(registry).NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "search_results_count",
			Help:      "Number of search results returned",
			Buckets:   []float64{0, 1, 5, 10, 25, 50, 100, 250, 500, 1000},
		},
		[]string{"search_type"},
	)

	// Cache metrics
	mm.cacheOperations = promauto.With(registry).NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "cache",
			Name:      "operations_total",
			Help:      "Total number of cache operations",
		},
		[]string{"operation", "cache_type"},
	)

	mm.cacheHits = promauto.With(registry).NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "cache",
			Name:      "hits_total",
			Help:      "Total number of cache hits",
		},
		[]string{"cache_type"},
	)

	mm.cacheMisses = promauto.With(registry).NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "cache",
			Name:      "misses_total",
			Help:      "Total number of cache misses",
		},
		[]string{"cache_type"},
	)

	mm.cacheSize = promauto.With(registry).NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "cache",
			Name:      "size_bytes",
			Help:      "Current cache size in bytes",
		},
	)

	// Database metrics
	mm.dbConnections = promauto.With(registry).NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "database",
			Name:      "connections_active",
			Help:      "Number of active database connections",
		},
	)

	mm.dbConnectionsMax = promauto.With(registry).NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "database",
			Name:      "connections_max",
			Help:      "Maximum number of database connections",
		},
	)

	mm.dbOperations = promauto.With(registry).NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "database",
			Name:      "operations_total",
			Help:      "Total number of database operations",
		},
		[]string{"operation", "table", "status"},
	)

	mm.dbOperationDuration = promauto.With(registry).NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "database",
			Name:      "operation_duration_seconds",
			Help:      "Database operation duration in seconds",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"operation", "table"},
	)

	// Lock metrics
	mm.lockOperations = promauto.With(registry).NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "lock",
			Name:      "operations_total",
			Help:      "Total number of lock operations",
		},
		[]string{"operation", "status"},
	)

	mm.lockWaitDuration = promauto.With(registry).NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "lock",
			Name:      "wait_duration_seconds",
			Help:      "Lock wait duration in seconds",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"lock_type"},
	)

	mm.activeLocks = promauto.With(registry).NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "lock",
			Name:      "active_total",
			Help:      "Number of currently active locks",
		},
	)

	// System metrics
	mm.uptimeSeconds = promauto.With(registry).NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "system",
			Name:      "uptime_seconds",
			Help:      "System uptime in seconds",
		},
	)

	mm.buildInfo = promauto.With(registry).NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "system",
			Name:      "build_info",
			Help:      "Build information",
		},
		[]string{"version", "commit", "build_time"},
	)

	return mm
}

// HTTP Metrics Methods

func (mm *MetricsManager) RecordHTTPRequest(method, path string, statusCode int, duration time.Duration, requestSize, responseSize int64) {
	if !mm.config.Enabled {
		return
	}
	
	mm.httpRequestsTotal.WithLabelValues(method, path, strconv.Itoa(statusCode)).Inc()
	mm.httpRequestDuration.WithLabelValues(method, path).Observe(duration.Seconds())
	mm.httpRequestSize.WithLabelValues(method, path).Observe(float64(requestSize))
	mm.httpResponseSize.WithLabelValues(method, path).Observe(float64(responseSize))
}

// Entity Metrics Methods

func (mm *MetricsManager) IncEntityCount(entityType string) {
	if !mm.config.Enabled {
		return
	}
	mm.entitiesTotal.WithLabelValues(entityType).Inc()
}

func (mm *MetricsManager) RecordEntityOperation(operation, entityType, status string, duration time.Duration) {
	if !mm.config.Enabled {
		return
	}
	
	mm.entityOperations.WithLabelValues(operation, entityType, status).Inc()
	mm.entityOperationDuration.WithLabelValues(operation, entityType).Observe(duration.Seconds())
}

// Relation Metrics Methods

func (mm *MetricsManager) IncRelationCount(relationType string) {
	if !mm.config.Enabled {
		return
	}
	mm.relationsTotal.WithLabelValues(relationType).Inc()
}

func (mm *MetricsManager) RecordRelationOperation(operation, relationType, status string, duration time.Duration) {
	if !mm.config.Enabled {
		return
	}
	
	mm.relationOperations.WithLabelValues(operation, relationType, status).Inc()
	mm.relationOperationDuration.WithLabelValues(operation, relationType).Observe(duration.Seconds())
}

// Search Metrics Methods

func (mm *MetricsManager) RecordSearchOperation(searchType, status string, duration time.Duration, resultCount int) {
	if !mm.config.Enabled {
		return
	}
	
	mm.searchOperations.WithLabelValues(searchType, status).Inc()
	mm.searchDuration.WithLabelValues(searchType).Observe(duration.Seconds())
	mm.searchResults.WithLabelValues(searchType).Observe(float64(resultCount))
}

// Cache Metrics Methods

func (mm *MetricsManager) RecordCacheOperation(operation, cacheType string) {
	if !mm.config.Enabled {
		return
	}
	mm.cacheOperations.WithLabelValues(operation, cacheType).Inc()
}

func (mm *MetricsManager) RecordCacheHit(cacheType string) {
	if !mm.config.Enabled {
		return
	}
	mm.cacheHits.WithLabelValues(cacheType).Inc()
}

func (mm *MetricsManager) RecordCacheMiss(cacheType string) {
	if !mm.config.Enabled {
		return
	}
	mm.cacheMisses.WithLabelValues(cacheType).Inc()
}

func (mm *MetricsManager) SetCacheSize(size float64) {
	if !mm.config.Enabled {
		return
	}
	mm.cacheSize.Set(size)
}

// Database Metrics Methods

func (mm *MetricsManager) SetDatabaseConnections(active, max int) {
	if !mm.config.Enabled {
		return
	}
	mm.dbConnections.Set(float64(active))
	mm.dbConnectionsMax.Set(float64(max))
}

func (mm *MetricsManager) RecordDatabaseOperation(operation, table, status string, duration time.Duration) {
	if !mm.config.Enabled {
		return
	}
	
	mm.dbOperations.WithLabelValues(operation, table, status).Inc()
	mm.dbOperationDuration.WithLabelValues(operation, table).Observe(duration.Seconds())
}

// Lock Metrics Methods

func (mm *MetricsManager) RecordLockOperation(operation, status string, waitDuration time.Duration, lockType string) {
	if !mm.config.Enabled {
		return
	}
	
	mm.lockOperations.WithLabelValues(operation, status).Inc()
	mm.lockWaitDuration.WithLabelValues(lockType).Observe(waitDuration.Seconds())
}

func (mm *MetricsManager) SetActiveLocks(count int) {
	if !mm.config.Enabled {
		return
	}
	mm.activeLocks.Set(float64(count))
}

// System Metrics Methods

func (mm *MetricsManager) SetUptime(startTime time.Time) {
	if !mm.config.Enabled {
		return
	}
	mm.uptimeSeconds.Set(time.Since(startTime).Seconds())
}

func (mm *MetricsManager) SetBuildInfo(version, commit, buildTime string) {
	if !mm.config.Enabled {
		return
	}
	mm.buildInfo.WithLabelValues(version, commit, buildTime).Set(1)
}

// Handler returns the Prometheus metrics handler
func (mm *MetricsManager) Handler() http.Handler {
	if !mm.config.Enabled {
		return http.NotFoundHandler()
	}
	return promhttp.HandlerFor(mm.registry, promhttp.HandlerOpts{})
}

// MetricsMiddleware creates middleware for automatic HTTP metrics collection
func (mm *MetricsManager) MetricsMiddleware() func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !mm.config.Enabled {
				next.ServeHTTP(w, r)
				return
			}

			start := time.Now()
			
			// Wrap response writer to capture metrics
			wrapped := &metricsResponseWriter{ResponseWriter: w}
			
			next.ServeHTTP(wrapped, r)
			
			duration := time.Since(start)
			requestSize := r.ContentLength
			if requestSize < 0 {
				requestSize = 0
			}
			
			mm.RecordHTTPRequest(
				r.Method,
				r.URL.Path,
				wrapped.statusCode,
				duration,
				requestSize,
				wrapped.size,
			)
		})
	}
}

type metricsResponseWriter struct {
	http.ResponseWriter
	statusCode int
	size       int64
}

func (mrw *metricsResponseWriter) WriteHeader(statusCode int) {
	mrw.statusCode = statusCode
	mrw.ResponseWriter.WriteHeader(statusCode)
}

func (mrw *metricsResponseWriter) Write(data []byte) (int, error) {
	size, err := mrw.ResponseWriter.Write(data)
	mrw.size += int64(size)
	return size, err
}

// IsEnabled returns whether metrics collection is enabled
func (mm *MetricsManager) IsEnabled() bool {
	return mm.config.Enabled
}

// StartUptimeTracker starts a goroutine to continuously update uptime metrics
func (mm *MetricsManager) StartUptimeTracker(ctx context.Context, startTime time.Time) {
	if !mm.config.Enabled {
		return
	}

	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				mm.SetUptime(startTime)
			}
		}
	}()
}