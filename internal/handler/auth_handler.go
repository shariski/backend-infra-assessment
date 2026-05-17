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

// Register creates a new user account.
//
// @Summary      Register a new user
// @Description  Creates a Viewer-role account. Returns the created user (without the password hash).
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body  body      RegisterRequest  true  "Email and password (min 8 chars)"
// @Success      201   {object}  UserResponse
// @Failure      400   {object}  ErrorResponse  "validation error"
// @Failure      409   {object}  ErrorResponse  "email already taken"
// @Router       /auth/register [post]
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

// Login authenticates a user and returns an access + refresh token pair.
//
// @Summary      Log in
// @Description  Verifies credentials and returns JWT access and refresh tokens. Subject to per-IP brute-force protection and per-account lockout.
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body  body      LoginRequest  true  "User credentials"
// @Success      200   {object}  TokenResponse
// @Failure      400   {object}  ErrorResponse  "validation error"
// @Failure      401   {object}  ErrorResponse  "invalid credentials"
// @Failure      429   {object}  ErrorResponse  "account locked or rate-limited"
// @Router       /auth/login [post]
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

// Refresh exchanges a refresh token for a new access + refresh token pair.
//
// @Summary      Refresh access token
// @Description  Rotates the refresh token. The previous refresh token is invalidated on success.
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body  body      RefreshRequest  true  "Refresh token"
// @Success      200   {object}  TokenResponse
// @Failure      400   {object}  ErrorResponse  "validation error"
// @Failure      401   {object}  ErrorResponse  "refresh token invalid, revoked, or expired"
// @Router       /auth/refresh [post]
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

// Logout revokes the supplied refresh token.
//
// @Summary      Log out
// @Description  Revokes the refresh token. The access token remains valid until it expires.
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body  body      LogoutRequest  true  "Refresh token to revoke"
// @Success      204   "no content"
// @Failure      400   {object}  ErrorResponse  "validation error"
// @Failure      401   {object}  ErrorResponse  "missing or invalid access token"
// @Security     BearerAuth
// @Router       /auth/logout [post]
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

// Me returns the authenticated user's profile.
//
// @Summary      Current user
// @Tags         auth
// @Produce      json
// @Success      200  {object}  UserResponse
// @Failure      401  {object}  ErrorResponse  "missing or invalid access token"
// @Security     BearerAuth
// @Router       /auth/me [get]
func (h *AuthHandler) Me(c *gin.Context) {
	userID := middleware.CurrentUserID(c)
	user, err := h.svc.GetUser(c.Request.Context(), userID)
	if err != nil {
		respondError(c, err)
		return
	}
	respondJSON(c, http.StatusOK, toUserResponse(user))
}

// ListUsers returns every registered user. Admin-only.
//
// @Summary      List all users (admin)
// @Tags         admin
// @Produce      json
// @Success      200  {object}  UsersListResponse
// @Failure      401  {object}  ErrorResponse  "missing or invalid access token"
// @Failure      403  {object}  ErrorResponse  "caller is not Admin"
// @Failure      429  {object}  ErrorResponse  "rate-limited"
// @Security     BearerAuth
// @Router       /admin/users [get]
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
	respondJSON(c, http.StatusOK, UsersListResponse{Users: out})
}
