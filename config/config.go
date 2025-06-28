package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Server      ServerConfig     `mapstructure:"server"`
	Database    DatabaseConfig   `mapstructure:"database"`
	Search      SearchConfig     `mapstructure:"search"`
	Cache       CacheConfig      `mapstructure:"cache"`
	Lock        LockConfig       `mapstructure:"lock"`
	Logging     LoggingConfig    `mapstructure:"logging"`
	Metrics     MetricsConfig    `mapstructure:"metrics"`
	Tracing     TracingConfig    `mapstructure:"tracing"`
	Security    SecurityConfig   `mapstructure:"security"`
	Environment string           `mapstructure:"environment"`
}

type ServerConfig struct {
	Host            string        `mapstructure:"host"`
	Port            int           `mapstructure:"port"`
	ReadTimeout     time.Duration `mapstructure:"read_timeout"`
	WriteTimeout    time.Duration `mapstructure:"write_timeout"`
	IdleTimeout     time.Duration `mapstructure:"idle_timeout"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`
	CORS            CORSConfig    `mapstructure:"cors"`
}

type CORSConfig struct {
	AllowedOrigins   []string `mapstructure:"allowed_origins"`
	AllowedMethods   []string `mapstructure:"allowed_methods"`
	AllowedHeaders   []string `mapstructure:"allowed_headers"`
	ExposedHeaders   []string `mapstructure:"exposed_headers"`
	AllowCredentials bool     `mapstructure:"allow_credentials"`
	MaxAge           int      `mapstructure:"max_age"`
}

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

type SearchConfig struct {
	Type    string           `mapstructure:"type"` 
	URL     string           `mapstructure:"url"`
	APIKey  string           `mapstructure:"api_key"`
	Timeout time.Duration    `mapstructure:"timeout"`
	Retry   RetryConfig      `mapstructure:"retry"`
	Batch   BatchConfig      `mapstructure:"batch"`
}

type RetryConfig struct {
	MaxRetries    int           `mapstructure:"max_retries"`
	InitialDelay  time.Duration `mapstructure:"initial_delay"`
	MaxDelay      time.Duration `mapstructure:"max_delay"`
	Multiplier    float64       `mapstructure:"multiplier"`
}

type BatchConfig struct {
	Size          int           `mapstructure:"size"`
	FlushInterval time.Duration `mapstructure:"flush_interval"`
	Concurrency   int           `mapstructure:"concurrency"`
}

type CacheConfig struct {
	TTL             time.Duration `mapstructure:"ttl"`
	CleanupInterval time.Duration `mapstructure:"cleanup_interval"`
}

type LockConfig struct {
	Type           string        `mapstructure:"type"` 
	DefaultTimeout time.Duration `mapstructure:"default_timeout"`
	MaxWaitTime    time.Duration `mapstructure:"max_wait_time"`
	Redis          RedisConfig   `mapstructure:"redis"`
}

type RedisConfig struct {
	Host     string        `mapstructure:"host"`
	Port     int           `mapstructure:"port"`
	Password string        `mapstructure:"password"`
	DB       int           `mapstructure:"db"`
	Timeout  time.Duration `mapstructure:"timeout"`
}

type LoggingConfig struct {
	Level  string `mapstructure:"level"`  
	Format string `mapstructure:"format"` 
	File   string `mapstructure:"file"`
	Output string `mapstructure:"output"`   
}

type MetricsConfig struct {
	Enabled          bool          `mapstructure:"enabled"`
	Path             string        `mapstructure:"path"`
	Interval         time.Duration `mapstructure:"interval"`
	PrometheusPort   int           `mapstructure:"prometheus_port"`
	CollectionPeriod time.Duration `mapstructure:"collection_period"`
	HistogramBuckets []float64     `mapstructure:"histogram_buckets"`
}

type SecurityConfig struct {
	RateLimit   RateLimitConfig `mapstructure:"rate_limit"`
	RequestSize RequestSizeConfig `mapstructure:"request_size"`
}

type RateLimitConfig struct {
	Enabled    bool          `mapstructure:"enabled"`
	Rate       int           `mapstructure:"rate"`        
	Period     time.Duration `mapstructure:"period"`      
	BurstSize  int           `mapstructure:"burst_size"`  
}

type RequestSizeConfig struct {
	MaxBodySize    int64 `mapstructure:"max_body_size"`    
	MaxHeaderSize  int   `mapstructure:"max_header_size"`  
}

