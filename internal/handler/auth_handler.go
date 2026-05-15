package handler

import (
	"net/http"

	"auth/internal/middleware"
	"auth/internal/service"

	"github.com/gin-gonic/gin"
)

// AuthHandler exposes the authentication endpoints over HTTP.
type AuthHandler struct {
	svc *service.AuthService
}

func NewAuthHandler(svc *service.AuthService) *AuthHandler {
	return &AuthHandler{svc: svc}
}

// Register handles POST /auth/register.
func (h *AuthHandler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondValidationError(c, err)
		return
	}
	user, err := h.svc.Register(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		respondError(c, err)
		return
	}
	respondJSON(c, http.StatusCreated, toUserResponse(user))
}

// Login handles POST /auth/login.
func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondValidationError(c, err)
		return
	}
	pair, err := h.svc.Login(c.Request.Context(), req.Email, req.Password, c.ClientIP())
	if err != nil {
		respondError(c, err)
		return
	}
	respondJSON(c, http.StatusOK, TokenResponse{
		AccessToken:  pair.AccessToken,
		RefreshToken: pair.RefreshToken,
	})
}

// Refresh handles POST /auth/refresh.
func (h *AuthHandler) Refresh(c *gin.Context) {
	var req RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondValidationError(c, err)
		return
	}
	pair, err := h.svc.Refresh(c.Request.Context(), req.RefreshToken)
	if err != nil {
		respondError(c, err)
		return
	}
	respondJSON(c, http.StatusOK, TokenResponse{
		AccessToken:  pair.AccessToken,
		RefreshToken: pair.RefreshToken,
	})
}

// Logout handles POST /auth/logout (requires a valid access token).
func (h *AuthHandler) Logout(c *gin.Context) {
	var req LogoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondValidationError(c, err)
		return
	}
	if err := h.svc.Logout(c.Request.Context(), req.RefreshToken); err != nil {
		respondError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// Me handles GET /auth/me (requires a valid access token).
func (h *AuthHandler) Me(c *gin.Context) {
	userID := middleware.CurrentUserID(c)
	user, err := h.svc.GetUser(c.Request.Context(), userID)
	if err != nil {
		respondError(c, err)
		return
	}
	respondJSON(c, http.StatusOK, toUserResponse(user))
}

// ListUsers handles GET /admin/users (requires the Admin role).
func (h *AuthHandler) ListUsers(c *gin.Context) {
	users, err := h.svc.ListUsers(c.Request.Context())
	if err != nil {
		respondError(c, err)
		return
	}
	out := make([]UserResponse, 0, len(users))
	for i := range users {
		out = append(out, toUserResponse(&users[i]))
	}
	respondJSON(c, http.StatusOK, gin.H{"users": out})
}
