package cache

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/entropic/entropic/internal/models"
	"github.com/entropic/entropic/internal/store"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockPrimaryStore implements store.PrimaryStore for testing
type mockPrimaryStore struct {
	entitySchemas       map[string]*models.EntitySchema
	relationshipSchemas map[string]*models.RelationshipSchema
	mu                  sync.RWMutex
}

func newMockPrimaryStore() *mockPrimaryStore {
	return &mockPrimaryStore{
		entitySchemas:       make(map[string]*models.EntitySchema),
		relationshipSchemas: make(map[string]*models.RelationshipSchema),
	}
}

func (m *mockPrimaryStore) CreateEntitySchema(ctx context.Context, schema *models.EntitySchema) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entitySchemas[schema.EntityType] = schema
	return nil
}

func (m *mockPrimaryStore) GetEntitySchema(ctx context.Context, entityType string) (*models.EntitySchema, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	schema, ok := m.entitySchemas[entityType]
	if !ok {
		return nil, fmt.Errorf("entity schema not found: %s", entityType)
	}
	return schema, nil
}

func (m *mockPrimaryStore) UpdateEntitySchema(ctx context.Context, schema *models.EntitySchema) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entitySchemas[schema.EntityType] = schema
	return nil
}

func (m *mockPrimaryStore) DeleteEntitySchema(ctx context.Context, entityType string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.entitySchemas, entityType)
	return nil
}

func (m *mockPrimaryStore) ListEntitySchemas(ctx context.Context) ([]*models.EntitySchema, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	schemas := make([]*models.EntitySchema, 0, len(m.entitySchemas))
	for _, schema := range m.entitySchemas {
		schemas = append(schemas, schema)
	}
	return schemas, nil
}

func (m *mockPrimaryStore) CreateRelationshipSchema(ctx context.Context, schema *models.RelationshipSchema) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.relationshipSchemas[schema.RelationshipType] = schema
	return nil
}

func (m *mockPrimaryStore) GetRelationshipSchema(ctx context.Context, relationshipType string) (*models.RelationshipSchema, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	schema, ok := m.relationshipSchemas[relationshipType]
	if !ok {
		return nil, fmt.Errorf("relationship schema not found: %s", relationshipType)
	}
	return schema, nil
}

func (m *mockPrimaryStore) UpdateRelationshipSchema(ctx context.Context, schema *models.RelationshipSchema) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.relationshipSchemas[schema.RelationshipType] = schema
	return nil
}

func (m *mockPrimaryStore) DeleteRelationshipSchema(ctx context.Context, relationshipType string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.relationshipSchemas, relationshipType)
	return nil
}

func (m *mockPrimaryStore) ListRelationshipSchemas(ctx context.Context) ([]*models.RelationshipSchema, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	schemas := make([]*models.RelationshipSchema, 0, len(m.relationshipSchemas))
	for _, schema := range m.relationshipSchemas {
		schemas = append(schemas, schema)
	}
	return schemas, nil
}

// Implement remaining PrimaryStore methods with no-op or panic
func (m *mockPrimaryStore) CreateEntity(ctx context.Context, entity *models.Entity) error { panic("not implemented") }
func (m *mockPrimaryStore) GetEntity(ctx context.Context, entityType string, entityID uuid.UUID) (*models.Entity, error) { panic("not implemented") }
func (m *mockPrimaryStore) UpdateEntity(ctx context.Context, entity *models.Entity) error { panic("not implemented") }
func (m *mockPrimaryStore) DeleteEntity(ctx context.Context, entityType string, entityID uuid.UUID) error { panic("not implemented") }
func (m *mockPrimaryStore) ListEntities(ctx context.Context, entityType string, limit, offset int) ([]*models.Entity, error) { panic("not implemented") }
func (m *mockPrimaryStore) CreateRelation(ctx context.Context, relation *models.Relation) error { panic("not implemented") }
func (m *mockPrimaryStore) GetRelation(ctx context.Context, relationID uuid.UUID) (*models.Relation, error) { panic("not implemented") }
func (m *mockPrimaryStore) DeleteRelation(ctx context.Context, relationID uuid.UUID) error { panic("not implemented") }
func (m *mockPrimaryStore) GetRelationsByEntity(ctx context.Context, entityID uuid.UUID, relationTypes []string) ([]*models.Relation, error) { panic("not implemented") }
func (m *mockPrimaryStore) CheckURNExists(ctx context.Context, urn string) (bool, error) { panic("not implemented") }
func (m *mockPrimaryStore) BeginTx(ctx context.Context) (store.Transaction, error) { panic("not implemented") }
func (m *mockPrimaryStore) Close() error { return nil }
func (m *mockPrimaryStore) Ping(ctx context.Context) error { return nil }

