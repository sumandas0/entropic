package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/sumandas0/entropic/internal/api/middleware"
	"github.com/sumandas0/entropic/internal/core"
	"github.com/sumandas0/entropic/internal/models"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
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
	RelationType     string                 `json:"relation_type" validate:"required"`
	FromEntityID     uuid.UUID              `json:"from_entity_id" validate:"required"`
	FromEntityType   string                 `json:"from_entity_type" validate:"required"`
	ToEntityID       uuid.UUID              `json:"to_entity_id" validate:"required"`
	ToEntityType     string                 `json:"to_entity_type" validate:"required"`
	Properties       map[string]interface{} `json:"properties,omitempty"`
}

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

func (h *RelationHandler) CreateRelation(w http.ResponseWriter, r *http.Request) {
	var req RelationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		middleware.SendValidationError(w, r, "invalid request body", map[string]interface{}{
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