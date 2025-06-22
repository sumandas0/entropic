package models

import (
	"time"

	"github.com/google/uuid"
)

type Relation struct {
	ID             uuid.UUID              `json:"id" validate:"required"`
	RelationType   string                 `json:"relation_type" validate:"required,min=1,max=100"`
	FromEntityID   uuid.UUID              `json:"from_entity_id" validate:"required"`
	FromEntityType string                 `json:"from_entity_type" validate:"required,min=1,max=100"`
	ToEntityID     uuid.UUID              `json:"to_entity_id" validate:"required"`
	ToEntityType   string                 `json:"to_entity_type" validate:"required,min=1,max=100"`
	Properties     map[string]interface{} `json:"properties"`
	CreatedAt      time.Time              `json:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at"`
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
	EntityID   uuid.UUID `json:"entity_id"`
	EntityType string    `json:"entity_type"`
	URN        string    `json:"urn,omitempty"`
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