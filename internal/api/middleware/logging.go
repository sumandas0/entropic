package middleware

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"
	"github.com/sumandas0/entropic/internal/integration"
)

func Logger() func(next http.Handler) http.Handler {
	return middleware.RequestLogger(&StructuredLogger{logger: zerolog.Nop()})
}

func LoggerWithObservability(obsManager *integration.ObservabilityManager) func(next http.Handler) http.Handler {
	logger := zerolog.Nop()
	if obsManager != nil {
		logger = obsManager.GetLogging().GetZerologLogger()
	}
	return middleware.RequestLogger(&StructuredLogger{logger: logger})
}

type StructuredLogger struct{
	logger zerolog.Logger
}

func (l *StructuredLogger) NewLogEntry(r *http.Request) middleware.LogEntry {
	entry := &StructuredLoggerEntry{
		request: r,
		logger: l.logger,
	}

	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}

	if l.logger.GetLevel() != zerolog.Disabled {
		l.logger.Info().
			Str("method", r.Method).
			Str("url", fmt.Sprintf("%s://%s%s", scheme, r.Host, r.RequestURI)).
			Str("proto", r.Proto).
			Str("remote_addr", r.RemoteAddr).
			Str("user_agent", r.UserAgent()).
			Msg("Request started")
	}

	return entry
}

type StructuredLoggerEntry struct {
	request *http.Request
	logger  zerolog.Logger
}

func (l *StructuredLoggerEntry) Write(status, bytes int, header http.Header, elapsed time.Duration, extra any) {
	if l.logger.GetLevel() != zerolog.Disabled {
		l.logger.Info().
			Str("method", l.request.Method).
			Str("path", l.request.RequestURI).
			Int("status", status).
			Int("bytes", bytes).
			Dur("duration", elapsed).
			Msg("Request completed")
	}
}

func (l *StructuredLoggerEntry) Panic(v any, stack []byte) {
	if l.logger.GetLevel() != zerolog.Disabled {
		l.logger.Error().
			Interface("error", v).
			Str("stack", string(stack)).
			Msg("PANIC recovered")
	}
}
