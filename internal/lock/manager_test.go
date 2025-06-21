package lock

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLockManager_Lock(t *testing.T) {
	manager := NewLockManager(NewInMemoryDistributedLock())

	ctx := context.Background()
	resource := "test-resource"

	// Test acquiring lock
	err := manager.Lock(ctx, resource, 5*time.Second)
	require.NoError(t, err)

	// Test lock is held
	assert.True(t, manager.IsLocked(resource))

	// Release lock
	err = manager.Unlock(ctx, resource)
	require.NoError(t, err)

	// Test lock is released
	assert.False(t, manager.IsLocked(resource))
}

func TestLockManager_LockTimeout(t *testing.T) {
	manager := NewLockManager(NewInMemoryDistributedLock())

	ctx := context.Background()
	resource := "test-resource"

	// Acquire lock with short timeout
	err := manager.Lock(ctx, resource, 100*time.Millisecond)
	require.NoError(t, err)

	// Wait for timeout
	time.Sleep(200 * time.Millisecond)

	// Lock should be automatically released
	assert.False(t, manager.IsLocked(resource))
}

func TestLockManager_ConcurrentLocking(t *testing.T) {
	manager := NewLockManager(NewInMemoryDistributedLock())

	ctx := context.Background()
	resource := "test-resource"
	numGoroutines := 10
	successCount := 0
	var mu sync.Mutex

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Try to acquire lock concurrently
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			
			err := manager.Lock(ctx, resource, 1*time.Second)
			if err == nil {
				mu.Lock()
				successCount++
				mu.Unlock()
				
				// Hold lock briefly
				time.Sleep(50 * time.Millisecond)
				
				manager.Unlock(ctx, resource)
			}
		}()
	}

	wg.Wait()

	// Only one goroutine should have successfully acquired the lock
	assert.Equal(t, 1, successCount)
}

func TestLockManager_DoubleUnlock(t *testing.T) {
	manager := NewLockManager(NewInMemoryDistributedLock())

	ctx := context.Background()
	resource := "test-resource"

	// Acquire lock
	err := manager.Lock(ctx, resource, 5*time.Second)
	require.NoError(t, err)

	// First unlock should succeed
	err = manager.Unlock(ctx, resource)
	require.NoError(t, err)

	// Second unlock should return error
	err = manager.Unlock(ctx, resource)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not locked")
}

func TestLockManager_LockAlreadyHeld(t *testing.T) {
	manager := NewLockManager(NewInMemoryDistributedLock())

	ctx := context.Background()
	resource := "test-resource"

	// Acquire lock
	err := manager.Lock(ctx, resource, 5*time.Second)
	require.NoError(t, err)

	// Try to acquire same lock again
	err = manager.Lock(ctx, resource, 1*time.Second)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already locked")

	// Clean up
	manager.Unlock(ctx, resource)
}

func TestLockManager_ContextCancellation(t *testing.T) {
	manager := NewLockManager(NewInMemoryDistributedLock())

	resource := "test-resource"

	// Acquire lock first
	ctx1 := context.Background()
	err := manager.Lock(ctx1, resource, 10*time.Second)
	require.NoError(t, err)

	// Try to acquire with cancelled context
	ctx2, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err = manager.Lock(ctx2, resource, 5*time.Second)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "context")

	// Clean up
	manager.Unlock(ctx1, resource)
}

func TestLockManager_MultipleResources(t *testing.T) {
	manager := NewLockManager(NewInMemoryDistributedLock())

	ctx := context.Background()
	resource1 := "test-resource-1"
	resource2 := "test-resource-2"

	// Acquire locks on different resources
	err := manager.Lock(ctx, resource1, 5*time.Second)
	require.NoError(t, err)

	err = manager.Lock(ctx, resource2, 5*time.Second)
	require.NoError(t, err)

	// Both should be locked
	assert.True(t, manager.IsLocked(resource1))
	assert.True(t, manager.IsLocked(resource2))

	// Release locks
	err = manager.Unlock(ctx, resource1)
	require.NoError(t, err)

	err = manager.Unlock(ctx, resource2)
	require.NoError(t, err)

	// Both should be unlocked
	assert.False(t, manager.IsLocked(resource1))
	assert.False(t, manager.IsLocked(resource2))
}

func TestInMemoryDistributedLock_BasicOperations(t *testing.T) {
	lock := NewInMemoryDistributedLock()

	ctx := context.Background()
	resource := "test-resource"

	// Test acquire
	err := lock.Acquire(ctx, resource, 5*time.Second)
	require.NoError(t, err)

	// Test is held
	held := lock.IsHeld(resource)
	assert.True(t, held)

	// Test release
	err = lock.Release(ctx, resource)
	require.NoError(t, err)

	// Test is no longer held
	held = lock.IsHeld(resource)
	assert.False(t, held)
}

