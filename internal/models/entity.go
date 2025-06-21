package models

import (
	"time"

	"github.com/google/uuid"
)

// Entity represents a core entity in the system
type Entity struct {
	ID         uuid.UUID              `json:"id" validate:"required"`
	EntityType string                 `json:"entity_type" validate:"required,min=1,max=100"`
	URN        string                 `json:"urn" validate:"required,min=1,max=500"`
	Properties map[string]interface{} `json:"properties" validate:"required"`
	CreatedAt  time.Time              `json:"created_at"`
	UpdatedAt  time.Time              `json:"updated_at"`
	Version    int                    `json:"version"`
}

// NewEntity creates a new entity with generated ID and timestamps
func NewEntity(entityType, urn string, properties map[string]interface{}) *Entity {
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

// Update updates the entity properties and increments version
func (e *Entity) Update(properties map[string]interface{}) {
	e.Properties = properties
	e.UpdatedAt = time.Now()
	e.Version++
}

// GetProperty retrieves a property value by key
func (e *Entity) GetProperty(key string) (interface{}, bool) {
	val, exists := e.Properties[key]
	return val, exists
}

// SetProperty sets a property value
func (e *Entity) SetProperty(key string, value interface{}) {
	if e.Properties == nil {
		e.Properties = make(map[string]interface{})
	}
	e.Properties[key] = value
	e.UpdatedAt = time.Now()
}