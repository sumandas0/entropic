package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"github.com/sumandas0/entropic/config"
	"github.com/sumandas0/entropic/internal/api"
	"github.com/sumandas0/entropic/internal/cache"
	"github.com/sumandas0/entropic/internal/core"
	"github.com/sumandas0/entropic/internal/health"
	"github.com/sumandas0/entropic/internal/integration"
	"github.com/sumandas0/entropic/internal/lock"
	"github.com/sumandas0/entropic/internal/observability"
	"github.com/sumandas0/entropic/internal/store/postgres"
	"github.com/sumandas0/entropic/internal/store/typesense"
	"go.opentelemetry.io/otel/trace"

	_ "github.com/sumandas0/entropic/docs"
)

var (
	version   = "dev"
	commit    = "unknown"
	buildTime = "unknown"
)

// @title Entropic Storage Engine API
// @version 1.0
// @description A next-generation storage engine with flexible entity-relationship model and dual-storage architecture
// @termsOfService http://swagger.io/terms/

// @contact.name API Support
// @contact.url http://www.entropic.io/support
// @contact.email support@entropic.io

// @license.name Apache 2.0
// @license.url http://www.apache.org/licenses/LICENSE-2.0.html

// @host localhost:8080
// @BasePath /api/v1

// @schemes http https
func main() {
	rootCmd := &cobra.Command{
		Use:   "entropic-server",
		Short: "Entropic Storage Engine Server",
		Long:  "A next-generation storage engine with flexible entity-relationship model and dual-storage architecture.",
		RunE:  runServer,
	}

	rootCmd.Flags().StringP("config", "c", "", "Path to configuration file")
	rootCmd.Flags().BoolP("version", "v", false, "Show version information")

	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Show version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("Entropic Server\n")
			fmt.Printf("Version: %s\n", version)
			fmt.Printf("Commit: %s\n", commit)
			fmt.Printf("Build Time: %s\n", buildTime)
		},
	}
	rootCmd.AddCommand(versionCmd)

	migrateCmd := &cobra.Command{
		Use:   "migrate",
		Short: "Run database migrations",
		RunE:  runMigrations,
	}
	migrateCmd.Flags().StringP("config", "c", "", "Path to configuration file")
	rootCmd.AddCommand(migrateCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runServer(cmd *cobra.Command, args []string) error {

	if versionFlag, _ := cmd.Flags().GetBool("version"); versionFlag {
		fmt.Printf("Entropic Server %s (commit: %s, built: %s)\n", version, commit, buildTime)
		return nil
	}

	configPath, _ := cmd.Flags().GetString("config")
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Initialize observability
	obsManager, err := initializeObservability(cfg)
	if err != nil {
		return fmt.Errorf("failed to initialize observability: %w", err)
	}
	defer obsManager.Shutdown(context.Background())

	logger := obsManager.GetLogging().GetZerologLogger()
	tracer := obsManager.GetTracing().GetTracer()

	// Create root span for server startup
	ctx, span := tracer.Start(context.Background(), "server.startup")
	defer span.End()

	logger.Info().
		Str("version", version).
		Str("commit", commit).
		Str("build_time", buildTime).
		Str("config_source", getConfigSource(configPath)).
		Msg("Starting Entropic Server")

	app, err := NewApplication(ctx, cfg, obsManager)
	if err != nil {
		obsManager.GetTracing().SetSpanError(span, err)
		return fmt.Errorf("failed to initialize application: %w", err)
	}
	defer app.Close()

	server := &http.Server{
		Addr:         cfg.GetServerAddress(),
		Handler:      app.Handler(),
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	serverChan := make(chan error, 1)
	go func() {
		logger.Info().Str("address", cfg.GetServerAddress()).Msg("Server starting")
		span.End() // End startup span when server is ready
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverChan <- fmt.Errorf("server failed to start: %w", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverChan:
		return err
	case sig := <-quit:
		logger.Info().Str("signal", sig.String()).Msg("Received signal")
	}

	// Create shutdown span
	shutdownCtx, shutdownSpan := tracer.Start(context.Background(), "server.shutdown")
	defer shutdownSpan.End()

	logger.Info().Msg("Shutting down server...")
	ctx, cancel := context.WithTimeout(shutdownCtx, cfg.Server.ShutdownTimeout)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		obsManager.GetTracing().SetSpanError(shutdownSpan, err)
		logger.Error().Err(err).Msg("Server forced to shutdown")
		return err
	}

	logger.Info().Msg("Server shutdown completed")
	return nil
}

func runMigrations(cmd *cobra.Command, args []string) error {

	configPath, _ := cmd.Flags().GetString("config")
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Initialize observability for migrations
	obsManager, err := initializeObservability(cfg)
	if err != nil {
		return fmt.Errorf("failed to initialize observability: %w", err)
	}
	defer obsManager.Shutdown(context.Background())

	logger := obsManager.GetLogging().GetZerologLogger()
	tracer := obsManager.GetTracing().GetTracer()

	ctx, span := tracer.Start(context.Background(), "migration.run")
	defer span.End()

	logger.Info().Msg("Running database migrations...")

	pgStore, err := postgres.NewPostgresStore(cfg.GetDatabaseURL())
	if err != nil {
		obsManager.GetTracing().SetSpanError(span, err)
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer pgStore.Close()

	migrator := postgres.NewMigrator(pgStore.GetPool())
	if err := migrator.Run(ctx); err != nil {
		obsManager.GetTracing().SetSpanError(span, err)
		return fmt.Errorf("migration failed: %w", err)
	}

	logger.Info().Msg("Database migrations completed successfully")
	return nil
}

type Application struct {
	cfg           *config.Config
	primaryStore  *postgres.PostgresStore
	indexStore    *typesense.TypesenseStore
	cacheManager  *cache.CacheAwareManager
	lockManager   *lock.LockManager
	engine        *core.Engine
	healthChecker *health.HealthChecker
	router        *api.Router
	obsManager    *integration.ObservabilityManager
	logger        zerolog.Logger
	tracer        trace.Tracer
}

func NewApplication(ctx context.Context, cfg *config.Config, obsManager *integration.ObservabilityManager) (*Application, error) {
	logger := obsManager.GetLogging().GetZerologLogger()
	tracer := obsManager.GetTracing().GetTracer()

	app := &Application{
		cfg:        cfg,
		obsManager: obsManager,
		logger:     logger,
		tracer:     tracer,
	}

	// Initialize PostgreSQL
	ctx, pgSpan := tracer.Start(ctx, "postgres.initialize")
	logger.Info().Msg("Initializing PostgreSQL connection...")
	primaryStore, err := postgres.NewPostgresStore(cfg.GetDatabaseURL())
	if err != nil {
		obsManager.GetTracing().SetSpanError(pgSpan, err)
		pgSpan.End()
		return nil, fmt.Errorf("failed to initialize PostgreSQL: %w", err)
	}
	primaryStore.SetObservability(obsManager)
	app.primaryStore = primaryStore

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := primaryStore.Ping(pingCtx); err != nil {
		obsManager.GetTracing().SetSpanError(pgSpan, err)
		pgSpan.End()
		return nil, fmt.Errorf("failed to ping PostgreSQL: %w", err)
	}
	logger.Info().Msg("PostgreSQL connection established")
	pgSpan.End()

	if cfg.Database.MigrateOnStart {
		ctx, migSpan := tracer.Start(ctx, "migration.auto")
		logger.Info().Msg("Running database migrations...")
		migrator := postgres.NewMigrator(primaryStore.GetPool())
		if err := migrator.Run(ctx); err != nil {
			obsManager.GetTracing().SetSpanError(migSpan, err)
			migSpan.End()
			return nil, fmt.Errorf("migration failed: %w", err)
		}
		logger.Info().Msg("Database migrations completed")
		migSpan.End()
	}

	// Initialize Typesense
	ctx, tsSpan := tracer.Start(ctx, "typesense.initialize")
	logger.Info().Msg("Initializing Typesense connection...")
	indexStore, err := typesense.NewTypesenseStore(cfg.Search.URL, cfg.Search.APIKey)
	if err != nil {
		obsManager.GetTracing().SetSpanError(tsSpan, err)
		tsSpan.End()
		return nil, fmt.Errorf("failed to initialize Typesense: %w", err)
	}
	indexStore.SetObservability(obsManager)
	app.indexStore = indexStore

	if err := indexStore.Ping(ctx); err != nil {
		obsManager.GetTracing().SetSpanError(tsSpan, err)
		logger.Warn().Err(err).Msg("Failed to ping Typesense (continuing anyway)")
	} else {
		logger.Info().Msg("Typesense connection established")
	}
	tsSpan.End()

	// Initialize cache manager
	ctx, cacheSpan := tracer.Start(ctx, "cache.initialize")
	logger.Info().Msg("Initializing cache manager...")
	cacheManager := cache.NewCacheAwareManager(primaryStore, cfg.Cache.TTL)
	app.cacheManager = cacheManager

	cacheManager.StartCleanupRoutine(context.Background(), cfg.Cache.CleanupInterval)
	logger.Info().Msg("Cache manager initialized")
	cacheSpan.End()

	// Initialize lock manager
	ctx, lockSpan := tracer.Start(ctx, "lock.initialize")
	logger.Info().Msg("Initializing lock manager...")
	var distributedLock lock.DistributedLock
	switch cfg.Lock.Type {
	case "memory":
		distributedLock = lock.NewInMemoryDistributedLock()
	case "redis":
		logger.Warn().Msg("Redis distributed lock not implemented, falling back to in-memory")
		distributedLock = lock.NewInMemoryDistributedLock()
	default:
		lockSpan.End()
		return nil, fmt.Errorf("unsupported lock type: %s", cfg.Lock.Type)
	}

	lockManager := lock.NewLockManager(distributedLock)
	app.lockManager = lockManager
	logger.Info().Msg("Lock manager initialized")
	lockSpan.End()

	// Initialize core engine
	ctx, engineSpan := tracer.Start(ctx, "engine.initialize")
	logger.Info().Msg("Initializing core engine...")
	engine, err := core.NewEngine(primaryStore, indexStore, cacheManager, lockManager, obsManager)
	if err != nil {
		obsManager.GetTracing().SetSpanError(engineSpan, err)
		engineSpan.End()
		return nil, fmt.Errorf("failed to initialize core engine: %w", err)
	}
	app.engine = engine
	logger.Info().Msg("Core engine initialized")
	engineSpan.End()

	// Preload schemas
	ctx, schemaSpan := tracer.Start(ctx, "schema.preload")
	logger.Info().Msg("Preloading schemas...")
	if err := cacheManager.PreloadSchemas(ctx); err != nil {
		obsManager.GetTracing().SetSpanError(schemaSpan, err)
		logger.Warn().Err(err).Msg("Failed to preload schemas")
	} else {
		logger.Info().Msg("Schemas preloaded successfully")
	}
	schemaSpan.End()

	// Initialize health checker
	ctx, healthSpan := tracer.Start(ctx, "health.initialize")
	logger.Info().Msg("Initializing health checker...")
	healthChecker := health.NewHealthChecker(5 * time.Second)
	healthChecker.RegisterComponent("database", health.CreateDatabaseHealthCheck(primaryStore))
	healthChecker.RegisterComponent("search", health.CreateSearchHealthCheck(indexStore))
	healthChecker.RegisterComponent("memory", health.CreateMemoryHealthCheck())

	healthChecker.StartPeriodicChecks(context.Background(), 30*time.Second)
	app.healthChecker = healthChecker
	logger.Info().Msg("Health checker initialized")
	healthSpan.End()

	// Initialize API router
	_, routerSpan := tracer.Start(ctx, "router.initialize")
	logger.Info().Msg("Initializing API router...")
	router := api.NewRouter(engine, healthChecker, obsManager)
	app.router = router
	logger.Info().Msg("API router initialized")
	routerSpan.End()

	logger.Info().Msg("Application initialization completed")
	return app, nil
}

func (app *Application) Handler() http.Handler {
	return app.router.SetupRoutes()
}

func (app *Application) Close() error {
	_, span := app.tracer.Start(context.Background(), "application.close")
	defer span.End()

	app.logger.Info().Msg("Closing application components...")

	var errors []error

	if app.engine != nil {
		if err := app.engine.Close(); err != nil {
			errors = append(errors, fmt.Errorf("engine close failed: %w", err))
		}
	}

	if app.lockManager != nil {
		if err := app.lockManager.Close(); err != nil {
			errors = append(errors, fmt.Errorf("lock manager close failed: %w", err))
		}
	}

	if app.indexStore != nil {
		if err := app.indexStore.Close(); err != nil {
			errors = append(errors, fmt.Errorf("index store close failed: %w", err))
		}
	}

	if app.primaryStore != nil {
		if err := app.primaryStore.Close(); err != nil {
			errors = append(errors, fmt.Errorf("primary store close failed: %w", err))
		}
	}

	if len(errors) > 0 {
		app.logger.Error().Int("error_count", len(errors)).Msg("Some components failed to close properly")
		for _, err := range errors {
			app.logger.Error().Err(err).Msg("Component close error")
		}
		app.obsManager.GetTracing().SetSpanError(span, fmt.Errorf("application close failed with %d errors", len(errors)))
		return fmt.Errorf("application close failed with %d errors", len(errors))
	}

	app.logger.Info().Msg("Application closed successfully")
	return nil
}

func initializeObservability(cfg *config.Config) (*integration.ObservabilityManager, error) {
	// Create observability config
	obsConfig := integration.AdvancedFeaturesConfig{
		Logging: observability.LoggingConfig{
			Level:  observability.LogLevel(cfg.Logging.Level),
			Format: observability.LogFormat(cfg.Logging.Format),
			Output: cfg.Logging.Output,
		},
		Tracing: observability.TracingConfig{
			Enabled:     cfg.Tracing.Enabled,
			JaegerURL:   cfg.Tracing.JaegerURL,
			ServiceName: "entropic-server",
			Environment: cfg.Environment,
			SampleRate:  cfg.Tracing.SampleRate,
		},
		Metrics: observability.MetricsConfig{
			Enabled:   cfg.Metrics.Enabled,
			Path:      cfg.Metrics.Path,
			Port:      cfg.Metrics.PrometheusPort,
			Namespace: "entropic",
			Subsystem: "server",
		},
	}

	return integration.NewObservabilityManager(
		obsConfig.Tracing,
		obsConfig.Logging,
		obsConfig.Metrics,
	)
}

func getConfigSource(configPath string) string {
	if configPath != "" {
		return configPath
	}
	return "defaults and environment variables"
}
