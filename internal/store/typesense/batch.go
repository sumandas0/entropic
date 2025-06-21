package typesense

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/entropic/entropic/internal/models"
	"github.com/typesense/typesense-go/typesense/api"
)

// BatchIndexer provides batch indexing capabilities
type BatchIndexer struct {
	store     *TypesenseStore
	batchSize int
	flushInterval time.Duration
	
	mu       sync.Mutex
	batches  map[string][]map[string]interface{} // collection name -> documents
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

// NewBatchIndexer creates a new batch indexer
func NewBatchIndexer(store *TypesenseStore, batchSize int, flushInterval time.Duration) *BatchIndexer {
	return &BatchIndexer{
		store:         store,
		batchSize:     batchSize,
		flushInterval: flushInterval,
		batches:       make(map[string][]map[string]interface{}),
		stopCh:        make(chan struct{}),
	}
}

// Start starts the batch indexer
func (b *BatchIndexer) Start(ctx context.Context) {
	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		ticker := time.NewTicker(b.flushInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				b.flushAll(ctx)
			case <-b.stopCh:
				b.flushAll(ctx)
				return
			case <-ctx.Done():
				b.flushAll(ctx)
				return
			}
		}
	}()
}

// Stop stops the batch indexer and flushes remaining documents
func (b *BatchIndexer) Stop() {
	close(b.stopCh)
	b.wg.Wait()
}

// IndexEntity adds an entity to the batch
func (b *BatchIndexer) IndexEntity(ctx context.Context, entity *models.Entity) error {
	collectionName := b.store.getCollectionName(entity.EntityType)
	document := b.store.flattenEntity(entity)
	
	b.mu.Lock()
	defer b.mu.Unlock()
	
	if b.batches[collectionName] == nil {
		b.batches[collectionName] = make([]map[string]interface{}, 0, b.batchSize)
	}
	
	b.batches[collectionName] = append(b.batches[collectionName], document)
	
	// Flush if batch is full
	if len(b.batches[collectionName]) >= b.batchSize {
		batch := b.batches[collectionName]
		b.batches[collectionName] = nil
		
		// Flush in background
		go b.flushBatch(ctx, collectionName, batch)
	}
	
	return nil
}

// flushAll flushes all pending batches
func (b *BatchIndexer) flushAll(ctx context.Context) {
	b.mu.Lock()
	batches := make(map[string][]map[string]interface{})
	for collection, docs := range b.batches {
		if len(docs) > 0 {
			batches[collection] = docs
			b.batches[collection] = nil
		}
	}
	b.mu.Unlock()
	
	// Flush all batches concurrently
	var wg sync.WaitGroup
	for collection, docs := range batches {
		wg.Add(1)
		go func(col string, documents []map[string]interface{}) {
			defer wg.Done()
			b.flushBatch(ctx, col, documents)
		}(collection, docs)
	}
	wg.Wait()
}

// flushBatch flushes a single batch of documents
func (b *BatchIndexer) flushBatch(ctx context.Context, collectionName string, documents []map[string]interface{}) {
	if len(documents) == 0 {
		return
	}
	
	// Convert documents to []interface{} as expected by Typesense
	interfaceDocs := make([]interface{}, len(documents))
	for i, doc := range documents {
		interfaceDocs[i] = doc
	}
	
	// Import documents with action parameter
	action := "upsert"
	params := &api.ImportDocumentsParams{
		Action: &action,
	}
	_, err := b.store.client.Collection(collectionName).Documents().Import(ctx, interfaceDocs, params)
	if err != nil {
		// Log error (you would use your logging framework here)
		fmt.Printf("Failed to batch index %d documents to collection %s: %v\n", 
			len(documents), collectionName, err)
		
		// Optionally, retry individual documents on batch failure
		b.retryIndividual(ctx, collectionName, documents)
	}
}

// retryIndividual retries indexing documents individually after batch failure
func (b *BatchIndexer) retryIndividual(ctx context.Context, collectionName string, documents []map[string]interface{}) {
	for _, doc := range documents {
		_, err := b.store.client.Collection(collectionName).Documents().Upsert(ctx, doc)
		if err != nil {
			// Log individual failure
			if id, ok := doc["id"].(string); ok {
				fmt.Printf("Failed to index document %s: %v\n", id, err)
			}
		}
	}
}