type TracingConfig struct {
	Enabled     bool    `mapstructure:"enabled"`
	JaegerURL   string  `mapstructure:"jaeger_url"`
	SampleRate  float64 `mapstructure:"sample_rate"`
}

func LoadConfig(configPath string) (*Config, error) {
	
	setDefaults()

	if configPath != "" {
		viper.SetConfigFile(configPath)
	} else {
		
		viper.SetConfigName("entropic")
		viper.SetConfigType("yaml")
		viper.AddConfigPath(".")
		viper.AddConfigPath("/etc/entropic/")
		viper.AddConfigPath("$HOME/.entropic/")
	}

	viper.SetEnvPrefix("ENTROPIC")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
		
	}

	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	if err := validateConfig(&config); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}
	
	return &config, nil
}

func setDefaults() {
	
	viper.SetDefault("server.host", "0.0.0.0")
	viper.SetDefault("server.port", 8080)
	viper.SetDefault("server.read_timeout", "30s")
	viper.SetDefault("server.write_timeout", "30s")
	viper.SetDefault("server.idle_timeout", "120s")
	viper.SetDefault("server.shutdown_timeout", "30s")

	viper.SetDefault("server.cors.allowed_origins", []string{"*"})
	viper.SetDefault("server.cors.allowed_methods", []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"})
	viper.SetDefault("server.cors.allowed_headers", []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"})
	viper.SetDefault("server.cors.exposed_headers", []string{"Link"})
	viper.SetDefault("server.cors.allow_credentials", false)
	viper.SetDefault("server.cors.max_age", 300)

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

	viper.SetDefault("cache.ttl", "5m")
	viper.SetDefault("cache.cleanup_interval", "1m")

	viper.SetDefault("lock.type", "memory")
	viper.SetDefault("lock.default_timeout", "30s")
	viper.SetDefault("lock.max_wait_time", "5m")
	viper.SetDefault("lock.redis.host", "localhost")
	viper.SetDefault("lock.redis.port", 6379)
	viper.SetDefault("lock.redis.password", "")
	viper.SetDefault("lock.redis.db", 0)
	viper.SetDefault("lock.redis.timeout", "5s")

	viper.SetDefault("logging.level", "info")
	viper.SetDefault("logging.format", "console")
	viper.SetDefault("logging.file", "")
	viper.SetDefault("logging.output", "stdout")

	viper.SetDefault("metrics.enabled", true)
	viper.SetDefault("metrics.path", "/metrics")
	viper.SetDefault("metrics.interval", "15s")
	viper.SetDefault("metrics.prometheus_port", 9090)
	viper.SetDefault("metrics.collection_period", "10s")
	viper.SetDefault("metrics.histogram_buckets", []float64{0.1, 0.5, 1, 2, 5, 10})

	viper.SetDefault("security.rate_limit.enabled", true)
	viper.SetDefault("security.rate_limit.rate", 100)
	viper.SetDefault("security.rate_limit.period", "1m")
	viper.SetDefault("security.rate_limit.burst_size", 10)
	viper.SetDefault("security.request_size.max_body_size", 10485760) 
	viper.SetDefault("security.request_size.max_header_size", 8192)

	viper.SetDefault("tracing.enabled", false)
	viper.SetDefault("tracing.jaeger_url", "http://localhost:14268/api/traces")
	viper.SetDefault("tracing.sample_rate", 1.0)

	viper.SetDefault("environment", "development")   
}

func validateConfig(config *Config) error {
	
	if config.Server.Port <= 0 || config.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", config.Server.Port)
	}

	if config.Database.Host == "" {
		return fmt.Errorf("database host is required")
	}
	if config.Database.Database == "" {
		return fmt.Errorf("database name is required")
	}
	if config.Database.Port <= 0 || config.Database.Port > 65535 {
		return fmt.Errorf("invalid database port: %d", config.Database.Port)
	}

	if config.Search.URL == "" {
		return fmt.Errorf("search URL is required")
	}
	if config.Search.APIKey == "" {
		return fmt.Errorf("search API key is required")
	}

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

	if config.Logging.Format != "json" && config.Logging.Format != "console" {
		return fmt.Errorf("invalid logging format: %s", config.Logging.Format)
	}

	if config.Lock.Type != "memory" && config.Lock.Type != "redis" {
		return fmt.Errorf("invalid lock type: %s", config.Lock.Type)
	}
	
	return nil
}

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

func (c *Config) GetServerAddress() string {
	return fmt.Sprintf("%s:%d", c.Server.Host, c.Server.Port)
}