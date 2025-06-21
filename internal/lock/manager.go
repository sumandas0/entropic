package lock

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Manager provides distributed locking capabilities
type Manager struct {
	locks     sync.Map // resource -> *resourceLock
	waitQueue sync.Map // resource -> []chan struct{}
	
	// Configuration
	defaultTimeout time.Duration
	maxWaitTime    time.Duration
	
	// Statistics
	mu         sync.RWMutex
	stats      LockStats
}

// resourceLock represents a lock on a specific resource
type resourceLock struct {
	mu       sync.Mutex
	holder   string    // ID of the lock holder
	acquired time.Time
	timeout  time.Duration
}

// LockStats holds locking statistics
type LockStats struct {
	ActiveLocks      int
	TotalAcquired    uint64
	TotalReleased    uint64
	TotalTimeouts    uint64
	TotalDeadlocks   uint64
	AverageWaitTime  time.Duration
}

// NewManager creates a new lock manager
func NewManager(defaultTimeout, maxWaitTime time.Duration) *Manager {
	if defaultTimeout <= 0 {
		defaultTimeout = 30 * time.Second
	}
	if maxWaitTime <= 0 {
		maxWaitTime = 5 * time.Minute
	}
	
	return &Manager{
		defaultTimeout: defaultTimeout,
		maxWaitTime:    maxWaitTime,
	}
}

// Lock acquires a lock on the specified resource
func (m *Manager) Lock(resource string) error {
	return m.LockWithTimeout(resource, m.defaultTimeout)
}

// LockWithTimeout acquires a lock with a specific timeout
func (m *Manager) LockWithTimeout(resource string, timeout time.Duration) error {
	holderID := uuid.New().String()
	
	// Get or create the resource lock
	lockInterface, _ := m.locks.LoadOrStore(resource, &resourceLock{})
	resLock := lockInterface.(*resourceLock)
	
	// Try to acquire the lock
	acquired := make(chan bool, 1)
	go func() {
		resLock.mu.Lock()
		resLock.holder = holderID
		resLock.acquired = time.Now()
		resLock.timeout = timeout
		acquired <- true
	}()
	
	// Wait for lock acquisition with timeout
	select {
	case <-acquired:
		m.recordAcquisition()
		
		// Start timeout monitor
		go m.monitorTimeout(resource, holderID, timeout)
		
		return nil
		
	case <-time.After(m.maxWaitTime):
		return fmt.Errorf("failed to acquire lock on resource %s: timeout", resource)
	}
}

// TryLock attempts to acquire a lock without blocking
func (m *Manager) TryLock(resource string, timeout time.Duration) error {
	holderID := uuid.New().String()
	
	// Get or create the resource lock
	lockInterface, _ := m.locks.LoadOrStore(resource, &resourceLock{})
	resLock := lockInterface.(*resourceLock)
	
	// Try to acquire the lock without blocking
	if !resLock.mu.TryLock() {
		return fmt.Errorf("resource %s is already locked", resource)
	}
	
	resLock.holder = holderID
	resLock.acquired = time.Now()
	resLock.timeout = timeout
	
	m.recordAcquisition()
	
	// Start timeout monitor
	go m.monitorTimeout(resource, holderID, timeout)
	
	return nil
}

// Unlock releases a lock on the specified resource
func (m *Manager) Unlock(resource string) error {
	lockInterface, ok := m.locks.Load(resource)
	if !ok {
		return fmt.Errorf("no lock found for resource %s", resource)
	}
	
	resLock := lockInterface.(*resourceLock)
	resLock.holder = ""
	resLock.mu.Unlock()
	
	m.recordRelease()
	
	// Notify any waiters
	m.notifyWaiters(resource)
	
	return nil
}

// IsLocked checks if a resource is currently locked
func (m *Manager) IsLocked(resource string) bool {
	lockInterface, ok := m.locks.Load(resource)
	if !ok {
		return false
	}
	
	resLock := lockInterface.(*resourceLock)
	// Try to acquire and immediately release to check if locked
	if resLock.mu.TryLock() {
		resLock.mu.Unlock()
		return false
	}
	return true
}

