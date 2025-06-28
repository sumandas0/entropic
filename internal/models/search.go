package models

import (
	"github.com/google/uuid"
)

type SearchQuery struct {
	EntityTypes []string       `json:"entity_types" validate:"required,min=1" example:"user,document"`
	Query       string         `json:"query" example:"john doe"`
	Filters     map[string]any `json:"filters" swaggertype:"object"`
	Facets      []string       `json:"facets" example:"status,type"`
	Sort        []SortOption   `json:"sort"`
	Limit       int            `json:"limit" validate:"min=1,max=1000" example:"20"`
	Offset      int            `json:"offset" validate:"min=0" example:"0"`
	IncludeURN  bool           `json:"include_urn" example:"true"`
}

type VectorQuery struct {
	EntityTypes    []string       `json:"entity_types" validate:"required,min=1" example:"document"`
	Vector         []float32      `json:"vector" validate:"required" swaggertype:"array,number"`
	VectorField    string         `json:"vector_field" validate:"required" example:"embedding"`
	TopK           int            `json:"top_k" validate:"required,min=1,max=1000" example:"10"`
	Filters        map[string]any `json:"filters" swaggertype:"object"`
	MinScore       float32        `json:"min_score" example:"0.7"`
	IncludeVectors bool           `json:"include_vectors" example:"false"`
}

type SortOption struct {
	Field string    `json:"field" validate:"required" example:"created_at"`
	Order SortOrder `json:"order" validate:"required" enums:"asc,desc"`
}

type SortOrder string

const (
	SortAsc  SortOrder = "asc"
	SortDesc SortOrder = "desc"
)

type SearchResult struct {
	Hits       []SearchHit             `json:"hits"`
	TotalHits  int64                   `json:"total_hits" example:"100"`
	Facets     map[string][]FacetValue `json:"facets,omitempty" swaggertype:"object"`
	SearchTime int64                   `json:"search_time_ms" example:"15"`
	Query      any                     `json:"query" swaggertype:"object"`
}

type SearchHit struct {
	ID         uuid.UUID           `json:"id" example:"550e8400-e29b-41d4-a716-446655440000"`
	EntityType string              `json:"entity_type" example:"user"`
	URN        string              `json:"urn,omitempty" example:"urn:entropic:user:123"`
	Score      float32             `json:"score" example:"0.95"`
	Properties map[string]any      `json:"properties" swaggertype:"object"`
	Highlights map[string][]string `json:"highlights,omitempty" swaggertype:"object"`
	Vector     []float32           `json:"vector,omitempty" swaggertype:"array,number"`
}

type FacetValue struct {
	Value string `json:"value" example:"active"`
	Count int64  `json:"count" example:"25"`
}

func NewSearchQuery(entityTypes []string, query string) *SearchQuery {
	return &SearchQuery{
		EntityTypes: entityTypes,
		Query:       query,
		Filters:     make(map[string]any),
		Facets:      []string{},
		Sort:        []SortOption{},
		Limit:       20,
		Offset:      0,
		IncludeURN:  true,
	}
}

func NewVectorQuery(entityTypes []string, vector []float32, vectorField string) *VectorQuery {
	return &VectorQuery{
		EntityTypes:    entityTypes,
		Vector:         vector,
		VectorField:    vectorField,
		TopK:           10,
		Filters:        make(map[string]any),
		MinScore:       0.0,
		IncludeVectors: false,
	}
}

func (q *SearchQuery) AddFilter(field string, value any) {
	if q.Filters == nil {
		q.Filters = make(map[string]any)
	}
	q.Filters[field] = value
}

func (q *SearchQuery) AddSort(field string, order SortOrder) {
	q.Sort = append(q.Sort, SortOption{Field: field, Order: order})
}
