package sdk

import (
	"context"
	"fmt"
	"net/http"
)

// SearchService handles search-related operations
type SearchService struct {
	client *Client
}

// Search performs a text search across entities
func (s *SearchService) Search(ctx context.Context, query *SearchQuery) (*SearchResult, error) {
	if query == nil {
		return nil, &APIError{
			Type:    ErrorTypeValidation,
			Message: "search query is required",
		}
	}

	if err := s.validateSearchQuery(query); err != nil {
		return nil, err
	}

	// Apply defaults if not set
	if query.Limit <= 0 || query.Limit > 1000 {
		query.Limit = 20
	}
	if query.Offset < 0 {
		query.Offset = 0
	}

	path := fmt.Sprintf("%s/search", apiV1BasePath)
	
	var result SearchResult
	err := s.client.doJSONRequest(ctx, http.MethodPost, path, nil, query, &result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

// VectorSearch performs a vector similarity search
func (s *SearchService) VectorSearch(ctx context.Context, query *VectorQuery) (*SearchResult, error) {
	if query == nil {
		return nil, &APIError{
			Type:    ErrorTypeValidation,
			Message: "vector query is required",
		}
	}

	if err := s.validateVectorQuery(query); err != nil {
		return nil, err
	}

	// Apply defaults if not set
	if query.TopK <= 0 || query.TopK > 1000 {
		query.TopK = 10
	}

	path := fmt.Sprintf("%s/search/vector", apiV1BasePath)
	
	var result SearchResult
	err := s.client.doJSONRequest(ctx, http.MethodPost, path, nil, query, &result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

// validateSearchQuery validates a search query
func (s *SearchService) validateSearchQuery(query *SearchQuery) error {
	if len(query.EntityTypes) == 0 {
		return &APIError{
			Type:    ErrorTypeValidation,
			Message: "entity_types is required and must not be empty",
		}
	}

	return nil
}

// validateVectorQuery validates a vector query
func (s *SearchService) validateVectorQuery(query *VectorQuery) error {
	if len(query.EntityTypes) == 0 {
		return &APIError{
			Type:    ErrorTypeValidation,
			Message: "entity_types is required and must not be empty",
		}
	}

	if len(query.Vector) == 0 {
		return &APIError{
			Type:    ErrorTypeValidation,
			Message: "vector is required and must not be empty",
		}
	}

	if query.VectorField == "" {
		return &APIError{
			Type:    ErrorTypeValidation,
			Message: "vector_field is required",
		}
	}

	return nil
}

// VectorQueryBuilder helps build vector search queries
type VectorQueryBuilder struct {
	query VectorQuery
}

// NewVectorQueryBuilder creates a new vector query builder
func NewVectorQueryBuilder(entityTypes []string, vector []float32, vectorField string) *VectorQueryBuilder {
	return &VectorQueryBuilder{
		query: VectorQuery{
			EntityTypes: entityTypes,
			Vector:      vector,
			VectorField: vectorField,
			TopK:        10,
			Filters:     make(map[string]interface{}),
		},
	}
}

// WithFilter adds a filter to the query
func (b *VectorQueryBuilder) WithFilter(field string, value interface{}) *VectorQueryBuilder {
	b.query.Filters[field] = value
	return b
}

// WithTopK sets the number of results to return
func (b *VectorQueryBuilder) WithTopK(topK int) *VectorQueryBuilder {
	b.query.TopK = topK
	return b
}

// WithMinScore sets the minimum similarity score
func (b *VectorQueryBuilder) WithMinScore(minScore float32) *VectorQueryBuilder {
	b.query.MinScore = minScore
	return b
}

// IncludeVectors sets whether to include vectors in the results
func (b *VectorQueryBuilder) IncludeVectors(include bool) *VectorQueryBuilder {
	b.query.IncludeVectors = include
	return b
}

// Build creates the vector query
func (b *VectorQueryBuilder) Build() *VectorQuery {
	return &b.query
}

// SearchResultIterator provides iteration over search results with pagination
type SearchResultIterator struct {
	service     *SearchService
	query       *SearchQuery
	currentPage *SearchResult
	nextOffset  int
	hasMore     bool
}

// NewSearchResultIterator creates a new search result iterator
func (s *SearchService) NewIterator(query *SearchQuery) *SearchResultIterator {
	return &SearchResultIterator{
		service: s,
		query:   query,
		hasMore: true,
	}
}

// Next fetches the next page of results
func (iter *SearchResultIterator) Next(ctx context.Context) (*SearchResult, error) {
	if !iter.hasMore {
		return nil, nil
	}

	// Update offset for pagination
	iter.query.Offset = iter.nextOffset

	result, err := iter.service.Search(ctx, iter.query)
	if err != nil {
		return nil, err
	}

	iter.currentPage = result
	iter.nextOffset = iter.query.Offset + len(result.Hits)
	
	// Check if there are more results
	iter.hasMore = int64(iter.nextOffset) < result.TotalHits

	return result, nil
}

// HasMore returns true if there are more results to fetch
func (iter *SearchResultIterator) HasMore() bool {
	return iter.hasMore
}