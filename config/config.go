package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config holds all configuration for the Entropic application
type Config struct {
	Server     ServerConfig     `mapstructure:"server"`
	Database   DatabaseConfig   `mapstructure:"database"`
	Search     SearchConfig     `mapstructure:"search"`
	Cache      CacheConfig      `mapstructure:"cache"`
	Lock       LockConfig       `mapstructure:"lock"`
	Logging    LoggingConfig    `mapstructure:"logging"`
	Metrics    MetricsConfig    `mapstructure:"metrics"`
	Security   SecurityConfig   `mapstructure:"security"`
}

// ServerConfig holds server configuration
type ServerConfig struct {
	Host            string        `mapstructure:"host"`
	Port            int           `mapstructure:"port"`
	ReadTimeout     time.Duration `mapstructure:"read_timeout"`
	WriteTimeout    time.Duration `mapstructure:"write_timeout"`
	IdleTimeout     time.Duration `mapstructure:"idle_timeout"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`
	CORS            CORSConfig    `mapstructure:"cors"`
}

// CORSConfig holds CORS configuration
type CORSConfig struct {
	AllowedOrigins   []string `mapstructure:"allowed_origins"`
	AllowedMethods   []string `mapstructure:"allowed_methods"`
	AllowedHeaders   []string `mapstructure:"allowed_headers"`
	ExposedHeaders   []string `mapstructure:"exposed_headers"`
	AllowCredentials bool     `mapstructure:"allow_credentials"`
	MaxAge           int      `mapstructure:"max_age"`
}

