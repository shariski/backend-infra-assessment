package middleware

import (
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// ipLimiter keeps a per-IP token-bucket rate limiter in memory.
type ipLimiter struct {
	mu       sync.Mutex
	limiters map[string]*rate.Limiter
	rate     rate.Limit
	burst    int
}

func newIPLimiter(r rate.Limit, burst int) *ipLimiter {
	return &ipLimiter{
		limiters: make(map[string]*rate.Limiter),
		rate:     r,
		burst:    burst,
	}
}

func (l *ipLimiter) get(ip string) *rate.Limiter {
	l.mu.Lock()
	defer l.mu.Unlock()
	lim, ok := l.limiters[ip]
	if !ok {
		lim = rate.NewLimiter(l.rate, l.burst)
		l.limiters[ip] = lim
	}
	return lim
}

// LoginRateLimit throttles login attempts per client IP. It is the first line
// of brute-force defence; the auth service adds account-level lockout on top.
// perMinute is the sustained rate; burst is the immediate allowance.
func LoginRateLimit(perMinute, burst int) gin.HandlerFunc {
	limiter := newIPLimiter(rate.Limit(float64(perMinute)/60.0), burst)
	return func(c *gin.Context) {
		if !limiter.get(c.ClientIP()).Allow() {
			abortError(c, http.StatusTooManyRequests, "RATE_LIMITED", "too many login attempts, slow down")
			return
		}
		c.Next()
	}
}
