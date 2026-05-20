package handler

import (
	"context"
	"errors"
	"net/http"

	"auth/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// threatSummarizer is the subset of *service.ThreatService the handler needs.
// Declared here so the handler is testable with a mock.
type threatSummarizer interface {
	SummarizeUser(ctx context.Context, id uuid.UUID) (*service.ThreatSummary, bool, error)
}

// ThreatHandler serves AI threat summaries.
type ThreatHandler struct {
	svc threatSummarizer
}

func NewThreatHandler(svc threatSummarizer) *ThreatHandler {
	return &ThreatHandler{svc: svc}
}

// Summarize returns an AI-generated risk assessment for a user.
//
// @Summary      AI threat summary for a user (admin)
// @Description  Summarizes a user's recent login attempts and audit events into a plain-language risk assessment using a self-hosted LLM. Cached per target user; see the X-Cache header (HIT/MISS). Returns 503 if the LLM backend is unavailable.
// @Tags         admin
// @Produce      json
// @Param        id   path      string  true  "User ID (UUID)"
// @Success      200  {object}  service.ThreatSummary
// @Failure      400  {object}  ErrorResponse  "invalid user id"
// @Failure      401  {object}  ErrorResponse  "missing or invalid access token"
// @Failure      403  {object}  ErrorResponse  "caller is not Admin"
// @Failure      404  {object}  ErrorResponse  "user not found"
// @Failure      503  {object}  ErrorResponse  "LLM backend unavailable"
// @Security     BearerAuth
// @Router       /admin/users/{id}/threat-summary [get]
func (h *ThreatHandler) Summarize(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondValidationError(c, errors.New("invalid user id: must be a UUID"))
		return
	}
	summary, hit, err := h.svc.SummarizeUser(c.Request.Context(), id)
	if err != nil {
		respondError(c, err)
		return
	}
	if hit {
		c.Header("X-Cache", "HIT")
	} else {
		c.Header("X-Cache", "MISS")
	}
	respondJSON(c, http.StatusOK, summary)
}
