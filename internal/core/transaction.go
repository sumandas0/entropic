package core

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog"
	"github.com/sumandas0/entropic/internal/integration"
	"github.com/sumandas0/entropic/internal/lock"
	"github.com/sumandas0/entropic/internal/models"
	"github.com/sumandas0/entropic/internal/observability"
	"github.com/sumandas0/entropic/internal/store"
	"github.com/sumandas0/entropic/pkg/utils"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type TransactionCoordinator struct {
	primaryStore store.PrimaryStore
	indexStore   store.IndexStore
	lockManager  *lock.LockManager
	obsManager   *integration.ObservabilityManager
	logger       zerolog.Logger
	tracer       trace.Tracer
	tracing      *observability.TracingManager
}

func NewTransactionCoordinator(primaryStore store.PrimaryStore, indexStore store.IndexStore, lockManager *lock.LockManager) *TransactionCoordinator {
	return &TransactionCoordinator{
		primaryStore: primaryStore,
		indexStore:   indexStore,
		lockManager:  lockManager,
	}
}

func (tc *TransactionCoordinator) SetObservability(obsManager *integration.ObservabilityManager) {
	if obsManager != nil {
		tc.obsManager = obsManager
		tc.logger = obsManager.GetLogging().GetZerologLogger()
		tc.tracer = obsManager.GetTracing().GetTracer()
		tc.tracing = obsManager.GetTracing()
	}
}

type TransactionContext struct {
	ID             string
	primaryTx      store.Transaction
	indexOperations []IndexOperation
	locks          []lock.LockHandle
	startTime      time.Time
}

type IndexOperation struct {
	Type     string 
	Entity   *models.Entity
	EntityType string
	EntityID uuid.UUID
}

func (tc *TransactionCoordinator) BeginTransaction(ctx context.Context) (*TransactionContext, error) {
	var span trace.Span
	if tc.tracer != nil {
		ctx, span = tc.tracer.Start(ctx, "transaction.begin")
		defer span.End()
	}
	
	primaryTx, err := tc.primaryStore.BeginTx(ctx)
	if err != nil {
		if tc.tracing != nil {
			tc.tracing.SetSpanError(span, err)
		}
		return nil, fmt.Errorf("failed to begin primary transaction: %w", err)
	}
	
	txID := uuid.New().String()
	txCtx := &TransactionContext{
		ID:              txID,
		primaryTx:       primaryTx,
		indexOperations: make([]IndexOperation, 0),
		locks:           make([]lock.LockHandle, 0),
		startTime:       time.Now(),
	}
	
	if span != nil {
		span.SetAttributes(attribute.String("transaction.id", txID))
	}
	
	return txCtx, nil
}

func (tc *TransactionCoordinator) CreateEntity(ctx context.Context, txCtx *TransactionContext, entity *models.Entity) error {
	var span trace.Span
	if tc.tracing != nil {
		ctx, span = tc.tracing.StartDatabaseOperation(ctx, "create_entity", "entities")
		defer span.End()
		span.SetAttributes(
			attribute.String("entity.type", entity.EntityType),
			attribute.String("entity.id", entity.ID.String()),
		)
	}
	
	lockHandle, err := tc.lockManager.AcquireEntityLock(ctx, entity.EntityType, entity.ID, 30*time.Second)
	if err != nil {
		if tc.tracing != nil {
			tc.tracing.SetSpanError(span, err)
		}
		return fmt.Errorf("failed to acquire entity lock: %w", err)
	}
	txCtx.locks = append(txCtx.locks, lockHandle)

	if err := txCtx.primaryTx.CreateEntity(ctx, entity); err != nil {
		if tc.tracing != nil {
			tc.tracing.SetSpanError(span, err)
		}
		return fmt.Errorf("failed to create entity in primary store: %w", err)
	}

	txCtx.indexOperations = append(txCtx.indexOperations, IndexOperation{
		Type:   "index",
		Entity: entity,
	})
	
	return nil
}

func (tc *TransactionCoordinator) UpdateEntity(ctx context.Context, txCtx *TransactionContext, entity *models.Entity) error {
	var span trace.Span
	if tc.tracing != nil {
		ctx, span = tc.tracing.StartDatabaseOperation(ctx, "update_entity", "entities")
		defer span.End()
		span.SetAttributes(
			attribute.String("entity.type", entity.EntityType),
			attribute.String("entity.id", entity.ID.String()),
		)
	}
	
	lockHandle, err := tc.lockManager.AcquireEntityLock(ctx, entity.EntityType, entity.ID, 30*time.Second)
	if err != nil {
		if tc.tracing != nil {
			tc.tracing.SetSpanError(span, err)
		}
		return fmt.Errorf("failed to acquire entity lock: %w", err)
	}
	txCtx.locks = append(txCtx.locks, lockHandle)

	if err := txCtx.primaryTx.UpdateEntity(ctx, entity); err != nil {
		if tc.tracing != nil {
			tc.tracing.SetSpanError(span, err)
		}
		return fmt.Errorf("failed to update entity in primary store: %w", err)
	}

	txCtx.indexOperations = append(txCtx.indexOperations, IndexOperation{
		Type:   "update",
		Entity: entity,
	})
	
	return nil
}

