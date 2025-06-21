package models

import (
	"time"

	"github.com/google/uuid"
)

// SearchQuery represents a search query
type SearchQuery struct {
	EntityTypes []string               `json:"entity_types" validate:"required,min=1"`
	Query       string                 `json:"query"`
	Filters     map[string]interface{} `json:"filters"`
	Facets      []string               `json:"facets"`
	Sort        []SortOption           `json:"sort"`
	Limit       int                    `json:"limit" validate:"min=1,max=1000"`
	Offset      int                    `json:"offset" validate:"min=0"`
	IncludeURN  bool                   `json:"include_urn"`
}

// VectorQuery represents a vector similarity search query
type VectorQuery struct {
	EntityTypes    []string               `json:"entity_types" validate:"required,min=1"`
	Vector         []float32              `json:"vector" validate:"required"`
	VectorField    string                 `json:"vector_field" validate:"required"`
	TopK           int                    `json:"top_k" validate:"required,min=1,max=1000"`
	Filters        map[string]interface{} `json:"filters"`
	MinScore       float32                `json:"min_score"`
	IncludeVectors bool                   `json:"include_vectors"`
}

// SortOption defines sorting configuration
type SortOption struct {
	Field string    `json:"field" validate:"required"`
	Order SortOrder `json:"order" validate:"required"`
}

// SortOrder defines sort order
type SortOrder string

const (
	SortAsc  SortOrder = "asc"
	SortDesc SortOrder = "desc"
)

// SearchResult represents search results
type SearchResult struct {
	Hits       []SearchHit            `json:"hits"`
	TotalHits  int64                  `json:"total_hits"`
	Facets     map[string][]FacetValue `json:"facets,omitempty"`
	SearchTime time.Duration          `json:"search_time_ms"`
	Query      interface{}            `json:"query"` // Original query for reference
}

// SearchHit represents a single search result
type SearchHit struct {
	ID         uuid.UUID              `json:"id"`
	EntityType string                 `json:"entity_type"`
	URN        string                 `json:"urn,omitempty"`
	Score      float32                `json:"score"`
	Properties map[string]interface{} `json:"properties"`
	Highlights map[string][]string    `json:"highlights,omitempty"`
	Vector     []float32              `json:"vector,omitempty"`
}

// FacetValue represents a facet value with count
type FacetValue struct {
	Value string `json:"value"`
	Count int64  `json:"count"`
}

// NewSearchQuery creates a new search query with defaults
func NewSearchQuery(entityTypes []string, query string) *SearchQuery {
	return &SearchQuery{
		EntityTypes: entityTypes,
		Query:       query,
		Filters:     make(map[string]interface{}),
		Facets:      []string{},
		Sort:        []SortOption{},
		Limit:       20,
		Offset:      0,
		IncludeURN:  true,
	}
}

// NewVectorQuery creates a new vector query with defaults
func NewVectorQuery(entityTypes []string, vector []float32, vectorField string) *VectorQuery {
	return &VectorQuery{
		EntityTypes:    entityTypes,
		Vector:         vector,
		VectorField:    vectorField,
		TopK:           10,
		Filters:        make(map[string]interface{}),
		MinScore:       0.0,
		IncludeVectors: false,
	}
}

// AddFilter adds a filter to the search query
func (q *SearchQuery) AddFilter(field string, value interface{}) {
	if q.Filters == nil {
		q.Filters = make(map[string]interface{})
	}
	q.Filters[field] = value
}

// AddSort adds a sort option to the search query
func (q *SearchQuery) AddSort(field string, order SortOrder) {
	q.Sort = append(q.Sort, SortOption{Field: field, Order: order})
}