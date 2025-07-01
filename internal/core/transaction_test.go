package core

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/sumandas0/entropic/internal/lock"
	"github.com/sumandas0/entropic/internal/models"
	"github.com/sumandas0/entropic/internal/store"
	"github.com/sumandas0/entropic/internal/store/postgres"
	"github.com/sumandas0/entropic/internal/store/testutils"
	"github.com/sumandas0/entropic/pkg/utils"
)

var (
	postgresContainer  *testutils.PostgresTestContainer
	primaryStore       *postgres.PostgresStore
	indexStore         store.IndexStore
)

func TestMain(m *testing.M) {
	var err error
	
	// Setup PostgreSQL
	postgresContainer, err = testutils.SetupTestPostgres()
	if err != nil {
		panic(err)
	}

	primaryStore, err = postgres.NewPostgresStore(postgresContainer.URL)
	if err != nil {
		postgresContainer.Cleanup()
		panic(err)
	}

	// Run migrations
	ctx := context.Background()
	migrator := postgres.NewMigrator(primaryStore.GetPool())
	if err := migrator.Run(ctx); err != nil {
		primaryStore.Close()
		postgresContainer.Cleanup()
		panic(err)
	}

	// Use mock index store for testing
	indexStore = testutils.NewMockTypesenseStore()

	// Run tests
	code := m.Run()

	// Cleanup
	primaryStore.Close()
	indexStore.Close()
	postgresContainer.Cleanup()

	if code != 0 {
		panic("tests failed")
	}
}

func cleanupDatabase(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	// Clean PostgreSQL
	_, err := primaryStore.GetPool().Exec(ctx, "TRUNCATE entities, relations, entity_schemas, relationship_schemas CASCADE")
	if err != nil {
		// If TRUNCATE fails due to locks, try DELETE
		_, err = primaryStore.GetPool().Exec(ctx, "DELETE FROM relations; DELETE FROM entities; DELETE FROM relationship_schemas; DELETE FROM entity_schemas")
	}
	require.NoError(t, err)
	
	// Clean mock index store by recreating it
	if _, ok := indexStore.(*testutils.MockTypesenseStore); ok {
		indexStore.Close()
		indexStore = testutils.NewMockTypesenseStore()
	}
}

