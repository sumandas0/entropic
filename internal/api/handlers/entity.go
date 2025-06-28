package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/sumandas0/entropic/internal/api/middleware"
	"github.com/sumandas0/entropic/internal/core"
	"github.com/sumandas0/entropic/internal/models"
)

type EntityHandler struct {
	engine *core.Engine
}

func NewEntityHandler(engine *core.Engine) *EntityHandler {
	return &EntityHandler{
		engine: engine,
	}
}

type EntityRequest struct {
	URN        string         `json:"urn" validate:"required" example:"urn:entropic:user:123"`
	Properties map[string]any `json:"properties" validate:"required" swaggertype:"object"`
}

type EntityResponse struct {
	ID         uuid.UUID      `json:"id" example:"550e8400-e29b-41d4-a716-446655440000"`
	EntityType string         `json:"entity_type" example:"user"`
	URN        string         `json:"urn" example:"urn:entropic:user:123"`
	Properties map[string]any `json:"properties" swaggertype:"object"`
	CreatedAt  time.Time      `json:"created_at" example:"2023-01-01T00:00:00Z"`
	UpdatedAt  time.Time      `json:"updated_at" example:"2023-01-01T00:00:00Z"`
	Version    int            `json:"version" example:"1"`
}

type EntityListResponse struct {
	Entities []EntityResponse `json:"entities"`
	Total    int              `json:"total" example:"100"`
	Limit    int              `json:"limit" example:"10"`
	Offset   int              `json:"offset" example:"0"`
}

// CreateEntity godoc
// @Summary Create a new entity
// @Description Create a new entity of the specified type
// @Tags entities
// @Accept json
// @Produce json
// @Param entityType path string true "Entity Type"
// @Param entity body EntityRequest true "Entity data"
// @Success 201 {object} EntityResponse
// @Failure 400 {object} middleware.ErrorResponse
// @Failure 409 {object} middleware.ErrorResponse
// @Failure 500 {object} middleware.ErrorResponse
// @Router /api/v1/entities/{entityType} [post]
func (h *EntityHandler) CreateEntity(w http.ResponseWriter, r *http.Request) {
	entityType := chi.URLParam(r, "entityType")
	if entityType == "" {
		middleware.SendValidationError(w, r, "entity type is required", nil)
		return
	}

	var req EntityRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		middleware.SendValidationError(w, r, "invalid request body", map[string]any{
			"error": err.Error(),
		})
		return
	}

	entity := models.NewEntity(entityType, req.URN, req.Properties)

	if err := h.engine.CreateEntity(r.Context(), entity); err != nil {
		statusCode := middleware.HTTPErrorFromAppError(err)
		middleware.SendError(w, r, err, statusCode)
		return
	}

	response := h.entityToResponse(entity)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

