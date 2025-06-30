package sdk

import (
	"time"

	"github.com/google/uuid"
)

// Entity represents an entity in the system
type Entity struct {
	ID         uuid.UUID              `json:"id"`
	EntityType string                 `json:"entity_type"`
	URN        string                 `json:"urn"`
	Properties map[string]interface{} `json:"properties"`
	CreatedAt  time.Time              `json:"created_at"`
	UpdatedAt  time.Time              `json:"updated_at"`
	Version    int                    `json:"version"`
}

// EntityRequest represents a request to create or update an entity
type EntityRequest struct {
	URN        string                 `json:"urn"`
	Properties map[string]interface{} `json:"properties"`
}

// EntityListResponse represents a paginated list of entities
type EntityListResponse struct {
	Entities []Entity `json:"entities"`
	Total    int      `json:"total"`
	Limit    int      `json:"limit"`
	Offset   int      `json:"offset"`
}

// Relation represents a relation between entities
type Relation struct {
	ID             uuid.UUID              `json:"id"`
	RelationType   string                 `json:"relation_type"`
	FromEntityID   uuid.UUID              `json:"from_entity_id"`
	FromEntityType string                 `json:"from_entity_type"`
	ToEntityID     uuid.UUID              `json:"to_entity_id"`
	ToEntityType   string                 `json:"to_entity_type"`
	Properties     map[string]interface{} `json:"properties,omitempty"`
	CreatedAt      time.Time              `json:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at"`
}

// RelationRequest represents a request to create a relation
type RelationRequest struct {
	RelationType   string                 `json:"relation_type"`
	FromEntityID   uuid.UUID              `json:"from_entity_id"`
	FromEntityType string                 `json:"from_entity_type"`
	ToEntityID     uuid.UUID              `json:"to_entity_id"`
	ToEntityType   string                 `json:"to_entity_type"`
	Properties     map[string]interface{} `json:"properties,omitempty"`
}

// PropertyType represents the type of a property
type PropertyType string

const (
	PropertyTypeString   PropertyType = "string"
	PropertyTypeNumber   PropertyType = "number"
	PropertyTypeBoolean  PropertyType = "boolean"
	PropertyTypeDate     PropertyType = "date"
	PropertyTypeObject   PropertyType = "object"
	PropertyTypeArray    PropertyType = "array"
	PropertyTypeVector   PropertyType = "vector"
	PropertyTypeGeoPoint PropertyType = "geopoint"
)

// PropertyDefinition defines a property in a schema
type PropertyDefinition struct {
	Type        PropertyType `json:"type"`
	Required    bool         `json:"required"`
	Indexed     bool         `json:"indexed"`
	Facetable   bool         `json:"facetable,omitempty"`
	Searchable  bool         `json:"searchable,omitempty"`
	Sortable    bool         `json:"sortable,omitempty"`
	VectorDim   int          `json:"vector_dim,omitempty"`
	Description string       `json:"description,omitempty"`
}

// PropertySchema is a map of property names to their definitions
type PropertySchema map[string]PropertyDefinition

// IndexConfig defines an index configuration
type IndexConfig struct {
	Name   string   `json:"name"`
	Fields []string `json:"fields"`
	Unique bool     `json:"unique,omitempty"`
}

