package middleware

import (
	"net/http"
	"sync"
	"time"
)

type RateLimiter struct {
	mu       sync.Mutex
	clients  map[string]*TokenBucket
	rate     int           
	period   time.Duration 
	cleanup  time.Duration 
}

type TokenBucket struct {
	tokens     float64
	lastUpdate time.Time
	rate       float64 
	capacity   float64 
}

func NewRateLimiter(rate int, period time.Duration) *RateLimiter {
	rl := &RateLimiter{
		clients: make(map[string]*TokenBucket),
		rate:    rate,
		period:  period,
		cleanup: time.Minute * 5, 
	}

	go rl.cleanupRoutine()

	return rl
}

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

	bucket.tokens += elapsed * bucket.rate
	if bucket.tokens > bucket.capacity {
		bucket.tokens = bucket.capacity
	}

	bucket.lastUpdate = now

	if bucket.tokens >= 1.0 {
		bucket.tokens -= 1.0
		return true
	}

	return false
}

func (rl *RateLimiter) cleanupRoutine() {
	ticker := time.NewTicker(rl.cleanup)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for clientID, bucket := range rl.clients {
			
			if now.Sub(bucket.lastUpdate) > rl.cleanup*2 {
				delete(rl.clients, clientID)
			}
		}
		rl.mu.Unlock()
	}
}

func RateLimit(rate int, period time.Duration) func(next http.Handler) http.Handler {
	limiter := NewRateLimiter(rate, period)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			
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

func getClientIP(r *http.Request) string {
	
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		
		if idx := len(xff); idx > 0 {
			for i, char := range xff {
				if char == ',' || char == ' ' {
					return xff[:i]
				}
			}
			return xff
		}
	}

	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	return r.RemoteAddr
}