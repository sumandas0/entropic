package core

import (
	"context"
	"fmt"
	"time"

	"github.com/entropic/entropic/internal/lock"
	"github.com/entropic/entropic/internal/models"
	"github.com/entropic/entropic/internal/store"
	"github.com/entropic/entropic/pkg/utils"
	"github.com/google/uuid"
)

// TransactionCoordinator manages two-phase commit transactions across primary and index stores
type TransactionCoordinator struct {
	primaryStore store.PrimaryStore
	indexStore   store.IndexStore
	lockManager  *lock.LockManager
}

// NewTransactionCoordinator creates a new transaction coordinator
func NewTransactionCoordinator(primaryStore store.PrimaryStore, indexStore store.IndexStore, lockManager *lock.LockManager) *TransactionCoordinator {
	return &TransactionCoordinator{
		primaryStore: primaryStore,
		indexStore:   indexStore,
		lockManager:  lockManager,
	}
}

// TransactionContext holds the context for a transaction
type TransactionContext struct {
	ID             string
	primaryTx      store.Transaction
	indexOperations []IndexOperation
	locks          []lock.LockHandle
	startTime      time.Time
}

// IndexOperation represents an operation to be performed on the index store
type IndexOperation struct {
	Type     string // "index", "update", "delete"
	Entity   *models.Entity
	EntityType string
	EntityID uuid.UUID
}

// BeginTransaction starts a new two-phase commit transaction
func (tc *TransactionCoordinator) BeginTransaction(ctx context.Context) (*TransactionContext, error) {
	// Start primary store transaction
	primaryTx, err := tc.primaryStore.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin primary transaction: %w", err)
	}
	
	txCtx := &TransactionContext{
		ID:              uuid.New().String(),
		primaryTx:       primaryTx,
		indexOperations: make([]IndexOperation, 0),
		locks:           make([]lock.LockHandle, 0),
		startTime:       time.Now(),
	}
	
	return txCtx, nil
}

// CreateEntity creates an entity within a transaction
func (tc *TransactionCoordinator) CreateEntity(ctx context.Context, txCtx *TransactionContext, entity *models.Entity) error {
	// Acquire entity lock
	lockHandle, err := tc.lockManager.AcquireEntityLock(ctx, entity.EntityType, entity.ID, 30*time.Second)
	if err != nil {
		return fmt.Errorf("failed to acquire entity lock: %w", err)
	}
	txCtx.locks = append(txCtx.locks, lockHandle)
	
	// Create in primary store
	if err := txCtx.primaryTx.CreateEntity(ctx, entity); err != nil {
		return fmt.Errorf("failed to create entity in primary store: %w", err)
	}
	
	// Queue index operation
	txCtx.indexOperations = append(txCtx.indexOperations, IndexOperation{
		Type:   "index",
		Entity: entity,
	})
	
	return nil
}

// UpdateEntity updates an entity within a transaction
func (tc *TransactionCoordinator) UpdateEntity(ctx context.Context, txCtx *TransactionContext, entity *models.Entity) error {
	// Acquire entity lock
	lockHandle, err := tc.lockManager.AcquireEntityLock(ctx, entity.EntityType, entity.ID, 30*time.Second)
	if err != nil {
		return fmt.Errorf("failed to acquire entity lock: %w", err)
	}
	txCtx.locks = append(txCtx.locks, lockHandle)
	
	// Update in primary store
	if err := txCtx.primaryTx.UpdateEntity(ctx, entity); err != nil {
		return fmt.Errorf("failed to update entity in primary store: %w", err)
	}
	
	// Queue index operation
	txCtx.indexOperations = append(txCtx.indexOperations, IndexOperation{
		Type:   "update",
		Entity: entity,
	})
	
	return nil
}

// DeleteEntity deletes an entity within a transaction
func (tc *TransactionCoordinator) DeleteEntity(ctx context.Context, txCtx *TransactionContext, entityType string, entityID uuid.UUID) error {
	// Acquire entity lock
	lockHandle, err := tc.lockManager.AcquireEntityLock(ctx, entityType, entityID, 30*time.Second)
	if err != nil {
		return fmt.Errorf("failed to acquire entity lock: %w", err)
	}
	txCtx.locks = append(txCtx.locks, lockHandle)
	
	// Delete from primary store
	if err := txCtx.primaryTx.DeleteEntity(ctx, entityType, entityID); err != nil {
		return fmt.Errorf("failed to delete entity from primary store: %w", err)
	}
	
	// Queue index operation
	txCtx.indexOperations = append(txCtx.indexOperations, IndexOperation{
		Type:       "delete",
		EntityType: entityType,
		EntityID:   entityID,
	})
	
	return nil
}

