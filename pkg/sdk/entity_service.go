package sdk

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

// EntityService handles entity-related operations
type EntityService struct {
	client *Client
}

// Create creates a new entity of the specified type
func (s *EntityService) Create(ctx context.Context, entityType string, req *EntityRequest) (*Entity, error) {
	if entityType == "" {
		return nil, &APIError{
			Type:    ErrorTypeValidation,
			Message: "entity type is required",
		}
	}

	if req == nil || req.Properties == nil {
		return nil, &APIError{
			Type:    ErrorTypeValidation,
			Message: "entity request with properties is required",
		}
	}

	path := fmt.Sprintf("%s/entities/%s", apiV1BasePath, entityType)
	
	var entity Entity
	err := s.client.doJSONRequest(ctx, http.MethodPost, path, nil, req, &entity)
	if err != nil {
		return nil, err
	}

	return &entity, nil
}

// Get retrieves an entity by its type and ID
func (s *EntityService) Get(ctx context.Context, entityType string, entityID uuid.UUID) (*Entity, error) {
	if entityType == "" {
		return nil, &APIError{
			Type:    ErrorTypeValidation,
			Message: "entity type is required",
		}
	}

	path := fmt.Sprintf("%s/entities/%s/%s", apiV1BasePath, entityType, entityID.String())
	
	var entity Entity
	err := s.client.doJSONRequest(ctx, http.MethodGet, path, nil, nil, &entity)
	if err != nil {
		return nil, err
	}

	return &entity, nil
}

// Update updates an existing entity
func (s *EntityService) Update(ctx context.Context, entityType string, entityID uuid.UUID, req *EntityRequest) (*Entity, error) {
	if entityType == "" {
		return nil, &APIError{
			Type:    ErrorTypeValidation,
			Message: "entity type is required",
		}
	}

	if req == nil {
		return nil, &APIError{
			Type:    ErrorTypeValidation,
			Message: "update request is required",
		}
	}

	path := fmt.Sprintf("%s/entities/%s/%s", apiV1BasePath, entityType, entityID.String())
	
	var entity Entity
	err := s.client.doJSONRequest(ctx, http.MethodPatch, path, nil, req, &entity)
	if err != nil {
		return nil, err
	}

	return &entity, nil
}

// Delete deletes an entity
func (s *EntityService) Delete(ctx context.Context, entityType string, entityID uuid.UUID) error {
	if entityType == "" {
		return &APIError{
			Type:    ErrorTypeValidation,
			Message: "entity type is required",
		}
	}

	path := fmt.Sprintf("%s/entities/%s/%s", apiV1BasePath, entityType, entityID.String())
	
	return s.client.doJSONRequest(ctx, http.MethodDelete, path, nil, nil, nil)
}

// List retrieves a paginated list of entities of a specific type
func (s *EntityService) List(ctx context.Context, entityType string, opts *ListOptions) (*EntityListResponse, error) {
	if entityType == "" {
		return nil, &APIError{
			Type:    ErrorTypeValidation,
			Message: "entity type is required",
		}
	}

	// Set default options if not provided
	if opts == nil {
		opts = &ListOptions{
			Limit:  20,
			Offset: 0,
		}
	}

	// Validate and apply limits
	if opts.Limit <= 0 || opts.Limit > 100 {
		opts.Limit = 20
	}
	if opts.Offset < 0 {
		opts.Offset = 0
	}

	// Build query parameters
	query := url.Values{}
	query.Set("limit", strconv.Itoa(opts.Limit))
	query.Set("offset", strconv.Itoa(opts.Offset))

	path := fmt.Sprintf("%s/entities/%s", apiV1BasePath, entityType)
	
	var response EntityListResponse
	err := s.client.doJSONRequest(ctx, http.MethodGet, path, query, nil, &response)
	if err != nil {
		return nil, err
	}

	return &response, nil
}

// GetRelations retrieves all relations for a specific entity
func (s *EntityService) GetRelations(ctx context.Context, entityType string, entityID uuid.UUID, relationTypes []string) ([]Relation, error) {
	if entityType == "" {
		return nil, &APIError{
			Type:    ErrorTypeValidation,
			Message: "entity type is required",
		}
	}

	// Build query parameters
	query := url.Values{}
	if len(relationTypes) > 0 {
		query.Set("relation_types", strings.Join(relationTypes, ","))
	}

	path := fmt.Sprintf("%s/entities/%s/%s/relations", apiV1BasePath, entityType, entityID.String())
	
	var relations []Relation
	err := s.client.doJSONRequest(ctx, http.MethodGet, path, query, nil, &relations)
	if err != nil {
		return nil, err
	}

	return relations, nil
}

// EntityBuilder provides a fluent interface for building entity requests
type EntityBuilder struct {
	urn        string
	properties map[string]interface{}
}

// NewEntityBuilder creates a new entity builder
func NewEntityBuilder() *EntityBuilder {
	return &EntityBuilder{
		properties: make(map[string]interface{}),
	}
}

// WithURN sets the URN for the entity
func (b *EntityBuilder) WithURN(urn string) *EntityBuilder {
	b.urn = urn
	return b
}

// WithProperty adds a property to the entity
func (b *EntityBuilder) WithProperty(key string, value interface{}) *EntityBuilder {
	b.properties[key] = value
	return b
}

// WithProperties adds multiple properties to the entity
func (b *EntityBuilder) WithProperties(properties map[string]interface{}) *EntityBuilder {
	for k, v := range properties {
		b.properties[k] = v
	}
	return b
}

// Build creates the entity request
func (b *EntityBuilder) Build() *EntityRequest {
	return &EntityRequest{
		URN:        b.urn,
		Properties: b.properties,
	}
}