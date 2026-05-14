package middleware

import (
	"net/http"

	"auth/internal/domain"

	"github.com/gin-gonic/gin"
)

// RequireRole allows the request through only if the authenticated user's role
// is one of the allowed roles. It must run after Auth.
func RequireRole(allowed ...domain.Role) gin.HandlerFunc {
	return func(c *gin.Context) {
		role := CurrentRole(c)
		for _, r := range allowed {
			if role == r {
				c.Next()
				return
			}
		}
		abortError(c, http.StatusForbidden, "FORBIDDEN", "insufficient permissions")
	}
}