// GetEntity godoc
// @Summary Get an entity by ID
// @Description Get a specific entity by its type and ID
// @Tags entities
// @Accept json
// @Produce json
// @Param entityType path string true "Entity Type"
// @Param entityID path string true "Entity ID" format(uuid)
// @Success 200 {object} EntityResponse
// @Failure 400 {object} middleware.ErrorResponse
// @Failure 404 {object} middleware.ErrorResponse
// @Failure 500 {object} middleware.ErrorResponse
// @Router /api/v1/entities/{entityType}/{entityID} [get]
func (h *EntityHandler) GetEntity(w http.ResponseWriter, r *http.Request) {
	entityType := chi.URLParam(r, "entityType")
	entityIDStr := chi.URLParam(r, "entityID")

	entityID, err := uuid.Parse(entityIDStr)
	if err != nil {
		middleware.SendValidationError(w, r, "invalid entity ID", map[string]any{
			"entity_id": entityIDStr,
		})
		return
	}

	entity, err := h.engine.GetEntity(r.Context(), entityType, entityID)
	if err != nil {
		statusCode := middleware.HTTPErrorFromAppError(err)
		middleware.SendError(w, r, err, statusCode)
		return
	}

	response := h.entityToResponse(entity)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// UpdateEntity godoc
// @Summary Update an entity
// @Description Update an existing entity's properties and/or URN
// @Tags entities
// @Accept json
// @Produce json
// @Param entityType path string true "Entity Type"
// @Param entityID path string true "Entity ID" format(uuid)
// @Param entity body EntityRequest true "Entity update data"
// @Success 200 {object} EntityResponse
// @Failure 400 {object} middleware.ErrorResponse
// @Failure 404 {object} middleware.ErrorResponse
// @Failure 409 {object} middleware.ErrorResponse
// @Failure 500 {object} middleware.ErrorResponse
// @Router /api/v1/entities/{entityType}/{entityID} [patch]
func (h *EntityHandler) UpdateEntity(w http.ResponseWriter, r *http.Request) {
	entityType := chi.URLParam(r, "entityType")
	entityIDStr := chi.URLParam(r, "entityID")

	entityID, err := uuid.Parse(entityIDStr)
	if err != nil {
		middleware.SendValidationError(w, r, "invalid entity ID", map[string]any{
			"entity_id": entityIDStr,
		})
		return
	}

	var req EntityRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		middleware.SendValidationError(w, r, "invalid request body", map[string]any{
			"error": err.Error(),
		})
		return
	}

	entity, err := h.engine.GetEntity(r.Context(), entityType, entityID)
	if err != nil {
		statusCode := middleware.HTTPErrorFromAppError(err)
		middleware.SendError(w, r, err, statusCode)
		return
	}

	entity.Properties = req.Properties
	if req.URN != "" {
		entity.URN = req.URN
	}

	if err := h.engine.UpdateEntity(r.Context(), entity); err != nil {
		statusCode := middleware.HTTPErrorFromAppError(err)
		middleware.SendError(w, r, err, statusCode)
		return
	}

	response := h.entityToResponse(entity)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// DeleteEntity godoc
// @Summary Delete an entity
// @Description Delete a specific entity by its type and ID
// @Tags entities
// @Accept json
// @Produce json
// @Param entityType path string true "Entity Type"
// @Param entityID path string true "Entity ID" format(uuid)
// @Success 204 "No Content"
// @Failure 400 {object} middleware.ErrorResponse
// @Failure 404 {object} middleware.ErrorResponse
// @Failure 500 {object} middleware.ErrorResponse
// @Router /api/v1/entities/{entityType}/{entityID} [delete]
func (h *EntityHandler) DeleteEntity(w http.ResponseWriter, r *http.Request) {
	entityType := chi.URLParam(r, "entityType")
	entityIDStr := chi.URLParam(r, "entityID")

	entityID, err := uuid.Parse(entityIDStr)
	if err != nil {
		middleware.SendValidationError(w, r, "invalid entity ID", map[string]any{
			"entity_id": entityIDStr,
		})
		return
	}

	if err := h.engine.DeleteEntity(r.Context(), entityType, entityID); err != nil {
		statusCode := middleware.HTTPErrorFromAppError(err)
		middleware.SendError(w, r, err, statusCode)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ListEntities godoc
// @Summary List entities of a specific type
// @Description Get a paginated list of entities of a specific type
// @Tags entities
// @Accept json
// @Produce json
// @Param entityType path string true "Entity Type"
// @Param limit query int false "Limit" default(20) minimum(1) maximum(100)
// @Param offset query int false "Offset" default(0) minimum(0)
// @Success 200 {object} EntityListResponse
// @Failure 400 {object} middleware.ErrorResponse
// @Failure 500 {object} middleware.ErrorResponse
// @Router /api/v1/entities/{entityType} [get]
func (h *EntityHandler) ListEntities(w http.ResponseWriter, r *http.Request) {
	entityType := chi.URLParam(r, "entityType")

	limit := 20
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	offset := 0
	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	entities, err := h.engine.ListEntities(r.Context(), entityType, limit, offset)
	if err != nil {
		statusCode := middleware.HTTPErrorFromAppError(err)
		middleware.SendError(w, r, err, statusCode)
		return
	}

	responses := make([]EntityResponse, len(entities))
	for i, entity := range entities {
		responses[i] = h.entityToResponse(entity)
	}

	response := EntityListResponse{
		Entities: responses,
		Total:    len(responses),
		Limit:    limit,
		Offset:   offset,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// GetEntityRelations godoc
// @Summary Get relations for an entity
// @Description Get all relations for a specific entity, optionally filtered by relation types
// @Tags entities
// @Accept json
// @Produce json
// @Param entityType path string true "Entity Type"
// @Param entityID path string true "Entity ID" format(uuid)
// @Param relation_types query string false "Comma-separated list of relation types to filter"
// @Success 200 {array} RelationResponse
// @Failure 400 {object} middleware.ErrorResponse
// @Failure 404 {object} middleware.ErrorResponse
// @Failure 500 {object} middleware.ErrorResponse
// @Router /api/v1/entities/{entityType}/{entityID}/relations [get]
func (h *EntityHandler) GetEntityRelations(w http.ResponseWriter, r *http.Request) {
	entityIDStr := chi.URLParam(r, "entityID")

	entityID, err := uuid.Parse(entityIDStr)
	if err != nil {
		middleware.SendValidationError(w, r, "invalid entity ID", map[string]any{
			"entity_id": entityIDStr,
		})
		return
	}

	var relationTypes []string
	if typesStr := r.URL.Query().Get("relation_types"); typesStr != "" {
		relationTypes = parseCommaSeparated(typesStr)
	}

	relations, err := h.engine.GetRelationsByEntity(r.Context(), entityID, relationTypes)
	if err != nil {
		statusCode := middleware.HTTPErrorFromAppError(err)
		middleware.SendError(w, r, err, statusCode)
		return
	}

	responses := make([]RelationResponse, len(relations))
	for i, relation := range relations {
		responses[i] = relationToResponse(relation)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(responses)
}

// Search godoc
// @Summary Search entities
// @Description Search for entities using text search with optional filters and facets
// @Tags search
// @Accept json
// @Produce json
// @Param query body models.SearchQuery true "Search query"
// @Success 200 {object} models.SearchResult
// @Failure 400 {object} middleware.ErrorResponse
// @Failure 500 {object} middleware.ErrorResponse
// @Router /api/v1/search [post]
func (h *EntityHandler) Search(w http.ResponseWriter, r *http.Request) {
	var query models.SearchQuery
	if err := json.NewDecoder(r.Body).Decode(&query); err != nil {
		middleware.SendValidationError(w, r, "invalid search query", map[string]any{
			"error": err.Error(),
		})
		return
	}

	if len(query.EntityTypes) == 0 {
		middleware.SendValidationError(w, r, "entity_types is required", nil)
		return
	}

	if query.Limit <= 0 || query.Limit > 1000 {
		query.Limit = 20
	}
	if query.Offset < 0 {
		query.Offset = 0
	}

	result, err := h.engine.Search(r.Context(), &query)
	if err != nil {
		statusCode := middleware.HTTPErrorFromAppError(err)
		middleware.SendError(w, r, err, statusCode)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(result)
}

// VectorSearch godoc
// @Summary Vector similarity search
// @Description Search for entities using vector similarity
// @Tags search
// @Accept json
// @Produce json
// @Param query body models.VectorQuery true "Vector search query"
// @Success 200 {object} models.SearchResult
// @Failure 400 {object} middleware.ErrorResponse
// @Failure 500 {object} middleware.ErrorResponse
// @Router /api/v1/search/vector [post]
func (h *EntityHandler) VectorSearch(w http.ResponseWriter, r *http.Request) {
	var query models.VectorQuery
	if err := json.NewDecoder(r.Body).Decode(&query); err != nil {
		middleware.SendValidationError(w, r, "invalid vector search query", map[string]any{
			"error": err.Error(),
		})
		return
	}

	if len(query.EntityTypes) == 0 {
		middleware.SendValidationError(w, r, "entity_types is required", nil)
		return
	}
	if len(query.Vector) == 0 {
		middleware.SendValidationError(w, r, "vector is required", nil)
		return
	}
	if query.VectorField == "" {
		middleware.SendValidationError(w, r, "vector_field is required", nil)
		return
	}

	if query.TopK <= 0 || query.TopK > 1000 {
		query.TopK = 10
	}

	result, err := h.engine.VectorSearch(r.Context(), &query)
	if err != nil {
		statusCode := middleware.HTTPErrorFromAppError(err)
		middleware.SendError(w, r, err, statusCode)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(result)
}

func (h *EntityHandler) entityToResponse(entity *models.Entity) EntityResponse {
	return EntityResponse{
		ID:         entity.ID,
		EntityType: entity.EntityType,
		URN:        entity.URN,
		Properties: entity.Properties,
		CreatedAt:  entity.CreatedAt,
		UpdatedAt:  entity.UpdatedAt,
		Version:    entity.Version,
	}
}

func parseCommaSeparated(s string) []string {
	var result []string
	if s == "" {
		return result
	}

	parts := splitByComma(s)
	for _, part := range parts {
		trimmed := trimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func splitByComma(s string) []string {
	var result []string
	var current string

	for _, char := range s {
		if char == ',' {
			result = append(result, current)
			current = ""
		} else {
			current += string(char)
		}
	}

	if current != "" {
		result = append(result, current)
	}

	return result
}

func trimSpace(s string) string {
	start := 0
	end := len(s)

	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n') {
		start++
	}

	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n') {
		end--
	}

	return s[start:end]
}
