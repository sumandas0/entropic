package lock

import (
	"context"
	"fmt"
	"time"
)

// RedisDistributedLock provides a Redis-based distributed lock implementation
// This is a placeholder implementation - you would need to add Redis client dependency
type RedisDistributedLock struct {
	// redisClient redis.Client // Uncomment when Redis client is added
	keyPrefix string
}

// NewRedisDistributedLock creates a new Redis-based distributed lock
// This is a placeholder - implement when Redis support is needed
func NewRedisDistributedLock(redisURL, keyPrefix string) (*RedisDistributedLock, error) {
	if keyPrefix == "" {
		keyPrefix = "entropic:lock:"
	}

	// TODO: Initialize Redis client
	return &RedisDistributedLock{
		keyPrefix: keyPrefix,
	}, fmt.Errorf("Redis distributed lock not implemented - add Redis client dependency")
}

// Acquire attempts to acquire a distributed lock using Redis
func (rdl *RedisDistributedLock) Acquire(ctx context.Context, resource string, ttl time.Duration) (LockHandle, error) {
	// This is a placeholder implementation
	// Real implementation would use Redis SET command with NX and EX options
	return nil, fmt.Errorf("Redis distributed lock not implemented")
}

// TryAcquire attempts to acquire a lock without blocking
func (rdl *RedisDistributedLock) TryAcquire(ctx context.Context, resource string, ttl time.Duration) (LockHandle, error) {
	// This is a placeholder implementation
	return nil, fmt.Errorf("Redis distributed lock not implemented")
}

// Release releases a lock using its handle
func (rdl *RedisDistributedLock) Release(ctx context.Context, handle LockHandle) error {
	// This is a placeholder implementation
	// Real implementation would use Redis DEL command with token validation
	return fmt.Errorf("Redis distributed lock not implemented")
}

// IsLocked checks if a resource is currently locked
func (rdl *RedisDistributedLock) IsLocked(ctx context.Context, resource string) (bool, error) {
	// This is a placeholder implementation
	// Real implementation would use Redis EXISTS command
	return false, fmt.Errorf("Redis distributed lock not implemented")
}

// Close closes the Redis connection
func (rdl *RedisDistributedLock) Close() error {
	// TODO: Close Redis client
	return nil
}

// redisLockHandle implements LockHandle for Redis locks
type redisLockHandle struct {
	resource  string
	token     string
	expiresAt time.Time
	manager   *RedisDistributedLock
}

// Resource returns the resource name
func (h *redisLockHandle) Resource() string {
	return h.resource
}

// Token returns the lock token
func (h *redisLockHandle) Token() string {
	return h.token
}

// ExpiresAt returns when the lock expires
func (h *redisLockHandle) ExpiresAt() time.Time {
	return h.expiresAt
}

// Extend extends the lock TTL using Redis EXPIRE command
func (h *redisLockHandle) Extend(ctx context.Context, ttl time.Duration) error {
	// This is a placeholder implementation
	// Real implementation would use Redis EXPIRE command with token validation
	h.expiresAt = time.Now().Add(ttl)
	return fmt.Errorf("Redis lock extend not implemented")
}

// IsValid checks if the lock is still valid
func (h *redisLockHandle) IsValid() bool {
	return time.Now().Before(h.expiresAt)
}

/*
Example Redis Lua script for atomic lock operations:

-- acquire_lock.lua
local key = KEYS[1]
local token = ARGV[1]
local ttl = ARGV[2]

local current = redis.call('GET', key)
if current == false then
    redis.call('SET', key, token, 'EX', ttl)
    return 1
else
    return 0
end

-- release_lock.lua
local key = KEYS[1]
local token = ARGV[1]

local current = redis.call('GET', key)
if current == token then
    redis.call('DEL', key)
    return 1
else
    return 0
end

-- extend_lock.lua
local key = KEYS[1]
local token = ARGV[1]
local ttl = ARGV[2]

local current = redis.call('GET', key)
if current == token then
    redis.call('EXPIRE', key, ttl)
    return 1
else
    return 0
end
*/

// RedisLockConfig holds Redis lock configuration
type RedisLockConfig struct {
	RedisURL        string
	KeyPrefix       string
	DefaultTTL      time.Duration
	MaxRetries      int
	RetryDelay      time.Duration
	CleanupInterval time.Duration
}

// DefaultRedisLockConfig returns default Redis lock configuration
func DefaultRedisLockConfig() *RedisLockConfig {
	return &RedisLockConfig{
		RedisURL:        "redis://localhost:6379",
		KeyPrefix:       "entropic:lock:",
		DefaultTTL:      30 * time.Second,
		MaxRetries:      3,
		RetryDelay:      100 * time.Millisecond,
		CleanupInterval: time.Minute,
	}
}

