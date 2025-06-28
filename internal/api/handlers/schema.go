package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/sumandas0/entropic/internal/api/middleware"
	"github.com/sumandas0/entropic/internal/core"
	"github.com/sumandas0/entropic/internal/models"
)

type SchemaHandler struct {
	engine *core.Engine
}

func NewSchemaHandler(engine *core.Engine) *SchemaHandler {
	return &SchemaHandler{
		engine: engine,
	}
}

type EntitySchemaRequest struct {
	EntityType string                `json:"entity_type" validate:"required" example:"user"`
	Properties models.PropertySchema `json:"properties" validate:"required"`
	Indexes    []models.IndexConfig  `json:"indexes,omitempty"`
}

type EntitySchemaResponse struct {
	ID         uuid.UUID             `json:"id" example:"550e8400-e29b-41d4-a716-446655440000"`
	EntityType string                `json:"entity_type" example:"user"`
	Properties models.PropertySchema `json:"properties"`
	Indexes    []models.IndexConfig  `json:"indexes"`
	CreatedAt  time.Time             `json:"created_at" example:"2023-01-01T00:00:00Z"`
	UpdatedAt  time.Time             `json:"updated_at" example:"2023-01-01T00:00:00Z"`
	Version    int                   `json:"version" example:"1"`
}

type RelationshipSchemaRequest struct {
	RelationshipType      string                       `json:"relationship_type" validate:"required" example:"owns"`
	FromEntityType        string                       `json:"from_entity_type" validate:"required" example:"user"`
	ToEntityType          string                       `json:"to_entity_type" validate:"required" example:"document"`
	Properties            models.PropertySchema        `json:"properties,omitempty"`
	Cardinality           models.CardinalityType       `json:"cardinality" validate:"required" enums:"one-to-one,one-to-many,many-to-one,many-to-many"`
	DenormalizationConfig models.DenormalizationConfig `json:"denormalization_config,omitempty"`
}

type RelationshipSchemaResponse struct {
	ID                    uuid.UUID                    `json:"id" example:"550e8400-e29b-41d4-a716-446655440000"`
	RelationshipType      string                       `json:"relationship_type" example:"owns"`
	FromEntityType        string                       `json:"from_entity_type" example:"user"`
	ToEntityType          string                       `json:"to_entity_type" example:"document"`
	Properties            models.PropertySchema        `json:"properties"`
	Cardinality           models.CardinalityType       `json:"cardinality" enums:"one-to-one,one-to-many,many-to-one,many-to-many"`
	DenormalizationConfig models.DenormalizationConfig `json:"denormalization_config"`
	CreatedAt             time.Time                    `json:"created_at" example:"2023-01-01T00:00:00Z"`
	UpdatedAt             time.Time                    `json:"updated_at" example:"2023-01-01T00:00:00Z"`
	Version               int                          `json:"version" example:"1"`
}

// CreateEntitySchema godoc
// @Summary Create an entity schema
// @Description Create a new schema for an entity type
// @Tags schemas
// @Accept json
// @Produce json
// @Param schema body EntitySchemaRequest true "Entity schema data"
// @Success 201 {object} EntitySchemaResponse
// @Failure 400 {object} middleware.ErrorResponse
// @Failure 409 {object} middleware.ErrorResponse
// @Failure 500 {object} middleware.ErrorResponse
// @Router /api/v1/schemas/entities [post]
func (h *SchemaHandler) CreateEntitySchema(w http.ResponseWriter, r *http.Request) {
	var req EntitySchemaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		middleware.SendValidationError(w, r, "invalid request body", map[string]any{
			"error": err.Error(),
		})
		return
	}

	if req.EntityType == "" {
		middleware.SendValidationError(w, r, "entity_type is required", nil)
		return
	}
	if req.Properties == nil {
		middleware.SendValidationError(w, r, "properties is required", nil)
		return
	}

	schema := models.NewEntitySchema(req.EntityType, req.Properties)
	if req.Indexes != nil {
		schema.Indexes = req.Indexes
	}

	if err := h.engine.CreateEntitySchema(r.Context(), schema); err != nil {
		statusCode := middleware.HTTPErrorFromAppError(err)
		middleware.SendError(w, r, err, statusCode)
		return
	}

	response := h.entitySchemaToResponse(schema)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

