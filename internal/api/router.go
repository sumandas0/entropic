package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/sumandas0/entropic/internal/api/handlers"
	"github.com/sumandas0/entropic/internal/api/middleware"
	"github.com/sumandas0/entropic/internal/core"
	"github.com/sumandas0/entropic/internal/health"
	"github.com/sumandas0/entropic/internal/integration"
	httpSwagger "github.com/swaggo/http-swagger"
)

type Router struct {
	engine          *core.Engine
	entityHandler   *handlers.EntityHandler
	relationHandler *handlers.RelationHandler
	schemaHandler   *handlers.SchemaHandler
	healthChecker   *health.HealthChecker
	obsManager      *integration.ObservabilityManager
}

func NewRouter(engine *core.Engine, healthChecker *health.HealthChecker, obsManager *integration.ObservabilityManager) *Router {
	return &Router{
		engine:          engine,
		entityHandler:   handlers.NewEntityHandler(engine),
		relationHandler: handlers.NewRelationHandler(engine),
		schemaHandler:   handlers.NewSchemaHandler(engine),
		healthChecker:   healthChecker,
		obsManager:      obsManager,
	}
}

func (r *Router) SetupRoutes() http.Handler {
	router := chi.NewRouter()

	// Add tracing middleware first
	if r.obsManager != nil && r.obsManager.GetTracing().IsEnabled() {
		router.Use(r.obsManager.GetTracing().TraceMiddleware())
	}

	// Add logging middleware
	if r.obsManager != nil {
		router.Use(r.obsManager.GetLogging().LoggingMiddleware())
	}

	router.Use(chiMiddleware.RequestID)
	router.Use(chiMiddleware.RealIP)
	router.Use(chiMiddleware.Recoverer)
	router.Use(middleware.ErrorHandlerWithObservability(r.obsManager))

	router.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"https://*", "http://*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	router.Use(chiMiddleware.Timeout(60 * time.Second))

	router.Use(middleware.RateLimit(100, time.Minute))

	router.Get("/health", r.healthCheck)
	router.Get("/ready", r.readinessCheck)

	router.Get("/metrics", r.metrics)

	router.Get("/swagger/*", httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"),
	))

	router.Route("/api/v1", func(apiRouter chi.Router) {
		apiRouter.Route("/entities", func(entityRouter chi.Router) {
			entityRouter.Route("/{entityType}", func(typeRouter chi.Router) {
				typeRouter.Post("/", r.entityHandler.CreateEntity)
				typeRouter.Get("/", r.entityHandler.ListEntities)

				typeRouter.Route("/{entityID}", func(idRouter chi.Router) {
					idRouter.Get("/", r.entityHandler.GetEntity)
					idRouter.Patch("/", r.entityHandler.UpdateEntity)
					idRouter.Delete("/", r.entityHandler.DeleteEntity)

					idRouter.Get("/relations", r.entityHandler.GetEntityRelations)
				})
			})
		})

		apiRouter.Route("/relations", func(relationRouter chi.Router) {
			relationRouter.Post("/", r.relationHandler.CreateRelation)

			relationRouter.Route("/{relationID}", func(idRouter chi.Router) {
				idRouter.Get("/", r.relationHandler.GetRelation)
				idRouter.Delete("/", r.relationHandler.DeleteRelation)
			})
		})

		apiRouter.Route("/schemas", func(schemaRouter chi.Router) {

			schemaRouter.Route("/entities", func(entitySchemaRouter chi.Router) {
				entitySchemaRouter.Post("/", r.schemaHandler.CreateEntitySchema)
				entitySchemaRouter.Get("/", r.schemaHandler.ListEntitySchemas)

				entitySchemaRouter.Route("/{entityType}", func(typeRouter chi.Router) {
					typeRouter.Get("/", r.schemaHandler.GetEntitySchema)
					typeRouter.Put("/", r.schemaHandler.UpdateEntitySchema)
					typeRouter.Delete("/", r.schemaHandler.DeleteEntitySchema)
				})
			})

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

		apiRouter.Route("/search", func(searchRouter chi.Router) {
			searchRouter.Post("/", r.entityHandler.Search)
			searchRouter.Post("/vector", r.entityHandler.VectorSearch)
		})
	})

	return router
}

// healthCheck godoc
// @Summary Health check
// @Description Check the health status of the service and its dependencies
// @Tags health
// @Accept json
// @Produce json
// @Success 200 {object} map[string]any "Service is healthy"
// @Failure 503 {object} map[string]any "Service is unhealthy"
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

	if err := r.engine.HealthCheck(ctx); err != nil {
		http.Error(w, "Service unhealthy", http.StatusServiceUnavailable)
		return
	}

	response := map[string]any{
		"status":    "healthy",
		"timestamp": time.Now().UTC(),
		"version":   "1.0.0",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// readinessCheck godoc
// @Summary Readiness check
// @Description Check if the service is ready to accept requests
// @Tags health
// @Accept json
// @Produce json
// @Success 200 {object} map[string]any "Service is ready"
// @Failure 503 "Service is not ready"
// @Router /ready [get]
func (r *Router) readinessCheck(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	if err := r.engine.HealthCheck(ctx); err != nil {
		http.Error(w, "Service not ready", http.StatusServiceUnavailable)
		return
	}

	response := map[string]any{
		"status":    "ready",
		"timestamp": time.Now().UTC(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// metrics godoc
// @Summary Get service metrics
// @Description Get performance metrics and statistics for the service
// @Tags monitoring
// @Accept json
// @Produce json
// @Success 200 {object} map[string]any "Service metrics including cache stats, transaction stats, and timestamp"
// @Router /metrics [get]
func (r *Router) metrics(w http.ResponseWriter, req *http.Request) {
	stats := r.engine.GetStats()

	response := map[string]any{
		"cache": map[string]any{
			"hits":                      stats.CacheStats.Hits,
			"misses":                    stats.CacheStats.Misses,
			"hit_rate":                  stats.CacheStats.HitRate,
			"entity_schema_count":       stats.CacheStats.EntitySchemaCount,
			"relationship_schema_count": stats.CacheStats.RelationshipSchemaCount,
		},
		"transactions": map[string]any{
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
