package lock

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// DistributedLock defines the interface for distributed locking
type DistributedLock interface {
	// Acquire attempts to acquire a distributed lock
	Acquire(ctx context.Context, resource string, ttl time.Duration) (LockHandle, error)
	
	// TryAcquire attempts to acquire a lock without blocking
	TryAcquire(ctx context.Context, resource string, ttl time.Duration) (LockHandle, error)
	
	// Release releases a lock using its handle
	Release(ctx context.Context, handle LockHandle) error
	
	// IsLocked checks if a resource is currently locked
	IsLocked(ctx context.Context, resource string) (bool, error)
	
	// Close closes the distributed lock manager
	Close() error
}

// LockHandle represents a handle to an acquired lock
type LockHandle interface {
	// Resource returns the resource name
	Resource() string
	
	// Token returns the lock token
	Token() string
	
	// ExpiresAt returns when the lock expires
	ExpiresAt() time.Time
	
	// Extend extends the lock TTL
	Extend(ctx context.Context, ttl time.Duration) error
	
	// IsValid checks if the lock is still valid
	IsValid() bool
}

// lockHandle implements LockHandle
type lockHandle struct {
	resource  string
	token     string
	expiresAt time.Time
	manager   DistributedLock
}

// Resource returns the resource name
func (h *lockHandle) Resource() string {
	return h.resource
}

// Token returns the lock token
func (h *lockHandle) Token() string {
	return h.token
}

// ExpiresAt returns when the lock expires
func (h *lockHandle) ExpiresAt() time.Time {
	return h.expiresAt
}

// Extend extends the lock TTL (implementation depends on the distributed lock)
func (h *lockHandle) Extend(ctx context.Context, ttl time.Duration) error {
	// This would need to be implemented by the specific distributed lock
	return nil
}

// IsValid checks if the lock is still valid
func (h *lockHandle) IsValid() bool {
	return time.Now().Before(h.expiresAt)
}

// NewLockHandle creates a new lock handle
func NewLockHandle(resource, token string, expiresAt time.Time, manager DistributedLock) LockHandle {
	return &lockHandle{
		resource:  resource,
		token:     token,
		expiresAt: expiresAt,
		manager:   manager,
	}
}

// InMemoryDistributedLock provides a distributed lock implementation using in-memory storage
// This is suitable for single-instance deployments
type InMemoryDistributedLock struct {
	*Manager
}

// NewInMemoryDistributedLock creates a new in-memory distributed lock
func NewInMemoryDistributedLock() *InMemoryDistributedLock {
	return &InMemoryDistributedLock{
		Manager: NewManager(30*time.Second, 5*time.Minute),
	}
}

// Acquire attempts to acquire a distributed lock
func (imdl *InMemoryDistributedLock) Acquire(ctx context.Context, resource string, ttl time.Duration) (LockHandle, error) {
	if err := imdl.LockWithTimeout(resource, ttl); err != nil {
		return nil, err
	}
	
	token := uuid.New().String()
	expiresAt := time.Now().Add(ttl)
	
	return NewLockHandle(resource, token, expiresAt, imdl), nil
}

// TryAcquire attempts to acquire a lock without blocking
func (imdl *InMemoryDistributedLock) TryAcquire(ctx context.Context, resource string, ttl time.Duration) (LockHandle, error) {
	if err := imdl.TryLock(resource, ttl); err != nil {
		return nil, err
	}
	
	token := uuid.New().String()
	expiresAt := time.Now().Add(ttl)
	
	return NewLockHandle(resource, token, expiresAt, imdl), nil
}

// Release releases a lock using its handle
func (imdl *InMemoryDistributedLock) Release(ctx context.Context, handle LockHandle) error {
	return imdl.Unlock(handle.Resource())
}

// IsLocked checks if a resource is currently locked
func (imdl *InMemoryDistributedLock) IsLocked(ctx context.Context, resource string) (bool, error) {
	return imdl.Manager.IsLocked(resource), nil
}

// Close closes the distributed lock manager
func (imdl *InMemoryDistributedLock) Close() error {
	// No cleanup needed for in-memory implementation
	return nil
}

// IsHeld checks if a resource is currently locked (convenience method for tests)
func (imdl *InMemoryDistributedLock) IsHeld(resource string) bool {
	return imdl.Manager.IsLocked(resource)
}

// LockManager provides high-level locking functionality with distributed support
type LockManager struct {
	distributedLock DistributedLock
	entityLock      *EntityLockManager
	schemaLock      *SchemaLockManager
}

// NewLockManager creates a new lock manager with distributed locking support
func NewLockManager(distributedLock DistributedLock) *LockManager {
	if distributedLock == nil {
		distributedLock = NewInMemoryDistributedLock()
	}
	
	return &LockManager{
		distributedLock: distributedLock,
		entityLock:      NewEntityLockManager(),
		schemaLock:      NewSchemaLockManager(),
	}
}

// AcquireEntityLock acquires a lock on an entity
func (lm *LockManager) AcquireEntityLock(ctx context.Context, entityType string, entityID uuid.UUID, ttl time.Duration) (LockHandle, error) {
	resource := buildEntityLockKey(entityType, entityID)
	return lm.distributedLock.Acquire(ctx, resource, ttl)
}

// AcquireSchemaLock acquires a lock on a schema
func (lm *LockManager) AcquireSchemaLock(ctx context.Context, schemaType, schemaName string, ttl time.Duration) (LockHandle, error) {
	resource := buildSchemaLockKey(schemaType, schemaName)
	return lm.distributedLock.Acquire(ctx, resource, ttl)
}

