package models

import (
	"time"

	"github.com/google/uuid"
)

type EntitySchema struct {
	ID         uuid.UUID      `json:"id" validate:"required" example:"550e8400-e29b-41d4-a716-446655440000"`
	EntityType string         `json:"entity_type" validate:"required,min=1,max=100" example:"user"`
	Properties PropertySchema `json:"properties" validate:"required"`
	Indexes    []IndexConfig  `json:"indexes"`
	CreatedAt  time.Time      `json:"created_at" example:"2023-01-01T00:00:00Z"`
	UpdatedAt  time.Time      `json:"updated_at" example:"2023-01-01T00:00:00Z"`
	Version    int            `json:"version" example:"1"`
}

type PropertySchema map[string]PropertyDefinition

type PropertyDefinition struct {
	Type         string                 `json:"type" validate:"required,oneof=string number boolean datetime object array vector" example:"string"`
	Required     bool                   `json:"required" example:"true"`
	Description  string                 `json:"description" example:"User's full name"`
	Default      interface{}            `json:"default,omitempty" swaggertype:"object"`
	Constraints  map[string]interface{} `json:"constraints,omitempty" swaggertype:"object"`
	VectorDim    int                    `json:"vector_dim,omitempty" example:"512"` 
	ElementType  string                 `json:"element_type,omitempty" example:"string"` 
	ObjectSchema PropertySchema         `json:"object_schema,omitempty"` 
}

type IndexConfig struct {
	Name       string   `json:"name" validate:"required" example:"idx_user_email"`
	Fields     []string `json:"fields" validate:"required,min=1" example:"email"`
	Type       string   `json:"type" validate:"required,oneof=btree hash gin gist vector" example:"btree"`
	Unique     bool     `json:"unique" example:"true"`
	VectorType string   `json:"vector_type,omitempty" example:"ivfflat"` 
}

type RelationshipSchema struct {
	ID                     uuid.UUID              `json:"id" validate:"required" example:"550e8400-e29b-41d4-a716-446655440000"`
	RelationshipType       string                 `json:"relationship_type" validate:"required,min=1,max=100" example:"owns"`
	FromEntityType         string                 `json:"from_entity_type" validate:"required,min=1,max=100" example:"user"`
	ToEntityType           string                 `json:"to_entity_type" validate:"required,min=1,max=100" example:"document"`
	Properties             PropertySchema         `json:"properties"`
	Cardinality            CardinalityType        `json:"cardinality" validate:"required" enums:"one-to-one,one-to-many,many-to-one,many-to-many"`
	DenormalizationConfig  DenormalizationConfig  `json:"denormalization_config"`
	CreatedAt              time.Time              `json:"created_at" example:"2023-01-01T00:00:00Z"`
	UpdatedAt              time.Time              `json:"updated_at" example:"2023-01-01T00:00:00Z"`
	Version                int                    `json:"version" example:"1"`
}

type CardinalityType string

const (
	OneToOne   CardinalityType = "one-to-one"
	OneToMany  CardinalityType = "one-to-many"
	ManyToOne  CardinalityType = "many-to-one"
	ManyToMany CardinalityType = "many-to-many"
)

type DenormalizationConfig struct {
	DenormalizeToFrom   []string `json:"denormalize_to_from" example:"name,email"` 
	DenormalizeFromTo   []string `json:"denormalize_from_to" example:"title,status"` 
	UpdateOnChange      bool     `json:"update_on_change" example:"true"`     
	IncludeRelationData bool     `json:"include_relation_data" example:"false"` 
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