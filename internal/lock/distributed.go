package lock

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/sumandas0/entropic/internal/integration"
)

type DistributedLock interface {
	
	Acquire(ctx context.Context, resource string, ttl time.Duration) (LockHandle, error)

	TryAcquire(ctx context.Context, resource string, ttl time.Duration) (LockHandle, error)

	Release(ctx context.Context, handle LockHandle) error

	IsLocked(ctx context.Context, resource string) (bool, error)

	Close() error
}

type LockHandle interface {
	
	Resource() string

	Token() string

	ExpiresAt() time.Time

	Extend(ctx context.Context, ttl time.Duration) error

	IsValid() bool
}

type lockHandle struct {
	resource  string
	token     string
	expiresAt time.Time
	manager   DistributedLock
}

func (h *lockHandle) Resource() string {
	return h.resource
}

func (h *lockHandle) Token() string {
	return h.token
}

func (h *lockHandle) ExpiresAt() time.Time {
	return h.expiresAt
}

func (h *lockHandle) Extend(ctx context.Context, ttl time.Duration) error {
	
	return nil
}

func (h *lockHandle) IsValid() bool {
	return time.Now().Before(h.expiresAt)
}

func NewLockHandle(resource, token string, expiresAt time.Time, manager DistributedLock) LockHandle {
	return &lockHandle{
		resource:  resource,
		token:     token,
		expiresAt: expiresAt,
		manager:   manager,
	}
}

type InMemoryDistributedLock struct {
	*Manager
}

func NewInMemoryDistributedLock() *InMemoryDistributedLock {
	return &InMemoryDistributedLock{
		Manager: NewManager(30*time.Second, 5*time.Minute),
	}
}

func (imdl *InMemoryDistributedLock) SetObservability(obsManager *integration.ObservabilityManager) {
	imdl.Manager.SetObservability(obsManager)
}

func (imdl *InMemoryDistributedLock) Acquire(ctx context.Context, resource string, ttl time.Duration) (LockHandle, error) {
	if err := imdl.LockWithTimeout(resource, ttl); err != nil {
		return nil, err
	}
	
	token := uuid.New().String()
	expiresAt := time.Now().Add(ttl)
	
	return NewLockHandle(resource, token, expiresAt, imdl), nil
}

func (imdl *InMemoryDistributedLock) TryAcquire(ctx context.Context, resource string, ttl time.Duration) (LockHandle, error) {
	if err := imdl.TryLock(resource, ttl); err != nil {
		return nil, err
	}
	
	token := uuid.New().String()
	expiresAt := time.Now().Add(ttl)
	
	return NewLockHandle(resource, token, expiresAt, imdl), nil
}

func (imdl *InMemoryDistributedLock) Release(ctx context.Context, handle LockHandle) error {
	return imdl.Unlock(handle.Resource())
}

func (imdl *InMemoryDistributedLock) IsLocked(ctx context.Context, resource string) (bool, error) {
	return imdl.Manager.IsLocked(resource), nil
}

func (imdl *InMemoryDistributedLock) Close() error {
	
	return nil
}

func (imdl *InMemoryDistributedLock) IsHeld(resource string) bool {
	return imdl.Manager.IsLocked(resource)
}

func (imdl *InMemoryDistributedLock) TryLock(resource string, ttl time.Duration) error {
	return imdl.Manager.TryLock(resource, ttl)
}

func (imdl *InMemoryDistributedLock) Unlock(resource string) error {
	return imdl.Manager.Unlock(resource)
}

type LockManager struct {
	distributedLock DistributedLock
	entityLock      *EntityLockManager
	schemaLock      *SchemaLockManager
	obsManager      *integration.ObservabilityManager
}

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

func (lm *LockManager) SetObservability(obsManager *integration.ObservabilityManager) {
	lm.obsManager = obsManager
	
	// Set observability on child managers
	lm.entityLock.SetObservability(obsManager)
	lm.schemaLock.SetObservability(obsManager)
	
	// Set observability on distributed lock if it's an InMemoryDistributedLock
	if imdl, ok := lm.distributedLock.(*InMemoryDistributedLock); ok {
		imdl.SetObservability(obsManager)
	}
}

func (lm *LockManager) AcquireEntityLock(ctx context.Context, entityType string, entityID uuid.UUID, ttl time.Duration) (LockHandle, error) {
	resource := buildEntityLockKey(entityType, entityID)
	return lm.distributedLock.Acquire(ctx, resource, ttl)
}

func (lm *LockManager) AcquireSchemaLock(ctx context.Context, schemaType, schemaName string, ttl time.Duration) (LockHandle, error) {
	resource := buildSchemaLockKey(schemaType, schemaName)
	return lm.distributedLock.Acquire(ctx, resource, ttl)
}

