package cache

import (
	"context"
	"testing"
	"time"

	"github.com/entropic/entropic/internal/models"
	"github.com/entropic/entropic/tests/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCacheAwareManager_GetEntitySchema(t *testing.T) {
	ctx := context.Background()
	env := testhelpers.SetupTestEnvironment(t, ctx)
	defer env.Cleanup(ctx)

	// Create test schema
	schema := testhelpers.CreateTestEntitySchema("user")
	err := env.PrimaryStore.CreateEntitySchema(ctx, schema)
	require.NoError(t, err)

	tests := []struct {
		name      string
		cacheHit  bool
		entityType string
		wantError bool
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
				env.CacheManager.InvalidateEntitySchema(tt.entityType)
			}

			retrieved, err := env.CacheManager.GetEntitySchema(ctx, tt.entityType)
			if tt.wantError {
				assert.Error(t, err)
				assert.Nil(t, retrieved)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, retrieved)
				assert.Equal(t, tt.entityType, retrieved.EntityType)

				// Second call should hit cache
				cached, err := env.CacheManager.GetEntitySchema(ctx, tt.entityType)
				assert.NoError(t, err)
				assert.Equal(t, retrieved, cached)
			}
		})
	}
}

func TestCacheAwareManager_GetRelationshipSchema(t *testing.T) {
	ctx := context.Background()
	env := testhelpers.SetupTestEnvironment(t, ctx)
	defer env.Cleanup(ctx)

	// Create entity schemas first
	userSchema := testhelpers.CreateTestEntitySchema("user")
	err := env.PrimaryStore.CreateEntitySchema(ctx, userSchema)
	require.NoError(t, err)

	orgSchema := testhelpers.CreateTestEntitySchema("organization")
	err = env.PrimaryStore.CreateEntitySchema(ctx, orgSchema)
	require.NoError(t, err)

	// Create relationship schema
	relationSchema := testhelpers.CreateTestRelationshipSchema("member_of", "user", "organization")
	err = env.PrimaryStore.CreateRelationshipSchema(ctx, relationSchema)
	require.NoError(t, err)

	// Test cache behavior
	retrieved, err := env.CacheManager.GetRelationshipSchema(ctx, "member_of")
	require.NoError(t, err)
	assert.Equal(t, "member_of", retrieved.RelationType)

	// Second call should hit cache
	cached, err := env.CacheManager.GetRelationshipSchema(ctx, "member_of")
	require.NoError(t, err)
	assert.Equal(t, retrieved, cached)

	// Test non-existent schema
	_, err = env.CacheManager.GetRelationshipSchema(ctx, "nonexistent")
	assert.Error(t, err)
}

func TestCacheAwareManager_InvalidateEntitySchema(t *testing.T) {
	ctx := context.Background()
	env := testhelpers.SetupTestEnvironment(t, ctx)
	defer env.Cleanup(ctx)

	// Create test schema
	schema := testhelpers.CreateTestEntitySchema("user")
	err := env.PrimaryStore.CreateEntitySchema(ctx, schema)
	require.NoError(t, err)

	// Load into cache
	_, err = env.CacheManager.GetEntitySchema(ctx, "user")
	require.NoError(t, err)

	// Verify cache contains the schema
	assert.True(t, env.CacheManager.HasEntitySchema("user"))

	// Invalidate cache
	env.CacheManager.InvalidateEntitySchema("user")

	// Verify cache no longer contains the schema
	assert.False(t, env.CacheManager.HasEntitySchema("user"))
}

func TestCacheAwareManager_InvalidateRelationshipSchema(t *testing.T) {
	ctx := context.Background()
	env := testhelpers.SetupTestEnvironment(t, ctx)
	defer env.Cleanup(ctx)

	// Create schemas
	userSchema := testhelpers.CreateTestEntitySchema("user")
	err := env.PrimaryStore.CreateEntitySchema(ctx, userSchema)
	require.NoError(t, err)

	orgSchema := testhelpers.CreateTestEntitySchema("organization")
	err = env.PrimaryStore.CreateEntitySchema(ctx, orgSchema)
	require.NoError(t, err)

	relationSchema := testhelpers.CreateTestRelationshipSchema("member_of", "user", "organization")
	err = env.PrimaryStore.CreateRelationshipSchema(ctx, relationSchema)
	require.NoError(t, err)

	// Load into cache
	_, err = env.CacheManager.GetRelationshipSchema(ctx, "member_of")
	require.NoError(t, err)

	// Verify cache contains the schema
	assert.True(t, env.CacheManager.HasRelationshipSchema("member_of"))

	// Invalidate cache
	env.CacheManager.InvalidateRelationshipSchema("member_of")

	// Verify cache no longer contains the schema
	assert.False(t, env.CacheManager.HasRelationshipSchema("member_of"))
}

