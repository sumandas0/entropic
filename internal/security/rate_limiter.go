package security

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// RateLimitConfig holds configuration for rate limiting
type RateLimitConfig struct {
	Enabled           bool          `yaml:"enabled" mapstructure:"enabled"`
	RequestsPerSecond float64       `yaml:"requests_per_second" mapstructure:"requests_per_second"`
	BurstSize         int           `yaml:"burst_size" mapstructure:"burst_size"`
	CleanupInterval   time.Duration `yaml:"cleanup_interval" mapstructure:"cleanup_interval"`

	// Different limits for different endpoints
	EndpointLimits map[string]EndpointLimit `yaml:"endpoint_limits" mapstructure:"endpoint_limits"`

	// IP-based rate limiting
	IPLimitEnabled      bool    `yaml:"ip_limit_enabled" mapstructure:"ip_limit_enabled"`
	IPRequestsPerSecond float64 `yaml:"ip_requests_per_second" mapstructure:"ip_requests_per_second"`
	IPBurstSize         int     `yaml:"ip_burst_size" mapstructure:"ip_burst_size"`

	// User-based rate limiting
	UserLimitEnabled      bool    `yaml:"user_limit_enabled" mapstructure:"user_limit_enabled"`
	UserRequestsPerSecond float64 `yaml:"user_requests_per_second" mapstructure:"user_requests_per_second"`
	UserBurstSize         int     `yaml:"user_burst_size" mapstructure:"user_burst_size"`
}

// EndpointLimit defines rate limits for specific endpoints
type EndpointLimit struct {
	RequestsPerSecond float64 `yaml:"requests_per_second" mapstructure:"requests_per_second"`
	BurstSize         int     `yaml:"burst_size" mapstructure:"burst_size"`
}

// RateLimiter manages rate limiting for different scopes
type RateLimiter struct {
	config        RateLimitConfig
	globalLimiter *rate.Limiter
	ipLimiters    map[string]*rateLimiterEntry
	userLimiters  map[string]*rateLimiterEntry
	mutex         sync.RWMutex
	stopCleanup   chan struct{}
}

type rateLimiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// NewRateLimiter creates a new rate limiter with the given configuration
func NewRateLimiter(config RateLimitConfig) *RateLimiter {
	rl := &RateLimiter{
		config:       config,
		ipLimiters:   make(map[string]*rateLimiterEntry),
		userLimiters: make(map[string]*rateLimiterEntry),
		stopCleanup:  make(chan struct{}),
	}

	if config.Enabled {
		// Create global rate limiter
		rl.globalLimiter = rate.NewLimiter(
			rate.Limit(config.RequestsPerSecond),
			config.BurstSize,
		)

		// Start cleanup goroutine
		go rl.cleanupRoutine()
	}

	return rl
}

// Allow checks if a request should be allowed based on global rate limits
func (rl *RateLimiter) Allow() bool {
	if !rl.config.Enabled || rl.globalLimiter == nil {
		return true
	}
	return rl.globalLimiter.Allow()
}

// AllowIP checks if a request from a specific IP should be allowed
func (rl *RateLimiter) AllowIP(ip string) bool {
	if !rl.config.Enabled || !rl.config.IPLimitEnabled {
		return true
	}

	limiter := rl.getIPLimiter(ip)
	return limiter.Allow()
}

// AllowUser checks if a request from a specific user should be allowed
func (rl *RateLimiter) AllowUser(userID string) bool {
	if !rl.config.Enabled || !rl.config.UserLimitEnabled {
		return true
	}

	limiter := rl.getUserLimiter(userID)
	return limiter.Allow()
}

// AllowEndpoint checks if a request to a specific endpoint should be allowed
func (rl *RateLimiter) AllowEndpoint(endpoint string) bool {
	if !rl.config.Enabled {
		return true
	}

	if endpointLimit, exists := rl.config.EndpointLimits[endpoint]; exists {
		limiter := rl.getEndpointLimiter(endpointLimit)
		return limiter.Allow()
	}

	return rl.Allow()
}

