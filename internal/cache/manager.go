package cache

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sumandas0/entropic/internal/models"
	"github.com/sumandas0/entropic/internal/store"
)

type Manager struct {
	
	entitySchemas       sync.Map 
	relationshipSchemas sync.Map 

	schemaTTL time.Duration

	primaryStore store.PrimaryStore

	hits   uint64
	misses uint64
	mu     sync.RWMutex
}

func NewManager(primaryStore store.PrimaryStore, schemaTTL time.Duration) *Manager {
	if schemaTTL <= 0 {
		schemaTTL = 5 * time.Minute 
	}

	return &Manager{
		primaryStore: primaryStore,
		schemaTTL:    schemaTTL,
	}
}

type cacheEntry struct {
	value      interface{}
	expiration time.Time
}

func (e *cacheEntry) isExpired() bool {
	return time.Now().After(e.expiration)
}

func (m *Manager) GetEntitySchema(ctx context.Context, entityType string) (*models.EntitySchema, error) {
	
	if cached, ok := m.entitySchemas.Load(entityType); ok {
		entry := cached.(*cacheEntry)
		if !entry.isExpired() {
			m.recordHit()
			return entry.value.(*models.EntitySchema), nil
		}
		
		m.entitySchemas.Delete(entityType)
	}

	m.recordMiss()

	schema, err := m.primaryStore.GetEntitySchema(ctx, entityType)
	if err != nil {
		return nil, err
	}

	m.SetEntitySchema(entityType, schema)

	return schema, nil
}

func (m *Manager) SetEntitySchema(entityType string, schema *models.EntitySchema) {
	entry := &cacheEntry{
		value:      schema,
		expiration: time.Now().Add(m.schemaTTL),
	}
	m.entitySchemas.Store(entityType, entry)
}

func (m *Manager) InvalidateEntitySchema(entityType string) {
	m.entitySchemas.Delete(entityType)
}

func (m *Manager) GetRelationshipSchema(ctx context.Context, relationshipType string) (*models.RelationshipSchema, error) {
	
	if cached, ok := m.relationshipSchemas.Load(relationshipType); ok {
		entry := cached.(*cacheEntry)
		if !entry.isExpired() {
			m.recordHit()
			return entry.value.(*models.RelationshipSchema), nil
		}
		
		m.relationshipSchemas.Delete(relationshipType)
	}

	m.recordMiss()

	schema, err := m.primaryStore.GetRelationshipSchema(ctx, relationshipType)
	if err != nil {
		return nil, err
	}

	m.SetRelationshipSchema(relationshipType, schema)

	return schema, nil
}

func (m *Manager) SetRelationshipSchema(relationshipType string, schema *models.RelationshipSchema) {
	entry := &cacheEntry{
		value:      schema,
		expiration: time.Now().Add(m.schemaTTL),
	}
	m.relationshipSchemas.Store(relationshipType, entry)
}

func (m *Manager) InvalidateRelationshipSchema(relationshipType string) {
	m.relationshipSchemas.Delete(relationshipType)
}

func (m *Manager) PreloadSchemas(ctx context.Context) error {
	
	entitySchemas, err := m.primaryStore.ListEntitySchemas(ctx)
	if err != nil {
		return fmt.Errorf("failed to preload entity schemas: %w", err)
	}

	for _, schema := range entitySchemas {
		m.SetEntitySchema(schema.EntityType, schema)
	}

	relationshipSchemas, err := m.primaryStore.ListRelationshipSchemas(ctx)
	if err != nil {
		return fmt.Errorf("failed to preload relationship schemas: %w", err)
	}

	for _, schema := range relationshipSchemas {
		m.SetRelationshipSchema(schema.RelationshipType, schema)
	}

	return nil
}

func (m *Manager) Clear() {
	m.entitySchemas.Range(func(key, value interface{}) bool {
		m.entitySchemas.Delete(key)
		return true
	})

	m.relationshipSchemas.Range(func(key, value interface{}) bool {
		m.relationshipSchemas.Delete(key)
		return true
	})

	m.mu.Lock()
	m.hits = 0
	m.misses = 0
	m.mu.Unlock()
}

func (m *Manager) CleanupExpired() {
	now := time.Now()

	m.entitySchemas.Range(func(key, value interface{}) bool {
		entry := value.(*cacheEntry)
		if now.After(entry.expiration) {
			m.entitySchemas.Delete(key)
		}
		return true
	})

	m.relationshipSchemas.Range(func(key, value interface{}) bool {
		entry := value.(*cacheEntry)
		if now.After(entry.expiration) {
			m.relationshipSchemas.Delete(key)
		}
		return true
	})
}

