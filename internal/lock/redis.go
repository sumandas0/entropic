package lock

import (
	"context"
	"fmt"
	"time"
)

type RedisDistributedLock struct {
	
	keyPrefix string
}

func NewRedisDistributedLock(redisURL, keyPrefix string) (*RedisDistributedLock, error) {
	if keyPrefix == "" {
		keyPrefix = "entropic:lock:"
	}

	return &RedisDistributedLock{
		keyPrefix: keyPrefix,
	}, fmt.Errorf("Redis distributed lock not implemented - add Redis client dependency")
}

func (rdl *RedisDistributedLock) Acquire(ctx context.Context, resource string, ttl time.Duration) (LockHandle, error) {

	return nil, fmt.Errorf("Redis distributed lock not implemented")
}

func (rdl *RedisDistributedLock) TryAcquire(ctx context.Context, resource string, ttl time.Duration) (LockHandle, error) {
	
	return nil, fmt.Errorf("Redis distributed lock not implemented")
}

func (rdl *RedisDistributedLock) Release(ctx context.Context, handle LockHandle) error {

	return fmt.Errorf("Redis distributed lock not implemented")
}

func (rdl *RedisDistributedLock) IsLocked(ctx context.Context, resource string) (bool, error) {

	return false, fmt.Errorf("Redis distributed lock not implemented")
}

func (rdl *RedisDistributedLock) Close() error {
	
	return nil
}

type redisLockHandle struct {
	resource  string
	token     string
	expiresAt time.Time
	manager   *RedisDistributedLock
}

func (h *redisLockHandle) Resource() string {
	return h.resource
}

func (h *redisLockHandle) Token() string {
	return h.token
}

func (h *redisLockHandle) ExpiresAt() time.Time {
	return h.expiresAt
}

func (h *redisLockHandle) Extend(ctx context.Context, ttl time.Duration) error {

	h.expiresAt = time.Now().Add(ttl)
	return fmt.Errorf("Redis lock extend not implemented")
}

func (h *redisLockHandle) IsValid() bool {
	return time.Now().Before(h.expiresAt)
}

type RedisLockConfig struct {
	RedisURL        string
	KeyPrefix       string
	DefaultTTL      time.Duration
	MaxRetries      int
	RetryDelay      time.Duration
	CleanupInterval time.Duration
}

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

type ConsistentHashingLock struct {
	locks    []DistributedLock
	hashRing *HashRing
}

type HashRing struct {
	nodes map[uint32]int 
	keys  []uint32       
}

func NewConsistentHashingLock(locks []DistributedLock) *ConsistentHashingLock {
	hashRing := &HashRing{
		nodes: make(map[uint32]int),
		keys:  make([]uint32, 0),
	}

	for i := range locks {
		for j := 0; j < 150; j++ { 
			hash := hashString(fmt.Sprintf("node-%d-%d", i, j))
			hashRing.nodes[hash] = i
			hashRing.keys = append(hashRing.keys, hash)
		}
	}

	sortUint32Slice(hashRing.keys)

	return &ConsistentHashingLock{
		locks:    locks,
		hashRing: hashRing,
	}
}

func (chl *ConsistentHashingLock) getLockForResource(resource string) DistributedLock {
	hash := hashString(resource)
	nodeIndex := chl.hashRing.getNode(hash)
	return chl.locks[nodeIndex]
}

func (hr *HashRing) getNode(hash uint32) int {
	
	left, right := 0, len(hr.keys)-1

	for left <= right {
		mid := (left + right) / 2
		if hr.keys[mid] >= hash {
			right = mid - 1
		} else {
			left = mid + 1
		}
	}

	if left >= len(hr.keys) {
		left = 0
	}

	return hr.nodes[hr.keys[left]]
}

func (chl *ConsistentHashingLock) Acquire(ctx context.Context, resource string, ttl time.Duration) (LockHandle, error) {
	lock := chl.getLockForResource(resource)
	return lock.Acquire(ctx, resource, ttl)
}

func (chl *ConsistentHashingLock) TryAcquire(ctx context.Context, resource string, ttl time.Duration) (LockHandle, error) {
	lock := chl.getLockForResource(resource)
	return lock.TryAcquire(ctx, resource, ttl)
}

func (chl *ConsistentHashingLock) Release(ctx context.Context, handle LockHandle) error {
	lock := chl.getLockForResource(handle.Resource())
	return lock.Release(ctx, handle)
}

func (chl *ConsistentHashingLock) IsLocked(ctx context.Context, resource string) (bool, error) {
	lock := chl.getLockForResource(resource)
	return lock.IsLocked(ctx, resource)
}

func (chl *ConsistentHashingLock) Close() error {
	var firstErr error
	for _, lock := range chl.locks {
		if err := lock.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func hashString(s string) uint32 {
	hash := uint32(0)
	for i := 0; i < len(s); i++ {
		hash = hash*31 + uint32(s[i])
	}
	return hash
}

func sortUint32Slice(slice []uint32) {
	for i := 0; i < len(slice)-1; i++ {
		for j := i + 1; j < len(slice); j++ {
			if slice[i] > slice[j] {
				slice[i], slice[j] = slice[j], slice[i]
			}
		}
	}
}
