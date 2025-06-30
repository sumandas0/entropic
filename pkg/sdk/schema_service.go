package sdk

import (
	"context"
	"fmt"
	"net/http"
)

// SchemaService handles schema-related operations
type SchemaService struct {
	client *Client
}

// Entity Schema Operations

// CreateEntitySchema creates a new entity schema
func (s *SchemaService) CreateEntitySchema(ctx context.Context, req *EntitySchemaRequest) (*EntitySchema, error) {
	if req == nil {
		return nil, &APIError{
			Type:    ErrorTypeValidation,
			Message: "entity schema request is required",
		}
	}

	if err := s.validateEntitySchemaRequest(req); err != nil {
		return nil, err
	}

	path := fmt.Sprintf("%s/schemas/entities", apiV1BasePath)
	
	var schema EntitySchema
	err := s.client.doJSONRequest(ctx, http.MethodPost, path, nil, req, &schema)
	if err != nil {
		return nil, err
	}

	return &schema, nil
}

// GetEntitySchema retrieves an entity schema by type
func (s *SchemaService) GetEntitySchema(ctx context.Context, entityType string) (*EntitySchema, error) {
	if entityType == "" {
		return nil, &APIError{
			Type:    ErrorTypeValidation,
			Message: "entity type is required",
		}
	}

	path := fmt.Sprintf("%s/schemas/entities/%s", apiV1BasePath, entityType)
	
	var schema EntitySchema
	err := s.client.doJSONRequest(ctx, http.MethodGet, path, nil, nil, &schema)
	if err != nil {
		return nil, err
	}

	return &schema, nil
}

// UpdateEntitySchema updates an existing entity schema
func (s *SchemaService) UpdateEntitySchema(ctx context.Context, entityType string, req *EntitySchemaRequest) (*EntitySchema, error) {
	if entityType == "" {
		return nil, &APIError{
			Type:    ErrorTypeValidation,
			Message: "entity type is required",
		}
	}

	if req == nil {
		return nil, &APIError{
			Type:    ErrorTypeValidation,
			Message: "entity schema request is required",
		}
	}

	path := fmt.Sprintf("%s/schemas/entities/%s", apiV1BasePath, entityType)
	
	var schema EntitySchema
	err := s.client.doJSONRequest(ctx, http.MethodPut, path, nil, req, &schema)
	if err != nil {
		return nil, err
	}

	return &schema, nil
}

// DeleteEntitySchema deletes an entity schema
func (s *SchemaService) DeleteEntitySchema(ctx context.Context, entityType string) error {
	if entityType == "" {
		return &APIError{
			Type:    ErrorTypeValidation,
			Message: "entity type is required",
		}
	}

	path := fmt.Sprintf("%s/schemas/entities/%s", apiV1BasePath, entityType)
	
	return s.client.doJSONRequest(ctx, http.MethodDelete, path, nil, nil, nil)
}

// ListEntitySchemas retrieves all entity schemas
func (s *SchemaService) ListEntitySchemas(ctx context.Context) ([]EntitySchema, error) {
	path := fmt.Sprintf("%s/schemas/entities", apiV1BasePath)
	
	var schemas []EntitySchema
	err := s.client.doJSONRequest(ctx, http.MethodGet, path, nil, nil, &schemas)
	if err != nil {
		return nil, err
	}

	return schemas, nil
}

// Relationship Schema Operations

// CreateRelationshipSchema creates a new relationship schema
func (s *SchemaService) CreateRelationshipSchema(ctx context.Context, req *RelationshipSchemaRequest) (*RelationshipSchema, error) {
	if req == nil {
		return nil, &APIError{
			Type:    ErrorTypeValidation,
			Message: "relationship schema request is required",
		}
	}

	if err := s.validateRelationshipSchemaRequest(req); err != nil {
		return nil, err
	}

	path := fmt.Sprintf("%s/schemas/relationships", apiV1BasePath)
	
	var schema RelationshipSchema
	err := s.client.doJSONRequest(ctx, http.MethodPost, path, nil, req, &schema)
	if err != nil {
		return nil, err
	}

	return &schema, nil
}

// GetRelationshipSchema retrieves a relationship schema by type
func (s *SchemaService) GetRelationshipSchema(ctx context.Context, relationshipType string) (*RelationshipSchema, error) {
	if relationshipType == "" {
		return nil, &APIError{
			Type:    ErrorTypeValidation,
			Message: "relationship type is required",
		}
	}

	path := fmt.Sprintf("%s/schemas/relationships/%s", apiV1BasePath, relationshipType)
	
	var schema RelationshipSchema
	err := s.client.doJSONRequest(ctx, http.MethodGet, path, nil, nil, &schema)
	if err != nil {
		return nil, err
	}

	return &schema, nil
}

