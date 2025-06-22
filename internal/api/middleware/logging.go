package middleware

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

func Logger() func(next http.Handler) http.Handler {
	return middleware.RequestLogger(&StructuredLogger{})
}

type StructuredLogger struct{}

func (l *StructuredLogger) NewLogEntry(r *http.Request) middleware.LogEntry {
	entry := &StructuredLoggerEntry{
		request: r,
	}

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

type StructuredLoggerEntry struct {
	request *http.Request
}

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

func (l *StructuredLoggerEntry) Panic(v interface{}, stack []byte) {
	fmt.Printf("[ERROR] %s PANIC: %v\nStack:\n%s\n",
		time.Now().Format(time.RFC3339),
		v,
		stack,
	)
}