// AcquireGlobalLock acquires a global lock for system-wide operations
func (lm *LockManager) AcquireGlobalLock(ctx context.Context, operation string, ttl time.Duration) (LockHandle, error) {
	resource := buildGlobalLockKey(operation)
	return lm.distributedLock.Acquire(ctx, resource, ttl)
}

// ReleaseLock releases any type of lock
func (lm *LockManager) ReleaseLock(ctx context.Context, handle LockHandle) error {
	return lm.distributedLock.Release(ctx, handle)
}

// WithEntityLock executes a function while holding an entity lock
func (lm *LockManager) WithEntityLock(ctx context.Context, entityType string, entityID uuid.UUID, ttl time.Duration, fn func() error) error {
	handle, err := lm.AcquireEntityLock(ctx, entityType, entityID, ttl)
	if err != nil {
		return err
	}
	defer lm.ReleaseLock(ctx, handle)
	
	return fn()
}

// WithSchemaLock executes a function while holding a schema lock
func (lm *LockManager) WithSchemaLock(ctx context.Context, schemaType, schemaName string, ttl time.Duration, fn func() error) error {
	handle, err := lm.AcquireSchemaLock(ctx, schemaType, schemaName, ttl)
	if err != nil {
		return err
	}
	defer lm.ReleaseLock(ctx, handle)
	
	return fn()
}

// WithGlobalLock executes a function while holding a global lock
func (lm *LockManager) WithGlobalLock(ctx context.Context, operation string, ttl time.Duration, fn func() error) error {
	handle, err := lm.AcquireGlobalLock(ctx, operation, ttl)
	if err != nil {
		return err
	}
	defer lm.ReleaseLock(ctx, handle)
	
	return fn()
}

// Close closes the lock manager
func (lm *LockManager) Close() error {
	return lm.distributedLock.Close()
}

// Lock acquires a generic lock with context and timeout
func (lm *LockManager) Lock(ctx context.Context, resource string, timeout time.Duration) error {
	_, err := lm.distributedLock.Acquire(ctx, resource, timeout)
	return err
}

// Unlock releases a generic lock
func (lm *LockManager) Unlock(ctx context.Context, resource string) error {
	// Since we don't have the handle, we need to check if the resource is locked
	// and release it directly through the underlying implementation
	if imdl, ok := lm.distributedLock.(*InMemoryDistributedLock); ok {
		return imdl.Unlock(resource)
	}
	return fmt.Errorf("unlock not supported for this distributed lock implementation")
}

// IsLocked checks if a resource is currently locked
func (lm *LockManager) IsLocked(resource string) bool {
	locked, _ := lm.distributedLock.IsLocked(context.Background(), resource)
	return locked
}

// LockEntity acquires a lock on an entity with context and timeout
func (lm *LockManager) LockEntity(ctx context.Context, entityType, entityID string, timeout time.Duration) error {
	resource := entityType + ":" + entityID
	return lm.Lock(ctx, resource, timeout)
}

// UnlockEntity releases a lock on an entity
func (lm *LockManager) UnlockEntity(ctx context.Context, entityType, entityID string) error {
	resource := entityType + ":" + entityID
	return lm.Unlock(ctx, resource)
}

// LockSchema acquires a lock on a schema with context and timeout
func (lm *LockManager) LockSchema(ctx context.Context, entityType string, timeout time.Duration) error {
	resource := "schema:" + entityType
	return lm.Lock(ctx, resource, timeout)
}

// UnlockSchema releases a lock on a schema
func (lm *LockManager) UnlockSchema(ctx context.Context, entityType string) error {
	resource := "schema:" + entityType
	return lm.Unlock(ctx, resource)
}

// Helper functions for building lock keys

func buildEntityLockKey(entityType string, entityID uuid.UUID) string {
	return "entity:" + entityType + ":" + entityID.String()
}

func buildSchemaLockKey(schemaType, schemaName string) string {
	return "schema:" + schemaType + ":" + schemaName
}

func buildGlobalLockKey(operation string) string {
	return "global:" + operation
}

// AutoRefreshLock automatically refreshes a lock before it expires
type AutoRefreshLock struct {
	handle     LockHandle
	manager    DistributedLock
	stopCh     chan struct{}
	refreshTTL time.Duration
}

// NewAutoRefreshLock creates a new auto-refreshing lock
func NewAutoRefreshLock(handle LockHandle, manager DistributedLock, refreshTTL time.Duration) *AutoRefreshLock {
	arl := &AutoRefreshLock{
		handle:     handle,
		manager:    manager,
		stopCh:     make(chan struct{}),
		refreshTTL: refreshTTL,
	}
	
	// Start auto-refresh routine
	go arl.refreshLoop()
	
	return arl
}

// refreshLoop continuously refreshes the lock
func (arl *AutoRefreshLock) refreshLoop() {
	// Refresh at 2/3 of the TTL interval
	interval := arl.refreshTTL * 2 / 3
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			if err := arl.handle.Extend(context.Background(), arl.refreshTTL); err != nil {
				// Log error but continue trying
				// In a real implementation, you'd use proper logging
				return
			}
		case <-arl.stopCh:
			return
		}
	}
}

// Stop stops the auto-refresh and releases the lock
func (arl *AutoRefreshLock) Stop(ctx context.Context) error {
	close(arl.stopCh)
	return arl.manager.Release(ctx, arl.handle)
}

// Handle returns the underlying lock handle
func (arl *AutoRefreshLock) Handle() LockHandle {
	return arl.handle
}