func TestInMemoryDistributedLock_Expiration(t *testing.T) {
	lock := NewInMemoryDistributedLock()

	ctx := context.Background()
	resource := "test-resource"

	// Acquire with short TTL
	err := lock.Acquire(ctx, resource, 100*time.Millisecond)
	require.NoError(t, err)

	// Should be held initially
	assert.True(t, lock.IsHeld(resource))

	// Wait for expiration
	time.Sleep(200 * time.Millisecond)

	// Should no longer be held
	assert.False(t, lock.IsHeld(resource))
}

func TestInMemoryDistributedLock_ConcurrentAccess(t *testing.T) {
	lock := NewInMemoryDistributedLock()

	ctx := context.Background()
	resource := "test-resource"
	numGoroutines := 20
	successCount := 0
	var mu sync.Mutex

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Try to acquire lock concurrently
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			
			err := lock.Acquire(ctx, resource, 1*time.Second)
			if err == nil {
				mu.Lock()
				successCount++
				mu.Unlock()
				
				// Hold lock briefly
				time.Sleep(10 * time.Millisecond)
				
				lock.Release(ctx, resource)
			}
		}()
	}

	wg.Wait()

	// Only one goroutine should have successfully acquired the lock
	assert.Equal(t, 1, successCount)
}

func TestLockManager_EntityLocking(t *testing.T) {
	manager := NewLockManager(NewInMemoryDistributedLock())

	ctx := context.Background()
	entityType := "user"
	entityID := "123e4567-e89b-12d3-a456-426614174000"

	// Test entity-specific locking
	err := manager.LockEntity(ctx, entityType, entityID, 5*time.Second)
	require.NoError(t, err)

	// Verify lock is held
	resource := entityType + ":" + entityID
	assert.True(t, manager.IsLocked(resource))

	// Release lock
	err = manager.UnlockEntity(ctx, entityType, entityID)
	require.NoError(t, err)

	// Verify lock is released
	assert.False(t, manager.IsLocked(resource))
}

func TestLockManager_SchemaLocking(t *testing.T) {
	manager := NewLockManager(NewInMemoryDistributedLock())

	ctx := context.Background()
	entityType := "user"

	// Test schema-specific locking
	err := manager.LockSchema(ctx, entityType, 5*time.Second)
	require.NoError(t, err)

	// Verify lock is held
	resource := "schema:" + entityType
	assert.True(t, manager.IsLocked(resource))

	// Release lock
	err = manager.UnlockSchema(ctx, entityType)
	require.NoError(t, err)

	// Verify lock is released
	assert.False(t, manager.IsLocked(resource))
}

func TestLockManager_DeadlockPrevention(t *testing.T) {
	manager := NewLockManager(NewInMemoryDistributedLock())

	ctx := context.Background()
	resource1 := "resource-1"
	resource2 := "resource-2"

	var wg sync.WaitGroup
	wg.Add(2)

	// Goroutine 1: Lock resource1 then resource2
	go func() {
		defer wg.Done()
		
		err := manager.Lock(ctx, resource1, 2*time.Second)
		if err == nil {
			time.Sleep(100 * time.Millisecond)
			
			err = manager.Lock(ctx, resource2, 1*time.Second)
			if err == nil {
				manager.Unlock(ctx, resource2)
			}
			
			manager.Unlock(ctx, resource1)
		}
	}()

	// Goroutine 2: Lock resource2 then resource1
	go func() {
		defer wg.Done()
		
		err := manager.Lock(ctx, resource2, 2*time.Second)
		if err == nil {
			time.Sleep(100 * time.Millisecond)
			
			err = manager.Lock(ctx, resource1, 1*time.Second)
			if err == nil {
				manager.Unlock(ctx, resource1)
			}
			
			manager.Unlock(ctx, resource2)
		}
	}()

	// Wait for completion - should not deadlock
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success - no deadlock
	case <-time.After(5 * time.Second):
		t.Fatal("Potential deadlock detected")
	}
}

func BenchmarkLockManager_Lock(b *testing.B) {
	manager := NewLockManager(NewInMemoryDistributedLock())
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resource := "test-resource"
		manager.Lock(ctx, resource, 1*time.Second)
		manager.Unlock(ctx, resource)
	}
}

func BenchmarkLockManager_ConcurrentLocking(b *testing.B) {
	manager := NewLockManager(NewInMemoryDistributedLock())
	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			resource := "test-resource"
			err := manager.Lock(ctx, resource, 10*time.Millisecond)
			if err == nil {
				manager.Unlock(ctx, resource)
			}
			i++
		}
	})
}