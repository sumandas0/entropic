package testutils

import (
	"context"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/sumandas0/entropic/internal/models"
	"github.com/sumandas0/entropic/internal/store"
)

// MockTypesenseStore is a simple in-memory implementation for testing
type MockTypesenseStore struct {
	mu         sync.RWMutex
	documents  map[string]map[string]interface{} // entityType -> id -> document
	closed     bool
}

// NewMockTypesenseStore creates a new mock Typesense store
func NewMockTypesenseStore() *MockTypesenseStore {
	return &MockTypesenseStore{
		documents: make(map[string]map[string]interface{}),
	}
}

// IndexEntity implements store.IndexStore
func (m *MockTypesenseStore) IndexEntity(ctx context.Context, entity *models.Entity) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if m.closed {
		return fmt.Errorf("store is closed")
	}
	
	if m.documents[entity.EntityType] == nil {
		m.documents[entity.EntityType] = make(map[string]interface{})
	}
	
	doc := map[string]interface{}{
		"id":          entity.ID.String(),
		"entity_type": entity.EntityType,
		"urn":         entity.URN,
		"created_at":  entity.CreatedAt,
		"updated_at":  entity.UpdatedAt,
	}
	
	// Flatten properties
	for k, v := range entity.Properties {
		doc[k] = v
	}
	
	m.documents[entity.EntityType][entity.ID.String()] = doc
	return nil
}

// UpdateEntityIndex implements store.IndexStore
func (m *MockTypesenseStore) UpdateEntityIndex(ctx context.Context, entity *models.Entity) error {
	return m.IndexEntity(ctx, entity)
}

// DeleteEntityIndex implements store.IndexStore
func (m *MockTypesenseStore) DeleteEntityIndex(ctx context.Context, entityType string, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if m.closed {
		return fmt.Errorf("store is closed")
	}
	
	if m.documents[entityType] != nil {
		delete(m.documents[entityType], id.String())
	}
	
	return nil
}


// Search implements store.IndexStore
func (m *MockTypesenseStore) Search(ctx context.Context, query *models.SearchQuery) (*models.SearchResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	if m.closed {
		return nil, fmt.Errorf("store is closed")
	}
	
	var hits []models.SearchHit
	
	// Simple text search simulation
	for _, entityType := range query.EntityTypes {
		if docs, ok := m.documents[entityType]; ok {
			for _, doc := range docs {
				// Very basic search - just return all documents
				docMap := doc.(map[string]interface{})
				hit := models.SearchHit{
					ID:         uuid.MustParse(docMap["id"].(string)),
					EntityType: entityType,
					Score:      1.0,
					Properties: make(map[string]interface{}),
				}
				
				// Copy properties
				for k, v := range docMap {
					if k != "id" && k != "entity_type" {
						hit.Properties[k] = v
					}
				}
				
				hits = append(hits, hit)
				
				// Respect limit
				if len(hits) >= query.Limit {
					break
				}
			}
		}
	}
	
	return &models.SearchResult{
		Hits:      hits,
		TotalHits: int64(len(hits)),
		Facets:    make(map[string][]models.FacetValue),
	}, nil
}

// VectorSearch implements store.IndexStore
func (m *MockTypesenseStore) VectorSearch(ctx context.Context, query *models.VectorQuery) (*models.SearchResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	if m.closed {
		return nil, fmt.Errorf("store is closed")
	}
	
	// For testing, just return empty results
	return &models.SearchResult{
		Hits:      []models.SearchHit{},
		TotalHits: 0,
		Facets:    make(map[string][]models.FacetValue),
	}, nil
}

// Ping implements store.IndexStore
func (m *MockTypesenseStore) Ping(ctx context.Context) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	if m.closed {
		return fmt.Errorf("store is closed")
	}
	
	return nil
}

// Close implements store.IndexStore
func (m *MockTypesenseStore) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.closed = true
	m.documents = nil
	
	return nil
}

// CreateCollection implements store.IndexStore
func (m *MockTypesenseStore) CreateCollection(ctx context.Context, entityType string, schema *models.EntitySchema) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if m.closed {
		return fmt.Errorf("store is closed")
	}
	
	// Initialize collection
	if m.documents[entityType] == nil {
		m.documents[entityType] = make(map[string]interface{})
	}
	
	return nil
}

// UpdateCollection implements store.IndexStore
func (m *MockTypesenseStore) UpdateCollection(ctx context.Context, entityType string, schema *models.EntitySchema) error {
	return m.CreateCollection(ctx, entityType, schema)
}

// DeleteCollection implements store.IndexStore
func (m *MockTypesenseStore) DeleteCollection(ctx context.Context, entityType string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if m.closed {
		return fmt.Errorf("store is closed")
	}
	
	delete(m.documents, entityType)
	return nil
}

// Ensure MockTypesenseStore implements store.IndexStore
var _ store.IndexStore = (*MockTypesenseStore)(nil)