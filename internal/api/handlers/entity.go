package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/entropic/entropic/internal/api/middleware"
	"github.com/entropic/entropic/internal/core"
	"github.com/entropic/entropic/internal/models"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
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
	URN        string                 `json:"urn" validate:"required"`
	Properties map[string]interface{} `json:"properties" validate:"required"`
}

type EntityResponse struct {
	ID         uuid.UUID              `json:"id"`
	EntityType string                 `json:"entity_type"`
	URN        string                 `json:"urn"`
	Properties map[string]interface{} `json:"properties"`
	CreatedAt  time.Time              `json:"created_at"`
	UpdatedAt  time.Time              `json:"updated_at"`
	Version    int                    `json:"version"`
}

type EntityListResponse struct {
	Entities []EntityResponse `json:"entities"`
	Total    int              `json:"total"`
	Limit    int              `json:"limit"`
	Offset   int              `json:"offset"`
}

// CreateEntity creates a new entity
// @Summary Create a new entity
// @Description Creates a new entity of the specified type
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
		middleware.SendValidationError(w, r, "invalid request body", map[string]interface{}{
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

// GetEntity retrieves an entity by ID
// @Summary Get an entity
// @Description Retrieves an entity by its type and ID
// @Tags entities
// @Produce json
// @Param entityType path string true "Entity Type"
// @Param entityID path string true "Entity ID"
// @Success 200 {object} EntityResponse
// @Failure 404 {object} middleware.ErrorResponse
// @Failure 500 {object} middleware.ErrorResponse
// @Router /api/v1/entities/{entityType}/{entityID} [get]
func (h *EntityHandler) GetEntity(w http.ResponseWriter, r *http.Request) {
	entityType := chi.URLParam(r, "entityType")
	entityIDStr := chi.URLParam(r, "entityID")

	entityID, err := uuid.Parse(entityIDStr)
	if err != nil {
		middleware.SendValidationError(w, r, "invalid entity ID", map[string]interface{}{
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

// UpdateEntity updates an existing entity
// @Summary Update an entity
// @Description Updates an existing entity's properties
// @Tags entities
// @Accept json
// @Produce json
// @Param entityType path string true "Entity Type"
// @Param entityID path string true "Entity ID"
// @Param entity body EntityRequest true "Updated entity data"
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
		middleware.SendValidationError(w, r, "invalid entity ID", map[string]interface{}{
			"entity_id": entityIDStr,
		})
		return
	}

	var req EntityRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		middleware.SendValidationError(w, r, "invalid request body", map[string]interface{}{
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

// DeleteEntity deletes an entity
// @Summary Delete an entity
// @Description Deletes an entity and all its relations
// @Tags entities
// @Param entityType path string true "Entity Type"
// @Param entityID path string true "Entity ID"
// @Success 204
// @Failure 404 {object} middleware.ErrorResponse
// @Failure 500 {object} middleware.ErrorResponse
// @Router /api/v1/entities/{entityType}/{entityID} [delete]
func (h *EntityHandler) DeleteEntity(w http.ResponseWriter, r *http.Request) {
	entityType := chi.URLParam(r, "entityType")
	entityIDStr := chi.URLParam(r, "entityID")

	entityID, err := uuid.Parse(entityIDStr)
	if err != nil {
		middleware.SendValidationError(w, r, "invalid entity ID", map[string]interface{}{
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

// ListEntities lists entities of a given type with pagination
// @Summary List entities
// @Description Lists entities of the specified type with pagination
// @Tags entities
// @Produce json
// @Param entityType path string true "Entity Type"
// @Param limit query int false "Number of entities to return (default: 20, max: 100)"
// @Param offset query int false "Number of entities to skip (default: 0)"
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
		Total:    len(responses), // TODO: should be total count from separate query
		Limit:    limit,
		Offset:   offset,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// GetEntityRelations gets all relations for an entity
// @Summary Get entity relations
// @Description Retrieves all relations involving the specified entity
// @Tags entities
// @Produce json
// @Param entityType path string true "Entity Type"
// @Param entityID path string true "Entity ID"
// @Param relation_types query string false "Comma-separated list of relation types to filter"
// @Success 200 {array} RelationResponse
// @Failure 404 {object} middleware.ErrorResponse
// @Failure 500 {object} middleware.ErrorResponse
// @Router /api/v1/entities/{entityType}/{entityID}/relations [get]
func (h *EntityHandler) GetEntityRelations(w http.ResponseWriter, r *http.Request) {
	entityIDStr := chi.URLParam(r, "entityID")

	entityID, err := uuid.Parse(entityIDStr)
	if err != nil {
		middleware.SendValidationError(w, r, "invalid entity ID", map[string]interface{}{
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

	// Convert to response format
	responses := make([]RelationResponse, len(relations))
	for i, relation := range relations {
		responses[i] = relationToResponse(relation)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(responses)
}

// Search performs text search across entities
// @Summary Search entities
// @Description Performs text search across entities
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
		middleware.SendValidationError(w, r, "invalid search query", map[string]interface{}{
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

// VectorSearch performs vector similarity search
// @Summary Vector search entities
// @Description Performs vector similarity search across entities
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
		middleware.SendValidationError(w, r, "invalid vector search query", map[string]interface{}{
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
