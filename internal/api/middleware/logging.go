package middleware

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

// Logger returns a logging middleware
func Logger() func(next http.Handler) http.Handler {
	return middleware.RequestLogger(&StructuredLogger{})
}

// StructuredLogger implements the middleware.LogFormatter interface
type StructuredLogger struct{}

// NewLogEntry creates a new log entry for each request
func (l *StructuredLogger) NewLogEntry(r *http.Request) middleware.LogEntry {
	entry := &StructuredLoggerEntry{
		request: r,
	}

	// Log request start
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}

	fmt.Printf("[INFO] %s \"%s %s %s\" from %s - %s\n",
		time.Now().Format(time.RFC3339),
		r.Method,
		fmt.Sprintf("%s://%s%s", scheme, r.Host, r.RequestURI),
		r.Proto,
		r.RemoteAddr,
		r.UserAgent(),
	)

	return entry
}

// StructuredLoggerEntry implements the middleware.LogEntry interface
type StructuredLoggerEntry struct {
	request *http.Request
}

// Write logs the response details
func (l *StructuredLoggerEntry) Write(status, bytes int, header http.Header, elapsed time.Duration, extra interface{}) {
	fmt.Printf("[INFO] %s \"%s %s\" %d %d %v\n",
		time.Now().Format(time.RFC3339),
		l.request.Method,
		l.request.RequestURI,
		status,
		bytes,
		elapsed,
	)
}

// Panic logs panic details
func (l *StructuredLoggerEntry) Panic(v interface{}, stack []byte) {
	fmt.Printf("[ERROR] %s PANIC: %v\nStack:\n%s\n",
		time.Now().Format(time.RFC3339),
		v,
		stack,
	)
}