// Test helper functions
func createTestEntitySchema(entityType string) *models.EntitySchema {
	return &models.EntitySchema{
		ID:         uuid.New(),
		EntityType: entityType,
		Properties: map[string]models.PropertyDefinition{
			"name": {
				Type:     "string",
				Required: true,
			},
			"email": {
				Type:     "string",
				Required: false,
			},
		},
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
}

func createTestRelationshipSchema(relationshipType, fromEntity, toEntity string) *models.RelationshipSchema {
	return &models.RelationshipSchema{
		ID:               uuid.New(),
		RelationshipType: relationshipType,
		FromEntityType:   fromEntity,
		ToEntityType:     toEntity,
		Properties: map[string]models.PropertyDefinition{
			"role": {
				Type:     "string",
				Required: false,
			},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

func TestCacheAwareManager_GetEntitySchema(t *testing.T) {
	ctx := context.Background()
	mockStore := newMockPrimaryStore()
	manager := NewCacheAwareManager(mockStore, 5*time.Minute)

	// Create test schema
	schema := createTestEntitySchema("user")
	err := mockStore.CreateEntitySchema(ctx, schema)
	require.NoError(t, err)

	tests := []struct {
		name       string
		cacheHit   bool
		entityType string
		wantError  bool
	}{
		{
			name:       "cache miss - loads from store",
			cacheHit:   false,
			entityType: "user",
			wantError:  false,
		},
		{
			name:       "cache hit - returns cached",
			cacheHit:   true,
			entityType: "user",
			wantError:  false,
		},
		{
			name:       "non-existent schema",
			cacheHit:   false,
			entityType: "nonexistent",
			wantError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.cacheHit {
				// Clear cache for cache miss test
				manager.InvalidateEntitySchema(tt.entityType)
			}

			retrieved, err := manager.GetEntitySchema(ctx, tt.entityType)
			if tt.wantError {
				assert.Error(t, err)
				assert.Nil(t, retrieved)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, retrieved)
				assert.Equal(t, tt.entityType, retrieved.EntityType)

				// Second call should hit cache
				cached, err := manager.GetEntitySchema(ctx, tt.entityType)
				assert.NoError(t, err)
				assert.Equal(t, retrieved, cached)
			}
		})
	}
}

func TestCacheAwareManager_GetRelationshipSchema(t *testing.T) {
	ctx := context.Background()
	mockStore := newMockPrimaryStore()
	manager := NewCacheAwareManager(mockStore, 5*time.Minute)

	// Create entity schemas first
	userSchema := createTestEntitySchema("user")
	err := mockStore.CreateEntitySchema(ctx, userSchema)
	require.NoError(t, err)

	orgSchema := createTestEntitySchema("organization")
	err = mockStore.CreateEntitySchema(ctx, orgSchema)
	require.NoError(t, err)

	// Create relationship schema
	relationSchema := createTestRelationshipSchema("member_of", "user", "organization")
	err = mockStore.CreateRelationshipSchema(ctx, relationSchema)
	require.NoError(t, err)

	// Test cache behavior
	retrieved, err := manager.GetRelationshipSchema(ctx, "member_of")
	require.NoError(t, err)
	assert.Equal(t, "member_of", retrieved.RelationshipType)

	// Second call should hit cache
	cached, err := manager.GetRelationshipSchema(ctx, "member_of")
	require.NoError(t, err)
	assert.Equal(t, retrieved, cached)

	// Test non-existent schema
	_, err = manager.GetRelationshipSchema(ctx, "nonexistent")
	assert.Error(t, err)
}

func TestCacheAwareManager_InvalidateEntitySchema(t *testing.T) {
	ctx := context.Background()
	mockStore := newMockPrimaryStore()
	manager := NewCacheAwareManager(mockStore, 5*time.Minute)

	// Create test schema
	schema := createTestEntitySchema("user")
	err := mockStore.CreateEntitySchema(ctx, schema)
	require.NoError(t, err)

	// Load into cache
	_, err = manager.GetEntitySchema(ctx, "user")
	require.NoError(t, err)

	// Verify cache contains the schema
	assert.True(t, manager.HasEntitySchema("user"))

	// Invalidate cache
	manager.InvalidateEntitySchema("user")

	// Verify cache no longer contains the schema
	assert.False(t, manager.HasEntitySchema("user"))
}

func TestCacheAwareManager_InvalidateRelationshipSchema(t *testing.T) {
	ctx := context.Background()
	mockStore := newMockPrimaryStore()
	manager := NewCacheAwareManager(mockStore, 5*time.Minute)

	// Create schemas
	userSchema := createTestEntitySchema("user")
	err := mockStore.CreateEntitySchema(ctx, userSchema)
	require.NoError(t, err)

	orgSchema := createTestEntitySchema("organization")
	err = mockStore.CreateEntitySchema(ctx, orgSchema)
	require.NoError(t, err)

	relationSchema := createTestRelationshipSchema("member_of", "user", "organization")
	err = mockStore.CreateRelationshipSchema(ctx, relationSchema)
	require.NoError(t, err)

	// Load into cache
	_, err = manager.GetRelationshipSchema(ctx, "member_of")
	require.NoError(t, err)

	// Verify cache contains the schema
	assert.True(t, manager.HasRelationshipSchema("member_of"))

	// Invalidate cache
	manager.InvalidateRelationshipSchema("member_of")

	// Verify cache no longer contains the schema
	assert.False(t, manager.HasRelationshipSchema("member_of"))
}

func TestCacheAwareManager_InvalidateAll(t *testing.T) {
	ctx := context.Background()
	mockStore := newMockPrimaryStore()
	manager := NewCacheAwareManager(mockStore, 5*time.Minute)

	// Create schemas
	userSchema := createTestEntitySchema("user")
	err := mockStore.CreateEntitySchema(ctx, userSchema)
	require.NoError(t, err)

	orgSchema := createTestEntitySchema("organization")
	err = mockStore.CreateEntitySchema(ctx, orgSchema)
	require.NoError(t, err)

	relationSchema := createTestRelationshipSchema("member_of", "user", "organization")
	err = mockStore.CreateRelationshipSchema(ctx, relationSchema)
	require.NoError(t, err)

	// Load schemas into cache
	_, err = manager.GetEntitySchema(ctx, "user")
	require.NoError(t, err)
	_, err = manager.GetEntitySchema(ctx, "organization")
	require.NoError(t, err)
	_, err = manager.GetRelationshipSchema(ctx, "member_of")
	require.NoError(t, err)

	// Verify cache contains schemas
	assert.True(t, manager.HasEntitySchema("user"))
	assert.True(t, manager.HasEntitySchema("organization"))
	assert.True(t, manager.HasRelationshipSchema("member_of"))

	// Invalidate all
	manager.InvalidateAll()

	// Verify cache is empty
	assert.False(t, manager.HasEntitySchema("user"))
	assert.False(t, manager.HasEntitySchema("organization"))
	assert.False(t, manager.HasRelationshipSchema("member_of"))
}

func TestCacheAwareManager_ConcurrentAccess(t *testing.T) {
	ctx := context.Background()
	mockStore := newMockPrimaryStore()
	manager := NewCacheAwareManager(mockStore, 5*time.Minute)

	// Create test schema
	schema := createTestEntitySchema("user")
	err := mockStore.CreateEntitySchema(ctx, schema)
	require.NoError(t, err)

	// Test concurrent access
	numGoroutines := 10
	results := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			_, err := manager.GetEntitySchema(ctx, "user")
			results <- err
		}()
	}

	// Collect results
	var errors []error
	for i := 0; i < numGoroutines; i++ {
		if err := <-results; err != nil {
			errors = append(errors, err)
		}
	}

	// All operations should succeed
	assert.Empty(t, errors, "Expected no errors in concurrent access")
}

