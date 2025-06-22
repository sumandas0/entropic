package models

import (
	"time"

	"github.com/google/uuid"
)

type EntitySchema struct {
	ID         uuid.UUID      `json:"id" validate:"required"`
	EntityType string         `json:"entity_type" validate:"required,min=1,max=100"`
	Properties PropertySchema `json:"properties" validate:"required"`
	Indexes    []IndexConfig  `json:"indexes"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	Version    int            `json:"version"`
}

type PropertySchema map[string]PropertyDefinition

type PropertyDefinition struct {
	Type         string                 `json:"type" validate:"required,oneof=string number boolean datetime object array vector"`
	Required     bool                   `json:"required"`
	Description  string                 `json:"description"`
	Default      interface{}            `json:"default,omitempty"`
	Constraints  map[string]interface{} `json:"constraints,omitempty"`
	VectorDim    int                    `json:"vector_dim,omitempty"` 
	ElementType  string                 `json:"element_type,omitempty"` 
	ObjectSchema PropertySchema         `json:"object_schema,omitempty"` 
}

type IndexConfig struct {
	Name       string   `json:"name" validate:"required"`
	Fields     []string `json:"fields" validate:"required,min=1"`
	Type       string   `json:"type" validate:"required,oneof=btree hash gin gist vector"`
	Unique     bool     `json:"unique"`
	VectorType string   `json:"vector_type,omitempty"` 
}

type RelationshipSchema struct {
	ID                     uuid.UUID              `json:"id" validate:"required"`
	RelationshipType       string                 `json:"relationship_type" validate:"required,min=1,max=100"`
	FromEntityType         string                 `json:"from_entity_type" validate:"required,min=1,max=100"`
	ToEntityType           string                 `json:"to_entity_type" validate:"required,min=1,max=100"`
	Properties             PropertySchema         `json:"properties"`
	Cardinality            CardinalityType        `json:"cardinality" validate:"required"`
	DenormalizationConfig  DenormalizationConfig  `json:"denormalization_config"`
	CreatedAt              time.Time              `json:"created_at"`
	UpdatedAt              time.Time              `json:"updated_at"`
	Version                int                    `json:"version"`
}

type CardinalityType string

const (
	OneToOne   CardinalityType = "one-to-one"
	OneToMany  CardinalityType = "one-to-many"
	ManyToOne  CardinalityType = "many-to-one"
	ManyToMany CardinalityType = "many-to-many"
)

type DenormalizationConfig struct {
	DenormalizeToFrom   []string `json:"denormalize_to_from"` 
	DenormalizeFromTo   []string `json:"denormalize_from_to"` 
	UpdateOnChange      bool     `json:"update_on_change"`     
	IncludeRelationData bool     `json:"include_relation_data"` 
}

func NewEntitySchema(entityType string, properties PropertySchema) *EntitySchema {
	now := time.Now()
	return &EntitySchema{
		ID:         uuid.New(),
		EntityType: entityType,
		Properties: properties,
		Indexes:    []IndexConfig{},
		CreatedAt:  now,
		UpdatedAt:  now,
		Version:    1,
	}
}

func NewRelationshipSchema(relationshipType, fromEntityType, toEntityType string, 
	cardinality CardinalityType) *RelationshipSchema {
	now := time.Now()
	return &RelationshipSchema{
		ID:               uuid.New(),
		RelationshipType: relationshipType,
		FromEntityType:   fromEntityType,
		ToEntityType:     toEntityType,
		Cardinality:      cardinality,
		Properties:       make(PropertySchema),
		CreatedAt:        now,
		UpdatedAt:        now,
		Version:          1,
	}
}