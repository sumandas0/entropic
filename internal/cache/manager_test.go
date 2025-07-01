package cache

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/sumandas0/entropic/internal/models"
	"github.com/sumandas0/entropic/internal/store/postgres"
	"github.com/sumandas0/entropic/internal/store/testutils"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	testContainer *testutils.PostgresTestContainer
	testStore     *postgres.PostgresStore
)

func TestMain(m *testing.M) {
	var err error
	testContainer, err = testutils.SetupTestPostgres()
	if err != nil {
		panic(err)
	}

	testStore, err = postgres.NewPostgresStore(testContainer.URL)
	if err != nil {
		testContainer.Cleanup()
		panic(err)
	}

	ctx := context.Background()
	migrator := postgres.NewMigrator(testStore.GetPool())
	if err := migrator.Run(ctx); err != nil {
		testStore.Close()
		testContainer.Cleanup()
		panic(err)
	}

	code := m.Run()

	testStore.Close()
	testContainer.Cleanup()

	if code != 0 {
		panic("tests failed")
	}
}

func cleanupDatabase(t *testing.T) {
	ctx := context.Background()
	
	// Clean up all data between tests
	_, err := testStore.GetPool().Exec(ctx, "TRUNCATE entities, relations, entity_schemas, relationship_schemas CASCADE")
	require.NoError(t, err)
}

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
			"score": {
				Type:     "number",
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
		Cardinality: models.ManyToMany,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

func TestCacheAwareManager_GetEntitySchema(t *testing.T) {
	cleanupDatabase(t)
	ctx := context.Background()
	manager := NewCacheAwareManager(testStore, 5*time.Minute)

	schema := createTestEntitySchema("user")
	err := testStore.CreateEntitySchema(ctx, schema)
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

				// Verify cache hit by checking it returns the same instance
				cached, err := manager.GetEntitySchema(ctx, tt.entityType)
				assert.NoError(t, err)
				assert.Equal(t, retrieved.ID, cached.ID)
				assert.Equal(t, retrieved.EntityType, cached.EntityType)
			}
		})
	}
}

func TestCacheAwareManager_GetRelationshipSchema(t *testing.T) {
	cleanupDatabase(t)
	ctx := context.Background()
	manager := NewCacheAwareManager(testStore, 5*time.Minute)

	userSchema := createTestEntitySchema("user")
	err := testStore.CreateEntitySchema(ctx, userSchema)
	require.NoError(t, err)

	orgSchema := createTestEntitySchema("organization")
	err = testStore.CreateEntitySchema(ctx, orgSchema)
	require.NoError(t, err)

	relationSchema := createTestRelationshipSchema("member_of", "user", "organization")
	err = testStore.CreateRelationshipSchema(ctx, relationSchema)
	require.NoError(t, err)

	retrieved, err := manager.GetRelationshipSchema(ctx, "member_of")
	require.NoError(t, err)
	assert.Equal(t, "member_of", retrieved.RelationshipType)

	cached, err := manager.GetRelationshipSchema(ctx, "member_of")
	require.NoError(t, err)
	assert.Equal(t, retrieved, cached)

	_, err = manager.GetRelationshipSchema(ctx, "nonexistent")
	assert.Error(t, err)
}

func TestCacheAwareManager_InvalidateEntitySchema(t *testing.T) {
	cleanupDatabase(t)
	ctx := context.Background()
	manager := NewCacheAwareManager(testStore, 5*time.Minute)

	schema := createTestEntitySchema("user")
	err := testStore.CreateEntitySchema(ctx, schema)
	require.NoError(t, err)

	_, err = manager.GetEntitySchema(ctx, "user")
	require.NoError(t, err)

	assert.True(t, manager.HasEntitySchema("user"))

	manager.InvalidateEntitySchema("user")

	assert.False(t, manager.HasEntitySchema("user"))
}

func TestCacheAwareManager_InvalidateRelationshipSchema(t *testing.T) {
	cleanupDatabase(t)
	ctx := context.Background()
	manager := NewCacheAwareManager(testStore, 5*time.Minute)

	userSchema := createTestEntitySchema("user")
	err := testStore.CreateEntitySchema(ctx, userSchema)
	require.NoError(t, err)

	orgSchema := createTestEntitySchema("organization")
	err = testStore.CreateEntitySchema(ctx, orgSchema)
	require.NoError(t, err)

	relationSchema := createTestRelationshipSchema("member_of", "user", "organization")
	err = testStore.CreateRelationshipSchema(ctx, relationSchema)
	require.NoError(t, err)

	_, err = manager.GetRelationshipSchema(ctx, "member_of")
	require.NoError(t, err)

	assert.True(t, manager.HasRelationshipSchema("member_of"))

	manager.InvalidateRelationshipSchema("member_of")

	assert.False(t, manager.HasRelationshipSchema("member_of"))
}

