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
	manager := NewManager(30*time.Second, 5*time.Minute)

	resource := "test-resource"

	err := manager.LockWithTimeout(resource, 5*time.Second)
	require.NoError(t, err)

	assert.True(t, manager.IsLocked(resource))

	err = manager.Unlock(resource)
	require.NoError(t, err)

	assert.False(t, manager.IsLocked(resource))
}

func TestLockManager_LockTimeout(t *testing.T) {
	manager := NewManager(30*time.Second, 5*time.Minute)

	resource := "test-resource"

	err := manager.LockWithTimeout(resource, 100*time.Millisecond)
	require.NoError(t, err)

	time.Sleep(200 * time.Millisecond)

	assert.False(t, manager.IsLocked(resource))
}

func TestLockManager_ConcurrentLocking(t *testing.T) {
	manager := NewManager(30*time.Second, 5*time.Minute)

	resource := "test-resource"
	numGoroutines := 10
	var executionOrder []int
	var mu sync.Mutex

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			
			err := manager.LockWithTimeout(resource, 5*time.Second)
			if err == nil {
				
				mu.Lock()
				executionOrder = append(executionOrder, id)
				mu.Unlock()

				time.Sleep(10 * time.Millisecond)
				
				manager.Unlock(resource)
			}
		}(i)
	}

	wg.Wait()

	assert.Len(t, executionOrder, numGoroutines)

}

func TestLockManager_DoubleUnlock(t *testing.T) {
	manager := NewManager(30*time.Second, 5*time.Minute)

	resource := "test-resource"

	err := manager.LockWithTimeout(resource, 5*time.Second)
	require.NoError(t, err)

	err = manager.Unlock(resource)
	require.NoError(t, err)

	err = manager.Unlock(resource)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not locked")
}

func TestLockManager_LockAlreadyHeld(t *testing.T) {
	manager := NewManager(30*time.Second, 5*time.Minute)

	resource := "test-resource"

	err := manager.LockWithTimeout(resource, 5*time.Second)
	require.NoError(t, err)

	// Try to acquire the same lock again - should fail immediately
	err = manager.TryLock(resource, 1*time.Second)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already locked")

	manager.Unlock(resource)
}

func TestLockManager_ContextCancellation(t *testing.T) {
	manager := NewManager(30*time.Second, 5*time.Minute)

	resource := "test-resource"

	err := manager.LockWithTimeout(resource, 10*time.Second)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() 

	err = manager.LockWithContext(ctx, resource, 5*time.Second)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "context")

	manager.Unlock(resource)
}

func TestLockManager_MultipleResources(t *testing.T) {
	manager := NewManager(30*time.Second, 5*time.Minute)

	resource1 := "test-resource-1"
	resource2 := "test-resource-2"

	err := manager.LockWithTimeout(resource1, 5*time.Second)
	require.NoError(t, err)

	err = manager.LockWithTimeout(resource2, 5*time.Second)
	require.NoError(t, err)

	assert.True(t, manager.IsLocked(resource1))
	assert.True(t, manager.IsLocked(resource2))

	err = manager.Unlock(resource1)
	require.NoError(t, err)

	err = manager.Unlock(resource2)
	require.NoError(t, err)

	assert.False(t, manager.IsLocked(resource1))
	assert.False(t, manager.IsLocked(resource2))
}

func TestInMemoryDistributedLock_BasicOperations(t *testing.T) {
	lock := NewInMemoryDistributedLock()

	ctx := context.Background()
	resource := "test-resource"

	handle, err := lock.Acquire(ctx, resource, 5*time.Second)
	require.NoError(t, err)
	require.NotNil(t, handle)

	held := lock.IsHeld(resource)
	assert.True(t, held)

	err = lock.Release(ctx, handle)
	require.NoError(t, err)

	held = lock.IsHeld(resource)
	assert.False(t, held)
}