// EntitySchema represents the schema for an entity type
type EntitySchema struct {
	ID         uuid.UUID      `json:"id"`
	EntityType string         `json:"entity_type"`
	Properties PropertySchema `json:"properties"`
	Indexes    []IndexConfig  `json:"indexes"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	Version    int            `json:"version"`
}

// EntitySchemaRequest represents a request to create or update an entity schema
type EntitySchemaRequest struct {
	EntityType string         `json:"entity_type"`
	Properties PropertySchema `json:"properties"`
	Indexes    []IndexConfig  `json:"indexes,omitempty"`
}

// CardinalityType represents the cardinality of a relationship
type CardinalityType string

const (
	CardinalityOneToOne   CardinalityType = "one-to-one"
	CardinalityOneToMany  CardinalityType = "one-to-many"
	CardinalityManyToOne  CardinalityType = "many-to-one"
	CardinalityManyToMany CardinalityType = "many-to-many"
)

// DenormalizationConfig defines how relationships should be denormalized
type DenormalizationConfig struct {
	DenormalizeToSource bool     `json:"denormalize_to_source"`
	DenormalizeToTarget bool     `json:"denormalize_to_target"`
	SourceFields        []string `json:"source_fields,omitempty"`
	TargetFields        []string `json:"target_fields,omitempty"`
}

// RelationshipSchema represents the schema for a relationship type
type RelationshipSchema struct {
	ID                    uuid.UUID             `json:"id"`
	RelationshipType      string                `json:"relationship_type"`
	FromEntityType        string                `json:"from_entity_type"`
	ToEntityType          string                `json:"to_entity_type"`
	Properties            PropertySchema        `json:"properties"`
	Cardinality           CardinalityType       `json:"cardinality"`
	DenormalizationConfig DenormalizationConfig `json:"denormalization_config"`
	CreatedAt             time.Time             `json:"created_at"`
	UpdatedAt             time.Time             `json:"updated_at"`
	Version               int                   `json:"version"`
}

// RelationshipSchemaRequest represents a request to create or update a relationship schema
type RelationshipSchemaRequest struct {
	RelationshipType      string                `json:"relationship_type"`
	FromEntityType        string                `json:"from_entity_type"`
	ToEntityType          string                `json:"to_entity_type"`
	Properties            PropertySchema        `json:"properties,omitempty"`
	Cardinality           CardinalityType       `json:"cardinality"`
	DenormalizationConfig DenormalizationConfig `json:"denormalization_config,omitempty"`
}

// SearchQuery represents a text search query
type SearchQuery struct {
	EntityTypes []string               `json:"entity_types"`
	Query       string                 `json:"query"`
	Filters     map[string]interface{} `json:"filters,omitempty"`
	Facets      []string               `json:"facets,omitempty"`
	Sort        []SortOption           `json:"sort,omitempty"`
	Limit       int                    `json:"limit"`
	Offset      int                    `json:"offset"`
	IncludeURN  bool                   `json:"include_urn"`
}

// VectorQuery represents a vector similarity search query
type VectorQuery struct {
	EntityTypes    []string               `json:"entity_types"`
	Vector         []float32              `json:"vector"`
	VectorField    string                 `json:"vector_field"`
	TopK           int                    `json:"top_k"`
	Filters        map[string]interface{} `json:"filters,omitempty"`
	MinScore       float32                `json:"min_score,omitempty"`
	IncludeVectors bool                   `json:"include_vectors"`
}

// SortOrder represents the sort order
type SortOrder string

const (
	SortAsc  SortOrder = "asc"
	SortDesc SortOrder = "desc"
)

// SortOption represents a sort option
type SortOption struct {
	Field string    `json:"field"`
	Order SortOrder `json:"order"`
}

// SearchResult represents search results
type SearchResult struct {
	Hits       []SearchHit            `json:"hits"`
	TotalHits  int64                  `json:"total_hits"`
	Facets     map[string][]FacetValue `json:"facets,omitempty"`
	SearchTime int64                  `json:"search_time_ms"`
	Query      interface{}            `json:"query"`
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

// FacetValue represents a facet value and its count
type FacetValue struct {
	Value string `json:"value"`
	Count int64  `json:"count"`
}

// ListOptions represents options for listing resources
type ListOptions struct {
	Limit  int
	Offset int
}

// NewSearchQueryBuilder creates a new search query builder
func NewSearchQueryBuilder(entityTypes []string, query string) *SearchQueryBuilder {
	return &SearchQueryBuilder{
		query: SearchQuery{
			EntityTypes: entityTypes,
			Query:       query,
			Filters:     make(map[string]interface{}),
			Limit:       20,
			Offset:      0,
			IncludeURN:  true,
		},
	}
}

// SearchQueryBuilder helps build search queries
type SearchQueryBuilder struct {
	query SearchQuery
}

func (b *SearchQueryBuilder) WithFilter(field string, value interface{}) *SearchQueryBuilder {
	b.query.Filters[field] = value
	return b
}

func (b *SearchQueryBuilder) WithFacets(facets ...string) *SearchQueryBuilder {
	b.query.Facets = facets
	return b
}

func (b *SearchQueryBuilder) WithSort(field string, order SortOrder) *SearchQueryBuilder {
	b.query.Sort = append(b.query.Sort, SortOption{Field: field, Order: order})
	return b
}

func (b *SearchQueryBuilder) WithLimit(limit int) *SearchQueryBuilder {
	b.query.Limit = limit
	return b
}

func (b *SearchQueryBuilder) WithOffset(offset int) *SearchQueryBuilder {
	b.query.Offset = offset
	return b
}

func (b *SearchQueryBuilder) IncludeURN(include bool) *SearchQueryBuilder {
	b.query.IncludeURN = include
	return b
}

func (b *SearchQueryBuilder) Build() *SearchQuery {
	return &b.query
}