// UpdateRelationshipSchema updates an existing relationship schema
func (s *SchemaService) UpdateRelationshipSchema(ctx context.Context, relationshipType string, req *RelationshipSchemaRequest) (*RelationshipSchema, error) {
	if relationshipType == "" {
		return nil, &APIError{
			Type:    ErrorTypeValidation,
			Message: "relationship type is required",
		}
	}

	if req == nil {
		return nil, &APIError{
			Type:    ErrorTypeValidation,
			Message: "relationship schema request is required",
		}
	}

	path := fmt.Sprintf("%s/schemas/relationships/%s", apiV1BasePath, relationshipType)
	
	var schema RelationshipSchema
	err := s.client.doJSONRequest(ctx, http.MethodPut, path, nil, req, &schema)
	if err != nil {
		return nil, err
	}

	return &schema, nil
}

// DeleteRelationshipSchema deletes a relationship schema
func (s *SchemaService) DeleteRelationshipSchema(ctx context.Context, relationshipType string) error {
	if relationshipType == "" {
		return &APIError{
			Type:    ErrorTypeValidation,
			Message: "relationship type is required",
		}
	}

	path := fmt.Sprintf("%s/schemas/relationships/%s", apiV1BasePath, relationshipType)
	
	return s.client.doJSONRequest(ctx, http.MethodDelete, path, nil, nil, nil)
}

// ListRelationshipSchemas retrieves all relationship schemas
func (s *SchemaService) ListRelationshipSchemas(ctx context.Context) ([]RelationshipSchema, error) {
	path := fmt.Sprintf("%s/schemas/relationships", apiV1BasePath)
	
	var schemas []RelationshipSchema
	err := s.client.doJSONRequest(ctx, http.MethodGet, path, nil, nil, &schemas)
	if err != nil {
		return nil, err
	}

	return schemas, nil
}

// Validation helpers

func (s *SchemaService) validateEntitySchemaRequest(req *EntitySchemaRequest) error {
	if req.EntityType == "" {
		return &APIError{
			Type:    ErrorTypeValidation,
			Message: "entity_type is required",
		}
	}

	if req.Properties == nil || len(req.Properties) == 0 {
		return &APIError{
			Type:    ErrorTypeValidation,
			Message: "properties is required and must not be empty",
		}
	}

	return nil
}

func (s *SchemaService) validateRelationshipSchemaRequest(req *RelationshipSchemaRequest) error {
	if req.RelationshipType == "" {
		return &APIError{
			Type:    ErrorTypeValidation,
			Message: "relationship_type is required",
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

	if req.Cardinality == "" {
		return &APIError{
			Type:    ErrorTypeValidation,
			Message: "cardinality is required",
		}
	}

	// Validate cardinality value
	switch req.Cardinality {
	case CardinalityOneToOne, CardinalityOneToMany, CardinalityManyToOne, CardinalityManyToMany:
		// Valid
	default:
		return &APIError{
			Type:    ErrorTypeValidation,
			Message: "invalid cardinality value",
			Details: map[string]interface{}{
				"valid_values": []string{
					string(CardinalityOneToOne),
					string(CardinalityOneToMany),
					string(CardinalityManyToOne),
					string(CardinalityManyToMany),
				},
			},
		}
	}

	return nil
}

// EntitySchemaBuilder provides a fluent interface for building entity schemas
type EntitySchemaBuilder struct {
	entityType string
	properties PropertySchema
	indexes    []IndexConfig
}

// NewEntitySchemaBuilder creates a new entity schema builder
func NewEntitySchemaBuilder(entityType string) *EntitySchemaBuilder {
	return &EntitySchemaBuilder{
		entityType: entityType,
		properties: make(PropertySchema),
		indexes:    []IndexConfig{},
	}
}

// AddProperty adds a property to the schema
func (b *EntitySchemaBuilder) AddProperty(name string, def PropertyDefinition) *EntitySchemaBuilder {
	b.properties[name] = def
	return b
}

// AddStringProperty adds a string property
func (b *EntitySchemaBuilder) AddStringProperty(name string, required, indexed, searchable bool) *EntitySchemaBuilder {
	b.properties[name] = PropertyDefinition{
		Type:       PropertyTypeString,
		Required:   required,
		Indexed:    indexed,
		Searchable: searchable,
	}
	return b
}

// AddNumberProperty adds a number property
func (b *EntitySchemaBuilder) AddNumberProperty(name string, required, indexed, sortable bool) *EntitySchemaBuilder {
	b.properties[name] = PropertyDefinition{
		Type:     PropertyTypeNumber,
		Required: required,
		Indexed:  indexed,
		Sortable: sortable,
	}
	return b
}

// AddVectorProperty adds a vector property
func (b *EntitySchemaBuilder) AddVectorProperty(name string, dimension int, required bool) *EntitySchemaBuilder {
	b.properties[name] = PropertyDefinition{
		Type:      PropertyTypeVector,
		Required:  required,
		VectorDim: dimension,
	}
	return b
}

// AddIndex adds an index to the schema
func (b *EntitySchemaBuilder) AddIndex(name string, fields []string, unique bool) *EntitySchemaBuilder {
	b.indexes = append(b.indexes, IndexConfig{
		Name:   name,
		Fields: fields,
		Unique: unique,
	})
	return b
}

// Build creates the entity schema request
func (b *EntitySchemaBuilder) Build() *EntitySchemaRequest {
	return &EntitySchemaRequest{
		EntityType: b.entityType,
		Properties: b.properties,
		Indexes:    b.indexes,
	}
}