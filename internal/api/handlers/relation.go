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

// RelationHandler handles relation-related HTTP requests
type RelationHandler struct {
	engine *core.Engine
}

// NewRelationHandler creates a new relation handler
func NewRelationHandler(engine *core.Engine) *RelationHandler {
	return &RelationHandler{
		engine: engine,
	}
}

// RelationRequest represents the request body for creating relations
type RelationRequest struct {
	RelationType     string                 `json:"relation_type" validate:"required"`
	FromEntityID     uuid.UUID              `json:"from_entity_id" validate:"required"`
	FromEntityType   string                 `json:"from_entity_type" validate:"required"`
	ToEntityID       uuid.UUID              `json:"to_entity_id" validate:"required"`
	ToEntityType     string                 `json:"to_entity_type" validate:"required"`
	Properties       map[string]interface{} `json:"properties,omitempty"`
}

// RelationResponse represents the response body for relation operations
type RelationResponse struct {
	ID             uuid.UUID              `json:"id"`
	RelationType   string                 `json:"relation_type"`
	FromEntityID   uuid.UUID              `json:"from_entity_id"`
	FromEntityType string                 `json:"from_entity_type"`
	ToEntityID     uuid.UUID              `json:"to_entity_id"`
	ToEntityType   string                 `json:"to_entity_type"`
	Properties     map[string]interface{} `json:"properties,omitempty"`
	CreatedAt      time.Time              `json:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at"`
}

// CreateRelation creates a new relation
// @Summary Create a new relation
// @Description Creates a new relation between two entities
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
		middleware.SendValidationError(w, r, "invalid request body", map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	// Validate required fields
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

	// Create relation model
	relation := models.NewRelation(
		req.RelationType,
		req.FromEntityID,
		req.FromEntityType,
		req.ToEntityID,
		req.ToEntityType,
		req.Properties,
	)

	// Create relation through engine
	if err := h.engine.CreateRelation(r.Context(), relation); err != nil {
		statusCode := middleware.HTTPErrorFromAppError(err)
		middleware.SendError(w, r, err, statusCode)
		return
	}

	// Return created relation
	response := relationToResponse(relation)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

// GetRelation retrieves a relation by ID
// @Summary Get a relation
// @Description Retrieves a relation by its ID
// @Tags relations
// @Produce json
// @Param relationID path string true "Relation ID"
// @Success 200 {object} RelationResponse
// @Failure 404 {object} middleware.ErrorResponse
// @Failure 500 {object} middleware.ErrorResponse
// @Router /api/v1/relations/{relationID} [get]
func (h *RelationHandler) GetRelation(w http.ResponseWriter, r *http.Request) {
	relationIDStr := chi.URLParam(r, "relationID")

	relationID, err := uuid.Parse(relationIDStr)
	if err != nil {
		middleware.SendValidationError(w, r, "invalid relation ID", map[string]interface{}{
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

// DeleteRelation deletes a relation
// @Summary Delete a relation
// @Description Deletes a relation by its ID
// @Tags relations
// @Param relationID path string true "Relation ID"
// @Success 204
// @Failure 404 {object} middleware.ErrorResponse
// @Failure 500 {object} middleware.ErrorResponse
// @Router /api/v1/relations/{relationID} [delete]
func (h *RelationHandler) DeleteRelation(w http.ResponseWriter, r *http.Request) {
	relationIDStr := chi.URLParam(r, "relationID")

	relationID, err := uuid.Parse(relationIDStr)
	if err != nil {
		middleware.SendValidationError(w, r, "invalid relation ID", map[string]interface{}{
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

// Helper function to convert relation to response
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