// getIPLimiter returns a rate limiter for the given IP address
func (rl *RateLimiter) getIPLimiter(ip string) *rate.Limiter {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	if entry, exists := rl.ipLimiters[ip]; exists {
		entry.lastSeen = time.Now()
		return entry.limiter
	}

	limiter := rate.NewLimiter(
		rate.Limit(rl.config.IPRequestsPerSecond),
		rl.config.IPBurstSize,
	)

	rl.ipLimiters[ip] = &rateLimiterEntry{
		limiter:  limiter,
		lastSeen: time.Now(),
	}

	return limiter
}

// getUserLimiter returns a rate limiter for the given user ID
func (rl *RateLimiter) getUserLimiter(userID string) *rate.Limiter {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	if entry, exists := rl.userLimiters[userID]; exists {
		entry.lastSeen = time.Now()
		return entry.limiter
	}

	limiter := rate.NewLimiter(
		rate.Limit(rl.config.UserRequestsPerSecond),
		rl.config.UserBurstSize,
	)

	rl.userLimiters[userID] = &rateLimiterEntry{
		limiter:  limiter,
		lastSeen: time.Now(),
	}

	return limiter
}

// getEndpointLimiter returns a rate limiter for the given endpoint
func (rl *RateLimiter) getEndpointLimiter(config EndpointLimit) *rate.Limiter {
	// For endpoint limiters, we use a simple approach and create them on-demand
	// In a production system, you might want to cache these as well
	return rate.NewLimiter(
		rate.Limit(config.RequestsPerSecond),
		config.BurstSize,
	)
}

// cleanupRoutine periodically removes inactive rate limiters to prevent memory leaks
func (rl *RateLimiter) cleanupRoutine() {
	ticker := time.NewTicker(rl.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rl.cleanup()
		case <-rl.stopCleanup:
			return
		}
	}
}

// cleanup removes inactive rate limiters
func (rl *RateLimiter) cleanup() {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.config.CleanupInterval * 2) // Remove entries not seen for 2 cleanup intervals

	// Clean up IP limiters
	for ip, entry := range rl.ipLimiters {
		if entry.lastSeen.Before(cutoff) {
			delete(rl.ipLimiters, ip)
		}
	}

	// Clean up user limiters
	for userID, entry := range rl.userLimiters {
		if entry.lastSeen.Before(cutoff) {
			delete(rl.userLimiters, userID)
		}
	}
}

// Stop stops the rate limiter and cleanup routines
func (rl *RateLimiter) Stop() {
	close(rl.stopCleanup)
}

// IsEnabled returns whether rate limiting is enabled
func (rl *RateLimiter) IsEnabled() bool {
	return rl.config.Enabled
}

// GetStats returns statistics about current rate limiters
func (rl *RateLimiter) GetStats() map[string]interface{} {
	rl.mutex.RLock()
	defer rl.mutex.RUnlock()

	return map[string]interface{}{
		"enabled":             rl.config.Enabled,
		"ip_limiters_count":   len(rl.ipLimiters),
		"user_limiters_count": len(rl.userLimiters),
		"global_limit": map[string]interface{}{
			"requests_per_second": rl.config.RequestsPerSecond,
			"burst_size":          rl.config.BurstSize,
		},
	}
}