func createTestEntitySchema(t *testing.T, entityType string) *models.EntitySchema {
	schema := &models.EntitySchema{
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
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	
	ctx := context.Background()
	err := primaryStore.CreateEntitySchema(ctx, schema)
	require.NoError(t, err)
	
	return schema
}

func createTestEntity(entityType, urn string) *models.Entity {
	return &models.Entity{
		ID:         uuid.New(),
		EntityType: entityType,
		URN:        urn,
		Properties: map[string]any{
			"name":  "Test User",
			"email": "test@example.com",
			"score": 100,
		},
		Version:   1,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

func TestTransactionCoordinator_BeginTransaction(t *testing.T) {
	cleanupDatabase(t)
	
	lockManager := lock.NewLockManager(nil)
	coordinator := NewTransactionCoordinator(primaryStore, indexStore, lockManager)
	
	ctx := context.Background()
	txCtx, err := coordinator.BeginTransaction(ctx)
	
	require.NoError(t, err)
	assert.NotNil(t, txCtx)
	assert.NotEmpty(t, txCtx.ID)
	assert.NotNil(t, txCtx.primaryTx)
	assert.Empty(t, txCtx.indexOperations)
	assert.Empty(t, txCtx.locks)
	
	// Clean up
	err = coordinator.RollbackTransaction(ctx, txCtx)
	require.NoError(t, err)
}

func TestTransactionCoordinator_CreateEntity(t *testing.T) {
	cleanupDatabase(t)
	
	// Setup
	createTestEntitySchema(t, "user")
	lockManager := lock.NewLockManager(nil)
	coordinator := NewTransactionCoordinator(primaryStore, indexStore, lockManager)
	
	ctx := context.Background()
	txCtx, err := coordinator.BeginTransaction(ctx)
	require.NoError(t, err)
	
	// Create entity
	entity := createTestEntity("user", "test:user:1")
	err = coordinator.CreateEntity(ctx, txCtx, entity)
	require.NoError(t, err)
	
	// Verify entity is in transaction context
	assert.Len(t, txCtx.indexOperations, 1)
	assert.Equal(t, "index", txCtx.indexOperations[0].Type)
	assert.Equal(t, entity, txCtx.indexOperations[0].Entity)
	
	// Verify lock was acquired
	assert.Len(t, txCtx.locks, 1)
	
	// Commit and verify
	err = coordinator.CommitTransaction(ctx, txCtx)
	require.NoError(t, err)
	
	// Verify entity exists in primary store
	retrieved, err := primaryStore.GetEntity(ctx, entity.EntityType, entity.ID)
	require.NoError(t, err)
	assert.Equal(t, entity.URN, retrieved.URN)
	
	// Verify entity exists in index store (eventually)
	time.Sleep(100 * time.Millisecond) // Give index time to update
	searchResult, err := indexStore.Search(ctx, &models.SearchQuery{
		EntityTypes: []string{"user"},
		Query:       "Test User",
		Limit:       10,
	})
	require.NoError(t, err)
	assert.Len(t, searchResult.Hits, 1)
}

func TestTransactionCoordinator_UpdateEntity(t *testing.T) {
	cleanupDatabase(t)
	
	// Setup
	createTestEntitySchema(t, "user")
	lockManager := lock.NewLockManager(nil)
	coordinator := NewTransactionCoordinator(primaryStore, indexStore, lockManager)
	
	ctx := context.Background()
	
	// First create an entity
	entity := createTestEntity("user", "test:user:update")
	err := primaryStore.CreateEntity(ctx, entity)
	require.NoError(t, err)
	err = indexStore.IndexEntity(ctx, entity)
	require.NoError(t, err)
	
	// Now update it in a transaction
	txCtx, err := coordinator.BeginTransaction(ctx)
	require.NoError(t, err)
	
	// Get the entity to ensure we have the correct version
	retrieved, err := primaryStore.GetEntity(ctx, entity.EntityType, entity.ID)
	require.NoError(t, err)
	
	retrieved.Properties["name"] = "Updated User"
	retrieved.Properties["score"] = 200
	entity = retrieved
	
	err = coordinator.UpdateEntity(ctx, txCtx, entity)
	require.NoError(t, err)
	
	// Verify update operation is tracked
	assert.Len(t, txCtx.indexOperations, 1)
	assert.Equal(t, "update", txCtx.indexOperations[0].Type)
	
	// Commit
	err = coordinator.CommitTransaction(ctx, txCtx)
	require.NoError(t, err)
	
	// Verify update in primary store
	updated, err := primaryStore.GetEntity(ctx, entity.EntityType, entity.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated User", updated.Properties["name"])
	assert.Equal(t, float64(200), updated.Properties["score"])
}

func TestTransactionCoordinator_DeleteEntity(t *testing.T) {
	cleanupDatabase(t)
	
	// Setup
	createTestEntitySchema(t, "user")
	lockManager := lock.NewLockManager(nil)
	coordinator := NewTransactionCoordinator(primaryStore, indexStore, lockManager)
	
	ctx := context.Background()
	
	// First create an entity
	entity := createTestEntity("user", "test:user:delete")
	err := primaryStore.CreateEntity(ctx, entity)
	require.NoError(t, err)
	err = indexStore.IndexEntity(ctx, entity)
	require.NoError(t, err)
	
	// Delete it in a transaction
	txCtx, err := coordinator.BeginTransaction(ctx)
	require.NoError(t, err)
	
	err = coordinator.DeleteEntity(ctx, txCtx, entity.EntityType, entity.ID)
	require.NoError(t, err)
	
	// Verify delete operation is tracked
	assert.Len(t, txCtx.indexOperations, 1)
	assert.Equal(t, "delete", txCtx.indexOperations[0].Type)
	assert.Equal(t, entity.EntityType, txCtx.indexOperations[0].EntityType)
	assert.Equal(t, entity.ID, txCtx.indexOperations[0].EntityID)
	
	// Commit
	err = coordinator.CommitTransaction(ctx, txCtx)
	require.NoError(t, err)
	
	// Verify entity is deleted from primary store
	_, err = primaryStore.GetEntity(ctx, entity.EntityType, entity.ID)
	assert.Error(t, err)
	
	var appErr *utils.AppError
	assert.ErrorAs(t, err, &appErr)
	assert.Equal(t, utils.CodeNotFound, appErr.Code)
}

func TestTransactionCoordinator_CreateRelation(t *testing.T) {
	cleanupDatabase(t)
	
	// Setup
	createTestEntitySchema(t, "user")
	createTestEntitySchema(t, "organization")
	lockManager := lock.NewLockManager(nil)
	coordinator := NewTransactionCoordinator(primaryStore, indexStore, lockManager)
	
	ctx := context.Background()
	
	// Create entities first
	user := createTestEntity("user", "test:user:rel1")
	org := createTestEntity("organization", "test:org:rel1")
	err := primaryStore.CreateEntity(ctx, user)
	require.NoError(t, err)
	err = primaryStore.CreateEntity(ctx, org)
	require.NoError(t, err)
	
	// Create relation in transaction
	txCtx, err := coordinator.BeginTransaction(ctx)
	require.NoError(t, err)
	
	relation := &models.Relation{
		ID:             uuid.New(),
		RelationType:   "member_of",
		FromEntityID:   user.ID,
		FromEntityType: user.EntityType,
		ToEntityID:     org.ID,
		ToEntityType:   org.EntityType,
		Properties: map[string]any{
			"role": "admin",
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	
	err = coordinator.CreateRelation(ctx, txCtx, relation)
	require.NoError(t, err)
	
	// Verify locks were acquired for both entities
	assert.Len(t, txCtx.locks, 2)
	
	// Commit
	err = coordinator.CommitTransaction(ctx, txCtx)
	require.NoError(t, err)
	
	// Verify relation exists
	retrieved, err := primaryStore.GetRelation(ctx, relation.ID)
	require.NoError(t, err)
	assert.Equal(t, relation.RelationType, retrieved.RelationType)
}

func TestTransactionCoordinator_Rollback(t *testing.T) {
	cleanupDatabase(t)
	
	// Setup
	createTestEntitySchema(t, "user")
	lockManager := lock.NewLockManager(nil)
	coordinator := NewTransactionCoordinator(primaryStore, indexStore, lockManager)
	
	ctx := context.Background()
	txCtx, err := coordinator.BeginTransaction(ctx)
	require.NoError(t, err)
	
	// Create entity in transaction
	entity := createTestEntity("user", "test:user:rollback")
	err = coordinator.CreateEntity(ctx, txCtx, entity)
	require.NoError(t, err)
	
	// Rollback instead of commit
	err = coordinator.RollbackTransaction(ctx, txCtx)
	require.NoError(t, err)
	
	// Verify entity doesn't exist
	_, err = primaryStore.GetEntity(ctx, entity.EntityType, entity.ID)
	assert.Error(t, err)
	
	// Verify locks were released
	assert.Empty(t, txCtx.locks)
}

func TestTransactionCoordinator_ConcurrentTransactions(t *testing.T) {
	cleanupDatabase(t)
	
	// Setup
	createTestEntitySchema(t, "user")
	lockManager := lock.NewLockManager(nil)
	coordinator := NewTransactionCoordinator(primaryStore, indexStore, lockManager)
	
	ctx := context.Background()
	
	// Create a base entity
	baseEntity := createTestEntity("user", "test:user:concurrent")
	err := primaryStore.CreateEntity(ctx, baseEntity)
	require.NoError(t, err)
	
	// Try to update the same entity concurrently
	var wg sync.WaitGroup
	errors := make(chan error, 2)
	
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			
			txCtx, err := coordinator.BeginTransaction(ctx)
			if err != nil {
				errors <- err
				return
			}
			
			// Try to update the entity
			entity := *baseEntity
			entity.Properties["score"] = 100 + idx
			entity.Version = 2
			
			err = coordinator.UpdateEntity(ctx, txCtx, &entity)
			if err != nil {
				coordinator.RollbackTransaction(ctx, txCtx)
				errors <- err
				return
			}
			
			// Small delay to increase chance of conflict
			time.Sleep(10 * time.Millisecond)
			
			err = coordinator.CommitTransaction(ctx, txCtx)
			errors <- err
		}(i)
	}
	
	wg.Wait()
	close(errors)
	
	// Collect results
	var successes, failures int
	for err := range errors {
		if err == nil {
			successes++
		} else {
			failures++
		}
	}
	
	// At least one should succeed
	assert.GreaterOrEqual(t, successes, 1)
}

func TestTransactionCoordinator_WithTransaction(t *testing.T) {
	cleanupDatabase(t)
	
	// Setup
	createTestEntitySchema(t, "user")
	lockManager := lock.NewLockManager(nil)
	coordinator := NewTransactionCoordinator(primaryStore, indexStore, lockManager)
	
	ctx := context.Background()
	
	// Test successful transaction
	entity := createTestEntity("user", "test:user:with_tx")
	err := coordinator.WithTransaction(ctx, func(ctx context.Context, txCtx *TransactionContext) error {
		return coordinator.CreateEntity(ctx, txCtx, entity)
	})
	require.NoError(t, err)
	
	// Verify entity was created
	retrieved, err := primaryStore.GetEntity(ctx, entity.EntityType, entity.ID)
	require.NoError(t, err)
	assert.Equal(t, entity.URN, retrieved.URN)
	
	// Test failed transaction
	failEntity := createTestEntity("user", "test:user:with_tx_fail")
	err = coordinator.WithTransaction(ctx, func(ctx context.Context, txCtx *TransactionContext) error {
		if err := coordinator.CreateEntity(ctx, txCtx, failEntity); err != nil {
			return err
		}
		// Simulate error
		return fmt.Errorf("simulated error")
	})
	assert.Error(t, err)
	
	// Verify entity was not created
	_, err = primaryStore.GetEntity(ctx, failEntity.EntityType, failEntity.ID)
	assert.Error(t, err)
}

func TestTransactionCoordinator_IndexFailureHandling(t *testing.T) {
	cleanupDatabase(t)
	
	// Setup with a broken index store
	createTestEntitySchema(t, "user")
	lockManager := lock.NewLockManager(nil)
	
	// Close index store to simulate failure
	indexStore.Close()
	coordinator := NewTransactionCoordinator(primaryStore, indexStore, lockManager)
	
	ctx := context.Background()
	txCtx, err := coordinator.BeginTransaction(ctx)
	require.NoError(t, err)
	
	// Create entity
	entity := createTestEntity("user", "test:user:index_fail")
	err = coordinator.CreateEntity(ctx, txCtx, entity)
	require.NoError(t, err)
	
	// Commit should succeed but report index errors
	err = coordinator.CommitTransaction(ctx, txCtx)
	assert.Error(t, err)
	
	// Entity should still exist in primary store (two-phase commit)
	retrieved, err := primaryStore.GetEntity(ctx, entity.EntityType, entity.ID)
	require.NoError(t, err)
	assert.Equal(t, entity.URN, retrieved.URN)
	
	// Reconnect index store for other tests
	indexStore = testutils.NewMockTypesenseStore()
}

func TestTransactionManager_ExecuteWithTimeout(t *testing.T) {
	cleanupDatabase(t)
	
	// Setup
	createTestEntitySchema(t, "user")
	lockManager := lock.NewLockManager(nil)
	coordinator := NewTransactionCoordinator(primaryStore, indexStore, lockManager)
	manager := NewTransactionManager(coordinator, 100*time.Millisecond)
	
	ctx := context.Background()
	
	// Test successful operation within timeout
	entity := createTestEntity("user", "test:user:timeout_success")
	err := manager.ExecuteWithTimeout(ctx, func(ctx context.Context, txCtx *TransactionContext) error {
		return coordinator.CreateEntity(ctx, txCtx, entity)
	})
	require.NoError(t, err)
	
	// Test timeout
	slowEntity := createTestEntity("user", "test:user:timeout_fail")
	err = manager.ExecuteWithTimeout(ctx, func(ctx context.Context, txCtx *TransactionContext) error {
		// Simulate slow operation
		time.Sleep(200 * time.Millisecond)
		return coordinator.CreateEntity(ctx, txCtx, slowEntity)
	})
	assert.Error(t, err)
	
	// Verify entity was not created due to timeout
	_, err = primaryStore.GetEntity(ctx, slowEntity.EntityType, slowEntity.ID)
	assert.Error(t, err)
	
	// Check stats
	stats := manager.GetStats()
	assert.Equal(t, uint64(1), stats.TotalCommitted)
	assert.Equal(t, uint64(1), stats.CommitTimeouts)
}

func TestBatchProcessor_ProcessBatch(t *testing.T) {
	cleanupDatabase(t)
	
	// Setup
	createTestEntitySchema(t, "user")
	lockManager := lock.NewLockManager(nil)
	coordinator := NewTransactionCoordinator(primaryStore, indexStore, lockManager)
	batchProcessor := NewBatchProcessor(coordinator, 5)
	
	ctx := context.Background()
	
	// Create batch operations
	operations := make([]BatchOperation, 10)
	for i := 0; i < 10; i++ {
		entity := createTestEntity("user", fmt.Sprintf("test:user:batch_%d", i))
		operations[i] = BatchOperation{
			Type:   "create_entity",
			Entity: entity,
		}
	}
	
	// Process batch
	err := batchProcessor.ProcessBatch(ctx, operations)
	require.NoError(t, err)
	
	// Verify all entities were created
	for i := 0; i < 10; i++ {
		entity := operations[i].Entity
		retrieved, err := primaryStore.GetEntity(ctx, entity.EntityType, entity.ID)
		require.NoError(t, err)
		assert.Equal(t, entity.URN, retrieved.URN)
	}
}

func TestBatchProcessor_MixedOperations(t *testing.T) {
	cleanupDatabase(t)
	
	// Setup
	createTestEntitySchema(t, "user")
	createTestEntitySchema(t, "organization")
	lockManager := lock.NewLockManager(nil)
	coordinator := NewTransactionCoordinator(primaryStore, indexStore, lockManager)
	batchProcessor := NewBatchProcessor(coordinator, 10)
	
	ctx := context.Background()
	
	// Create some entities first
	existingEntity := createTestEntity("user", "test:user:batch_existing")
	err := primaryStore.CreateEntity(ctx, existingEntity)
	require.NoError(t, err)
	
	user := createTestEntity("user", "test:user:batch_rel")
	org := createTestEntity("organization", "test:org:batch_rel")
	err = primaryStore.CreateEntity(ctx, user)
	require.NoError(t, err)
	err = primaryStore.CreateEntity(ctx, org)
	require.NoError(t, err)
	
	// Create mixed batch operations
	operations := []BatchOperation{
		// Create new entity
		{
			Type:   "create_entity",
			Entity: createTestEntity("user", "test:user:batch_new"),
		},
		// Update existing entity
		{
			Type: "update_entity",
			Entity: func() *models.Entity {
				e := *existingEntity
				e.Properties["name"] = "Updated Batch User"
				e.Version = 2
				return &e
			}(),
		},
		// Create relation
		{
			Type: "create_relation",
			Relation: &models.Relation{
				ID:             uuid.New(),
				RelationType:   "member_of",
				FromEntityID:   user.ID,
				FromEntityType: user.EntityType,
				ToEntityID:     org.ID,
				ToEntityType:   org.EntityType,
				Properties: map[string]any{
					"role": "member",
				},
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
		},
		// Delete entity
		{
			Type:       "delete_entity",
			EntityType: existingEntity.EntityType,
			EntityID:   existingEntity.ID,
		},
	}
	
	// Process batch
	err = batchProcessor.ProcessBatch(ctx, operations)
	require.NoError(t, err)
	
	// Verify operations
	// New entity should exist
	_, err = primaryStore.GetEntity(ctx, operations[0].Entity.EntityType, operations[0].Entity.ID)
	require.NoError(t, err)
	
	// Existing entity should be deleted
	_, err = primaryStore.GetEntity(ctx, existingEntity.EntityType, existingEntity.ID)
	assert.Error(t, err)
	
	// Relation should exist
	_, err = primaryStore.GetRelation(ctx, operations[2].Relation.ID)
	require.NoError(t, err)
}

func TestTransactionCoordinator_DeadlockPrevention(t *testing.T) {
	cleanupDatabase(t)
	
	// Setup
	createTestEntitySchema(t, "user")
	lockManager := lock.NewLockManager(nil)
	coordinator := NewTransactionCoordinator(primaryStore, indexStore, lockManager)
	
	ctx := context.Background()
	
	// Create two entities
	entity1 := createTestEntity("user", "test:user:deadlock1")
	entity2 := createTestEntity("user", "test:user:deadlock2")
	err := primaryStore.CreateEntity(ctx, entity1)
	require.NoError(t, err)
	err = primaryStore.CreateEntity(ctx, entity2)
	require.NoError(t, err)
	
	// Create two relations that would create a potential deadlock
	// if not handled properly (A->B and B->A acquired in different order)
	var wg sync.WaitGroup
	errors := make(chan error, 2)
	
	wg.Add(2)
	
	// Transaction 1: Create relation from entity1 to entity2
	go func() {
		defer wg.Done()
		
		txCtx, err := coordinator.BeginTransaction(ctx)
		if err != nil {
			errors <- err
			return
		}
		
		relation := &models.Relation{
			ID:             uuid.New(),
			RelationType:   "follows",
			FromEntityID:   entity1.ID,
			FromEntityType: entity1.EntityType,
			ToEntityID:     entity2.ID,
			ToEntityType:   entity2.EntityType,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}
		
		err = coordinator.CreateRelation(ctx, txCtx, relation)
		if err != nil {
			coordinator.RollbackTransaction(ctx, txCtx)
			errors <- err
			return
		}
		
		// Small delay to increase chance of deadlock
		time.Sleep(10 * time.Millisecond)
		
		err = coordinator.CommitTransaction(ctx, txCtx)
		errors <- err
	}()
	
	// Transaction 2: Create relation from entity2 to entity1
	go func() {
		defer wg.Done()
		
		// Small delay to ensure different lock acquisition order
		time.Sleep(5 * time.Millisecond)
		
		txCtx, err := coordinator.BeginTransaction(ctx)
		if err != nil {
			errors <- err
			return
		}
		
		relation := &models.Relation{
			ID:             uuid.New(),
			RelationType:   "follows",
			FromEntityID:   entity2.ID,
			FromEntityType: entity2.EntityType,
			ToEntityID:     entity1.ID,
			ToEntityType:   entity1.EntityType,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}
		
		err = coordinator.CreateRelation(ctx, txCtx, relation)
		if err != nil {
			coordinator.RollbackTransaction(ctx, txCtx)
			errors <- err
			return
		}
		
		err = coordinator.CommitTransaction(ctx, txCtx)
		errors <- err
	}()
	
	wg.Wait()
	close(errors)
	
	// Both operations should complete without deadlock
	errorCount := 0
	for err := range errors {
		if err != nil {
			errorCount++
			t.Logf("Transaction error: %v", err)
		}
	}
	
	// At least one should succeed, possibly both
	assert.Less(t, errorCount, 2)
}