// DatabaseConfig holds database configuration
type DatabaseConfig struct {
	Host            string        `mapstructure:"host"`
	Port            int           `mapstructure:"port"`
	Database        string        `mapstructure:"database"`
	Username        string        `mapstructure:"username"`
	Password        string        `mapstructure:"password"`
	SSLMode         string        `mapstructure:"ssl_mode"`
	MaxOpenConns    int           `mapstructure:"max_open_conns"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
	ConnMaxIdleTime time.Duration `mapstructure:"conn_max_idle_time"`
	MigrateOnStart  bool          `mapstructure:"migrate_on_start"`
}

// SearchConfig holds search engine configuration
type SearchConfig struct {
	Type    string           `mapstructure:"type"` // "typesense"
	URL     string           `mapstructure:"url"`
	APIKey  string           `mapstructure:"api_key"`
	Timeout time.Duration    `mapstructure:"timeout"`
	Retry   RetryConfig      `mapstructure:"retry"`
	Batch   BatchConfig      `mapstructure:"batch"`
}

// RetryConfig holds retry configuration
type RetryConfig struct {
	MaxRetries    int           `mapstructure:"max_retries"`
	InitialDelay  time.Duration `mapstructure:"initial_delay"`
	MaxDelay      time.Duration `mapstructure:"max_delay"`
	Multiplier    float64       `mapstructure:"multiplier"`
}

// BatchConfig holds batch processing configuration
type BatchConfig struct {
	Size          int           `mapstructure:"size"`
	FlushInterval time.Duration `mapstructure:"flush_interval"`
	Concurrency   int           `mapstructure:"concurrency"`
}

// CacheConfig holds cache configuration
type CacheConfig struct {
	TTL             time.Duration `mapstructure:"ttl"`
	CleanupInterval time.Duration `mapstructure:"cleanup_interval"`
}

// LockConfig holds lock configuration
type LockConfig struct {
	Type           string        `mapstructure:"type"` // "memory", "redis"
	DefaultTimeout time.Duration `mapstructure:"default_timeout"`
	MaxWaitTime    time.Duration `mapstructure:"max_wait_time"`
	Redis          RedisConfig   `mapstructure:"redis"`
}

// RedisConfig holds Redis configuration for distributed locking
type RedisConfig struct {
	Host     string        `mapstructure:"host"`
	Port     int           `mapstructure:"port"`
	Password string        `mapstructure:"password"`
	DB       int           `mapstructure:"db"`
	Timeout  time.Duration `mapstructure:"timeout"`
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level  string `mapstructure:"level"`  // "debug", "info", "warn", "error"
	Format string `mapstructure:"format"` // "json", "console"
	File   string `mapstructure:"file"`   // empty for stdout
}

// MetricsConfig holds metrics configuration
type MetricsConfig struct {
	Enabled  bool   `mapstructure:"enabled"`
	Path     string `mapstructure:"path"`
	Interval time.Duration `mapstructure:"interval"`
}

// SecurityConfig holds security configuration
type SecurityConfig struct {
	RateLimit   RateLimitConfig `mapstructure:"rate_limit"`
	RequestSize RequestSizeConfig `mapstructure:"request_size"`
}

// RateLimitConfig holds rate limiting configuration
type RateLimitConfig struct {
	Enabled    bool          `mapstructure:"enabled"`
	Rate       int           `mapstructure:"rate"`        // requests per period
	Period     time.Duration `mapstructure:"period"`      // time period
	BurstSize  int           `mapstructure:"burst_size"`  // burst allowance
}

// RequestSizeConfig holds request size limits
type RequestSizeConfig struct {
	MaxBodySize    int64 `mapstructure:"max_body_size"`    // in bytes
	MaxHeaderSize  int   `mapstructure:"max_header_size"`  // in bytes
}

// LoadConfig loads configuration from various sources
func LoadConfig(configPath string) (*Config, error) {
	// Set default configuration
	setDefaults()
	
	// Set config file path if provided
	if configPath != "" {
		viper.SetConfigFile(configPath)
	} else {
		// Look for config in current directory and /etc/entropic/
		viper.SetConfigName("entropic")
		viper.SetConfigType("yaml")
		viper.AddConfigPath(".")
		viper.AddConfigPath("/etc/entropic/")
		viper.AddConfigPath("$HOME/.entropic/")
	}
	
	// Environment variables
	viper.SetEnvPrefix("ENTROPIC")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()
	
	// Read config file
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
		// Config file not found, use defaults and environment variables
	}
	
	// Unmarshal into struct
	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}
	
	// Validate configuration
	if err := validateConfig(&config); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}
	
	return &config, nil
}

// setDefaults sets default configuration values
func setDefaults() {
	// Server defaults
	viper.SetDefault("server.host", "0.0.0.0")
	viper.SetDefault("server.port", 8080)
	viper.SetDefault("server.read_timeout", "30s")
	viper.SetDefault("server.write_timeout", "30s")
	viper.SetDefault("server.idle_timeout", "120s")
	viper.SetDefault("server.shutdown_timeout", "30s")
	
	// CORS defaults
	viper.SetDefault("server.cors.allowed_origins", []string{"*"})
	viper.SetDefault("server.cors.allowed_methods", []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"})
	viper.SetDefault("server.cors.allowed_headers", []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"})
	viper.SetDefault("server.cors.exposed_headers", []string{"Link"})
	viper.SetDefault("server.cors.allow_credentials", false)
	viper.SetDefault("server.cors.max_age", 300)
	
	// Database defaults
	viper.SetDefault("database.host", "localhost")
	viper.SetDefault("database.port", 5432)
	viper.SetDefault("database.database", "entropic")
	viper.SetDefault("database.username", "postgres")
	viper.SetDefault("database.password", "postgres")
	viper.SetDefault("database.ssl_mode", "disable")
	viper.SetDefault("database.max_open_conns", 25)
	viper.SetDefault("database.max_idle_conns", 5)
	viper.SetDefault("database.conn_max_lifetime", "1h")
	viper.SetDefault("database.conn_max_idle_time", "30m")
	viper.SetDefault("database.migrate_on_start", true)
	
	// Search defaults
	viper.SetDefault("search.type", "typesense")
	viper.SetDefault("search.url", "http://localhost:8108")
	viper.SetDefault("search.api_key", "xyz")
	viper.SetDefault("search.timeout", "30s")
	viper.SetDefault("search.retry.max_retries", 3)
	viper.SetDefault("search.retry.initial_delay", "100ms")
	viper.SetDefault("search.retry.max_delay", "5s")
	viper.SetDefault("search.retry.multiplier", 2.0)
	viper.SetDefault("search.batch.size", 100)
	viper.SetDefault("search.batch.flush_interval", "5s")
	viper.SetDefault("search.batch.concurrency", 4)
	
	// Cache defaults
	viper.SetDefault("cache.ttl", "5m")
	viper.SetDefault("cache.cleanup_interval", "1m")
	
	// Lock defaults
	viper.SetDefault("lock.type", "memory")
	viper.SetDefault("lock.default_timeout", "30s")
	viper.SetDefault("lock.max_wait_time", "5m")
	viper.SetDefault("lock.redis.host", "localhost")
	viper.SetDefault("lock.redis.port", 6379)
	viper.SetDefault("lock.redis.password", "")
	viper.SetDefault("lock.redis.db", 0)
	viper.SetDefault("lock.redis.timeout", "5s")
	
	// Logging defaults
	viper.SetDefault("logging.level", "info")
	viper.SetDefault("logging.format", "console")
	viper.SetDefault("logging.file", "")
	
	// Metrics defaults
	viper.SetDefault("metrics.enabled", true)
	viper.SetDefault("metrics.path", "/metrics")
	viper.SetDefault("metrics.interval", "15s")
	
	// Security defaults
	viper.SetDefault("security.rate_limit.enabled", true)
	viper.SetDefault("security.rate_limit.rate", 100)
	viper.SetDefault("security.rate_limit.period", "1m")
	viper.SetDefault("security.rate_limit.burst_size", 10)
	viper.SetDefault("security.request_size.max_body_size", 10485760) // 10MB
	viper.SetDefault("security.request_size.max_header_size", 8192)   // 8KB
}

// validateConfig validates the configuration
func validateConfig(config *Config) error {
	// Validate server configuration
	if config.Server.Port <= 0 || config.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", config.Server.Port)
	}
	
	// Validate database configuration
	if config.Database.Host == "" {
		return fmt.Errorf("database host is required")
	}
	if config.Database.Database == "" {
		return fmt.Errorf("database name is required")
	}
	if config.Database.Port <= 0 || config.Database.Port > 65535 {
		return fmt.Errorf("invalid database port: %d", config.Database.Port)
	}
	
	// Validate search configuration
	if config.Search.URL == "" {
		return fmt.Errorf("search URL is required")
	}
	if config.Search.APIKey == "" {
		return fmt.Errorf("search API key is required")
	}
	
	// Validate logging level
	validLevels := []string{"debug", "info", "warn", "error"}
	isValidLevel := false
	for _, level := range validLevels {
		if config.Logging.Level == level {
			isValidLevel = true
			break
		}
	}
	if !isValidLevel {
		return fmt.Errorf("invalid logging level: %s", config.Logging.Level)
	}
	
	// Validate logging format
	if config.Logging.Format != "json" && config.Logging.Format != "console" {
		return fmt.Errorf("invalid logging format: %s", config.Logging.Format)
	}
	
	// Validate lock type
	if config.Lock.Type != "memory" && config.Lock.Type != "redis" {
		return fmt.Errorf("invalid lock type: %s", config.Lock.Type)
	}
	
	return nil
}

// GetDatabaseURL returns the database connection URL
func (c *Config) GetDatabaseURL() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		c.Database.Username,
		c.Database.Password,
		c.Database.Host,
		c.Database.Port,
		c.Database.Database,
		c.Database.SSLMode,
	)
}

// GetRedisURL returns the Redis connection URL
func (c *Config) GetRedisURL() string {
	if c.Lock.Redis.Password != "" {
		return fmt.Sprintf("redis://:%s@%s:%d/%d",
			c.Lock.Redis.Password,
			c.Lock.Redis.Host,
			c.Lock.Redis.Port,
			c.Lock.Redis.DB,
		)
	}
	return fmt.Sprintf("redis://%s:%d/%d",
		c.Lock.Redis.Host,
		c.Lock.Redis.Port,
		c.Lock.Redis.DB,
	)
}

// GetServerAddress returns the server address
func (c *Config) GetServerAddress() string {
	return fmt.Sprintf("%s:%d", c.Server.Host, c.Server.Port)
}