package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"auth/pkg/ratelimit"
)

// LoginRateLimit throttles login attempts per client IP via the shared
// Redis-backed token-bucket limiter. It is the first line of brute-force
// defence; the auth service adds account-level lockout on top.
// perMinute is the sustained refill rate; burst is the immediate allowance.
func LoginRateLimit(lim ratelimit.Limiter, perMinute, burst int) gin.HandlerFunc {
	ratePerSec := float64(perMinute) / 60.0
	return func(c *gin.Context) {
		key := "rl:login:" + c.ClientIP()
		allowed, _ := lim.Allow(c.Request.Context(), key, ratePerSec, burst)
		if !allowed {
			abortError(c, http.StatusTooManyRequests, "RATE_LIMITED", "too many login attempts, slow down")
			return
		}
		c.Next()
	}
}