// RateLimitMiddleware creates HTTP middleware for rate limiting
func (rl *RateLimiter) RateLimitMiddleware() func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !rl.config.Enabled {
				next.ServeHTTP(w, r)
				return
			}

			// Extract client IP
			clientIP := getClientIP(r)

			// Extract user ID if available (from authentication middleware)
			userID := getUserID(r)

			// Check global rate limit
			if !rl.Allow() {
				rl.sendRateLimitResponse(w, "Global rate limit exceeded")
				return
			}

			// Check IP-based rate limit
			if rl.config.IPLimitEnabled && !rl.AllowIP(clientIP) {
				rl.sendRateLimitResponse(w, "IP rate limit exceeded")
				return
			}

			// Check user-based rate limit
			if rl.config.UserLimitEnabled && userID != "" && !rl.AllowUser(userID) {
				rl.sendRateLimitResponse(w, "User rate limit exceeded")
				return
			}

			// Check endpoint-specific rate limit
			endpoint := getEndpointPattern(r)
			if !rl.AllowEndpoint(endpoint) {
				rl.sendRateLimitResponse(w, "Endpoint rate limit exceeded")
				return
			}

			// Add rate limit headers
			rl.addRateLimitHeaders(w, clientIP, userID)

			next.ServeHTTP(w, r)
		})
	}
}

// sendRateLimitResponse sends a rate limit exceeded response
func (rl *RateLimiter) sendRateLimitResponse(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Retry-After", "60") // Suggest retry after 60 seconds
	w.WriteHeader(http.StatusTooManyRequests)

	response := fmt.Sprintf(`{"error": "rate_limit_exceeded", "message": "%s"}`, message)
	w.Write([]byte(response))
}

// addRateLimitHeaders adds rate limit information to response headers
func (rl *RateLimiter) addRateLimitHeaders(w http.ResponseWriter, clientIP, userID string) {
	// Add global rate limit headers
	if rl.globalLimiter != nil {
		w.Header().Set("X-RateLimit-Limit", strconv.FormatFloat(float64(rl.globalLimiter.Limit()), 'f', 0, 64))
		w.Header().Set("X-RateLimit-Burst", strconv.Itoa(rl.globalLimiter.Burst()))
	}

	// Add IP-specific headers if IP limiting is enabled
	if rl.config.IPLimitEnabled {
		ipLimiter := rl.getIPLimiter(clientIP)
		w.Header().Set("X-RateLimit-IP-Limit", strconv.FormatFloat(float64(ipLimiter.Limit()), 'f', 0, 64))
		w.Header().Set("X-RateLimit-IP-Burst", strconv.Itoa(ipLimiter.Burst()))
	}

	// Add user-specific headers if user limiting is enabled and user is identified
	if rl.config.UserLimitEnabled && userID != "" {
		userLimiter := rl.getUserLimiter(userID)
		w.Header().Set("X-RateLimit-User-Limit", strconv.FormatFloat(float64(userLimiter.Limit()), 'f', 0, 64))
		w.Header().Set("X-RateLimit-User-Burst", strconv.Itoa(userLimiter.Burst()))
	}
}

// Helper functions

