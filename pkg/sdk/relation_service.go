package sdk

import (
	"context"
	"fmt"
	"net/http"

	"github.com/google/uuid"
)

// RelationService handles relation-related operations
type RelationService struct {
	client *Client
}

// Create creates a new relation between two entities
func (s *RelationService) Create(ctx context.Context, req *RelationRequest) (*Relation, error) {
	if req == nil {
		return nil, &APIError{
			Type:    ErrorTypeValidation,
			Message: "relation request is required",
		}
	}

	if err := s.validateRelationRequest(req); err != nil {
		return nil, err
	}

	path := fmt.Sprintf("%s/relations", apiV1BasePath)
	
	var relation Relation
	err := s.client.doJSONRequest(ctx, http.MethodPost, path, nil, req, &relation)
	if err != nil {
		return nil, err
	}

	return &relation, nil
}

// Get retrieves a relation by its ID
func (s *RelationService) Get(ctx context.Context, relationID uuid.UUID) (*Relation, error) {
	path := fmt.Sprintf("%s/relations/%s", apiV1BasePath, relationID.String())
	
	var relation Relation
	err := s.client.doJSONRequest(ctx, http.MethodGet, path, nil, nil, &relation)
	if err != nil {
		return nil, err
	}

	return &relation, nil
}

// Delete deletes a relation
func (s *RelationService) Delete(ctx context.Context, relationID uuid.UUID) error {
	path := fmt.Sprintf("%s/relations/%s", apiV1BasePath, relationID.String())
	
	return s.client.doJSONRequest(ctx, http.MethodDelete, path, nil, nil, nil)
}

// validateRelationRequest validates a relation request
func (s *RelationService) validateRelationRequest(req *RelationRequest) error {
	if req.RelationType == "" {
		return &APIError{
			Type:    ErrorTypeValidation,
			Message: "relation_type is required",
		}
	}

	if req.FromEntityType == "" {
		return &APIError{
			Type:    ErrorTypeValidation,
			Message: "from_entity_type is required",
		}
	}

	if req.ToEntityType == "" {
		return &APIError{
			Type:    ErrorTypeValidation,
			Message: "to_entity_type is required",
		}
	}

	if req.FromEntityID == uuid.Nil {
		return &APIError{
			Type:    ErrorTypeValidation,
			Message: "from_entity_id is required",
		}
	}

	if req.ToEntityID == uuid.Nil {
		return &APIError{
			Type:    ErrorTypeValidation,
			Message: "to_entity_id is required",
		}
	}

	return nil
}

// RelationBuilder provides a fluent interface for building relation requests
type RelationBuilder struct {
	relationType   string
	fromEntityID   uuid.UUID
	fromEntityType string
	toEntityID     uuid.UUID
	toEntityType   string
	properties     map[string]interface{}
}

// NewRelationBuilder creates a new relation builder
func NewRelationBuilder(relationType string) *RelationBuilder {
	return &RelationBuilder{
		relationType: relationType,
		properties:   make(map[string]interface{}),
	}
}

// From sets the source entity for the relation
func (b *RelationBuilder) From(entityType string, entityID uuid.UUID) *RelationBuilder {
	b.fromEntityType = entityType
	b.fromEntityID = entityID
	return b
}

// To sets the target entity for the relation
func (b *RelationBuilder) To(entityType string, entityID uuid.UUID) *RelationBuilder {
	b.toEntityType = entityType
	b.toEntityID = entityID
	return b
}

// WithProperty adds a property to the relation
func (b *RelationBuilder) WithProperty(key string, value interface{}) *RelationBuilder {
	b.properties[key] = value
	return b
}

// WithProperties adds multiple properties to the relation
func (b *RelationBuilder) WithProperties(properties map[string]interface{}) *RelationBuilder {
	for k, v := range properties {
		b.properties[k] = v
	}
	return b
}

// Build creates the relation request
func (b *RelationBuilder) Build() *RelationRequest {
	req := &RelationRequest{
		RelationType:   b.relationType,
		FromEntityID:   b.fromEntityID,
		FromEntityType: b.fromEntityType,
		ToEntityID:     b.toEntityID,
		ToEntityType:   b.toEntityType,
	}

	if len(b.properties) > 0 {
		req.Properties = b.properties
	}

	return req
}