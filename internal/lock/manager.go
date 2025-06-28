package lock

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/sumandas0/entropic/internal/integration"
	"github.com/sumandas0/entropic/internal/observability"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type Manager struct {
	locks     sync.Map
	waitQueue sync.Map

	defaultTimeout time.Duration
	maxWaitTime    time.Duration

	mu    sync.RWMutex
	stats LockStats
	
	obsManager *integration.ObservabilityManager
	logger     zerolog.Logger
	tracer     trace.Tracer
	tracing    *observability.TracingManager
}

type resourceLock struct {
	mu       sync.Mutex
	holder   string
	acquired time.Time
	timeout  time.Duration
}

type LockStats struct {
	ActiveLocks     int
	TotalAcquired   uint64
	TotalReleased   uint64
	TotalTimeouts   uint64
	TotalDeadlocks  uint64
	AverageWaitTime time.Duration
}

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

func (m *Manager) SetObservability(obsManager *integration.ObservabilityManager) {
	if obsManager != nil {
		m.obsManager = obsManager
		m.logger = obsManager.GetLogging().GetZerologLogger()
		m.tracer = obsManager.GetTracing().GetTracer()
		m.tracing = obsManager.GetTracing()
	}
}

func (m *Manager) Lock(resource string) error {
	return m.LockWithTimeout(resource, m.defaultTimeout)
}

func (m *Manager) LockWithTimeout(resource string, timeout time.Duration) error {
	var span trace.Span
	if m.tracer != nil {
		_, span = m.tracer.Start(context.Background(), "lock.acquire")
		defer span.End()
		span.SetAttributes(
			attribute.String("resource", resource),
			attribute.String("timeout", timeout.String()),
		)
	}
	
	holderID := uuid.New().String()
	startTime := time.Now()

	lockInterface, _ := m.locks.LoadOrStore(resource, &resourceLock{})
	resLock := lockInterface.(*resourceLock)

	acquired := make(chan bool, 1)
	go func() {
		resLock.mu.Lock()
		resLock.holder = holderID
		resLock.acquired = time.Now()
		resLock.timeout = timeout
		acquired <- true
	}()

	select {
	case <-acquired:
		waitTime := time.Since(startTime)
		if span != nil {
			span.SetAttributes(
				attribute.Bool("acquired", true),
				attribute.String("wait_time", waitTime.String()),
			)
		}
		
		if m.logger.GetLevel() != zerolog.Disabled {
			m.logger.Debug().
				Str("resource", resource).
				Dur("wait_time", waitTime).
				Str("holder_id", holderID).
				Msg("Lock acquired")
		}
		
		m.recordAcquisition()

		go m.monitorTimeout(resource, holderID, timeout)

		return nil

	case <-time.After(m.maxWaitTime):
		if span != nil {
			span.SetAttributes(attribute.Bool("acquired", false))
			if m.tracing != nil {
				m.tracing.SetSpanError(span, fmt.Errorf("lock acquisition timeout"))
			}
		}
		return fmt.Errorf("failed to acquire lock on resource %s: timeout", resource)
	}
}

func (m *Manager) TryLock(resource string, timeout time.Duration) error {
	holderID := uuid.New().String()

	lockInterface, _ := m.locks.LoadOrStore(resource, &resourceLock{})
	resLock := lockInterface.(*resourceLock)

	if !resLock.mu.TryLock() {
		return fmt.Errorf("resource %s is already locked", resource)
	}

	resLock.holder = holderID
	resLock.acquired = time.Now()
	resLock.timeout = timeout

	m.recordAcquisition()

	go m.monitorTimeout(resource, holderID, timeout)

	return nil
}

func (m *Manager) Unlock(resource string) error {
	var span trace.Span
	if m.tracer != nil {
		_, span = m.tracer.Start(context.Background(), "lock.release")
		defer span.End()
		span.SetAttributes(attribute.String("resource", resource))
	}
	
	lockInterface, ok := m.locks.Load(resource)
	if !ok {
		err := fmt.Errorf("no lock found for resource %s", resource)
		if m.tracing != nil {
			m.tracing.SetSpanError(span, err)
		}
		return err
	}

	resLock := lockInterface.(*resourceLock)
	holdTime := time.Since(resLock.acquired)
	resLock.holder = ""
	resLock.mu.Unlock()

	if m.logger.GetLevel() != zerolog.Disabled {
		m.logger.Debug().
			Str("resource", resource).
			Dur("hold_time", holdTime).
			Msg("Lock released")
	}

	m.recordRelease()

	m.notifyWaiters(resource)

	return nil
}

func (m *Manager) IsLocked(resource string) bool {
	lockInterface, ok := m.locks.Load(resource)
	if !ok {
		return false
	}

	resLock := lockInterface.(*resourceLock)

	if resLock.mu.TryLock() {
		resLock.mu.Unlock()
		return false
	}
	return true
}

func (m *Manager) WaitForUnlock(ctx context.Context, resource string) error {
	if !m.IsLocked(resource) {
		return nil
	}

	waitCh := make(chan struct{})

	queueInterface, _ := m.waitQueue.LoadOrStore(resource, &sync.Map{})
	queue := queueInterface.(*sync.Map)
	queueID := uuid.New().String()
	queue.Store(queueID, waitCh)

	defer func() {
		queue.Delete(queueID)
	}()

	select {
	case <-waitCh:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *Manager) monitorTimeout(resource, holderID string, timeout time.Duration) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	<-timer.C

	lockInterface, ok := m.locks.Load(resource)
	if !ok {
		return
	}

	resLock := lockInterface.(*resourceLock)
	if resLock.holder == holderID {
		if m.logger.GetLevel() != zerolog.Disabled {
			m.logger.Warn().
				Str("resource", resource).
				Str("holder_id", holderID).
				Dur("timeout", timeout).
				Msg("Lock timeout - forcibly releasing")
		}

		resLock.holder = ""
		resLock.mu.Unlock()

		m.recordTimeout()
		m.notifyWaiters(resource)
	}
}