func (tc *TransactionCoordinator) DeleteEntity(ctx context.Context, txCtx *TransactionContext, entityType string, entityID uuid.UUID) error {
	var span trace.Span
	if tc.tracing != nil {
		ctx, span = tc.tracing.StartDatabaseOperation(ctx, "delete_entity", "entities")
		defer span.End()
		span.SetAttributes(
			attribute.String("entity.type", entityType),
			attribute.String("entity.id", entityID.String()),
		)
	}
	
	lockHandle, err := tc.lockManager.AcquireEntityLock(ctx, entityType, entityID, 30*time.Second)
	if err != nil {
		if tc.tracing != nil {
			tc.tracing.SetSpanError(span, err)
		}
		return fmt.Errorf("failed to acquire entity lock: %w", err)
	}
	txCtx.locks = append(txCtx.locks, lockHandle)

	if err := txCtx.primaryTx.DeleteEntity(ctx, entityType, entityID); err != nil {
		if tc.tracing != nil {
			tc.tracing.SetSpanError(span, err)
		}
		return fmt.Errorf("failed to delete entity from primary store: %w", err)
	}

	txCtx.indexOperations = append(txCtx.indexOperations, IndexOperation{
		Type:       "delete",
		EntityType: entityType,
		EntityID:   entityID,
	})
	
	return nil
}

func (tc *TransactionCoordinator) CreateRelation(ctx context.Context, txCtx *TransactionContext, relation *models.Relation) error {
	var span trace.Span
	if tc.tracing != nil {
		ctx, span = tc.tracing.StartDatabaseOperation(ctx, "create_relation", "relations")
		defer span.End()
		span.SetAttributes(
			attribute.String("relation.type", relation.RelationType),
			attribute.String("relation.id", relation.ID.String()),
		)
	}
	
	fromLockHandle, err := tc.lockManager.AcquireEntityLock(ctx, relation.FromEntityType, relation.FromEntityID, 30*time.Second)
	if err != nil {
		if tc.tracing != nil {
			tc.tracing.SetSpanError(span, err)
		}
		return fmt.Errorf("failed to acquire from entity lock: %w", err)
	}
	txCtx.locks = append(txCtx.locks, fromLockHandle)
	
	toLockHandle, err := tc.lockManager.AcquireEntityLock(ctx, relation.ToEntityType, relation.ToEntityID, 30*time.Second)
	if err != nil {
		if tc.tracing != nil {
			tc.tracing.SetSpanError(span, err)
		}
		return fmt.Errorf("failed to acquire to entity lock: %w", err)
	}
	txCtx.locks = append(txCtx.locks, toLockHandle)

	if err := txCtx.primaryTx.CreateRelation(ctx, relation); err != nil {
		if tc.tracing != nil {
			tc.tracing.SetSpanError(span, err)
		}
		return fmt.Errorf("failed to create relation in primary store: %w", err)
	}
	
	return nil
}

func (tc *TransactionCoordinator) DeleteRelation(ctx context.Context, txCtx *TransactionContext, relationID uuid.UUID) error {
	
	relation, err := tc.primaryStore.GetRelation(ctx, relationID)
	if err != nil {
		return fmt.Errorf("failed to get relation: %w", err)
	}

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

	if err := txCtx.primaryTx.DeleteRelation(ctx, relationID); err != nil {
		return fmt.Errorf("failed to delete relation from primary store: %w", err)
	}
	
	return nil
}

func (tc *TransactionCoordinator) CommitTransaction(ctx context.Context, txCtx *TransactionContext) error {
	var span trace.Span
	if tc.tracer != nil {
		ctx, span = tc.tracer.Start(ctx, "transaction.commit")
		defer span.End()
		span.SetAttributes(
			attribute.String("transaction.id", txCtx.ID),
			attribute.Int("index_operations.count", len(txCtx.indexOperations)),
		)
	}
	
	if err := txCtx.primaryTx.Commit(); err != nil {
		if tc.tracing != nil {
			tc.tracing.SetSpanError(span, err)
		}
		tc.rollbackTransaction(ctx, txCtx)
		return utils.NewAppError(utils.CodeTransactionFailed, "primary store commit failed", err)
	}

	var indexErrors []error
	for _, op := range txCtx.indexOperations {
		if err := tc.applyIndexOperation(ctx, op); err != nil {
			indexErrors = append(indexErrors, err)
		}
	}

	tc.releaseLocks(ctx, txCtx)

	if len(indexErrors) > 0 {
		err := utils.NewAppError(utils.CodeInternal, "index operations failed", fmt.Errorf("%d index operations failed", len(indexErrors)))
		if tc.tracing != nil {
			tc.tracing.SetSpanError(span, err)
		}
		return err
	}
	
	return nil
}

