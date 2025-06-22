package models

import (
	"time"

	"github.com/google/uuid"
)

type Relation struct {
	ID             uuid.UUID              `json:"id" validate:"required" example:"750e8400-e29b-41d4-a716-446655440002"`
	RelationType   string                 `json:"relation_type" validate:"required,min=1,max=100" example:"owns"`
	FromEntityID   uuid.UUID              `json:"from_entity_id" validate:"required" example:"550e8400-e29b-41d4-a716-446655440000"`
	FromEntityType string                 `json:"from_entity_type" validate:"required,min=1,max=100" example:"user"`
	ToEntityID     uuid.UUID              `json:"to_entity_id" validate:"required" example:"650e8400-e29b-41d4-a716-446655440001"`
	ToEntityType   string                 `json:"to_entity_type" validate:"required,min=1,max=100" example:"document"`
	Properties     map[string]interface{} `json:"properties" swaggertype:"object"`
	CreatedAt      time.Time              `json:"created_at" example:"2023-01-01T00:00:00Z"`
	UpdatedAt      time.Time              `json:"updated_at" example:"2023-01-01T00:00:00Z"`
}

func NewRelation(relationType string, fromEntityID uuid.UUID, fromEntityType string, 
	toEntityID uuid.UUID, toEntityType string, properties map[string]interface{}) *Relation {
	now := time.Now()
	return &Relation{
		ID:             uuid.New(),
		RelationType:   relationType,
		FromEntityID:   fromEntityID,
		FromEntityType: fromEntityType,
		ToEntityID:     toEntityID,
		ToEntityType:   toEntityType,
		Properties:     properties,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

type RelationReference struct {
	EntityID   uuid.UUID `json:"entity_id" example:"550e8400-e29b-41d4-a716-446655440000"`
	EntityType string    `json:"entity_type" example:"user"`
	URN        string    `json:"urn,omitempty" example:"urn:entropic:user:123"`
}

func (r *Relation) GetFromReference() RelationReference {
	return RelationReference{
		EntityID:   r.FromEntityID,
		EntityType: r.FromEntityType,
	}
}

func (r *Relation) GetToReference() RelationReference {
	return RelationReference{
		EntityID:   r.ToEntityID,
		EntityType: r.ToEntityType,
	}
}