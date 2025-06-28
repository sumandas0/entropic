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

type LogFormat string

const (
	LogFormatJSON    LogFormat = "json"
	LogFormatConsole LogFormat = "console"
)

type LoggingConfig struct {
	Level      LogLevel        `yaml:"level" mapstructure:"level"`
	Format     LogFormat       `yaml:"format" mapstructure:"format"`
	Output     string          `yaml:"output" mapstructure:"output"`
	TimeFormat string          `yaml:"time_format" mapstructure:"time_format"`
	Sampling   *SamplingConfig `yaml:"sampling" mapstructure:"sampling"`
}

type SamplingConfig struct {
	Enabled    bool          `yaml:"enabled" mapstructure:"enabled"`
	Tick       time.Duration `yaml:"tick" mapstructure:"tick"`
	First      int           `yaml:"first" mapstructure:"first"`
	Thereafter int           `yaml:"thereafter" mapstructure:"thereafter"`
}

type Logger struct {
	logger zerolog.Logger
	config LoggingConfig
}

func NewLogger(config LoggingConfig) (*Logger, error) {

	zerolog.ErrorStackMarshaler = pkgerrors.MarshalStack

	level, err := parseLogLevel(config.Level)
	if err != nil {
		return nil, err
	}
	zerolog.SetGlobalLevel(level)

	var output io.Writer
	switch config.Output {
	case "stdout", "":
		output = os.Stdout
	case "stderr":
		output = os.Stderr
	default:

		file, err := os.OpenFile(config.Output, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			return nil, err
		}
		output = file
	}

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

	logger = logger.With().
		Timestamp().
		Caller().
		Str("service", "entropic").
		Logger()

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

func (l *Logger) WithContext(ctx context.Context) *zerolog.Logger {
	logger := l.logger.With()

	if traceInfo := ExtractTraceInfo(ctx); traceInfo != nil {
		for key, value := range traceInfo {
			logger = logger.Interface(key, value)
		}
	}

	contextLogger := logger.Logger()
	return &contextLogger
}

func (l *Logger) WithEntity(entityType, entityID string) *zerolog.Logger {
	logger := l.logger.With().
		Str("entity_type", entityType).
		Str("entity_id", entityID).
		Logger()
	return &logger
}

func (l *Logger) WithRelation(relationType, relationID string) *zerolog.Logger {
	logger := l.logger.With().
		Str("relation_type", relationType).
		Str("relation_id", relationID).
		Logger()
	return &logger
}

func (l *Logger) WithOperation(operation string) *zerolog.Logger {
	logger := l.logger.With().
		Str("operation", operation).
		Logger()
	return &logger
}

func (l *Logger) WithUser(userID string) *zerolog.Logger {
	logger := l.logger.With().
		Str("user_id", userID).
		Logger()
	return &logger
}

func (l *Logger) WithRequest(method, path, userAgent, clientIP string) *zerolog.Logger {
	logger := l.logger.With().
		Str("http_method", method).
		Str("http_path", path).
		Str("user_agent", userAgent).
		Str("client_ip", clientIP).
		Logger()
	return &logger
}

func (l *Logger) WithDuration(duration time.Duration) *zerolog.Logger {
	logger := l.logger.With().
		Dur("duration", duration).
		Logger()
	return &logger
}

func (l *Logger) WithError(err error) *zerolog.Logger {
	logger := l.logger.With().
		Stack().
		Err(err).
		Logger()
	return &logger
}

func (l *Logger) Debug(msg string) {
	l.logger.Debug().Msg(msg)
}

func (l *Logger) Info(msg string) {
	l.logger.Info().Msg(msg)
}

func (l *Logger) Warn(msg string) {
	l.logger.Warn().Msg(msg)
}

func (l *Logger) Error(msg string) {
	l.logger.Error().Msg(msg)
}

func (l *Logger) Fatal(msg string) {
	l.logger.Fatal().Msg(msg)
}

func (l *Logger) GetZerologLogger() zerolog.Logger {
	return l.logger
}

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

func (l *Logger) LoggingMiddleware() func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			wrapped := &loggingResponseWriter{ResponseWriter: w}

			logger := l.WithContext(r.Context()).
				With().
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Str("user_agent", r.UserAgent()).
				Str("remote_addr", r.RemoteAddr).
				Logger()

			logger.Info().Msg("HTTP request started")

			next.ServeHTTP(wrapped, r)

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

type StructuredError struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Details   map[string]any `json:"details,omitempty"`
	Cause     error          `json:"cause,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
}

func (se *StructuredError) Error() string {
	return se.Message
}

func NewStructuredError(code, message string, details map[string]any) *StructuredError {
	return &StructuredError{
		Code:      code,
		Message:   message,
		Details:   details,
		Timestamp: time.Now(),
	}
}

func (se *StructuredError) WithCause(cause error) *StructuredError {
	se.Cause = cause
	return se
}

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

func SetGlobalLogger(logger *Logger) {
	log.Logger = logger.logger
}

func GetGlobalLogger() zerolog.Logger {
	return log.Logger
}
