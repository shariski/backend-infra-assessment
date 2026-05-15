package middleware

import (
	"github.com/gin-gonic/gin"
)

// RateLimitByRole is a STUB for per-role API rate limiting (Security Analytics).
// TODO: read CurrentRole(c), look up the role's quota, and enforce it with a
// per-user token bucket (similar to LoginRateLimit). It must run after Auth.
func RateLimitByRole() gin.HandlerFunc {
	return func(c *gin.Context) {
		// TODO: enforce role-specific rate limits here.
		c.Next()
	}
}