func (tc *TransactionCoordinator) RollbackTransaction(ctx context.Context, txCtx *TransactionContext) error {
	return tc.rollbackTransaction(ctx, txCtx)
}

func (tc *TransactionCoordinator) rollbackTransaction(ctx context.Context, txCtx *TransactionContext) error {
	
	var rollbackErr error
	if txCtx.primaryTx != nil {
		rollbackErr = txCtx.primaryTx.Rollback()
	}

	tc.releaseLocks(ctx, txCtx)
	
	return rollbackErr
}

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

func (tc *TransactionCoordinator) releaseLocks(ctx context.Context, txCtx *TransactionContext) {
	for _, lockHandle := range txCtx.locks {
		if err := tc.lockManager.ReleaseLock(ctx, lockHandle); err != nil {
			if tc.logger.GetLevel() != zerolog.Disabled {
				tc.logger.Error().
					Err(err).
					Str("resource", lockHandle.Resource()).
					Msg("Failed to release lock")
			}
		}
	}
	txCtx.locks = nil
}

func (tc *TransactionCoordinator) WithTransaction(ctx context.Context, fn func(context.Context, *TransactionContext) error) error {
	var span trace.Span
	if tc.tracer != nil {
		ctx, span = tc.tracer.Start(ctx, "transaction.with")
		defer span.End()
	}

	txCtx, err := tc.BeginTransaction(ctx)
	if err != nil {
		return err
	}

	if err := fn(ctx, txCtx); err != nil {
		if rollbackErr := tc.RollbackTransaction(ctx, txCtx); rollbackErr != nil {
			return fmt.Errorf("function failed: %w, rollback failed: %v", err, rollbackErr)
		}
		return err
	}

	return tc.CommitTransaction(ctx, txCtx)
}

type TransactionStats struct {
	ActiveTransactions   int
	TotalCommitted      uint64
	TotalRolledBack     uint64
	TotalIndexErrors    uint64
	AverageCommitTime   time.Duration
	CommitTimeouts      uint64
}

type TransactionManager struct {
	coordinator *TransactionCoordinator
	stats       TransactionStats
	timeout     time.Duration
	obsManager  *integration.ObservabilityManager
	logger      zerolog.Logger
	tracer      trace.Tracer
	tracing     *observability.TracingManager
}

func NewTransactionManager(coordinator *TransactionCoordinator, timeout time.Duration) *TransactionManager {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	
	return &TransactionManager{
		coordinator: coordinator,
		timeout:     timeout,
	}
}

func (tm *TransactionManager) SetObservability(obsManager *integration.ObservabilityManager) {
	if obsManager != nil {
		tm.obsManager = obsManager
		tm.logger = obsManager.GetLogging().GetZerologLogger()
		tm.tracer = obsManager.GetTracing().GetTracer()
		tm.tracing = obsManager.GetTracing()
		tm.coordinator.SetObservability(obsManager)
	}
}

func (tm *TransactionManager) ExecuteWithTimeout(ctx context.Context, fn func(context.Context, *TransactionContext) error) error {
	var span trace.Span
	if tm.tracer != nil {
		ctx, span = tm.tracer.Start(ctx, "transaction.execute_with_timeout")
		defer span.End()
		span.SetAttributes(attribute.String("timeout", tm.timeout.String()))
	}
	
	timeoutCtx, cancel := context.WithTimeout(ctx, tm.timeout)
	defer cancel()

	startTime := time.Now()
	err := tm.coordinator.WithTransaction(timeoutCtx, fn)
	duration := time.Since(startTime)

	if span != nil {
		span.SetAttributes(attribute.String("duration", duration.String()))
	}

	if err != nil {
		if timeoutCtx.Err() == context.DeadlineExceeded {
			tm.stats.CommitTimeouts++
			if tm.tracing != nil {
				tm.tracing.AddSpanEvent(span, "transaction.timeout")
			}
		} else {
			tm.stats.TotalRolledBack++
			if tm.tracing != nil {
				tm.tracing.SetSpanError(span, err)
			}
		}
	} else {
		tm.stats.TotalCommitted++
		
		if tm.stats.TotalCommitted == 1 {
			tm.stats.AverageCommitTime = duration
		} else {
			tm.stats.AverageCommitTime = (tm.stats.AverageCommitTime + duration) / 2
		}
	}
	
	return err
}

func (tm *TransactionManager) GetStats() TransactionStats {
	return tm.stats
}

type BatchProcessor struct {
	coordinator *TransactionCoordinator
	batchSize   int
}

func NewBatchProcessor(coordinator *TransactionCoordinator, batchSize int) *BatchProcessor {
	if batchSize <= 0 {
		batchSize = 100
	}
	
	return &BatchProcessor{
		coordinator: coordinator,
		batchSize:   batchSize,
	}
}

type BatchOperation struct {
	Type     string 
	Entity   *models.Entity
	Relation *models.Relation
	EntityType string
	EntityID   uuid.UUID
	RelationID uuid.UUID
}

func (bp *BatchProcessor) ProcessBatch(ctx context.Context, operations []BatchOperation) error {
	
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