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
// Tiers descend with privilege: Admins run automation and dashboards, Analysts
// drive interactive tooling, Viewers do occasional reads. burst absorbs short
// page-load fan-out without letting sustained traffic exceed perMinute.
func quotaForRole(role domain.Role) (roleQuota, bool) {
	switch role {
	case domain.RoleAdmin:
		return roleQuota{perMinute: 120, burst: 30}, true
	case domain.RoleAnalyst:
		return roleQuota{perMinute: 60, burst: 20}, true
	case domain.RoleViewer:
		return roleQuota{perMinute: 30, burst: 10}, true
	default:
		return roleQuota{}, false
	}
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
