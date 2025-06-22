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

type RateLimitConfig struct {
	Enabled           bool          `yaml:"enabled" mapstructure:"enabled"`
	RequestsPerSecond float64       `yaml:"requests_per_second" mapstructure:"requests_per_second"`
	BurstSize         int           `yaml:"burst_size" mapstructure:"burst_size"`
	CleanupInterval   time.Duration `yaml:"cleanup_interval" mapstructure:"cleanup_interval"`

	EndpointLimits map[string]EndpointLimit `yaml:"endpoint_limits" mapstructure:"endpoint_limits"`

	IPLimitEnabled      bool    `yaml:"ip_limit_enabled" mapstructure:"ip_limit_enabled"`
	IPRequestsPerSecond float64 `yaml:"ip_requests_per_second" mapstructure:"ip_requests_per_second"`
	IPBurstSize         int     `yaml:"ip_burst_size" mapstructure:"ip_burst_size"`

	UserLimitEnabled      bool    `yaml:"user_limit_enabled" mapstructure:"user_limit_enabled"`
	UserRequestsPerSecond float64 `yaml:"user_requests_per_second" mapstructure:"user_requests_per_second"`
	UserBurstSize         int     `yaml:"user_burst_size" mapstructure:"user_burst_size"`
}

type EndpointLimit struct {
	RequestsPerSecond float64 `yaml:"requests_per_second" mapstructure:"requests_per_second"`
	BurstSize         int     `yaml:"burst_size" mapstructure:"burst_size"`
}

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

func NewRateLimiter(config RateLimitConfig) *RateLimiter {
	rl := &RateLimiter{
		config:       config,
		ipLimiters:   make(map[string]*rateLimiterEntry),
		userLimiters: make(map[string]*rateLimiterEntry),
		stopCleanup:  make(chan struct{}),
	}

	if config.Enabled {
		
		rl.globalLimiter = rate.NewLimiter(
			rate.Limit(config.RequestsPerSecond),
			config.BurstSize,
		)

		go rl.cleanupRoutine()
	}

	return rl
}

func (rl *RateLimiter) Allow() bool {
	if !rl.config.Enabled || rl.globalLimiter == nil {
		return true
	}
	return rl.globalLimiter.Allow()
}

func (rl *RateLimiter) AllowIP(ip string) bool {
	if !rl.config.Enabled || !rl.config.IPLimitEnabled {
		return true
	}

	limiter := rl.getIPLimiter(ip)
	return limiter.Allow()
}

func (rl *RateLimiter) AllowUser(userID string) bool {
	if !rl.config.Enabled || !rl.config.UserLimitEnabled {
		return true
	}

	limiter := rl.getUserLimiter(userID)
	return limiter.Allow()
}

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

func (rl *RateLimiter) getEndpointLimiter(config EndpointLimit) *rate.Limiter {

	return rate.NewLimiter(
		rate.Limit(config.RequestsPerSecond),
		config.BurstSize,
	)
}

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

func (rl *RateLimiter) cleanup() {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.config.CleanupInterval * 2) 

	for ip, entry := range rl.ipLimiters {
		if entry.lastSeen.Before(cutoff) {
			delete(rl.ipLimiters, ip)
		}
	}

	for userID, entry := range rl.userLimiters {
		if entry.lastSeen.Before(cutoff) {
			delete(rl.userLimiters, userID)
		}
	}
}

func (rl *RateLimiter) Stop() {
	close(rl.stopCleanup)
}

func (rl *RateLimiter) IsEnabled() bool {
	return rl.config.Enabled
}

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

func (rl *RateLimiter) RateLimitMiddleware() func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !rl.config.Enabled {
				next.ServeHTTP(w, r)
				return
			}

			clientIP := getClientIP(r)

			userID := getUserID(r)

			if !rl.Allow() {
				rl.sendRateLimitResponse(w, "Global rate limit exceeded")
				return
			}

			if rl.config.IPLimitEnabled && !rl.AllowIP(clientIP) {
				rl.sendRateLimitResponse(w, "IP rate limit exceeded")
				return
			}

			if rl.config.UserLimitEnabled && userID != "" && !rl.AllowUser(userID) {
				rl.sendRateLimitResponse(w, "User rate limit exceeded")
				return
			}

			endpoint := getEndpointPattern(r)
			if !rl.AllowEndpoint(endpoint) {
				rl.sendRateLimitResponse(w, "Endpoint rate limit exceeded")
				return
			}

			rl.addRateLimitHeaders(w, clientIP, userID)

			next.ServeHTTP(w, r)
		})
	}
}

func (rl *RateLimiter) sendRateLimitResponse(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Retry-After", "60") 
	w.WriteHeader(http.StatusTooManyRequests)

	response := fmt.Sprintf(`{"error": "rate_limit_exceeded", "message": "%s"}`, message)
	w.Write([]byte(response))
}

