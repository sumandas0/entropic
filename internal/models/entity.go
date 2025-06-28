package models

import (
	"time"

	"github.com/google/uuid"
)

type Entity struct {
	ID         uuid.UUID      `json:"id" validate:"required" example:"550e8400-e29b-41d4-a716-446655440000"`
	EntityType string         `json:"entity_type" validate:"required,min=1,max=100" example:"user"`
	URN        string         `json:"urn" validate:"required,min=1,max=500" example:"urn:entropic:user:123"`
	Properties map[string]any `json:"properties" validate:"required" swaggertype:"object"`
	CreatedAt  time.Time      `json:"created_at" example:"2023-01-01T00:00:00Z"`
	UpdatedAt  time.Time      `json:"updated_at" example:"2023-01-01T00:00:00Z"`
	Version    int            `json:"version" example:"1"`
}

func NewEntity(entityType, urn string, properties map[string]any) *Entity {
	now := time.Now()
	return &Entity{
		ID:         uuid.New(),
		EntityType: entityType,
		URN:        urn,
		Properties: properties,
		CreatedAt:  now,
		UpdatedAt:  now,
		Version:    1,
	}
}

func (e *Entity) Update(properties map[string]any) {
	e.Properties = properties
	e.UpdatedAt = time.Now()
	e.Version++
}

func (e *Entity) GetProperty(key string) (any, bool) {
	val, exists := e.Properties[key]
	return val, exists
}

func (e *Entity) SetProperty(key string, value any) {
	if e.Properties == nil {
		e.Properties = make(map[string]any)
	}
	e.Properties[key] = value
	e.UpdatedAt = time.Now()
}
