package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/entropic/entropic/internal/api/middleware"
	"github.com/entropic/entropic/internal/core"
	"github.com/entropic/entropic/internal/models"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
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
	EntityType string                   `json:"entity_type" validate:"required"`
	Properties models.PropertySchema    `json:"properties" validate:"required"`
	Indexes    []models.IndexConfig     `json:"indexes,omitempty"`
}

type EntitySchemaResponse struct {
	ID         uuid.UUID                `json:"id"`
	EntityType string                   `json:"entity_type"`
	Properties models.PropertySchema    `json:"properties"`
	Indexes    []models.IndexConfig     `json:"indexes"`
	CreatedAt  time.Time                `json:"created_at"`
	UpdatedAt  time.Time                `json:"updated_at"`
	Version    int                      `json:"version"`
}

type RelationshipSchemaRequest struct {
	RelationshipType      string                        `json:"relationship_type" validate:"required"`
	FromEntityType        string                        `json:"from_entity_type" validate:"required"`
	ToEntityType          string                        `json:"to_entity_type" validate:"required"`
	Properties            models.PropertySchema         `json:"properties,omitempty"`
	Cardinality           models.CardinalityType        `json:"cardinality" validate:"required"`
	DenormalizationConfig models.DenormalizationConfig `json:"denormalization_config,omitempty"`
}

type RelationshipSchemaResponse struct {
	ID                    uuid.UUID                     `json:"id"`
	RelationshipType      string                        `json:"relationship_type"`
	FromEntityType        string                        `json:"from_entity_type"`
	ToEntityType          string                        `json:"to_entity_type"`
	Properties            models.PropertySchema         `json:"properties"`
	Cardinality           models.CardinalityType        `json:"cardinality"`
	DenormalizationConfig models.DenormalizationConfig `json:"denormalization_config"`
	CreatedAt             time.Time                     `json:"created_at"`
	UpdatedAt             time.Time                     `json:"updated_at"`
	Version               int                           `json:"version"`
}

// CreateEntitySchema creates a new entity schema
// @Summary Create entity schema
// @Description Creates a new schema for an entity type
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
		middleware.SendValidationError(w, r, "invalid request body", map[string]interface{}{
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

// GetEntitySchema retrieves an entity schema
// @Summary Get entity schema
// @Description Retrieves a schema for the specified entity type
// @Tags schemas
// @Produce json
// @Param entityType path string true "Entity Type"
// @Success 200 {object} EntitySchemaResponse
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

// UpdateEntitySchema updates an entity schema
// @Summary Update entity schema
// @Description Updates an existing entity schema
// @Tags schemas
// @Accept json
// @Produce json
// @Param entityType path string true "Entity Type"
// @Param schema body EntitySchemaRequest true "Updated entity schema data"
// @Success 200 {object} EntitySchemaResponse
// @Failure 400 {object} middleware.ErrorResponse
// @Failure 404 {object} middleware.ErrorResponse
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
		middleware.SendValidationError(w, r, "invalid request body", map[string]interface{}{
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

// DeleteEntitySchema deletes an entity schema
// @Summary Delete entity schema
// @Description Deletes an entity schema if no entities of this type exist
// @Tags schemas
// @Param entityType path string true "Entity Type"
// @Success 204
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

// ListEntitySchemas lists all entity schemas
// @Summary List entity schemas
// @Description Lists all entity schemas in the system
// @Tags schemas
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

// CreateRelationshipSchema creates a new relationship schema
// @Summary Create relationship schema
// @Description Creates a new schema for a relationship type
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
		middleware.SendValidationError(w, r, "invalid request body", map[string]interface{}{
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

// GetRelationshipSchema retrieves a relationship schema
// @Summary Get relationship schema
// @Description Retrieves a schema for the specified relationship type
// @Tags schemas
// @Produce json
// @Param relationshipType path string true "Relationship Type"
// @Success 200 {object} RelationshipSchemaResponse
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

// UpdateRelationshipSchema updates a relationship schema
// @Summary Update relationship schema
// @Description Updates an existing relationship schema
// @Tags schemas
// @Accept json
// @Produce json
// @Param relationshipType path string true "Relationship Type"
// @Param schema body RelationshipSchemaRequest true "Updated relationship schema data"
// @Success 200 {object} RelationshipSchemaResponse
// @Failure 400 {object} middleware.ErrorResponse
// @Failure 404 {object} middleware.ErrorResponse
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
		middleware.SendValidationError(w, r, "invalid request body", map[string]interface{}{
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

// DeleteRelationshipSchema deletes a relationship schema
// @Summary Delete relationship schema
// @Description Deletes a relationship schema
// @Tags schemas
// @Param relationshipType path string true "Relationship Type"
// @Success 204
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

// ListRelationshipSchemas lists all relationship schemas
// @Summary List relationship schemas
// @Description Lists all relationship schemas in the system
// @Tags schemas
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