func (m *Manager) StartCleanupRoutine(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = time.Minute
	}

	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				m.CleanupExpired()
			case <-ctx.Done():
				return
			}
		}
	}()
}

func (m *Manager) Stats() CacheStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entityCount := 0
	m.entitySchemas.Range(func(_, _ interface{}) bool {
		entityCount++
		return true
	})

	relationshipCount := 0
	m.relationshipSchemas.Range(func(_, _ interface{}) bool {
		relationshipCount++
		return true
	})

	total := m.hits + m.misses
	hitRate := float64(0)
	if total > 0 {
		hitRate = float64(m.hits) / float64(total)
	}

	return CacheStats{
		Hits:                    m.hits,
		Misses:                  m.misses,
		HitRate:                 hitRate,
		EntitySchemaCount:       entityCount,
		RelationshipSchemaCount: relationshipCount,
	}
}

type CacheStats struct {
	Hits                    uint64
	Misses                  uint64
	HitRate                 float64
	EntitySchemaCount       int
	RelationshipSchemaCount int
}

func (m *Manager) recordHit() {
	m.mu.Lock()
	m.hits++
	m.mu.Unlock()
}

func (m *Manager) recordMiss() {
	m.mu.Lock()
	m.misses++
	m.mu.Unlock()
}

type SchemaChangeListener interface {
	OnEntitySchemaChange(entityType string, action string)
	OnRelationshipSchemaChange(relationshipType string, action string)
}

type SchemaChangeNotifier struct {
	listeners []SchemaChangeListener
	mu        sync.RWMutex
}

func NewSchemaChangeNotifier() *SchemaChangeNotifier {
	return &SchemaChangeNotifier{
		listeners: make([]SchemaChangeListener, 0),
	}
}

func (n *SchemaChangeNotifier) AddListener(listener SchemaChangeListener) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.listeners = append(n.listeners, listener)
}

func (n *SchemaChangeNotifier) NotifyEntitySchemaChange(entityType string, action string) {
	n.mu.RLock()
	defer n.mu.RUnlock()

	for _, listener := range n.listeners {
		go listener.OnEntitySchemaChange(entityType, action)
	}
}

func (n *SchemaChangeNotifier) NotifyRelationshipSchemaChange(relationshipType string, action string) {
	n.mu.RLock()
	defer n.mu.RUnlock()

	for _, listener := range n.listeners {
		go listener.OnRelationshipSchemaChange(relationshipType, action)
	}
}

type CacheAwareManager struct {
	*Manager
	notifier *SchemaChangeNotifier
}

func NewCacheAwareManager(primaryStore store.PrimaryStore, schemaTTL time.Duration) *CacheAwareManager {
	manager := NewManager(primaryStore, schemaTTL)
	notifier := NewSchemaChangeNotifier()

	cam := &CacheAwareManager{
		Manager:  manager,
		notifier: notifier,
	}

	notifier.AddListener(cam)

	return cam
}

func (cam *CacheAwareManager) OnEntitySchemaChange(entityType string, action string) {
	switch action {
	case "update", "delete":
		cam.InvalidateEntitySchema(entityType)
	}
}

func (cam *CacheAwareManager) OnRelationshipSchemaChange(relationshipType string, action string) {
	switch action {
	case "update", "delete":
		cam.InvalidateRelationshipSchema(relationshipType)
	}
}

func (cam *CacheAwareManager) GetNotifier() *SchemaChangeNotifier {
	return cam.notifier
}

func (cam *CacheAwareManager) HasEntitySchema(entityType string) bool {
	if cached, ok := cam.entitySchemas.Load(entityType); ok {
		entry := cached.(*cacheEntry)
		return !entry.isExpired()
	}
	return false
}

func (cam *CacheAwareManager) HasRelationshipSchema(relationshipType string) bool {
	if cached, ok := cam.relationshipSchemas.Load(relationshipType); ok {
		entry := cached.(*cacheEntry)
		return !entry.isExpired()
	}
	return false
}

func (cam *CacheAwareManager) InvalidateAll() {
	cam.Clear()
}

func (cam *CacheAwareManager) cleanup() {
	cam.CleanupExpired()
}
