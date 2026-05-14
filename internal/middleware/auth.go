package middleware

import (
	"net/http"
	"strings"

	"auth/internal/domain"
	"auth/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Auth validates the Bearer access token and stores the user ID and role
// in the Gin context for downstream handlers.
func Auth(jwtSvc *service.JWTService) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			abortError(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing or malformed Authorization header")
			return
		}
		tokenStr := strings.TrimPrefix(header, "Bearer ")

		claims, err := jwtSvc.ParseAccessToken(tokenStr)
		if err != nil {
			abortError(c, http.StatusUnauthorized, "UNAUTHORIZED", "invalid or expired token")
			return
		}
		userID, err := uuid.Parse(claims.UserID)
		if err != nil {
			abortError(c, http.StatusUnauthorized, "UNAUTHORIZED", "invalid token subject")
			return
		}

		c.Set(ctxUserID, userID)
		c.Set(ctxRole, domain.Role(claims.Role))
		c.Next()
	}
}

// CurrentUserID returns the authenticated user's ID, or uuid.Nil if absent.
func CurrentUserID(c *gin.Context) uuid.UUID {
	v, _ := c.Get(ctxUserID)
	id, _ := v.(uuid.UUID)
	return id
}

// CurrentRole returns the authenticated user's role, or "" if absent.
func CurrentRole(c *gin.Context) domain.Role {
	v, _ := c.Get(ctxRole)
	role, _ := v.(domain.Role)
	return role
}