// GetEntitySchema godoc
// @Summary Get an entity schema
// @Description Get the schema for a specific entity type
// @Tags schemas
// @Accept json
// @Produce json
// @Param entityType path string true "Entity Type"
// @Success 200 {object} EntitySchemaResponse
// @Failure 400 {object} middleware.ErrorResponse
// @Failure 404 {object} middleware.ErrorResponse
// @Failure 500 {object} middleware.ErrorResponse
// @Router /api/v1/schemas/entities/{entityType} [get]
func (h *SchemaHandler) GetEntitySchema(w http.ResponseWriter, r *http.Request) {
	entityType := chi.URLParam(r, "entityType")
	if entityType == "" {
		middleware.SendValidationError(w, r, "entity type is required", nil)
		return
	}

	schema, err := h.engine.GetEntitySchema(r.Context(), entityType)
	if err != nil {
		statusCode := middleware.HTTPErrorFromAppError(err)
		middleware.SendError(w, r, err, statusCode)
		return
	}

	response := h.entitySchemaToResponse(schema)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// UpdateEntitySchema godoc
// @Summary Update an entity schema
// @Description Update the schema for an existing entity type
// @Tags schemas
// @Accept json
// @Produce json
// @Param entityType path string true "Entity Type"
// @Param schema body EntitySchemaRequest true "Entity schema update data"
// @Success 200 {object} EntitySchemaResponse
// @Failure 400 {object} middleware.ErrorResponse
// @Failure 404 {object} middleware.ErrorResponse
// @Failure 409 {object} middleware.ErrorResponse
// @Failure 500 {object} middleware.ErrorResponse
// @Router /api/v1/schemas/entities/{entityType} [put]
func (h *SchemaHandler) UpdateEntitySchema(w http.ResponseWriter, r *http.Request) {
	entityType := chi.URLParam(r, "entityType")
	if entityType == "" {
		middleware.SendValidationError(w, r, "entity type is required", nil)
		return
	}

	var req EntitySchemaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		middleware.SendValidationError(w, r, "invalid request body", map[string]any{
			"error": err.Error(),
		})
		return
	}

	schema, err := h.engine.GetEntitySchema(r.Context(), entityType)
	if err != nil {
		statusCode := middleware.HTTPErrorFromAppError(err)
		middleware.SendError(w, r, err, statusCode)
		return
	}

	schema.Properties = req.Properties
	if req.Indexes != nil {
		schema.Indexes = req.Indexes
	}

	if err := h.engine.UpdateEntitySchema(r.Context(), schema); err != nil {
		statusCode := middleware.HTTPErrorFromAppError(err)
		middleware.SendError(w, r, err, statusCode)
		return
	}

	response := h.entitySchemaToResponse(schema)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// DeleteEntitySchema godoc
// @Summary Delete an entity schema
// @Description Delete the schema for an entity type. This operation cannot be undone.
// @Tags schemas
// @Accept json
// @Produce json
// @Param entityType path string true "Entity Type"
// @Success 204 "No Content"
// @Failure 400 {object} middleware.ErrorResponse
// @Failure 404 {object} middleware.ErrorResponse
// @Failure 500 {object} middleware.ErrorResponse
// @Router /api/v1/schemas/entities/{entityType} [delete]
func (h *SchemaHandler) DeleteEntitySchema(w http.ResponseWriter, r *http.Request) {
	entityType := chi.URLParam(r, "entityType")
	if entityType == "" {
		middleware.SendValidationError(w, r, "entity type is required", nil)
		return
	}

	if err := h.engine.DeleteEntitySchema(r.Context(), entityType); err != nil {
		statusCode := middleware.HTTPErrorFromAppError(err)
		middleware.SendError(w, r, err, statusCode)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ListEntitySchemas godoc
// @Summary List all entity schemas
// @Description Get a list of all entity schemas in the system
// @Tags schemas
// @Accept json
// @Produce json
// @Success 200 {array} EntitySchemaResponse
// @Failure 500 {object} middleware.ErrorResponse
// @Router /api/v1/schemas/entities [get]
func (h *SchemaHandler) ListEntitySchemas(w http.ResponseWriter, r *http.Request) {
	schemas, err := h.engine.ListEntitySchemas(r.Context())
	if err != nil {
		statusCode := middleware.HTTPErrorFromAppError(err)
		middleware.SendError(w, r, err, statusCode)
		return
	}

	responses := make([]EntitySchemaResponse, len(schemas))
	for i, schema := range schemas {
		responses[i] = h.entitySchemaToResponse(schema)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(responses)
}

// CreateRelationshipSchema godoc
// @Summary Create a relationship schema
// @Description Create a new schema for a relationship type
// @Tags schemas
// @Accept json
// @Produce json
// @Param schema body RelationshipSchemaRequest true "Relationship schema data"
// @Success 201 {object} RelationshipSchemaResponse
// @Failure 400 {object} middleware.ErrorResponse
// @Failure 409 {object} middleware.ErrorResponse
// @Failure 500 {object} middleware.ErrorResponse
// @Router /api/v1/schemas/relationships [post]
func (h *SchemaHandler) CreateRelationshipSchema(w http.ResponseWriter, r *http.Request) {
	var req RelationshipSchemaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		middleware.SendValidationError(w, r, "invalid request body", map[string]any{
			"error": err.Error(),
		})
		return
	}

	if req.RelationshipType == "" {
		middleware.SendValidationError(w, r, "relationship_type is required", nil)
		return
	}
	if req.FromEntityType == "" {
		middleware.SendValidationError(w, r, "from_entity_type is required", nil)
		return
	}
	if req.ToEntityType == "" {
		middleware.SendValidationError(w, r, "to_entity_type is required", nil)
		return
	}
	if req.Cardinality == "" {
		middleware.SendValidationError(w, r, "cardinality is required", nil)
		return
	}

	schema := models.NewRelationshipSchema(
		req.RelationshipType,
		req.FromEntityType,
		req.ToEntityType,
		req.Cardinality,
	)

	if req.Properties != nil {
		schema.Properties = req.Properties
	}
	schema.DenormalizationConfig = req.DenormalizationConfig

	if err := h.engine.CreateRelationshipSchema(r.Context(), schema); err != nil {
		statusCode := middleware.HTTPErrorFromAppError(err)
		middleware.SendError(w, r, err, statusCode)
		return
	}

	response := h.relationshipSchemaToResponse(schema)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

// GetRelationshipSchema godoc
// @Summary Get a relationship schema
// @Description Get the schema for a specific relationship type
// @Tags schemas
// @Accept json
// @Produce json
// @Param relationshipType path string true "Relationship Type"
// @Success 200 {object} RelationshipSchemaResponse
// @Failure 400 {object} middleware.ErrorResponse
// @Failure 404 {object} middleware.ErrorResponse
// @Failure 500 {object} middleware.ErrorResponse
// @Router /api/v1/schemas/relationships/{relationshipType} [get]
func (h *SchemaHandler) GetRelationshipSchema(w http.ResponseWriter, r *http.Request) {
	relationshipType := chi.URLParam(r, "relationshipType")
	if relationshipType == "" {
		middleware.SendValidationError(w, r, "relationship type is required", nil)
		return
	}

	schema, err := h.engine.GetRelationshipSchema(r.Context(), relationshipType)
	if err != nil {
		statusCode := middleware.HTTPErrorFromAppError(err)
		middleware.SendError(w, r, err, statusCode)
		return
	}

	response := h.relationshipSchemaToResponse(schema)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// UpdateRelationshipSchema godoc
// @Summary Update a relationship schema
// @Description Update the schema for an existing relationship type
// @Tags schemas
// @Accept json
// @Produce json
// @Param relationshipType path string true "Relationship Type"
// @Param schema body RelationshipSchemaRequest true "Relationship schema update data"
// @Success 200 {object} RelationshipSchemaResponse
// @Failure 400 {object} middleware.ErrorResponse
// @Failure 404 {object} middleware.ErrorResponse
// @Failure 409 {object} middleware.ErrorResponse
// @Failure 500 {object} middleware.ErrorResponse
// @Router /api/v1/schemas/relationships/{relationshipType} [put]
func (h *SchemaHandler) UpdateRelationshipSchema(w http.ResponseWriter, r *http.Request) {
	relationshipType := chi.URLParam(r, "relationshipType")
	if relationshipType == "" {
		middleware.SendValidationError(w, r, "relationship type is required", nil)
		return
	}

	var req RelationshipSchemaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		middleware.SendValidationError(w, r, "invalid request body", map[string]any{
			"error": err.Error(),
		})
		return
	}

	schema, err := h.engine.GetRelationshipSchema(r.Context(), relationshipType)
	if err != nil {
		statusCode := middleware.HTTPErrorFromAppError(err)
		middleware.SendError(w, r, err, statusCode)
		return
	}

	if req.Properties != nil {
		schema.Properties = req.Properties
	}
	if req.Cardinality != "" {
		schema.Cardinality = req.Cardinality
	}
	schema.DenormalizationConfig = req.DenormalizationConfig

	if err := h.engine.UpdateRelationshipSchema(r.Context(), schema); err != nil {
		statusCode := middleware.HTTPErrorFromAppError(err)
		middleware.SendError(w, r, err, statusCode)
		return
	}

	response := h.relationshipSchemaToResponse(schema)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// DeleteRelationshipSchema godoc
// @Summary Delete a relationship schema
// @Description Delete the schema for a relationship type. This operation cannot be undone.
// @Tags schemas
// @Accept json
// @Produce json
// @Param relationshipType path string true "Relationship Type"
// @Success 204 "No Content"
// @Failure 400 {object} middleware.ErrorResponse
// @Failure 404 {object} middleware.ErrorResponse
// @Failure 500 {object} middleware.ErrorResponse
// @Router /api/v1/schemas/relationships/{relationshipType} [delete]
func (h *SchemaHandler) DeleteRelationshipSchema(w http.ResponseWriter, r *http.Request) {
	relationshipType := chi.URLParam(r, "relationshipType")
	if relationshipType == "" {
		middleware.SendValidationError(w, r, "relationship type is required", nil)
		return
	}

	if err := h.engine.DeleteRelationshipSchema(r.Context(), relationshipType); err != nil {
		statusCode := middleware.HTTPErrorFromAppError(err)
		middleware.SendError(w, r, err, statusCode)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ListRelationshipSchemas godoc
// @Summary List all relationship schemas
// @Description Get a list of all relationship schemas in the system
// @Tags schemas
// @Accept json
// @Produce json
// @Success 200 {array} RelationshipSchemaResponse
// @Failure 500 {object} middleware.ErrorResponse
// @Router /api/v1/schemas/relationships [get]
func (h *SchemaHandler) ListRelationshipSchemas(w http.ResponseWriter, r *http.Request) {
	schemas, err := h.engine.ListRelationshipSchemas(r.Context())
	if err != nil {
		statusCode := middleware.HTTPErrorFromAppError(err)
		middleware.SendError(w, r, err, statusCode)
		return
	}

	responses := make([]RelationshipSchemaResponse, len(schemas))
	for i, schema := range schemas {
		responses[i] = h.relationshipSchemaToResponse(schema)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(responses)
}

func (h *SchemaHandler) entitySchemaToResponse(schema *models.EntitySchema) EntitySchemaResponse {
	return EntitySchemaResponse{
		ID:         schema.ID,
		EntityType: schema.EntityType,
		Properties: schema.Properties,
		Indexes:    schema.Indexes,
		CreatedAt:  schema.CreatedAt,
		UpdatedAt:  schema.UpdatedAt,
		Version:    schema.Version,
	}
}

func (h *SchemaHandler) relationshipSchemaToResponse(schema *models.RelationshipSchema) RelationshipSchemaResponse {
	return RelationshipSchemaResponse{
		ID:                    schema.ID,
		RelationshipType:      schema.RelationshipType,
		FromEntityType:        schema.FromEntityType,
		ToEntityType:          schema.ToEntityType,
		Properties:            schema.Properties,
		Cardinality:           schema.Cardinality,
		DenormalizationConfig: schema.DenormalizationConfig,
		CreatedAt:             schema.CreatedAt,
		UpdatedAt:             schema.UpdatedAt,
		Version:               schema.Version,
	}
}
