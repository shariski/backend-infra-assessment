package handler

import (
	"errors"
	"net/http"

	"auth/internal/domain"

	"github.com/gin-gonic/gin"
)

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type errorResponse struct {
	Error errorBody `json:"error"`
}

// respondJSON writes a successful JSON response.
func respondJSON(c *gin.Context, status int, data any) {
	c.JSON(status, data)
}

// respondValidationError reports a request-binding/validation failure.
func respondValidationError(c *gin.Context, err error) {
	c.JSON(http.StatusBadRequest, errorResponse{
		Error: errorBody{Code: "VALIDATION_ERROR", Message: err.Error()},
	})
}

// respondError maps a domain error to an HTTP status and JSON error envelope.
func respondError(c *gin.Context, err error) {
	status, code, message := http.StatusInternalServerError, "INTERNAL", "internal server error"

	switch {
	case errors.Is(err, domain.ErrInvalidCredentials):
		status, code, message = http.StatusUnauthorized, "INVALID_CREDENTIALS", err.Error()
	case errors.Is(err, domain.ErrEmailTaken):
		status, code, message = http.StatusConflict, "EMAIL_TAKEN", err.Error()
	case errors.Is(err, domain.ErrUserNotFound):
		status, code, message = http.StatusNotFound, "USER_NOT_FOUND", err.Error()
	case errors.Is(err, domain.ErrTokenNotFound),
		errors.Is(err, domain.ErrTokenRevoked),
		errors.Is(err, domain.ErrTokenExpired):
		status, code, message = http.StatusUnauthorized, "INVALID_REFRESH_TOKEN", err.Error()
	case errors.Is(err, domain.ErrAccountLocked):
		status, code, message = http.StatusTooManyRequests, "ACCOUNT_LOCKED", err.Error()
	case errors.Is(err, domain.ErrForbidden):
		status, code, message = http.StatusForbidden, "FORBIDDEN", err.Error()
	case errors.Is(err, domain.ErrUnauthorized):
		status, code, message = http.StatusUnauthorized, "UNAUTHORIZED", err.Error()
	}

	c.JSON(status, errorResponse{Error: errorBody{Code: code, Message: message}})
}
