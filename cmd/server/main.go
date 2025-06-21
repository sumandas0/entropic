package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/entropic/entropic/config"
	"github.com/entropic/entropic/internal/api"
	"github.com/entropic/entropic/internal/cache"
	"github.com/entropic/entropic/internal/core"
	"github.com/entropic/entropic/internal/health"
	"github.com/entropic/entropic/internal/lock"
	"github.com/entropic/entropic/internal/store/postgres"
	"github.com/entropic/entropic/internal/store/typesense"
	"github.com/spf13/cobra"
)

var (
	// Build-time variables (set via ldflags)
	version   = "dev"
	commit    = "unknown"
	buildTime = "unknown"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "entropic-server",
		Short: "Entropic Storage Engine Server",
		Long:  "A next-generation storage engine with flexible entity-relationship model and dual-storage architecture.",
		RunE:  runServer,
	}

	// Add flags
	rootCmd.Flags().StringP("config", "c", "", "Path to configuration file")
	rootCmd.Flags().BoolP("version", "v", false, "Show version information")

	// Add version command
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

	// Add migration command
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
	// Check version flag
	if versionFlag, _ := cmd.Flags().GetBool("version"); versionFlag {
		fmt.Printf("Entropic Server %s (commit: %s, built: %s)\n", version, commit, buildTime)
		return nil
	}

	// Load configuration
	configPath, _ := cmd.Flags().GetString("config")
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Setup logging
	setupLogging(cfg.Logging)

	// Log startup information
	logInfo("Starting Entropic Server")
	logInfo("Version: %s, Commit: %s, Build Time: %s", version, commit, buildTime)
	logInfo("Configuration loaded from: %s", getConfigSource(configPath))

	// Initialize application
	app, err := NewApplication(cfg)
	if err != nil {
		return fmt.Errorf("failed to initialize application: %w", err)
	}
	defer app.Close()

	// Create HTTP server
	server := &http.Server{
		Addr:         cfg.GetServerAddress(),
		Handler:      app.Handler(),
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	// Start server in a goroutine
	serverChan := make(chan error, 1)
	go func() {
		logInfo("Server starting on %s", cfg.GetServerAddress())
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverChan <- fmt.Errorf("server failed to start: %w", err)
		}
	}()

	// Wait for interrupt signal or server error
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverChan:
		return err
	case sig := <-quit:
		logInfo("Received signal: %v", sig)
	}

	// Graceful shutdown
	logInfo("Shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logError("Server forced to shutdown: %v", err)
		return err
	}

	logInfo("Server shutdown completed")
	return nil
}

func runMigrations(cmd *cobra.Command, args []string) error {
	// Load configuration
	configPath, _ := cmd.Flags().GetString("config")
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Setup logging
	setupLogging(cfg.Logging)

	logInfo("Running database migrations...")

	// Initialize PostgreSQL connection
	pgStore, err := postgres.NewPostgresStore(cfg.GetDatabaseURL())
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer pgStore.Close()

	// Run migrations
	migrator := postgres.NewMigrator(pgStore.GetPool())
	if err := migrator.Run(context.Background()); err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	logInfo("Database migrations completed successfully")
	return nil
}

// Application holds all components of the Entropic application
type Application struct {
	cfg           *config.Config
	primaryStore  *postgres.PostgresStore
	indexStore    *typesense.TypesenseStore
	cacheManager  *cache.CacheAwareManager
	lockManager   *lock.LockManager
	engine        *core.Engine
	healthChecker *health.HealthChecker
	router        *api.Router
}

