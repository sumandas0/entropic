package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/entropic/entropic/internal/api/handlers"
	"github.com/entropic/entropic/internal/api/middleware"
	"github.com/entropic/entropic/internal/core"
	"github.com/entropic/entropic/internal/health"
	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

// Router sets up and configures the HTTP router
type Router struct {
	engine          *core.Engine
	entityHandler   *handlers.EntityHandler
	relationHandler *handlers.RelationHandler
	schemaHandler   *handlers.SchemaHandler
	healthChecker   *health.HealthChecker
}

// NewRouter creates a new router instance
func NewRouter(engine *core.Engine, healthChecker *health.HealthChecker) *Router {
	return &Router{
		engine:          engine,
		entityHandler:   handlers.NewEntityHandler(engine),
		relationHandler: handlers.NewRelationHandler(engine),
		schemaHandler:   handlers.NewSchemaHandler(engine),
		healthChecker:   healthChecker,
	}
}

// SetupRoutes configures all routes and middleware
func (r *Router) SetupRoutes() http.Handler {
	router := chi.NewRouter()

	// Basic middleware
	router.Use(chiMiddleware.RequestID)
	router.Use(chiMiddleware.RealIP)
	router.Use(middleware.Logger())
	router.Use(chiMiddleware.Recoverer)
	router.Use(middleware.ErrorHandler())

	// CORS configuration
	router.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"https://*", "http://*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	// Timeout middleware
	router.Use(chiMiddleware.Timeout(60 * time.Second))

	// Rate limiting middleware (basic implementation)
	router.Use(middleware.RateLimit(100, time.Minute)) // 100 requests per minute

	// Health check endpoint
	router.Get("/health", r.healthCheck)
	router.Get("/ready", r.readinessCheck)

	// Metrics endpoint
	router.Get("/metrics", r.metrics)

	// API version prefix
	router.Route("/api/v1", func(r chi.Router) {
		// Entity routes
		r.Route("/entities", func(r chi.Router) {
			r.Route("/{entityType}", func(r chi.Router) {
				r.Post("/", r.entityHandler.CreateEntity)
				r.Get("/", r.entityHandler.ListEntities)
				
				r.Route("/{entityID}", func(r chi.Router) {
					r.Get("/", r.entityHandler.GetEntity)
					r.Patch("/", r.entityHandler.UpdateEntity)
					r.Delete("/", r.entityHandler.DeleteEntity)
					
					// Entity relations
					r.Get("/relations", r.entityHandler.GetEntityRelations)
				})
			})
		})

		// Relation routes
		r.Route("/relations", func(r chi.Router) {
			r.Post("/", r.relationHandler.CreateRelation)
			
			r.Route("/{relationID}", func(r chi.Router) {
				r.Get("/", r.relationHandler.GetRelation)
				r.Delete("/", r.relationHandler.DeleteRelation)
			})
		})

		// Schema routes
		r.Route("/schemas", func(r chi.Router) {
			// Entity schemas
			r.Route("/entities", func(r chi.Router) {
				r.Post("/", r.schemaHandler.CreateEntitySchema)
				r.Get("/", r.schemaHandler.ListEntitySchemas)
				
				r.Route("/{entityType}", func(r chi.Router) {
					r.Get("/", r.schemaHandler.GetEntitySchema)
					r.Put("/", r.schemaHandler.UpdateEntitySchema)
					r.Delete("/", r.schemaHandler.DeleteEntitySchema)
				})
			})

			// Relationship schemas
			r.Route("/relationships", func(r chi.Router) {
				r.Post("/", r.schemaHandler.CreateRelationshipSchema)
				r.Get("/", r.schemaHandler.ListRelationshipSchemas)
				
				r.Route("/{relationshipType}", func(r chi.Router) {
					r.Get("/", r.schemaHandler.GetRelationshipSchema)
					r.Put("/", r.schemaHandler.UpdateRelationshipSchema)
					r.Delete("/", r.schemaHandler.DeleteRelationshipSchema)
				})
			})
		})

		// Search routes
		r.Route("/search", func(r chi.Router) {
			r.Post("/", r.entityHandler.Search)
			r.Post("/vector", r.entityHandler.VectorSearch)
		})
	})

	return router
}

// healthCheck returns the health status of the system
func (r *Router) healthCheck(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	
	if r.healthChecker != nil {
		systemHealth := r.healthChecker.Check(ctx)
		
		statusCode := http.StatusOK
		if systemHealth.Status == health.StatusUnhealthy {
			statusCode = http.StatusServiceUnavailable
		}
		
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		json.NewEncoder(w).Encode(systemHealth)
		return
	}
	
	// Fallback to simple health check
	if err := r.engine.HealthCheck(ctx); err != nil {
		http.Error(w, "Service unhealthy", http.StatusServiceUnavailable)
		return
	}

	response := map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now().UTC(),
		"version":   "1.0.0", // This would come from build info
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// readinessCheck returns the readiness status of the system
func (r *Router) readinessCheck(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	
	// Check if system is ready to accept requests
	if err := r.engine.HealthCheck(ctx); err != nil {
		http.Error(w, "Service not ready", http.StatusServiceUnavailable)
		return
	}

	response := map[string]interface{}{
		"status":    "ready",
		"timestamp": time.Now().UTC(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// metrics returns system metrics
func (r *Router) metrics(w http.ResponseWriter, req *http.Request) {
	stats := r.engine.GetStats()

	response := map[string]interface{}{
		"cache": map[string]interface{}{
			"hits":                     stats.CacheStats.Hits,
			"misses":                   stats.CacheStats.Misses,
			"hit_rate":                 stats.CacheStats.HitRate,
			"entity_schema_count":      stats.CacheStats.EntitySchemaCount,
			"relationship_schema_count": stats.CacheStats.RelationshipSchemaCount,
		},
		"transactions": map[string]interface{}{
			"active_transactions": stats.TransactionStats.ActiveTransactions,
			"total_committed":     stats.TransactionStats.TotalCommitted,
			"total_rolled_back":   stats.TransactionStats.TotalRolledBack,
			"total_timeouts":      stats.TransactionStats.CommitTimeouts,
			"average_commit_time": stats.TransactionStats.AverageCommitTime.Milliseconds(),
		},
		"timestamp": time.Now().UTC(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}