func TestCacheAwareManager_TTLExpiration(t *testing.T) {
	ctx := context.Background()
	mockStore := newMockPrimaryStore()

	// Create cache manager with short TTL
	shortTTLManager := NewCacheAwareManager(mockStore, 100*time.Millisecond)

	// Create test schema
	schema := createTestEntitySchema("user")
	err := mockStore.CreateEntitySchema(ctx, schema)
	require.NoError(t, err)

	// Load into cache
	_, err = shortTTLManager.GetEntitySchema(ctx, "user")
	require.NoError(t, err)

	// Verify cache contains the schema
	assert.True(t, shortTTLManager.HasEntitySchema("user"))

	// Wait for TTL expiration
	time.Sleep(200 * time.Millisecond)

	// Cache should be empty after cleanup
	shortTTLManager.cleanup()
	assert.False(t, shortTTLManager.HasEntitySchema("user"))
}

func BenchmarkCacheAwareManager_GetEntitySchema(b *testing.B) {
	ctx := context.Background()
	mockStore := newMockPrimaryStore()
	manager := NewCacheAwareManager(mockStore, 5*time.Minute)

	// Create test schema
	schema := createTestEntitySchema("user")
	mockStore.CreateEntitySchema(ctx, schema)

	// Pre-load cache
	manager.GetEntitySchema(ctx, "user")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		manager.GetEntitySchema(ctx, "user")
	}
}

func BenchmarkCacheAwareManager_CacheMiss(b *testing.B) {
	ctx := context.Background()
	mockStore := newMockPrimaryStore()
	manager := NewCacheAwareManager(mockStore, 5*time.Minute)

	// Create test schema
	schema := createTestEntitySchema("user")
	mockStore.CreateEntitySchema(ctx, schema)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Clear cache before each iteration to force cache miss
		manager.InvalidateEntitySchema("user")
		manager.GetEntitySchema(ctx, "user")
	}
}
