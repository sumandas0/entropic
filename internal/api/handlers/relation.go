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

type RelationHandler struct {
	engine *core.Engine
}

func NewRelationHandler(engine *core.Engine) *RelationHandler {
	return &RelationHandler{
		engine: engine,
	}
}

type RelationRequest struct {
	RelationType   string         `json:"relation_type" validate:"required" example:"owns"`
	FromEntityID   uuid.UUID      `json:"from_entity_id" validate:"required" example:"550e8400-e29b-41d4-a716-446655440000"`
	FromEntityType string         `json:"from_entity_type" validate:"required" example:"user"`
	ToEntityID     uuid.UUID      `json:"to_entity_id" validate:"required" example:"650e8400-e29b-41d4-a716-446655440001"`
	ToEntityType   string         `json:"to_entity_type" validate:"required" example:"document"`
	Properties     map[string]any `json:"properties,omitempty" swaggertype:"object"`
}

type RelationResponse struct {
	ID             uuid.UUID      `json:"id" example:"750e8400-e29b-41d4-a716-446655440002"`
	RelationType   string         `json:"relation_type" example:"owns"`
	FromEntityID   uuid.UUID      `json:"from_entity_id" example:"550e8400-e29b-41d4-a716-446655440000"`
	FromEntityType string         `json:"from_entity_type" example:"user"`
	ToEntityID     uuid.UUID      `json:"to_entity_id" example:"650e8400-e29b-41d4-a716-446655440001"`
	ToEntityType   string         `json:"to_entity_type" example:"document"`
	Properties     map[string]any `json:"properties,omitempty" swaggertype:"object"`
	CreatedAt      time.Time      `json:"created_at" example:"2023-01-01T00:00:00Z"`
	UpdatedAt      time.Time      `json:"updated_at" example:"2023-01-01T00:00:00Z"`
}

// CreateRelation godoc
// @Summary Create a new relation
// @Description Create a new relation between two entities
// @Tags relations
// @Accept json
// @Produce json
// @Param relation body RelationRequest true "Relation data"
// @Success 201 {object} RelationResponse
// @Failure 400 {object} middleware.ErrorResponse
// @Failure 404 {object} middleware.ErrorResponse
// @Failure 409 {object} middleware.ErrorResponse
// @Failure 500 {object} middleware.ErrorResponse
// @Router /api/v1/relations [post]
func (h *RelationHandler) CreateRelation(w http.ResponseWriter, r *http.Request) {
	var req RelationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		middleware.SendValidationError(w, r, "invalid request body", map[string]any{
			"error": err.Error(),
		})
		return
	}

	if req.RelationType == "" {
		middleware.SendValidationError(w, r, "relation_type is required", nil)
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

	relation := models.NewRelation(
		req.RelationType,
		req.FromEntityID,
		req.FromEntityType,
		req.ToEntityID,
		req.ToEntityType,
		req.Properties,
	)

	if err := h.engine.CreateRelation(r.Context(), relation); err != nil {
		statusCode := middleware.HTTPErrorFromAppError(err)
		middleware.SendError(w, r, err, statusCode)
		return
	}

	response := relationToResponse(relation)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

// GetRelation godoc
// @Summary Get a relation by ID
// @Description Get a specific relation by its ID
// @Tags relations
// @Accept json
// @Produce json
// @Param relationID path string true "Relation ID" format(uuid)
// @Success 200 {object} RelationResponse
// @Failure 400 {object} middleware.ErrorResponse
// @Failure 404 {object} middleware.ErrorResponse
// @Failure 500 {object} middleware.ErrorResponse
// @Router /api/v1/relations/{relationID} [get]
func (h *RelationHandler) GetRelation(w http.ResponseWriter, r *http.Request) {
	relationIDStr := chi.URLParam(r, "relationID")

	relationID, err := uuid.Parse(relationIDStr)
	if err != nil {
		middleware.SendValidationError(w, r, "invalid relation ID", map[string]any{
			"relation_id": relationIDStr,
		})
		return
	}

	relation, err := h.engine.GetRelation(r.Context(), relationID)
	if err != nil {
		statusCode := middleware.HTTPErrorFromAppError(err)
		middleware.SendError(w, r, err, statusCode)
		return
	}

	response := relationToResponse(relation)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// DeleteRelation godoc
// @Summary Delete a relation
// @Description Delete a specific relation by its ID
// @Tags relations
// @Accept json
// @Produce json
// @Param relationID path string true "Relation ID" format(uuid)
// @Success 204 "No Content"
// @Failure 400 {object} middleware.ErrorResponse
// @Failure 404 {object} middleware.ErrorResponse
// @Failure 500 {object} middleware.ErrorResponse
// @Router /api/v1/relations/{relationID} [delete]
func (h *RelationHandler) DeleteRelation(w http.ResponseWriter, r *http.Request) {
	relationIDStr := chi.URLParam(r, "relationID")

	relationID, err := uuid.Parse(relationIDStr)
	if err != nil {
		middleware.SendValidationError(w, r, "invalid relation ID", map[string]any{
			"relation_id": relationIDStr,
		})
		return
	}

	if err := h.engine.DeleteRelation(r.Context(), relationID); err != nil {
		statusCode := middleware.HTTPErrorFromAppError(err)
		middleware.SendError(w, r, err, statusCode)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func relationToResponse(relation *models.Relation) RelationResponse {
	return RelationResponse{
		ID:             relation.ID,
		RelationType:   relation.RelationType,
		FromEntityID:   relation.FromEntityID,
		FromEntityType: relation.FromEntityType,
		ToEntityID:     relation.ToEntityID,
		ToEntityType:   relation.ToEntityType,
		Properties:     relation.Properties,
		CreatedAt:      relation.CreatedAt,
		UpdatedAt:      relation.UpdatedAt,
	}
}