func (rl *RateLimiter) addRateLimitHeaders(w http.ResponseWriter, clientIP, userID string) {
	
	if rl.globalLimiter != nil {
		w.Header().Set("X-RateLimit-Limit", strconv.FormatFloat(float64(rl.globalLimiter.Limit()), 'f', 0, 64))
		w.Header().Set("X-RateLimit-Burst", strconv.Itoa(rl.globalLimiter.Burst()))
	}

	if rl.config.IPLimitEnabled {
		ipLimiter := rl.getIPLimiter(clientIP)
		w.Header().Set("X-RateLimit-IP-Limit", strconv.FormatFloat(float64(ipLimiter.Limit()), 'f', 0, 64))
		w.Header().Set("X-RateLimit-IP-Burst", strconv.Itoa(ipLimiter.Burst()))
	}

	if rl.config.UserLimitEnabled && userID != "" {
		userLimiter := rl.getUserLimiter(userID)
		w.Header().Set("X-RateLimit-User-Limit", strconv.FormatFloat(float64(userLimiter.Limit()), 'f', 0, 64))
		w.Header().Set("X-RateLimit-User-Burst", strconv.Itoa(userLimiter.Burst()))
	}
}

func getClientIP(r *http.Request) string {
	
	xForwardedFor := r.Header.Get("X-Forwarded-For")
	if xForwardedFor != "" {
		
		ips := strings.Split(xForwardedFor, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	xRealIP := r.Header.Get("X-Real-IP")
	if xRealIP != "" {
		return xRealIP
	}

	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

func getUserID(r *http.Request) string {
	
	if userID := r.Header.Get("X-User-ID"); userID != "" {
		return userID
	}

	if userID := r.Context().Value("user_id"); userID != nil {
		if id, ok := userID.(string); ok {
			return id
		}
	}

	return ""
}

func getEndpointPattern(r *http.Request) string {
	
	path := r.URL.Path
	method := r.Method

	segments := strings.Split(path, "/")
	for i, segment := range segments {
		if isUUID(segment) || isNumeric(segment) {
			segments[i] = "*"
		}
	}

	normalizedPath := strings.Join(segments, "/")
	return fmt.Sprintf("%s %s", method, normalizedPath)
}

func isUUID(s string) bool {
	return len(s) == 36 && strings.Count(s, "-") == 4
}

func isNumeric(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil
}

type AdaptiveRateLimiter struct {
	baseLimiter    *RateLimiter
	loadThresholds map[float64]float64 
	getCurrentLoad func() float64
}

func NewAdaptiveRateLimiter(config RateLimitConfig, loadThresholds map[float64]float64, getCurrentLoad func() float64) *AdaptiveRateLimiter {
	return &AdaptiveRateLimiter{
		baseLimiter:    NewRateLimiter(config),
		loadThresholds: loadThresholds,
		getCurrentLoad: getCurrentLoad,
	}
}

func (arl *AdaptiveRateLimiter) Allow() bool {
	if !arl.baseLimiter.config.Enabled {
		return true
	}

	load := arl.getCurrentLoad()
	multiplier := arl.getLoadMultiplier(load)

	if multiplier < 1.0 {

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

func (arl *AdaptiveRateLimiter) getLoadMultiplier(load float64) float64 {
	var multiplier float64 = 1.0

	for threshold, mult := range arl.loadThresholds {
		if load >= threshold {
			multiplier = mult
		}
	}

	return multiplier
}

type EntityRateLimiter struct {
	limiter *RateLimiter
}

func NewEntityRateLimiter(config RateLimitConfig) *EntityRateLimiter {
	return &EntityRateLimiter{
		limiter: NewRateLimiter(config),
	}
}

func (erl *EntityRateLimiter) AllowEntityCreation(userID string) bool {
	return erl.limiter.AllowUser(userID) && erl.limiter.AllowEndpoint("POST /entities")
}

func (erl *EntityRateLimiter) AllowEntityUpdate(userID string) bool {
	return erl.limiter.AllowUser(userID) && erl.limiter.AllowEndpoint("PATCH /entities/*")
}

func (erl *EntityRateLimiter) AllowEntityQuery(userID string) bool {
	return erl.limiter.AllowUser(userID) && erl.limiter.AllowEndpoint("GET /entities/*")
}

type SearchRateLimiter struct {
	limiter *RateLimiter
}

func NewSearchRateLimiter(config RateLimitConfig) *SearchRateLimiter {
	return &SearchRateLimiter{
		limiter: NewRateLimiter(config),
	}
}

func (srl *SearchRateLimiter) AllowTextSearch(userID string) bool {
	return srl.limiter.AllowUser(userID) && srl.limiter.AllowEndpoint("POST /search")
}

func (srl *SearchRateLimiter) AllowVectorSearch(userID string) bool {
	return srl.limiter.AllowUser(userID) && srl.limiter.AllowEndpoint("POST /search/vector")
}