func TestInMemoryDistributedLock_Expiration(t *testing.T) {
	lock := NewInMemoryDistributedLock()

	ctx := context.Background()
	resource := "test-resource"

	handle, err := lock.Acquire(ctx, resource, 100*time.Millisecond)
	require.NoError(t, err)
	require.NotNil(t, handle)

	assert.True(t, lock.IsHeld(resource))

	time.Sleep(200 * time.Millisecond)

	assert.False(t, lock.IsHeld(resource))
}

func TestInMemoryDistributedLock_ConcurrentAccess(t *testing.T) {
	lock := NewInMemoryDistributedLock()

	ctx := context.Background()
	resource := "test-resource"
	numGoroutines := 20
	var executionOrder []int
	var mu sync.Mutex

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			
			handle, err := lock.Acquire(ctx, resource, 1*time.Second)
			if err == nil {
				
				mu.Lock()
				executionOrder = append(executionOrder, id)
				mu.Unlock()

				time.Sleep(5 * time.Millisecond)
				
				lock.Release(ctx, handle)
			}
		}(i)
	}

	wg.Wait()

	assert.Len(t, executionOrder, numGoroutines)
}

func TestLockManager_EntityLocking(t *testing.T) {
	distributedLock := NewInMemoryDistributedLock()
	manager := NewLockManager(distributedLock)

	ctx := context.Background()
	entityType := "user"
	entityID := "123e4567-e89b-12d3-a456-426614174000"

	err := manager.LockEntity(ctx, entityType, entityID, 5*time.Second)
	require.NoError(t, err)

	resource := entityType + ":" + entityID
	assert.True(t, manager.IsLocked(resource))

	err = manager.UnlockEntity(ctx, entityType, entityID)
	require.NoError(t, err)

	assert.False(t, manager.IsLocked(resource))
}

func TestLockManager_SchemaLocking(t *testing.T) {
	distributedLock := NewInMemoryDistributedLock()
	manager := NewLockManager(distributedLock)

	ctx := context.Background()
	entityType := "user"

	err := manager.LockSchema(ctx, entityType, 5*time.Second)
	require.NoError(t, err)

	resource := "schema:" + entityType
	assert.True(t, manager.IsLocked(resource))

	err = manager.UnlockSchema(ctx, entityType)
	require.NoError(t, err)

	assert.False(t, manager.IsLocked(resource))
}

func TestLockManager_DeadlockPrevention(t *testing.T) {
	manager := NewManager(30*time.Second, 5*time.Minute)

	resource1 := "resource-1"
	resource2 := "resource-2"

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		
		err := manager.LockWithTimeout(resource1, 2*time.Second)
		if err == nil {
			time.Sleep(100 * time.Millisecond)
			
			err = manager.LockWithTimeout(resource2, 1*time.Second)
			if err == nil {
				manager.Unlock(resource2)
			}
			
			manager.Unlock(resource1)
		}
	}()

	go func() {
		defer wg.Done()
		
		err := manager.LockWithTimeout(resource2, 2*time.Second)
		if err == nil {
			time.Sleep(100 * time.Millisecond)
			
			err = manager.LockWithTimeout(resource1, 1*time.Second)
			if err == nil {
				manager.Unlock(resource1)
			}
			
			manager.Unlock(resource2)
		}
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		
	case <-time.After(5 * time.Second):
		t.Fatal("Potential deadlock detected")
	}
}

func BenchmarkLockManager_Lock(b *testing.B) {
	manager := NewManager(30*time.Second, 5*time.Minute)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resource := "test-resource"
		manager.LockWithTimeout(resource, 1*time.Second)
		manager.Unlock(resource)
	}
}

func BenchmarkLockManager_ConcurrentLocking(b *testing.B) {
	manager := NewManager(30*time.Second, 5*time.Minute)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			resource := "test-resource"
			err := manager.LockWithTimeout(resource, 10*time.Millisecond)
			if err == nil {
				manager.Unlock(resource)
			}
			i++
		}
	})
}