// CreateRelation creates a relation within a transaction
func (tc *TransactionCoordinator) CreateRelation(ctx context.Context, txCtx *TransactionContext, relation *models.Relation) error {
	// Acquire locks for both entities
	fromLockHandle, err := tc.lockManager.AcquireEntityLock(ctx, relation.FromEntityType, relation.FromEntityID, 30*time.Second)
	if err != nil {
		return fmt.Errorf("failed to acquire from entity lock: %w", err)
	}
	txCtx.locks = append(txCtx.locks, fromLockHandle)
	
	toLockHandle, err := tc.lockManager.AcquireEntityLock(ctx, relation.ToEntityType, relation.ToEntityID, 30*time.Second)
	if err != nil {
		return fmt.Errorf("failed to acquire to entity lock: %w", err)
	}
	txCtx.locks = append(txCtx.locks, toLockHandle)
	
	// Create in primary store
	if err := txCtx.primaryTx.CreateRelation(ctx, relation); err != nil {
		return fmt.Errorf("failed to create relation in primary store: %w", err)
	}
	
	return nil
}

// DeleteRelation deletes a relation within a transaction
func (tc *TransactionCoordinator) DeleteRelation(ctx context.Context, txCtx *TransactionContext, relationID uuid.UUID) error {
	// Get the relation first to know which entities to lock
	relation, err := tc.primaryStore.GetRelation(ctx, relationID)
	if err != nil {
		return fmt.Errorf("failed to get relation: %w", err)
	}
	
	// Acquire locks for both entities
	fromLockHandle, err := tc.lockManager.AcquireEntityLock(ctx, relation.FromEntityType, relation.FromEntityID, 30*time.Second)
	if err != nil {
		return fmt.Errorf("failed to acquire from entity lock: %w", err)
	}
	txCtx.locks = append(txCtx.locks, fromLockHandle)
	
	toLockHandle, err := tc.lockManager.AcquireEntityLock(ctx, relation.ToEntityType, relation.ToEntityID, 30*time.Second)
	if err != nil {
		return fmt.Errorf("failed to acquire to entity lock: %w", err)
	}
	txCtx.locks = append(txCtx.locks, toLockHandle)
	
	// Delete from primary store
	if err := txCtx.primaryTx.DeleteRelation(ctx, relationID); err != nil {
		return fmt.Errorf("failed to delete relation from primary store: %w", err)
	}
	
	return nil
}

// CommitTransaction commits the two-phase transaction
func (tc *TransactionCoordinator) CommitTransaction(ctx context.Context, txCtx *TransactionContext) error {
	// Phase 1: Prepare - commit primary store transaction
	if err := txCtx.primaryTx.Commit(); err != nil {
		// Rollback if primary commit fails
		tc.rollbackTransaction(ctx, txCtx)
		return utils.NewAppError(utils.CodeTransactionFailed, "primary store commit failed", err)
	}
	
	// Phase 2: Apply index operations
	var indexErrors []error
	for _, op := range txCtx.indexOperations {
		if err := tc.applyIndexOperation(ctx, op); err != nil {
			indexErrors = append(indexErrors, err)
		}
	}
	
	// Release all locks
	tc.releaseLocks(ctx, txCtx)
	
	// Report index errors but don't fail the transaction
	// Index can be rebuilt if necessary
	if len(indexErrors) > 0 {
		return utils.NewAppError(utils.CodeInternal, "index operations failed", fmt.Errorf("%d index operations failed", len(indexErrors)))
	}
	
	return nil
}

// RollbackTransaction rolls back the transaction
func (tc *TransactionCoordinator) RollbackTransaction(ctx context.Context, txCtx *TransactionContext) error {
	return tc.rollbackTransaction(ctx, txCtx)
}

// rollbackTransaction performs the actual rollback
func (tc *TransactionCoordinator) rollbackTransaction(ctx context.Context, txCtx *TransactionContext) error {
	// Rollback primary transaction
	var rollbackErr error
	if txCtx.primaryTx != nil {
		rollbackErr = txCtx.primaryTx.Rollback()
	}
	
	// Release all locks
	tc.releaseLocks(ctx, txCtx)
	
	return rollbackErr
}

// applyIndexOperation applies a single index operation
func (tc *TransactionCoordinator) applyIndexOperation(ctx context.Context, op IndexOperation) error {
	switch op.Type {
	case "index":
		return tc.indexStore.IndexEntity(ctx, op.Entity)
	case "update":
		return tc.indexStore.UpdateEntityIndex(ctx, op.Entity)
	case "delete":
		return tc.indexStore.DeleteEntityIndex(ctx, op.EntityType, op.EntityID)
	default:
		return fmt.Errorf("unknown index operation type: %s", op.Type)
	}
}

// releaseLocks releases all acquired locks
func (tc *TransactionCoordinator) releaseLocks(ctx context.Context, txCtx *TransactionContext) {
	for _, lockHandle := range txCtx.locks {
		if err := tc.lockManager.ReleaseLock(ctx, lockHandle); err != nil {
			// Log error but continue releasing other locks
			// In a real implementation, you'd use proper logging
			fmt.Printf("Failed to release lock %s: %v\n", lockHandle.Resource(), err)
		}
	}
	txCtx.locks = nil
}

