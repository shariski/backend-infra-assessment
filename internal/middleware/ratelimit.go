package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"auth/internal/domain"
	"auth/pkg/ratelimit"
)

// roleQuota describes a token-bucket policy for one role.
//
//	perMinute = sustained refill rate (tokens per minute)
//	burst     = max tokens held at once (allows short spikes)
type roleQuota struct {
	perMinute int
	burst     int
}

// quotaForRole returns the per-user rate-limit policy for the given role.
//
// === LEARNING-MODE CONTRIBUTION POINT ============================================
// This is YOUR design decision. The trade-off:
//   - too generous → an abusive/compromised account can hammer /admin/* freely
//   - too tight    → legitimate power users (especially admins running bulk ops)
//     hit 429s and get frustrated
//
// Roles available: domain.RoleAdmin, domain.RoleAnalyst, domain.RoleViewer
// (see internal/domain/user.go). An "unknown role" branch is required because
// custom roles could be added later — pick the safest fallback.
//
// Suggested shape (5-10 lines):
//
//	switch role {
//	case domain.RoleAdmin:   return roleQuota{perMinute: ???, burst: ???}, true
//	case domain.RoleAnalyst: return roleQuota{perMinute: ???, burst: ???}, true
//	case domain.RoleViewer:  return roleQuota{perMinute: ???, burst: ???}, true
//	default:                 return roleQuota{}, false   // unknown role → deny? or allow?
//	}
//
// The second return value tells the middleware whether a quota was found.
// If false, the middleware will TODO based on your choice (currently: deny).
// =================================================================================
func quotaForRole(role domain.Role) (roleQuota, bool) {
	// TODO: implement
	return roleQuota{}, false
}

// RateLimitByRole enforces a per-user, per-role token-bucket on protected
// routes. MUST be installed AFTER middleware.Auth so CurrentUserID and
// CurrentRole are populated.
func RateLimitByRole(lim ratelimit.Limiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		role := CurrentRole(c)
		uid := CurrentUserID(c)

		quota, ok := quotaForRole(role)
		if !ok {
			abortError(c, http.StatusForbidden, "ROLE_UNKNOWN", "no rate-limit policy for this role")
			return
		}

		key := "rl:role:" + string(role) + ":" + uid.String()
		ratePerSec := float64(quota.perMinute) / 60.0
		allowed, _ := lim.Allow(c.Request.Context(), key, ratePerSec, quota.burst)
		if !allowed {
			abortError(c, http.StatusTooManyRequests, "RATE_LIMITED", "request quota exceeded for your role")
			return
		}
		c.Next()
	}
}
