package observability

import (
	"context"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/rs/zerolog/pkgerrors"
)

// LogLevel represents log levels
type LogLevel string

const (
	LogLevelTrace LogLevel = "trace"
	LogLevelDebug LogLevel = "debug"
	LogLevelInfo  LogLevel = "info"
	LogLevelWarn  LogLevel = "warn"
	LogLevelError LogLevel = "error"
	LogLevelFatal LogLevel = "fatal"
	LogLevelPanic LogLevel = "panic"
)

// LogFormat represents log output formats
type LogFormat string

const (
	LogFormatJSON    LogFormat = "json"
	LogFormatConsole LogFormat = "console"
)

// LoggingConfig holds configuration for structured logging
type LoggingConfig struct {
	Level      LogLevel  `yaml:"level" mapstructure:"level"`
	Format     LogFormat `yaml:"format" mapstructure:"format"`
	Output     string    `yaml:"output" mapstructure:"output"` // "stdout", "stderr", or file path
	TimeFormat string    `yaml:"time_format" mapstructure:"time_format"`
	Sampling   *SamplingConfig `yaml:"sampling" mapstructure:"sampling"`
}

// SamplingConfig configures log sampling to reduce volume
type SamplingConfig struct {
	Enabled  bool `yaml:"enabled" mapstructure:"enabled"`
	Tick     time.Duration `yaml:"tick" mapstructure:"tick"`
	First    int  `yaml:"first" mapstructure:"first"`
	Thereafter int `yaml:"thereafter" mapstructure:"thereafter"`
}

// Logger wraps zerolog with additional functionality
type Logger struct {
	logger zerolog.Logger
	config LoggingConfig
}

// NewLogger creates a new structured logger with the given configuration
func NewLogger(config LoggingConfig) (*Logger, error) {
	// Configure error stack marshaling
	zerolog.ErrorStackMarshaler = pkgerrors.MarshalStack

	// Set global log level
	level, err := parseLogLevel(config.Level)
	if err != nil {
		return nil, err
	}
	zerolog.SetGlobalLevel(level)

	// Configure output
	var output io.Writer
	switch config.Output {
	case "stdout", "":
		output = os.Stdout
	case "stderr":
		output = os.Stderr
	default:
		// File output
		file, err := os.OpenFile(config.Output, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			return nil, err
		}
		output = file
	}

	// Configure logger based on format
	var logger zerolog.Logger
	switch config.Format {
	case LogFormatConsole:
		output = zerolog.ConsoleWriter{
			Out:        output,
			TimeFormat: getTimeFormat(config.TimeFormat),
		}
		logger = zerolog.New(output)
	case LogFormatJSON:
		logger = zerolog.New(output)
	default:
		logger = zerolog.New(output)
	}

	// Add timestamp and caller information
	logger = logger.With().
		Timestamp().
		Caller().
		Str("service", "entropic").
		Logger()

	// Configure sampling if enabled
	if config.Sampling != nil && config.Sampling.Enabled {
		logger = logger.Sample(&zerolog.BasicSampler{
			N: uint32(config.Sampling.Thereafter),
		})
	}

	return &Logger{
		logger: logger,
		config: config,
	}, nil
}

// WithContext adds trace information from context to the logger
func (l *Logger) WithContext(ctx context.Context) *zerolog.Logger {
	logger := l.logger.With()
	
	// Add trace information if available
	if traceInfo := ExtractTraceInfo(ctx); traceInfo != nil {
		for key, value := range traceInfo {
			logger = logger.Interface(key, value)
		}
	}
	
	contextLogger := logger.Logger()
	return &contextLogger
}

// WithEntity adds entity information to the logger
func (l *Logger) WithEntity(entityType, entityID string) *zerolog.Logger {
	logger := l.logger.With().
		Str("entity_type", entityType).
		Str("entity_id", entityID).
		Logger()
	return &logger
}

// WithRelation adds relation information to the logger
func (l *Logger) WithRelation(relationType, relationID string) *zerolog.Logger {
	logger := l.logger.With().
		Str("relation_type", relationType).
		Str("relation_id", relationID).
		Logger()
	return &logger
}

// WithOperation adds operation information to the logger
func (l *Logger) WithOperation(operation string) *zerolog.Logger {
	logger := l.logger.With().
		Str("operation", operation).
		Logger()
	return &logger
}

// WithUser adds user information to the logger
func (l *Logger) WithUser(userID string) *zerolog.Logger {
	logger := l.logger.With().
		Str("user_id", userID).
		Logger()
	return &logger
}

// WithRequest adds HTTP request information to the logger
func (l *Logger) WithRequest(method, path, userAgent, clientIP string) *zerolog.Logger {
	logger := l.logger.With().
		Str("http_method", method).
		Str("http_path", path).
		Str("user_agent", userAgent).
		Str("client_ip", clientIP).
		Logger()
	return &logger
}