// WithTransaction executes a function within a transaction
func (tc *TransactionCoordinator) WithTransaction(ctx context.Context, fn func(context.Context, *TransactionContext) error) error {
	txCtx, err := tc.BeginTransaction(ctx)
	if err != nil {
		return err
	}
	
	// Execute the function
	if err := fn(ctx, txCtx); err != nil {
		// Rollback on error
		if rollbackErr := tc.RollbackTransaction(ctx, txCtx); rollbackErr != nil {
			return fmt.Errorf("function failed: %w, rollback failed: %v", err, rollbackErr)
		}
		return err
	}
	
	// Commit if successful
	return tc.CommitTransaction(ctx, txCtx)
}

// TransactionStats holds transaction statistics
type TransactionStats struct {
	ActiveTransactions   int
	TotalCommitted      uint64
	TotalRolledBack     uint64
	TotalIndexErrors    uint64
	AverageCommitTime   time.Duration
	CommitTimeouts      uint64
}

// TransactionManager provides high-level transaction management
type TransactionManager struct {
	coordinator *TransactionCoordinator
	stats       TransactionStats
	timeout     time.Duration
}

// NewTransactionManager creates a new transaction manager
func NewTransactionManager(coordinator *TransactionCoordinator, timeout time.Duration) *TransactionManager {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	
	return &TransactionManager{
		coordinator: coordinator,
		timeout:     timeout,
	}
}

// ExecuteWithTimeout executes a transaction function with a timeout
func (tm *TransactionManager) ExecuteWithTimeout(ctx context.Context, fn func(context.Context, *TransactionContext) error) error {
	// Create context with timeout
	timeoutCtx, cancel := context.WithTimeout(ctx, tm.timeout)
	defer cancel()
	
	// Execute transaction
	startTime := time.Now()
	err := tm.coordinator.WithTransaction(timeoutCtx, fn)
	duration := time.Since(startTime)
	
	// Update statistics
	if err != nil {
		if timeoutCtx.Err() == context.DeadlineExceeded {
			tm.stats.CommitTimeouts++
		} else {
			tm.stats.TotalRolledBack++
		}
	} else {
		tm.stats.TotalCommitted++
		// Update average commit time
		if tm.stats.TotalCommitted == 1 {
			tm.stats.AverageCommitTime = duration
		} else {
			tm.stats.AverageCommitTime = (tm.stats.AverageCommitTime + duration) / 2
		}
	}
	
	return err
}

// GetStats returns transaction statistics
func (tm *TransactionManager) GetStats() TransactionStats {
	return tm.stats
}

// BatchProcessor processes multiple operations in a single transaction
type BatchProcessor struct {
	coordinator *TransactionCoordinator
	batchSize   int
}

// NewBatchProcessor creates a new batch processor
func NewBatchProcessor(coordinator *TransactionCoordinator, batchSize int) *BatchProcessor {
	if batchSize <= 0 {
		batchSize = 100
	}
	
	return &BatchProcessor{
		coordinator: coordinator,
		batchSize:   batchSize,
	}
}

// BatchOperation represents a single operation in a batch
type BatchOperation struct {
	Type     string // "create_entity", "update_entity", "delete_entity", "create_relation", "delete_relation"
	Entity   *models.Entity
	Relation *models.Relation
	EntityType string
	EntityID   uuid.UUID
	RelationID uuid.UUID
}

// ProcessBatch processes a batch of operations
func (bp *BatchProcessor) ProcessBatch(ctx context.Context, operations []BatchOperation) error {
	// Process in chunks
	for i := 0; i < len(operations); i += bp.batchSize {
		end := i + bp.batchSize
		if end > len(operations) {
			end = len(operations)
		}
		
		chunk := operations[i:end]
		if err := bp.processChunk(ctx, chunk); err != nil {
			return fmt.Errorf("failed to process batch chunk %d-%d: %w", i, end, err)
		}
	}
	
	return nil
}

// processChunk processes a single chunk of operations
func (bp *BatchProcessor) processChunk(ctx context.Context, operations []BatchOperation) error {
	return bp.coordinator.WithTransaction(ctx, func(ctx context.Context, txCtx *TransactionContext) error {
		for _, op := range operations {
			if err := bp.processOperation(ctx, txCtx, op); err != nil {
				return err
			}
		}
		return nil
	})
}

// processOperation processes a single operation
func (bp *BatchProcessor) processOperation(ctx context.Context, txCtx *TransactionContext, op BatchOperation) error {
	switch op.Type {
	case "create_entity":
		return bp.coordinator.CreateEntity(ctx, txCtx, op.Entity)
	case "update_entity":
		return bp.coordinator.UpdateEntity(ctx, txCtx, op.Entity)
	case "delete_entity":
		return bp.coordinator.DeleteEntity(ctx, txCtx, op.EntityType, op.EntityID)
	case "create_relation":
		return bp.coordinator.CreateRelation(ctx, txCtx, op.Relation)
	case "delete_relation":
		return bp.coordinator.DeleteRelation(ctx, txCtx, op.RelationID)
	default:
		return fmt.Errorf("unknown operation type: %s", op.Type)
	}
}