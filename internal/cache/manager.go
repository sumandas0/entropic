package cache

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sumandas0/entropic/internal/models"
	"github.com/sumandas0/entropic/internal/store"
)

// Manager provides thread-safe caching for schemas and other frequently accessed data
type Manager struct {
	// Schema caches
	entitySchemas       sync.Map // entity_type -> *models.EntitySchema
	relationshipSchemas sync.Map // relationship_type -> *models.RelationshipSchema

	// TTL configuration
	schemaTTL time.Duration

	// Lazy loading
	primaryStore store.PrimaryStore

	// Statistics
	hits   uint64
	misses uint64
	mu     sync.RWMutex
}

// NewManager creates a new cache manager
func NewManager(primaryStore store.PrimaryStore, schemaTTL time.Duration) *Manager {
	if schemaTTL <= 0 {
		schemaTTL = 5 * time.Minute // Default TTL
	}

	return &Manager{
		primaryStore: primaryStore,
		schemaTTL:    schemaTTL,
	}
}

// cacheEntry wraps a cached value with expiration time
type cacheEntry struct {
	value      interface{}
	expiration time.Time
}

// isExpired checks if the cache entry is expired
func (e *cacheEntry) isExpired() bool {
	return time.Now().After(e.expiration)
}

// GetEntitySchema retrieves an entity schema from cache or loads it from store
func (m *Manager) GetEntitySchema(ctx context.Context, entityType string) (*models.EntitySchema, error) {
	// Try to get from cache
	if cached, ok := m.entitySchemas.Load(entityType); ok {
		entry := cached.(*cacheEntry)
		if !entry.isExpired() {
			m.recordHit()
			return entry.value.(*models.EntitySchema), nil
		}
		// Remove expired entry
		m.entitySchemas.Delete(entityType)
	}

	m.recordMiss()

	// Load from store
	schema, err := m.primaryStore.GetEntitySchema(ctx, entityType)
	if err != nil {
		return nil, err
	}

	// Cache the result
	m.SetEntitySchema(entityType, schema)

	return schema, nil
}

// SetEntitySchema adds or updates an entity schema in the cache
func (m *Manager) SetEntitySchema(entityType string, schema *models.EntitySchema) {
	entry := &cacheEntry{
		value:      schema,
		expiration: time.Now().Add(m.schemaTTL),
	}
	m.entitySchemas.Store(entityType, entry)
}

// InvalidateEntitySchema removes an entity schema from the cache
func (m *Manager) InvalidateEntitySchema(entityType string) {
	m.entitySchemas.Delete(entityType)
}

// GetRelationshipSchema retrieves a relationship schema from cache or loads it from store
func (m *Manager) GetRelationshipSchema(ctx context.Context, relationshipType string) (*models.RelationshipSchema, error) {
	// Try to get from cache
	if cached, ok := m.relationshipSchemas.Load(relationshipType); ok {
		entry := cached.(*cacheEntry)
		if !entry.isExpired() {
			m.recordHit()
			return entry.value.(*models.RelationshipSchema), nil
		}
		// Remove expired entry
		m.relationshipSchemas.Delete(relationshipType)
	}

	m.recordMiss()

	// Load from store
	schema, err := m.primaryStore.GetRelationshipSchema(ctx, relationshipType)
	if err != nil {
		return nil, err
	}

	// Cache the result
	m.SetRelationshipSchema(relationshipType, schema)

	return schema, nil
}

// SetRelationshipSchema adds or updates a relationship schema in the cache
func (m *Manager) SetRelationshipSchema(relationshipType string, schema *models.RelationshipSchema) {
	entry := &cacheEntry{
		value:      schema,
		expiration: time.Now().Add(m.schemaTTL),
	}
	m.relationshipSchemas.Store(relationshipType, entry)
}

// InvalidateRelationshipSchema removes a relationship schema from the cache
func (m *Manager) InvalidateRelationshipSchema(relationshipType string) {
	m.relationshipSchemas.Delete(relationshipType)
}

// PreloadSchemas loads all schemas into cache
func (m *Manager) PreloadSchemas(ctx context.Context) error {
	// Load entity schemas
	entitySchemas, err := m.primaryStore.ListEntitySchemas(ctx)
	if err != nil {
		return fmt.Errorf("failed to preload entity schemas: %w", err)
	}

	for _, schema := range entitySchemas {
		m.SetEntitySchema(schema.EntityType, schema)
	}

	// Load relationship schemas
	relationshipSchemas, err := m.primaryStore.ListRelationshipSchemas(ctx)
	if err != nil {
		return fmt.Errorf("failed to preload relationship schemas: %w", err)
	}

	for _, schema := range relationshipSchemas {
		m.SetRelationshipSchema(schema.RelationshipType, schema)
	}

	return nil
}

// Clear removes all entries from the cache
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