// NewApplication creates and initializes a new application instance
func NewApplication(cfg *config.Config) (*Application, error) {
	app := &Application{cfg: cfg}

	// Initialize primary store (PostgreSQL)
	logInfo("Initializing PostgreSQL connection...")
	primaryStore, err := postgres.NewPostgresStore(cfg.GetDatabaseURL())
	if err != nil {
		return nil, fmt.Errorf("failed to initialize PostgreSQL: %w", err)
	}
	app.primaryStore = primaryStore

	// Test primary store connection
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := primaryStore.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping PostgreSQL: %w", err)
	}
	logInfo("PostgreSQL connection established")

	// Run migrations if enabled
	if cfg.Database.MigrateOnStart {
		logInfo("Running database migrations...")
		migrator := postgres.NewMigrator(primaryStore.GetPool())
		if err := migrator.Run(ctx); err != nil {
			return nil, fmt.Errorf("migration failed: %w", err)
		}
		logInfo("Database migrations completed")
	}

	// Initialize index store (Typesense)
	logInfo("Initializing Typesense connection...")
	indexStore, err := typesense.NewTypesenseStore(cfg.Search.URL, cfg.Search.APIKey)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Typesense: %w", err)
	}
	app.indexStore = indexStore

	// Test index store connection
	if err := indexStore.Ping(ctx); err != nil {
		logWarn("Failed to ping Typesense (continuing anyway): %v", err)
	} else {
		logInfo("Typesense connection established")
	}

	// Initialize cache manager
	logInfo("Initializing cache manager...")
	cacheManager := cache.NewCacheAwareManager(primaryStore, cfg.Cache.TTL)
	app.cacheManager = cacheManager

	// Start cache cleanup routine
	cacheManager.StartCleanupRoutine(context.Background(), cfg.Cache.CleanupInterval)
	logInfo("Cache manager initialized")

	// Initialize lock manager
	logInfo("Initializing lock manager...")
	var distributedLock lock.DistributedLock
	switch cfg.Lock.Type {
	case "memory":
		distributedLock = lock.NewInMemoryDistributedLock()
	case "redis":
		// Redis implementation would go here
		logWarn("Redis distributed lock not implemented, falling back to in-memory")
		distributedLock = lock.NewInMemoryDistributedLock()
	default:
		return nil, fmt.Errorf("unsupported lock type: %s", cfg.Lock.Type)
	}

	lockManager := lock.NewLockManager(distributedLock)
	app.lockManager = lockManager
	logInfo("Lock manager initialized")

	// Initialize core engine
	logInfo("Initializing core engine...")
	engine, err := core.NewEngine(primaryStore, indexStore, cacheManager, lockManager)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize core engine: %w", err)
	}
	app.engine = engine
	logInfo("Core engine initialized")

	// Preload schemas into cache
	logInfo("Preloading schemas...")
	if err := cacheManager.PreloadSchemas(ctx); err != nil {
		logWarn("Failed to preload schemas: %v", err)
	} else {
		logInfo("Schemas preloaded successfully")
	}

	// Initialize health checker
	logInfo("Initializing health checker...")
	healthChecker := health.NewHealthChecker(5 * time.Second)
	healthChecker.RegisterComponent("database", health.CreateDatabaseHealthCheck(primaryStore))
	healthChecker.RegisterComponent("search", health.CreateSearchHealthCheck(indexStore))
	healthChecker.RegisterComponent("memory", health.CreateMemoryHealthCheck())
	
	// Start periodic health checks
	healthChecker.StartPeriodicChecks(context.Background(), 30*time.Second)
	app.healthChecker = healthChecker
	logInfo("Health checker initialized")

	// Initialize router
	logInfo("Initializing API router...")
	router := api.NewRouter(engine, healthChecker)
	app.router = router
	logInfo("API router initialized")

	logInfo("Application initialization completed")
	return app, nil
}

// Handler returns the HTTP handler for the application
func (app *Application) Handler() http.Handler {
	return app.router.SetupRoutes()
}

// Close gracefully closes all application components
func (app *Application) Close() error {
	logInfo("Closing application components...")

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
		logError("Some components failed to close properly:")
		for _, err := range errors {
			logError("  %v", err)
		}
		return fmt.Errorf("application close failed with %d errors", len(errors))
	}

	logInfo("Application closed successfully")
	return nil
}

// Logging functions (simple implementation - in production you'd use a proper logging library)

func setupLogging(cfg config.LoggingConfig) {
	// This is a simple implementation
	// In production, you'd set up structured logging with zerolog, logrus, etc.
	logInfo("Logging configured: level=%s, format=%s", cfg.Level, cfg.Format)
}

func logInfo(format string, args ...interface{}) {
	fmt.Printf("[INFO] %s %s\n", time.Now().Format(time.RFC3339), fmt.Sprintf(format, args...))
}

func logWarn(format string, args ...interface{}) {
	fmt.Printf("[WARN] %s %s\n", time.Now().Format(time.RFC3339), fmt.Sprintf(format, args...))
}

func logError(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "[ERROR] %s %s\n", time.Now().Format(time.RFC3339), fmt.Sprintf(format, args...))
}

func getConfigSource(configPath string) string {
	if configPath != "" {
		return configPath
	}
	return "defaults and environment variables"
}