// Validate validates the Redis lock configuration
func (c *RedisLockConfig) Validate() error {
	if c.RedisURL == "" {
		return fmt.Errorf("Redis URL is required")
	}
	if c.KeyPrefix == "" {
		c.KeyPrefix = "entropic:lock:"
	}
	if c.DefaultTTL <= 0 {
		c.DefaultTTL = 30 * time.Second
	}
	if c.MaxRetries < 0 {
		c.MaxRetries = 3
	}
	if c.RetryDelay <= 0 {
		c.RetryDelay = 100 * time.Millisecond
	}
	return nil
}

// ConsistentHashingLock provides distributed locking with consistent hashing
// This can be used to distribute locks across multiple Redis instances
type ConsistentHashingLock struct {
	locks    []DistributedLock
	hashRing *HashRing
}

// HashRing represents a consistent hash ring
type HashRing struct {
	nodes map[uint32]int // hash -> node index
	keys  []uint32       // sorted hash keys
}

// NewConsistentHashingLock creates a distributed lock with consistent hashing
func NewConsistentHashingLock(locks []DistributedLock) *ConsistentHashingLock {
	hashRing := &HashRing{
		nodes: make(map[uint32]int),
		keys:  make([]uint32, 0),
	}

	// Add nodes to hash ring
	for i := range locks {
		for j := 0; j < 150; j++ { // 150 virtual nodes per physical node
			hash := hashString(fmt.Sprintf("node-%d-%d", i, j))
			hashRing.nodes[hash] = i
			hashRing.keys = append(hashRing.keys, hash)
		}
	}

	// Sort keys
	sortUint32Slice(hashRing.keys)

	return &ConsistentHashingLock{
		locks:    locks,
		hashRing: hashRing,
	}
}

// getLockForResource returns the appropriate lock for a resource
func (chl *ConsistentHashingLock) getLockForResource(resource string) DistributedLock {
	hash := hashString(resource)
	nodeIndex := chl.hashRing.getNode(hash)
	return chl.locks[nodeIndex]
}

// getNode returns the node index for a given hash
func (hr *HashRing) getNode(hash uint32) int {
	// Binary search for the first node >= hash
	left, right := 0, len(hr.keys)-1

	for left <= right {
		mid := (left + right) / 2
		if hr.keys[mid] >= hash {
			right = mid - 1
		} else {
			left = mid + 1
		}
	}

	// Wrap around if necessary
	if left >= len(hr.keys) {
		left = 0
	}

	return hr.nodes[hr.keys[left]]
}

// Acquire acquires a lock using consistent hashing
func (chl *ConsistentHashingLock) Acquire(ctx context.Context, resource string, ttl time.Duration) (LockHandle, error) {
	lock := chl.getLockForResource(resource)
	return lock.Acquire(ctx, resource, ttl)
}

// TryAcquire tries to acquire a lock using consistent hashing
func (chl *ConsistentHashingLock) TryAcquire(ctx context.Context, resource string, ttl time.Duration) (LockHandle, error) {
	lock := chl.getLockForResource(resource)
	return lock.TryAcquire(ctx, resource, ttl)
}

// Release releases a lock
func (chl *ConsistentHashingLock) Release(ctx context.Context, handle LockHandle) error {
	lock := chl.getLockForResource(handle.Resource())
	return lock.Release(ctx, handle)
}

// IsLocked checks if a resource is locked
func (chl *ConsistentHashingLock) IsLocked(ctx context.Context, resource string) (bool, error) {
	lock := chl.getLockForResource(resource)
	return lock.IsLocked(ctx, resource)
}

// Close closes all underlying locks
func (chl *ConsistentHashingLock) Close() error {
	var firstErr error
	for _, lock := range chl.locks {
		if err := lock.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// Simple hash function (in production, use a proper hash function like FNV or murmur3)
func hashString(s string) uint32 {
	hash := uint32(0)
	for i := 0; i < len(s); i++ {
		hash = hash*31 + uint32(s[i])
	}
	return hash
}

// Simple uint32 slice sorting (in production, use sort.Slice)
func sortUint32Slice(slice []uint32) {
	for i := 0; i < len(slice)-1; i++ {
		for j := i + 1; j < len(slice); j++ {
			if slice[i] > slice[j] {
				slice[i], slice[j] = slice[j], slice[i]
			}
		}
	}
}