// WaitForUnlock waits for a resource to become unlocked
func (m *Manager) WaitForUnlock(ctx context.Context, resource string) error {
	if !m.IsLocked(resource) {
		return nil
	}
	
	// Create wait channel
	waitCh := make(chan struct{})
	
	// Add to wait queue
	queueInterface, _ := m.waitQueue.LoadOrStore(resource, &sync.Map{})
	queue := queueInterface.(*sync.Map)
	queueID := uuid.New().String()
	queue.Store(queueID, waitCh)
	
	// Clean up on exit
	defer func() {
		queue.Delete(queueID)
	}()
	
	// Wait for unlock or context cancellation
	select {
	case <-waitCh:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// monitorTimeout monitors lock timeout and releases if expired
func (m *Manager) monitorTimeout(resource, holderID string, timeout time.Duration) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	
	<-timer.C
	
	// Check if this holder still owns the lock
	lockInterface, ok := m.locks.Load(resource)
	if !ok {
		return
	}
	
	resLock := lockInterface.(*resourceLock)
	if resLock.holder == holderID {
		// Force unlock due to timeout
		resLock.holder = ""
		resLock.mu.Unlock()
		
		m.recordTimeout()
		m.notifyWaiters(resource)
	}
}

// notifyWaiters notifies all waiters for a resource
func (m *Manager) notifyWaiters(resource string) {
	queueInterface, ok := m.waitQueue.Load(resource)
	if !ok {
		return
	}
	
	queue := queueInterface.(*sync.Map)
	queue.Range(func(key, value interface{}) bool {
		waitCh := value.(chan struct{})
		close(waitCh)
		return true
	})
	
	// Clean up the wait queue
	m.waitQueue.Delete(resource)
}

// GetStats returns current lock statistics
func (m *Manager) GetStats() LockStats {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	activeLocks := 0
	m.locks.Range(func(key, value interface{}) bool {
		resLock := value.(*resourceLock)
		if resLock.holder != "" {
			activeLocks++
		}
		return true
	})
	
	stats := m.stats
	stats.ActiveLocks = activeLocks
	
	return stats
}

// recordAcquisition records a lock acquisition
func (m *Manager) recordAcquisition() {
	m.mu.Lock()
	m.stats.TotalAcquired++
	m.mu.Unlock()
}

// recordRelease records a lock release
func (m *Manager) recordRelease() {
	m.mu.Lock()
	m.stats.TotalReleased++
	m.mu.Unlock()
}

// recordTimeout records a lock timeout
func (m *Manager) recordTimeout() {
	m.mu.Lock()
	m.stats.TotalTimeouts++
	m.mu.Unlock()
}

// LockGuard provides RAII-style locking
type LockGuard struct {
	manager  *Manager
	resource string
	locked   bool
}

// NewLockGuard creates a new lock guard
func (m *Manager) NewLockGuard(resource string) (*LockGuard, error) {
	guard := &LockGuard{
		manager:  m,
		resource: resource,
	}
	
	if err := m.Lock(resource); err != nil {
		return nil, err
	}
	
	guard.locked = true
	return guard, nil
}

// Release releases the lock
func (g *LockGuard) Release() error {
	if !g.locked {
		return fmt.Errorf("lock already released")
	}
	
	g.locked = false
	return g.manager.Unlock(g.resource)
}

// SchemaLockManager provides schema-specific locking
type SchemaLockManager struct {
	*Manager
}

// NewSchemaLockManager creates a lock manager for schema operations
func NewSchemaLockManager() *SchemaLockManager {
	return &SchemaLockManager{
		Manager: NewManager(30*time.Second, 5*time.Minute),
	}
}

// LockEntitySchema locks an entity schema for modification
func (slm *SchemaLockManager) LockEntitySchema(entityType string) error {
	return slm.Lock(fmt.Sprintf("schema:entity:%s", entityType))
}

// UnlockEntitySchema unlocks an entity schema
func (slm *SchemaLockManager) UnlockEntitySchema(entityType string) error {
	return slm.Unlock(fmt.Sprintf("schema:entity:%s", entityType))
}

// LockRelationshipSchema locks a relationship schema for modification
func (slm *SchemaLockManager) LockRelationshipSchema(relationshipType string) error {
	return slm.Lock(fmt.Sprintf("schema:relationship:%s", relationshipType))
}

// UnlockRelationshipSchema unlocks a relationship schema
func (slm *SchemaLockManager) UnlockRelationshipSchema(relationshipType string) error {
	return slm.Unlock(fmt.Sprintf("schema:relationship:%s", relationshipType))
}

// EntityLockManager provides entity-specific locking
type EntityLockManager struct {
	*Manager
}

// NewEntityLockManager creates a lock manager for entity operations
func NewEntityLockManager() *EntityLockManager {
	return &EntityLockManager{
		Manager: NewManager(10*time.Second, 1*time.Minute),
	}
}

// LockEntity locks a specific entity
func (elm *EntityLockManager) LockEntity(entityType string, entityID uuid.UUID) error {
	return elm.Lock(fmt.Sprintf("entity:%s:%s", entityType, entityID.String()))
}

// UnlockEntity unlocks a specific entity
func (elm *EntityLockManager) UnlockEntity(entityType string, entityID uuid.UUID) error {
	return elm.Unlock(fmt.Sprintf("entity:%s:%s", entityType, entityID.String()))
}

// LockEntities locks multiple entities in a consistent order to prevent deadlocks
func (elm *EntityLockManager) LockEntities(entities []EntityRef) error {
	// Sort entities to prevent deadlocks
	sortedEntities := sortEntityRefs(entities)
	
	// Acquire locks in order
	locked := make([]EntityRef, 0, len(sortedEntities))
	for _, entity := range sortedEntities {
		if err := elm.LockEntity(entity.Type, entity.ID); err != nil {
			// Release already acquired locks
			for _, lockedEntity := range locked {
				elm.UnlockEntity(lockedEntity.Type, lockedEntity.ID)
			}
			return fmt.Errorf("failed to lock entity %s:%s: %w", entity.Type, entity.ID, err)
		}
		locked = append(locked, entity)
	}
	
	return nil
}

// UnlockEntities unlocks multiple entities
func (elm *EntityLockManager) UnlockEntities(entities []EntityRef) error {
	var firstErr error
	for _, entity := range entities {
		if err := elm.UnlockEntity(entity.Type, entity.ID); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// EntityRef represents a reference to an entity
type EntityRef struct {
	Type string
	ID   uuid.UUID
}

// sortEntityRefs sorts entity references to prevent deadlocks
func sortEntityRefs(refs []EntityRef) []EntityRef {
	sorted := make([]EntityRef, len(refs))
	copy(sorted, refs)
	
	// Sort by type first, then by ID
	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i].Type > sorted[j].Type ||
				(sorted[i].Type == sorted[j].Type && sorted[i].ID.String() > sorted[j].ID.String()) {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	
	return sorted
}

// DeadlockDetector detects potential deadlocks
type DeadlockDetector struct {
	manager      *Manager
	dependencies sync.Map // holder -> []waiting_for
}

// NewDeadlockDetector creates a new deadlock detector
func NewDeadlockDetector(manager *Manager) *DeadlockDetector {
	return &DeadlockDetector{
		manager: manager,
	}
}

// CheckDeadlock checks for potential deadlocks
func (dd *DeadlockDetector) CheckDeadlock(holder string, waitingFor []string) bool {
	// Simple cycle detection algorithm
	visited := make(map[string]bool)
	return dd.hasCycle(holder, waitingFor, visited)
}

// hasCycle performs DFS to detect cycles
func (dd *DeadlockDetector) hasCycle(current string, waitingFor []string, visited map[string]bool) bool {
	visited[current] = true
	
	for _, resource := range waitingFor {
		if visited[resource] {
			return true // Cycle detected
		}
		
		// Check if this resource has dependencies
		if deps, ok := dd.dependencies.Load(resource); ok {
			if dd.hasCycle(resource, deps.([]string), visited) {
				return true
			}
		}
	}
	
	return false
}