// BulkReindexer provides bulk reindexing capabilities
type BulkReindexer struct {
	store       *TypesenseStore
	batchSize   int
	concurrency int
}

// NewBulkReindexer creates a new bulk reindexer
func NewBulkReindexer(store *TypesenseStore, batchSize, concurrency int) *BulkReindexer {
	return &BulkReindexer{
		store:       store,
		batchSize:   batchSize,
		concurrency: concurrency,
	}
}

// ReindexEntities reindexes a batch of entities
func (r *BulkReindexer) ReindexEntities(ctx context.Context, entities []*models.Entity) error {
	// Group entities by type
	entityGroups := make(map[string][]*models.Entity)
	for _, entity := range entities {
		entityGroups[entity.EntityType] = append(entityGroups[entity.EntityType], entity)
	}
	
	// Process each entity type concurrently
	errCh := make(chan error, len(entityGroups))
	var wg sync.WaitGroup
	
	for entityType, group := range entityGroups {
		wg.Add(1)
		go func(eType string, entities []*models.Entity) {
			defer wg.Done()
			if err := r.reindexEntityType(ctx, eType, entities); err != nil {
				errCh <- fmt.Errorf("failed to reindex entity type %s: %w", eType, err)
			}
		}(entityType, group)
	}
	
	wg.Wait()
	close(errCh)
	
	// Collect any errors
	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}
	
	if len(errs) > 0 {
		return fmt.Errorf("reindexing failed with %d errors: %v", len(errs), errs[0])
	}
	
	return nil
}

// reindexEntityType reindexes all entities of a specific type
func (r *BulkReindexer) reindexEntityType(ctx context.Context, entityType string, entities []*models.Entity) error {
	collectionName := r.store.getCollectionName(entityType)
	
	// Process in batches
	for i := 0; i < len(entities); i += r.batchSize {
		end := i + r.batchSize
		if end > len(entities) {
			end = len(entities)
		}
		
		batch := entities[i:end]
		documents := make([]map[string]interface{}, len(batch))
		
		for j, entity := range batch {
			documents[j] = r.store.flattenEntity(entity)
		}
		
		// Convert documents to []interface{} as expected by Typesense
		interfaceDocs := make([]interface{}, len(documents))
		for j, doc := range documents {
			interfaceDocs[j] = doc
		}
		
		// Import batch
		action := "upsert"
		params := &api.ImportDocumentsParams{
			Action: &action,
		}
		_, err := r.store.client.Collection(collectionName).Documents().Import(ctx, interfaceDocs, params)
		if err != nil {
			return fmt.Errorf("failed to import batch %d-%d: %w", i, end, err)
		}
	}
	
	return nil
}

// ProgressCallback is called during reindexing to report progress
type ProgressCallback func(processed, total int)

// ReindexWithProgress reindexes entities with progress reporting
func (r *BulkReindexer) ReindexWithProgress(ctx context.Context, entities []*models.Entity, callback ProgressCallback) error {
	total := len(entities)
	processed := 0
	
	// Group entities by type
	entityGroups := make(map[string][]*models.Entity)
	for _, entity := range entities {
		entityGroups[entity.EntityType] = append(entityGroups[entity.EntityType], entity)
	}
	
	for entityType, group := range entityGroups {
		collectionName := r.store.getCollectionName(entityType)
		
		// Process in batches
		for i := 0; i < len(group); i += r.batchSize {
			end := i + r.batchSize
			if end > len(group) {
				end = len(group)
			}
			
			batch := group[i:end]
			documents := make([]map[string]interface{}, len(batch))
			
			for j, entity := range batch {
				documents[j] = r.store.flattenEntity(entity)
			}
			
			// Convert documents to []interface{} as expected by Typesense
			interfaceDocs := make([]interface{}, len(documents))
			for j, doc := range documents {
				interfaceDocs[j] = doc
			}
			
			// Import batch
			action := "upsert"
			params := &api.ImportDocumentsParams{
				Action: &action,
			}
			_, err := r.store.client.Collection(collectionName).Documents().Import(ctx, interfaceDocs, params)
			if err != nil {
				return fmt.Errorf("failed to import batch for %s: %w", entityType, err)
			}
			
			processed += len(batch)
			if callback != nil {
				callback(processed, total)
			}
		}
	}
	
	return nil
}