func TestCacheAwareManager_InvalidateAll(t *testing.T) {
	cleanupDatabase(t)
	ctx := context.Background()
	manager := NewCacheAwareManager(testStore, 5*time.Minute)

	userSchema := createTestEntitySchema("user")
	err := testStore.CreateEntitySchema(ctx, userSchema)
	require.NoError(t, err)

	orgSchema := createTestEntitySchema("organization")
	err = testStore.CreateEntitySchema(ctx, orgSchema)
	require.NoError(t, err)

	relationSchema := createTestRelationshipSchema("member_of", "user", "organization")
	err = testStore.CreateRelationshipSchema(ctx, relationSchema)
	require.NoError(t, err)

	_, err = manager.GetEntitySchema(ctx, "user")
	require.NoError(t, err)
	_, err = manager.GetEntitySchema(ctx, "organization")
	require.NoError(t, err)
	_, err = manager.GetRelationshipSchema(ctx, "member_of")
	require.NoError(t, err)

	assert.True(t, manager.HasEntitySchema("user"))
	assert.True(t, manager.HasEntitySchema("organization"))
	assert.True(t, manager.HasRelationshipSchema("member_of"))

	manager.InvalidateAll()

	assert.False(t, manager.HasEntitySchema("user"))
	assert.False(t, manager.HasEntitySchema("organization"))
	assert.False(t, manager.HasRelationshipSchema("member_of"))
}

func TestCacheAwareManager_ConcurrentAccess(t *testing.T) {
	cleanupDatabase(t)
	ctx := context.Background()
	manager := NewCacheAwareManager(testStore, 5*time.Minute)

	schema := createTestEntitySchema("user")
	err := testStore.CreateEntitySchema(ctx, schema)
	require.NoError(t, err)

	numGoroutines := 10
	results := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			_, err := manager.GetEntitySchema(ctx, "user")
			results <- err
		}()
	}

	var errors []error
	for i := 0; i < numGoroutines; i++ {
		if err := <-results; err != nil {
			errors = append(errors, err)
		}
	}

	assert.Empty(t, errors, "Expected no errors in concurrent access")
}

func TestCacheAwareManager_TTLExpiration(t *testing.T) {
	cleanupDatabase(t)
	ctx := context.Background()

	shortTTLManager := NewCacheAwareManager(testStore, 100*time.Millisecond)

	schema := createTestEntitySchema("user")
	err := testStore.CreateEntitySchema(ctx, schema)
	require.NoError(t, err)

	_, err = shortTTLManager.GetEntitySchema(ctx, "user")
	require.NoError(t, err)

	assert.True(t, shortTTLManager.HasEntitySchema("user"))

	time.Sleep(200 * time.Millisecond)

	shortTTLManager.cleanup()
	assert.False(t, shortTTLManager.HasEntitySchema("user"))
}

func TestCacheAwareManager_CacheWarming(t *testing.T) {
	cleanupDatabase(t)
	ctx := context.Background()

	// Create multiple schemas
	for i := 0; i < 5; i++ {
		entitySchema := createTestEntitySchema(fmt.Sprintf("entity_%d", i))
		err := testStore.CreateEntitySchema(ctx, entitySchema)
		require.NoError(t, err)
	}

	// Create manager and warm cache
	manager := NewCacheAwareManager(testStore, 5*time.Minute)

	// Pre-load schemas into cache
	err := manager.PreloadSchemas(ctx)
	require.NoError(t, err)

	// Verify all schemas are cached
	for i := 0; i < 5; i++ {
		assert.True(t, manager.HasEntitySchema(fmt.Sprintf("entity_%d", i)))
	}
}

func TestCacheAwareManager_ConcurrentUpdates(t *testing.T) {
	cleanupDatabase(t)
	ctx := context.Background()
	manager := NewCacheAwareManager(testStore, 5*time.Minute)

	schema := createTestEntitySchema("user")
	err := testStore.CreateEntitySchema(ctx, schema)
	require.NoError(t, err)

	// Pre-warm cache
	_, err = manager.GetEntitySchema(ctx, "user")
	require.NoError(t, err)

	var wg sync.WaitGroup
	numGoroutines := 10

	// Concurrent updates
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			
			if idx%2 == 0 {
				// Even goroutines: invalidate and re-fetch
				manager.InvalidateEntitySchema("user")
				_, err := manager.GetEntitySchema(ctx, "user")
				assert.NoError(t, err)
			} else {
				// Odd goroutines: just fetch
				_, err := manager.GetEntitySchema(ctx, "user")
				assert.NoError(t, err)
			}
		}(i)
	}

	wg.Wait()
}