func (m *Manager) notifyWaiters(resource string) {
	queueInterface, ok := m.waitQueue.Load(resource)
	if !ok {
		return
	}

	queue := queueInterface.(*sync.Map)
	queue.Range(func(key, value any) bool {
		waitCh := value.(chan struct{})
		close(waitCh)
		return true
	})

	m.waitQueue.Delete(resource)
}

func (m *Manager) GetStats() LockStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	activeLocks := 0
	m.locks.Range(func(key, value any) bool {
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

func (m *Manager) recordAcquisition() {
	m.mu.Lock()
	m.stats.TotalAcquired++
	m.mu.Unlock()
}

func (m *Manager) recordRelease() {
	m.mu.Lock()
	m.stats.TotalReleased++
	m.mu.Unlock()
}

func (m *Manager) recordTimeout() {
	m.mu.Lock()
	m.stats.TotalTimeouts++
	m.mu.Unlock()
}

type LockGuard struct {
	manager  *Manager
	resource string
	locked   bool
}

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

func (g *LockGuard) Release() error {
	if !g.locked {
		return fmt.Errorf("lock already released")
	}

	g.locked = false
	return g.manager.Unlock(g.resource)
}

type SchemaLockManager struct {
	*Manager
}

func NewSchemaLockManager() *SchemaLockManager {
	return &SchemaLockManager{
		Manager: NewManager(30*time.Second, 5*time.Minute),
	}
}

func (slm *SchemaLockManager) SetObservability(obsManager *integration.ObservabilityManager) {
	slm.Manager.SetObservability(obsManager)
}

func (slm *SchemaLockManager) LockEntitySchema(entityType string) error {
	return slm.Lock(fmt.Sprintf("schema:entity:%s", entityType))
}

func (slm *SchemaLockManager) UnlockEntitySchema(entityType string) error {
	return slm.Unlock(fmt.Sprintf("schema:entity:%s", entityType))
}

func (slm *SchemaLockManager) LockRelationshipSchema(relationshipType string) error {
	return slm.Lock(fmt.Sprintf("schema:relationship:%s", relationshipType))
}

func (slm *SchemaLockManager) UnlockRelationshipSchema(relationshipType string) error {
	return slm.Unlock(fmt.Sprintf("schema:relationship:%s", relationshipType))
}

type EntityLockManager struct {
	*Manager
}

func NewEntityLockManager() *EntityLockManager {
	return &EntityLockManager{
		Manager: NewManager(10*time.Second, 1*time.Minute),
	}
}

func (elm *EntityLockManager) SetObservability(obsManager *integration.ObservabilityManager) {
	elm.Manager.SetObservability(obsManager)
}

func (elm *EntityLockManager) LockEntity(entityType string, entityID uuid.UUID) error {
	return elm.Lock(fmt.Sprintf("entity:%s:%s", entityType, entityID.String()))
}

func (elm *EntityLockManager) UnlockEntity(entityType string, entityID uuid.UUID) error {
	return elm.Unlock(fmt.Sprintf("entity:%s:%s", entityType, entityID.String()))
}

func (elm *EntityLockManager) LockEntities(entities []EntityRef) error {

	sortedEntities := sortEntityRefs(entities)

	locked := make([]EntityRef, 0, len(sortedEntities))
	for _, entity := range sortedEntities {
		if err := elm.LockEntity(entity.Type, entity.ID); err != nil {

			for _, lockedEntity := range locked {
				elm.UnlockEntity(lockedEntity.Type, lockedEntity.ID)
			}
			return fmt.Errorf("failed to lock entity %s:%s: %w", entity.Type, entity.ID, err)
		}
		locked = append(locked, entity)
	}

	return nil
}

func (elm *EntityLockManager) UnlockEntities(entities []EntityRef) error {
	var firstErr error
	for _, entity := range entities {
		if err := elm.UnlockEntity(entity.Type, entity.ID); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

type EntityRef struct {
	Type string
	ID   uuid.UUID
}

func sortEntityRefs(refs []EntityRef) []EntityRef {
	sorted := make([]EntityRef, len(refs))
	copy(sorted, refs)

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

type DeadlockDetector struct {
	manager      *Manager
	dependencies sync.Map
}

func NewDeadlockDetector(manager *Manager) *DeadlockDetector {
	return &DeadlockDetector{
		manager: manager,
	}
}

func (dd *DeadlockDetector) CheckDeadlock(holder string, waitingFor []string) bool {

	visited := make(map[string]bool)
	return dd.hasCycle(holder, waitingFor, visited)
}

func (dd *DeadlockDetector) hasCycle(current string, waitingFor []string, visited map[string]bool) bool {
	visited[current] = true

	for _, resource := range waitingFor {
		if visited[resource] {
			return true
		}

		if deps, ok := dd.dependencies.Load(resource); ok {
			if dd.hasCycle(resource, deps.([]string), visited) {
				return true
			}
		}
	}

	return false
}