func TestCacheAwareManager_InvalidateAll(t *testing.T) {
	ctx := context.Background()
	env := testhelpers.SetupTestEnvironment(t, ctx)
	defer env.Cleanup(ctx)

	// Create schemas
	userSchema := testhelpers.CreateTestEntitySchema("user")
	err := env.PrimaryStore.CreateEntitySchema(ctx, userSchema)
	require.NoError(t, err)

	orgSchema := testhelpers.CreateTestEntitySchema("organization")
	err = env.PrimaryStore.CreateEntitySchema(ctx, orgSchema)
	require.NoError(t, err)

	relationSchema := testhelpers.CreateTestRelationshipSchema("member_of", "user", "organization")
	err = env.PrimaryStore.CreateRelationshipSchema(ctx, relationSchema)
	require.NoError(t, err)

	// Load schemas into cache
	_, err = env.CacheManager.GetEntitySchema(ctx, "user")
	require.NoError(t, err)
	_, err = env.CacheManager.GetEntitySchema(ctx, "organization")
	require.NoError(t, err)
	_, err = env.CacheManager.GetRelationshipSchema(ctx, "member_of")
	require.NoError(t, err)

	// Verify cache contains schemas
	assert.True(t, env.CacheManager.HasEntitySchema("user"))
	assert.True(t, env.CacheManager.HasEntitySchema("organization"))
	assert.True(t, env.CacheManager.HasRelationshipSchema("member_of"))

	// Invalidate all
	env.CacheManager.InvalidateAll()

	// Verify cache is empty
	assert.False(t, env.CacheManager.HasEntitySchema("user"))
	assert.False(t, env.CacheManager.HasEntitySchema("organization"))
	assert.False(t, env.CacheManager.HasRelationshipSchema("member_of"))
}

func TestCacheAwareManager_ConcurrentAccess(t *testing.T) {
	ctx := context.Background()
	env := testhelpers.SetupTestEnvironment(t, ctx)
	defer env.Cleanup(ctx)

	// Create test schema
	schema := testhelpers.CreateTestEntitySchema("user")
	err := env.PrimaryStore.CreateEntitySchema(ctx, schema)
	require.NoError(t, err)

	// Test concurrent access
	numGoroutines := 10
	results := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			_, err := env.CacheManager.GetEntitySchema(ctx, "user")
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
	env := testhelpers.SetupTestEnvironment(t, ctx)
	defer env.Cleanup(ctx)

	// Create cache manager with short TTL
	shortTTLManager := NewCacheAwareManager(env.PrimaryStore, 100*time.Millisecond)

	// Create test schema
	schema := testhelpers.CreateTestEntitySchema("user")
	err := env.PrimaryStore.CreateEntitySchema(ctx, schema)
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
	env := testhelpers.SetupTestEnvironment(&testing.T{}, ctx)
	defer env.Cleanup(ctx)

	// Create test schema
	schema := testhelpers.CreateTestEntitySchema("user")
	env.PrimaryStore.CreateEntitySchema(ctx, schema)

	// Pre-load cache
	env.CacheManager.GetEntitySchema(ctx, "user")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		env.CacheManager.GetEntitySchema(ctx, "user")
	}
}

func BenchmarkCacheAwareManager_CacheMiss(b *testing.B) {
	ctx := context.Background()
	env := testhelpers.SetupTestEnvironment(&testing.T{}, ctx)
	defer env.Cleanup(ctx)

	// Create test schema
	schema := testhelpers.CreateTestEntitySchema("user")
	env.PrimaryStore.CreateEntitySchema(ctx, schema)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Clear cache before each iteration to force cache miss
		env.CacheManager.InvalidateEntitySchema("user")
		env.CacheManager.GetEntitySchema(ctx, "user")
	}
}