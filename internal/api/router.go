package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/sumandas0/entropic/internal/api/handlers"
	"github.com/sumandas0/entropic/internal/api/middleware"
	"github.com/sumandas0/entropic/internal/core"
	"github.com/sumandas0/entropic/internal/health"
	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	httpSwagger "github.com/swaggo/http-swagger"
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

	// Swagger documentation
	router.Get("/swagger/*", httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"), // The url pointing to API definition
	))

	// API version prefix
	router.Route("/api/v1", func(apiRouter chi.Router) {
		// Entity routes
		apiRouter.Route("/entities", func(entityRouter chi.Router) {
			entityRouter.Route("/{entityType}", func(typeRouter chi.Router) {
				typeRouter.Post("/", r.entityHandler.CreateEntity)
				typeRouter.Get("/", r.entityHandler.ListEntities)
				
				typeRouter.Route("/{entityID}", func(idRouter chi.Router) {
					idRouter.Get("/", r.entityHandler.GetEntity)
					idRouter.Patch("/", r.entityHandler.UpdateEntity)
					idRouter.Delete("/", r.entityHandler.DeleteEntity)
					
					// Entity relations
					idRouter.Get("/relations", r.entityHandler.GetEntityRelations)
				})
			})
		})

		// Relation routes
		apiRouter.Route("/relations", func(relationRouter chi.Router) {
			relationRouter.Post("/", r.relationHandler.CreateRelation)
			
			relationRouter.Route("/{relationID}", func(idRouter chi.Router) {
				idRouter.Get("/", r.relationHandler.GetRelation)
				idRouter.Delete("/", r.relationHandler.DeleteRelation)
			})
		})

		// Schema routes
		apiRouter.Route("/schemas", func(schemaRouter chi.Router) {
			// Entity schemas
			schemaRouter.Route("/entities", func(entitySchemaRouter chi.Router) {
				entitySchemaRouter.Post("/", r.schemaHandler.CreateEntitySchema)
				entitySchemaRouter.Get("/", r.schemaHandler.ListEntitySchemas)
				
				entitySchemaRouter.Route("/{entityType}", func(typeRouter chi.Router) {
					typeRouter.Get("/", r.schemaHandler.GetEntitySchema)
					typeRouter.Put("/", r.schemaHandler.UpdateEntitySchema)
					typeRouter.Delete("/", r.schemaHandler.DeleteEntitySchema)
				})
			})

			// Relationship schemas
			schemaRouter.Route("/relationships", func(relSchemaRouter chi.Router) {
				relSchemaRouter.Post("/", r.schemaHandler.CreateRelationshipSchema)
				relSchemaRouter.Get("/", r.schemaHandler.ListRelationshipSchemas)
				
				relSchemaRouter.Route("/{relationshipType}", func(typeRouter chi.Router) {
					typeRouter.Get("/", r.schemaHandler.GetRelationshipSchema)
					typeRouter.Put("/", r.schemaHandler.UpdateRelationshipSchema)
					typeRouter.Delete("/", r.schemaHandler.DeleteRelationshipSchema)
				})
			})
		})

		// Search routes
		apiRouter.Route("/search", func(searchRouter chi.Router) {
			searchRouter.Post("/", r.entityHandler.Search)
			searchRouter.Post("/vector", r.entityHandler.VectorSearch)
		})
	})

	return router
}

// healthCheck returns the health status of the system
// @Summary Health check
// @Description Returns the health status of the service and its dependencies
// @Tags health
// @Produce json
// @Success 200 {object} health.SystemHealth
// @Failure 503 {object} health.SystemHealth
// @Router /health [get]
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
// @Summary Readiness check
// @Description Indicates if the service is ready to accept requests
// @Tags health
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 503 {string} string "Service not ready"
// @Router /ready [get]
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
// @Summary Get metrics
// @Description Returns service metrics including cache and transaction statistics
// @Tags health
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /metrics [get]
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