// CleanupExpired removes all expired entries from the cache
func (m *Manager) CleanupExpired() {
	now := time.Now()

	// Clean entity schemas
	m.entitySchemas.Range(func(key, value interface{}) bool {
		entry := value.(*cacheEntry)
		if now.After(entry.expiration) {
			m.entitySchemas.Delete(key)
		}
		return true
	})

	// Clean relationship schemas
	m.relationshipSchemas.Range(func(key, value interface{}) bool {
		entry := value.(*cacheEntry)
		if now.After(entry.expiration) {
			m.relationshipSchemas.Delete(key)
		}
		return true
	})
}

// StartCleanupRoutine starts a background routine to clean expired entries
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

// Stats returns cache statistics
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

// CacheStats holds cache statistics
type CacheStats struct {
	Hits                    uint64
	Misses                  uint64
	HitRate                 float64
	EntitySchemaCount       int
	RelationshipSchemaCount int
}

// recordHit increments the hit counter
func (m *Manager) recordHit() {
	m.mu.Lock()
	m.hits++
	m.mu.Unlock()
}

// recordMiss increments the miss counter
func (m *Manager) recordMiss() {
	m.mu.Lock()
	m.misses++
	m.mu.Unlock()
}

// SchemaChangeListener can be implemented to listen for schema changes
type SchemaChangeListener interface {
	OnEntitySchemaChange(entityType string, action string)
	OnRelationshipSchemaChange(relationshipType string, action string)
}

// SchemaChangeNotifier manages schema change notifications
type SchemaChangeNotifier struct {
	listeners []SchemaChangeListener
	mu        sync.RWMutex
}

// NewSchemaChangeNotifier creates a new schema change notifier
func NewSchemaChangeNotifier() *SchemaChangeNotifier {
	return &SchemaChangeNotifier{
		listeners: make([]SchemaChangeListener, 0),
	}
}

// AddListener adds a schema change listener
func (n *SchemaChangeNotifier) AddListener(listener SchemaChangeListener) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.listeners = append(n.listeners, listener)
}

// NotifyEntitySchemaChange notifies all listeners of an entity schema change
func (n *SchemaChangeNotifier) NotifyEntitySchemaChange(entityType string, action string) {
	n.mu.RLock()
	defer n.mu.RUnlock()

	for _, listener := range n.listeners {
		go listener.OnEntitySchemaChange(entityType, action)
	}
}

// NotifyRelationshipSchemaChange notifies all listeners of a relationship schema change
func (n *SchemaChangeNotifier) NotifyRelationshipSchemaChange(relationshipType string, action string) {
	n.mu.RLock()
	defer n.mu.RUnlock()

	for _, listener := range n.listeners {
		go listener.OnRelationshipSchemaChange(relationshipType, action)
	}
}

// CacheAwareManager extends Manager with schema change listener support
type CacheAwareManager struct {
	*Manager
	notifier *SchemaChangeNotifier
}

// NewCacheAwareManager creates a cache manager that listens for schema changes
func NewCacheAwareManager(primaryStore store.PrimaryStore, schemaTTL time.Duration) *CacheAwareManager {
	manager := NewManager(primaryStore, schemaTTL)
	notifier := NewSchemaChangeNotifier()

	cam := &CacheAwareManager{
		Manager:  manager,
		notifier: notifier,
	}

	// Register itself as a listener
	notifier.AddListener(cam)

	return cam
}

// OnEntitySchemaChange handles entity schema change notifications
func (cam *CacheAwareManager) OnEntitySchemaChange(entityType string, action string) {
	switch action {
	case "update", "delete":
		cam.InvalidateEntitySchema(entityType)
	}
}

// OnRelationshipSchemaChange handles relationship schema change notifications
func (cam *CacheAwareManager) OnRelationshipSchemaChange(relationshipType string, action string) {
	switch action {
	case "update", "delete":
		cam.InvalidateRelationshipSchema(relationshipType)
	}
}

// GetNotifier returns the schema change notifier
func (cam *CacheAwareManager) GetNotifier() *SchemaChangeNotifier {
	return cam.notifier
}

// HasEntitySchema checks if an entity schema is in the cache (not expired)
func (cam *CacheAwareManager) HasEntitySchema(entityType string) bool {
	if cached, ok := cam.entitySchemas.Load(entityType); ok {
		entry := cached.(*cacheEntry)
		return !entry.isExpired()
	}
	return false
}

// HasRelationshipSchema checks if a relationship schema is in the cache (not expired)
func (cam *CacheAwareManager) HasRelationshipSchema(relationshipType string) bool {
	if cached, ok := cam.relationshipSchemas.Load(relationshipType); ok {
		entry := cached.(*cacheEntry)
		return !entry.isExpired()
	}
	return false
}

// InvalidateAll removes all entries from the cache
func (cam *CacheAwareManager) InvalidateAll() {
	cam.Clear()
}

// cleanup is an alias for CleanupExpired for test compatibility
func (cam *CacheAwareManager) cleanup() {
	cam.CleanupExpired()
}
