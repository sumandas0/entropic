package middleware

import (
	"net/http"
	"sync"
	"time"
)

// RateLimiter implements a simple token bucket rate limiter
type RateLimiter struct {
	mu       sync.Mutex
	clients  map[string]*TokenBucket
	rate     int           // requests per period
	period   time.Duration // time period
	cleanup  time.Duration // cleanup interval
}

// TokenBucket represents a token bucket for rate limiting
type TokenBucket struct {
	tokens     float64
	lastUpdate time.Time
	rate       float64 // tokens per second
	capacity   float64 // maximum tokens
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(rate int, period time.Duration) *RateLimiter {
	rl := &RateLimiter{
		clients: make(map[string]*TokenBucket),
		rate:    rate,
		period:  period,
		cleanup: time.Minute * 5, // cleanup every 5 minutes
	}

	// Start cleanup routine
	go rl.cleanupRoutine()

	return rl
}

// Allow checks if a request should be allowed
func (rl *RateLimiter) Allow(clientID string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	bucket, exists := rl.clients[clientID]
	if !exists {
		bucket = &TokenBucket{
			tokens:     float64(rl.rate),
			lastUpdate: time.Now(),
			rate:       float64(rl.rate) / rl.period.Seconds(),
			capacity:   float64(rl.rate),
		}
		rl.clients[clientID] = bucket
	}

	now := time.Now()
	elapsed := now.Sub(bucket.lastUpdate).Seconds()

	// Add tokens based on elapsed time
	bucket.tokens += elapsed * bucket.rate
	if bucket.tokens > bucket.capacity {
		bucket.tokens = bucket.capacity
	}

	bucket.lastUpdate = now

	// Check if we can consume a token
	if bucket.tokens >= 1.0 {
		bucket.tokens -= 1.0
		return true
	}

	return false
}

// cleanupRoutine removes inactive clients
func (rl *RateLimiter) cleanupRoutine() {
	ticker := time.NewTicker(rl.cleanup)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for clientID, bucket := range rl.clients {
			// Remove clients that haven't been active for 2x cleanup interval
			if now.Sub(bucket.lastUpdate) > rl.cleanup*2 {
				delete(rl.clients, clientID)
			}
		}
		rl.mu.Unlock()
	}
}

// RateLimit returns a rate limiting middleware
func RateLimit(rate int, period time.Duration) func(next http.Handler) http.Handler {
	limiter := NewRateLimiter(rate, period)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Use client IP as identifier
			clientID := getClientIP(r)

			if !limiter.Allow(clientID) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				w.Write([]byte(`{"error":{"code":"RATE_LIMIT_EXCEEDED","message":"Rate limit exceeded"}}`))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// getClientIP extracts the client IP address from the request
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header first
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP in the list
		if idx := len(xff); idx > 0 {
			for i, char := range xff {
				if char == ',' || char == ' ' {
					return xff[:i]
				}
			}
			return xff
		}
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	return r.RemoteAddr
}