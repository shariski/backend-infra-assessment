package middleware

import (
	"github.com/gin-gonic/gin"
)

// context keys for values set by the auth middleware.
const (
	ctxUserID = "ctx.user.id"
	ctxRole   = "ctx.user.role"
)

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type errorResponse struct {
	Error errorBody `json:"error"`
}

// abortError writes a JSON error envelope and stops the handler chain.
// Middleware uses its own writer to avoid importing the handler package.
func abortError(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, errorResponse{Error: errorBody{Code: code, Message: message}})
}