// getClientIP extracts the client IP from the request
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header first
	xForwardedFor := r.Header.Get("X-Forwarded-For")
	if xForwardedFor != "" {
		// X-Forwarded-For can contain multiple IPs, take the first one
		ips := strings.Split(xForwardedFor, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	// Check X-Real-IP header
	xRealIP := r.Header.Get("X-Real-IP")
	if xRealIP != "" {
		return xRealIP
	}

	// Fall back to RemoteAddr
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

// getUserID extracts the user ID from the request context
func getUserID(r *http.Request) string {
	// This would typically be set by authentication middleware
	if userID := r.Header.Get("X-User-ID"); userID != "" {
		return userID
	}

	// Check context for user information
	if userID := r.Context().Value("user_id"); userID != nil {
		if id, ok := userID.(string); ok {
			return id
		}
	}

	return ""
}

// getEndpointPattern extracts a pattern for the endpoint
func getEndpointPattern(r *http.Request) string {
	// Simple pattern matching - in production, you might want more sophisticated routing
	path := r.URL.Path
	method := r.Method

	// Normalize common patterns
	// For example, /entities/123 becomes /entities/*
	segments := strings.Split(path, "/")
	for i, segment := range segments {
		if isUUID(segment) || isNumeric(segment) {
			segments[i] = "*"
		}
	}

	normalizedPath := strings.Join(segments, "/")
	return fmt.Sprintf("%s %s", method, normalizedPath)
}

// isUUID checks if a string looks like a UUID
func isUUID(s string) bool {
	return len(s) == 36 && strings.Count(s, "-") == 4
}

// isNumeric checks if a string is numeric
func isNumeric(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil
}

// AdaptiveRateLimiter implements adaptive rate limiting based on system load
type AdaptiveRateLimiter struct {
	baseLimiter    *RateLimiter
	loadThresholds map[float64]float64 // load -> rate multiplier
	getCurrentLoad func() float64
}

// NewAdaptiveRateLimiter creates a new adaptive rate limiter
func NewAdaptiveRateLimiter(config RateLimitConfig, loadThresholds map[float64]float64, getCurrentLoad func() float64) *AdaptiveRateLimiter {
	return &AdaptiveRateLimiter{
		baseLimiter:    NewRateLimiter(config),
		loadThresholds: loadThresholds,
		getCurrentLoad: getCurrentLoad,
	}
}

// Allow checks if a request should be allowed based on current system load
func (arl *AdaptiveRateLimiter) Allow() bool {
	if !arl.baseLimiter.config.Enabled {
		return true
	}

	load := arl.getCurrentLoad()
	multiplier := arl.getLoadMultiplier(load)

	// Adjust rate limit based on load
	if multiplier < 1.0 {
		// Higher load = stricter limits
		// We implement this by checking multiple times
		checks := int(1.0 / multiplier)
		for i := 0; i < checks; i++ {
			if !arl.baseLimiter.Allow() {
				return false
			}
		}
		return true
	}

	return arl.baseLimiter.Allow()
}

// getLoadMultiplier returns the rate multiplier based on current load
func (arl *AdaptiveRateLimiter) getLoadMultiplier(load float64) float64 {
	var multiplier float64 = 1.0

	for threshold, mult := range arl.loadThresholds {
		if load >= threshold {
			multiplier = mult
		}
	}

	return multiplier
}

// Rate limiting for specific operations

// EntityRateLimiter provides rate limiting for entity operations
type EntityRateLimiter struct {
	limiter *RateLimiter
}

// NewEntityRateLimiter creates a new entity rate limiter
func NewEntityRateLimiter(config RateLimitConfig) *EntityRateLimiter {
	return &EntityRateLimiter{
		limiter: NewRateLimiter(config),
	}
}

// AllowEntityCreation checks if entity creation is allowed
func (erl *EntityRateLimiter) AllowEntityCreation(userID string) bool {
	return erl.limiter.AllowUser(userID) && erl.limiter.AllowEndpoint("POST /entities")
}

// AllowEntityUpdate checks if entity update is allowed
func (erl *EntityRateLimiter) AllowEntityUpdate(userID string) bool {
	return erl.limiter.AllowUser(userID) && erl.limiter.AllowEndpoint("PATCH /entities/*")
}

// AllowEntityQuery checks if entity query is allowed
func (erl *EntityRateLimiter) AllowEntityQuery(userID string) bool {
	return erl.limiter.AllowUser(userID) && erl.limiter.AllowEndpoint("GET /entities/*")
}

// SearchRateLimiter provides rate limiting for search operations
type SearchRateLimiter struct {
	limiter *RateLimiter
}

// NewSearchRateLimiter creates a new search rate limiter
func NewSearchRateLimiter(config RateLimitConfig) *SearchRateLimiter {
	return &SearchRateLimiter{
		limiter: NewRateLimiter(config),
	}
}

// AllowTextSearch checks if text search is allowed
func (srl *SearchRateLimiter) AllowTextSearch(userID string) bool {
	return srl.limiter.AllowUser(userID) && srl.limiter.AllowEndpoint("POST /search")
}

// AllowVectorSearch checks if vector search is allowed
func (srl *SearchRateLimiter) AllowVectorSearch(userID string) bool {
	return srl.limiter.AllowUser(userID) && srl.limiter.AllowEndpoint("POST /search/vector")
}