func TestCacheAwareManager_SchemaUpdate(t *testing.T) {
	cleanupDatabase(t)
	ctx := context.Background()
	manager := NewCacheAwareManager(testStore, 5*time.Minute)

	// Create initial schema
	schema := createTestEntitySchema("user")
	err := testStore.CreateEntitySchema(ctx, schema)
	require.NoError(t, err)

	// Cache it
	cached1, err := manager.GetEntitySchema(ctx, "user")
	require.NoError(t, err)

	// Update schema in store
	schema.Properties["age"] = models.PropertyDefinition{
		Type:     "number",
		Required: true,
	}
	err = testStore.UpdateEntitySchema(ctx, schema)
	require.NoError(t, err)
	
	// Simulate schema change notification
	manager.OnEntitySchemaChange("user", "update")

	// Verify cache was invalidated and new version is fetched
	cached2, err := manager.GetEntitySchema(ctx, "user")
	require.NoError(t, err)
	assert.NotEqual(t, cached1, cached2)
	assert.Contains(t, cached2.Properties, "age")
}

func TestCacheAwareManager_MemoryLeakPrevention(t *testing.T) {
	cleanupDatabase(t)
	ctx := context.Background()
	
	// Create manager with very short TTL
	manager := NewCacheAwareManager(testStore, 50*time.Millisecond)

	// Create and cache many schemas
	for i := 0; i < 100; i++ {
		schema := createTestEntitySchema(fmt.Sprintf("entity_%d", i))
		err := testStore.CreateEntitySchema(ctx, schema)
		require.NoError(t, err)
		
		_, err = manager.GetEntitySchema(ctx, schema.EntityType)
		require.NoError(t, err)
	}

	// Wait for TTL to expire
	time.Sleep(100 * time.Millisecond)
	
	// Force cleanup
	manager.cleanup()

	// Verify all entries are removed
	for i := 0; i < 100; i++ {
		assert.False(t, manager.HasEntitySchema(fmt.Sprintf("entity_%d", i)))
	}
}

func TestCacheAwareManager_CacheStatistics(t *testing.T) {
	cleanupDatabase(t)
	ctx := context.Background()
	manager := NewCacheAwareManager(testStore, 5*time.Minute)

	// Create test data
	schema := createTestEntitySchema("user")
	err := testStore.CreateEntitySchema(ctx, schema)
	require.NoError(t, err)

	// Test cache miss
	_, err = manager.GetEntitySchema(ctx, "user")
	require.NoError(t, err)

	// Test cache hits
	for i := 0; i < 5; i++ {
		_, err = manager.GetEntitySchema(ctx, "user")
		require.NoError(t, err)
	}

	// Test non-existent schema
	_, err = manager.GetEntitySchema(ctx, "nonexistent")
	assert.Error(t, err)

	// Verify cache behavior
	assert.True(t, manager.HasEntitySchema("user"))
	assert.False(t, manager.HasEntitySchema("nonexistent"))
}

func BenchmarkCacheAwareManager_GetEntitySchema(b *testing.B) {
	ctx := context.Background()
	manager := NewCacheAwareManager(testStore, 5*time.Minute)

	schema := createTestEntitySchema("bench_user")
	testStore.CreateEntitySchema(ctx, schema)

	// Pre-warm cache
	manager.GetEntitySchema(ctx, "bench_user")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		manager.GetEntitySchema(ctx, "bench_user")
	}
}

func BenchmarkCacheAwareManager_CacheMiss(b *testing.B) {
	ctx := context.Background()
	manager := NewCacheAwareManager(testStore, 5*time.Minute)

	schema := createTestEntitySchema("bench_miss_user")
	testStore.CreateEntitySchema(ctx, schema)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		manager.InvalidateEntitySchema("bench_miss_user")
		manager.GetEntitySchema(ctx, "bench_miss_user")
	}
}

func BenchmarkCacheAwareManager_ConcurrentAccess(b *testing.B) {
	ctx := context.Background()
	manager := NewCacheAwareManager(testStore, 5*time.Minute)

	schema := createTestEntitySchema("bench_concurrent")
	testStore.CreateEntitySchema(ctx, schema)

	// Pre-warm cache
	manager.GetEntitySchema(ctx, "bench_concurrent")

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			manager.GetEntitySchema(ctx, "bench_concurrent")
		}
	})
}