// WithDuration adds duration information to the logger
func (l *Logger) WithDuration(duration time.Duration) *zerolog.Logger {
	logger := l.logger.With().
		Dur("duration", duration).
		Logger()
	return &logger
}

// WithError adds error information to the logger with stack trace
func (l *Logger) WithError(err error) *zerolog.Logger {
	logger := l.logger.With().
		Stack().
		Err(err).
		Logger()
	return &logger
}

// Debug logs a debug message
func (l *Logger) Debug(msg string) {
	l.logger.Debug().Msg(msg)
}

// Info logs an info message
func (l *Logger) Info(msg string) {
	l.logger.Info().Msg(msg)
}

// Warn logs a warning message
func (l *Logger) Warn(msg string) {
	l.logger.Warn().Msg(msg)
}

// Error logs an error message
func (l *Logger) Error(msg string) {
	l.logger.Error().Msg(msg)
}

// Fatal logs a fatal message and exits
func (l *Logger) Fatal(msg string) {
	l.logger.Fatal().Msg(msg)
}

// GetZerologLogger returns the underlying zerolog logger
func (l *Logger) GetZerologLogger() zerolog.Logger {
	return l.logger
}

// Helper functions

func parseLogLevel(level LogLevel) (zerolog.Level, error) {
	switch level {
	case LogLevelTrace:
		return zerolog.TraceLevel, nil
	case LogLevelDebug:
		return zerolog.DebugLevel, nil
	case LogLevelInfo:
		return zerolog.InfoLevel, nil
	case LogLevelWarn:
		return zerolog.WarnLevel, nil
	case LogLevelError:
		return zerolog.ErrorLevel, nil
	case LogLevelFatal:
		return zerolog.FatalLevel, nil
	case LogLevelPanic:
		return zerolog.PanicLevel, nil
	default:
		return zerolog.InfoLevel, nil
	}
}

func getTimeFormat(format string) string {
	if format == "" {
		return time.RFC3339
	}
	return format
}

// LoggingMiddleware creates middleware for HTTP request logging
func (l *Logger) LoggingMiddleware() func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			
			// Wrap response writer to capture status code and size
			wrapped := &loggingResponseWriter{ResponseWriter: w}
			
			// Log request start
			logger := l.WithContext(r.Context()).
				WithRequest(r.Method, r.URL.Path, r.UserAgent(), r.RemoteAddr)
			
			logger.Info().Msg("HTTP request started")
			
			// Process request
			next.ServeHTTP(wrapped, r)
			
			// Log request completion
			duration := time.Since(start)
			logEvent := logger.Info().
				Int("status_code", wrapped.statusCode).
				Int64("response_size", wrapped.size).
				Dur("duration", duration)
			
			if wrapped.statusCode >= 400 {
				logEvent = logger.Warn().
					Int("status_code", wrapped.statusCode).
					Int64("response_size", wrapped.size).
					Dur("duration", duration)
			}
			
			if wrapped.statusCode >= 500 {
				logEvent = logger.Error().
					Int("status_code", wrapped.statusCode).
					Int64("response_size", wrapped.size).
					Dur("duration", duration)
			}
			
			logEvent.Msg("HTTP request completed")
		})
	}
}

type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
	size       int64
}

func (lrw *loggingResponseWriter) WriteHeader(statusCode int) {
	lrw.statusCode = statusCode
	lrw.ResponseWriter.WriteHeader(statusCode)
}

func (lrw *loggingResponseWriter) Write(data []byte) (int, error) {
	size, err := lrw.ResponseWriter.Write(data)
	lrw.size += int64(size)
	return size, err
}

// StructuredError represents a structured error for consistent logging
type StructuredError struct {
	Code      string                 `json:"code"`
	Message   string                 `json:"message"`
	Details   map[string]interface{} `json:"details,omitempty"`
	Cause     error                  `json:"cause,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
}

func (se *StructuredError) Error() string {
	return se.Message
}

// NewStructuredError creates a new structured error
func NewStructuredError(code, message string, details map[string]interface{}) *StructuredError {
	return &StructuredError{
		Code:      code,
		Message:   message,
		Details:   details,
		Timestamp: time.Now(),
	}
}

// WithCause adds a cause to the structured error
func (se *StructuredError) WithCause(cause error) *StructuredError {
	se.Cause = cause
	return se
}

// LogStructuredError logs a structured error with appropriate context
func (l *Logger) LogStructuredError(ctx context.Context, err *StructuredError) {
	logger := l.WithContext(ctx)
	
	logEvent := logger.Error().
		Str("error_code", err.Code).
		Interface("error_details", err.Details).
		Time("error_timestamp", err.Timestamp)
	
	if err.Cause != nil {
		logEvent = logEvent.AnErr("cause", err.Cause)
	}
	
	logEvent.Msg(err.Message)
}

// SetGlobalLogger sets the global logger instance
func SetGlobalLogger(logger *Logger) {
	log.Logger = logger.logger
}

// GetGlobalLogger returns a logger from the global context
func GetGlobalLogger() zerolog.Logger {
	return log.Logger
}