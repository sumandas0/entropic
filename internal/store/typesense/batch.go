package typesense

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sumandas0/entropic/internal/models"
	"github.com/typesense/typesense-go/typesense/api"
)

type BatchIndexer struct {
	store     *TypesenseStore
	batchSize int
	flushInterval time.Duration
	
	mu       sync.Mutex
	batches  map[string][]map[string]interface{} 
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

func NewBatchIndexer(store *TypesenseStore, batchSize int, flushInterval time.Duration) *BatchIndexer {
	return &BatchIndexer{
		store:         store,
		batchSize:     batchSize,
		flushInterval: flushInterval,
		batches:       make(map[string][]map[string]interface{}),
		stopCh:        make(chan struct{}),
	}
}

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

func (b *BatchIndexer) Stop() {
	close(b.stopCh)
	b.wg.Wait()
}

func (b *BatchIndexer) IndexEntity(ctx context.Context, entity *models.Entity) error {
	collectionName := b.store.getCollectionName(entity.EntityType)
	document := b.store.flattenEntity(entity)
	
	b.mu.Lock()
	defer b.mu.Unlock()
	
	if b.batches[collectionName] == nil {
		b.batches[collectionName] = make([]map[string]interface{}, 0, b.batchSize)
	}
	
	b.batches[collectionName] = append(b.batches[collectionName], document)

	if len(b.batches[collectionName]) >= b.batchSize {
		batch := b.batches[collectionName]
		b.batches[collectionName] = nil

		go b.flushBatch(ctx, collectionName, batch)
	}
	
	return nil
}

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

func (b *BatchIndexer) flushBatch(ctx context.Context, collectionName string, documents []map[string]interface{}) {
	if len(documents) == 0 {
		return
	}

	interfaceDocs := make([]interface{}, len(documents))
	for i, doc := range documents {
		interfaceDocs[i] = doc
	}

	action := "upsert"
	params := &api.ImportDocumentsParams{
		Action: &action,
	}
	_, err := b.store.client.Collection(collectionName).Documents().Import(ctx, interfaceDocs, params)
	if err != nil {
		
		fmt.Printf("Failed to batch index %d documents to collection %s: %v\n", 
			len(documents), collectionName, err)

		b.retryIndividual(ctx, collectionName, documents)
	}
}

func (b *BatchIndexer) retryIndividual(ctx context.Context, collectionName string, documents []map[string]interface{}) {
	for _, doc := range documents {
		_, err := b.store.client.Collection(collectionName).Documents().Upsert(ctx, doc)
		if err != nil {
			
			if id, ok := doc["id"].(string); ok {
				fmt.Printf("Failed to index document %s: %v\n", id, err)
			}
		}
	}
}

type BulkReindexer struct {
	store       *TypesenseStore
	batchSize   int
	concurrency int
}

func NewBulkReindexer(store *TypesenseStore, batchSize, concurrency int) *BulkReindexer {
	return &BulkReindexer{
		store:       store,
		batchSize:   batchSize,
		concurrency: concurrency,
	}
}

func (r *BulkReindexer) ReindexEntities(ctx context.Context, entities []*models.Entity) error {
	
	entityGroups := make(map[string][]*models.Entity)
	for _, entity := range entities {
		entityGroups[entity.EntityType] = append(entityGroups[entity.EntityType], entity)
	}

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

	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}
	
	if len(errs) > 0 {
		return fmt.Errorf("reindexing failed with %d errors: %v", len(errs), errs[0])
	}
	
	return nil
}

func (r *BulkReindexer) reindexEntityType(ctx context.Context, entityType string, entities []*models.Entity) error {
	collectionName := r.store.getCollectionName(entityType)

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

		interfaceDocs := make([]interface{}, len(documents))
		for j, doc := range documents {
			interfaceDocs[j] = doc
		}

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

type ProgressCallback func(processed, total int)

func (r *BulkReindexer) ReindexWithProgress(ctx context.Context, entities []*models.Entity, callback ProgressCallback) error {
	total := len(entities)
	processed := 0

	entityGroups := make(map[string][]*models.Entity)
	for _, entity := range entities {
		entityGroups[entity.EntityType] = append(entityGroups[entity.EntityType], entity)
	}
	
	for entityType, group := range entityGroups {
		collectionName := r.store.getCollectionName(entityType)

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

			interfaceDocs := make([]interface{}, len(documents))
			for j, doc := range documents {
				interfaceDocs[j] = doc
			}

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