func (lm *LockManager) AcquireGlobalLock(ctx context.Context, operation string, ttl time.Duration) (LockHandle, error) {
	resource := buildGlobalLockKey(operation)
	return lm.distributedLock.Acquire(ctx, resource, ttl)
}

func (lm *LockManager) ReleaseLock(ctx context.Context, handle LockHandle) error {
	return lm.distributedLock.Release(ctx, handle)
}

func (lm *LockManager) WithEntityLock(ctx context.Context, entityType string, entityID uuid.UUID, ttl time.Duration, fn func() error) error {
	handle, err := lm.AcquireEntityLock(ctx, entityType, entityID, ttl)
	if err != nil {
		return err
	}
	defer lm.ReleaseLock(ctx, handle)
	
	return fn()
}

func (lm *LockManager) WithSchemaLock(ctx context.Context, schemaType, schemaName string, ttl time.Duration, fn func() error) error {
	handle, err := lm.AcquireSchemaLock(ctx, schemaType, schemaName, ttl)
	if err != nil {
		return err
	}
	defer lm.ReleaseLock(ctx, handle)
	
	return fn()
}

func (lm *LockManager) WithGlobalLock(ctx context.Context, operation string, ttl time.Duration, fn func() error) error {
	handle, err := lm.AcquireGlobalLock(ctx, operation, ttl)
	if err != nil {
		return err
	}
	defer lm.ReleaseLock(ctx, handle)
	
	return fn()
}

func (lm *LockManager) Close() error {
	return lm.distributedLock.Close()
}

func (lm *LockManager) Lock(ctx context.Context, resource string, timeout time.Duration) error {
	// If using InMemoryDistributedLock, use context-aware locking
	if imdl, ok := lm.distributedLock.(*InMemoryDistributedLock); ok {
		return imdl.LockWithContext(ctx, resource, timeout)
	}
	_, err := lm.distributedLock.Acquire(ctx, resource, timeout)
	return err
}

func (lm *LockManager) Unlock(ctx context.Context, resource string) error {

	if imdl, ok := lm.distributedLock.(*InMemoryDistributedLock); ok {
		return imdl.Unlock(resource)
	}
	return fmt.Errorf("unlock not supported for this distributed lock implementation")
}

func (lm *LockManager) IsLocked(resource string) bool {
	locked, _ := lm.distributedLock.IsLocked(context.Background(), resource)
	return locked
}

func (lm *LockManager) LockEntity(ctx context.Context, entityType, entityID string, timeout time.Duration) error {
	resource := entityType + ":" + entityID
	return lm.Lock(ctx, resource, timeout)
}

func (lm *LockManager) UnlockEntity(ctx context.Context, entityType, entityID string) error {
	resource := entityType + ":" + entityID
	return lm.Unlock(ctx, resource)
}

func (lm *LockManager) LockSchema(ctx context.Context, entityType string, timeout time.Duration) error {
	resource := "schema:" + entityType
	return lm.Lock(ctx, resource, timeout)
}

func (lm *LockManager) UnlockSchema(ctx context.Context, entityType string) error {
	resource := "schema:" + entityType
	return lm.Unlock(ctx, resource)
}

func buildEntityLockKey(entityType string, entityID uuid.UUID) string {
	return "entity:" + entityType + ":" + entityID.String()
}

func buildSchemaLockKey(schemaType, schemaName string) string {
	return "schema:" + schemaType + ":" + schemaName
}

func buildGlobalLockKey(operation string) string {
	return "global:" + operation
}

type AutoRefreshLock struct {
	handle     LockHandle
	manager    DistributedLock
	stopCh     chan struct{}
	refreshTTL time.Duration
}

func NewAutoRefreshLock(handle LockHandle, manager DistributedLock, refreshTTL time.Duration) *AutoRefreshLock {
	arl := &AutoRefreshLock{
		handle:     handle,
		manager:    manager,
		stopCh:     make(chan struct{}),
		refreshTTL: refreshTTL,
	}

	go arl.refreshLoop()
	
	return arl
}

func (arl *AutoRefreshLock) refreshLoop() {
	
	interval := arl.refreshTTL * 2 / 3
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			if err := arl.handle.Extend(context.Background(), arl.refreshTTL); err != nil {

				return
			}
		case <-arl.stopCh:
			return
		}
	}
}

func (arl *AutoRefreshLock) Stop(ctx context.Context) error {
	close(arl.stopCh)
	return arl.manager.Release(ctx, arl.handle)
}

func (arl *AutoRefreshLock) Handle() LockHandle {
	return arl.handle
}