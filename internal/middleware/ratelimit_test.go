package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"auth/internal/domain"
	"auth/pkg/ratelimit"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// stubLimiter is a controllable ratelimit.Limiter for tests. It records the
// arguments of the last Allow call so we can assert the per-role key and quota.
type stubLimiter struct {
	allow     bool
	err       error
	calls     int
	lastKey   string
	lastRate  float64
	lastBurst int
}

func (s *stubLimiter) Allow(_ context.Context, key string, ratePerSec float64, burst int) (bool, error) {
	s.calls++
	s.lastKey = key
	s.lastRate = ratePerSec
	s.lastBurst = burst
	return s.allow, s.err
}

// newRateLimitEngine builds a minimal engine that injects the given identity
// (mimicking the Auth middleware) and then applies RateLimitByRole. role == ""
// simulates an unauthenticated request where no role was set.
func newRateLimitEngine(lim ratelimit.Limiter, role domain.Role, uid uuid.UUID) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(ctxUserID, uid)
		if role != "" {
			c.Set(ctxRole, role)
		}
		c.Next()
	})
	r.GET("/x", RateLimitByRole(lim), func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})
	return r
}

func do(r *gin.Engine) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
	return w
}

func TestQuotaForRole(t *testing.T) {
	tests := []struct {
		role          domain.Role
		wantPerMinute int
		wantBurst     int
		wantOK        bool
	}{
		{domain.RoleAdmin, 120, 30, true},
		{domain.RoleAnalyst, 60, 20, true},
		{domain.RoleViewer, 30, 10, true},
		{domain.Role("ghost"), 0, 0, false},
		{domain.Role(""), 0, 0, false},
	}
	for _, tt := range tests {
		t.Run(string(tt.role), func(t *testing.T) {
			got, ok := quotaForRole(tt.role)
			if ok != tt.wantOK {
				t.Fatalf("quotaForRole(%q) ok = %v, want %v", tt.role, ok, tt.wantOK)
			}
			if got.perMinute != tt.wantPerMinute || got.burst != tt.wantBurst {
				t.Errorf("quotaForRole(%q) = {perMinute:%d burst:%d}, want {perMinute:%d burst:%d}",
					tt.role, got.perMinute, got.burst, tt.wantPerMinute, tt.wantBurst)
			}
		})
	}
}

func TestRateLimitByRole_AllowedPassesThroughWithRoleQuota(t *testing.T) {
	uid := uuid.New()
	lim := &stubLimiter{allow: true}
	w := do(newRateLimitEngine(lim, domain.RoleAdmin, uid))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if lim.calls != 1 {
		t.Fatalf("limiter calls = %d, want 1", lim.calls)
	}
	if wantKey := "rl:role:admin:" + uid.String(); lim.lastKey != wantKey {
		t.Errorf("key = %q, want %q", lim.lastKey, wantKey)
	}
	// Admin quota is 120/min -> 2 tokens/sec, burst 30.
	if lim.lastRate != 2.0 {
		t.Errorf("ratePerSec = %v, want 2.0", lim.lastRate)
	}
	if lim.lastBurst != 30 {
		t.Errorf("burst = %d, want 30", lim.lastBurst)
	}
}

func TestRateLimitByRole_DeniedReturns429(t *testing.T) {
	lim := &stubLimiter{allow: false}
	w := do(newRateLimitEngine(lim, domain.RoleViewer, uuid.New()))

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusTooManyRequests)
	}
}

func TestRateLimitByRole_UnknownRoleReturns403AndSkipsLimiter(t *testing.T) {
	lim := &stubLimiter{allow: true}
	w := do(newRateLimitEngine(lim, domain.Role(""), uuid.New()))

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
	if lim.calls != 0 {
		t.Errorf("limiter calls = %d, want 0 (must not consult limiter for unknown role)", lim.calls)
	}
}
