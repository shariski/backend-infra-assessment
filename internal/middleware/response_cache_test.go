package middleware

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"auth/internal/domain"
	"auth/pkg/cache"
)

// fakeCache is an in-memory cache.Cache for testing the middleware in
// isolation from Redis. It records call counts and lets tests inject errors.
type fakeCache struct {
	mu     sync.Mutex
	store  map[string][]byte
	getErr error
	setErr error
	getN   int32
	setN   int32
}

func newFakeCache() *fakeCache { return &fakeCache{store: map[string][]byte{}} }

func (f *fakeCache) Get(_ context.Context, key string) ([]byte, bool, error) {
	atomic.AddInt32(&f.getN, 1)
	if f.getErr != nil {
		return nil, false, f.getErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.store[key]
	return v, ok, nil
}

func (f *fakeCache) Set(_ context.Context, key string, value []byte, _ time.Duration) error {
	atomic.AddInt32(&f.setN, 1)
	if f.setErr != nil {
		return f.setErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.store[key] = value
	return nil
}

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// withTestUser pre-populates the gin context the way middleware.Auth would,
// so the cache middleware can read CurrentUserID without a real JWT setup.
func withTestUser(uid uuid.UUID, role domain.Role) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(ctxUserID, uid)
		c.Set(ctxRole, role)
		c.Next()
	}
}

func newCacheRouter(c cache.Cache, uid uuid.UUID, handler gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/auth/me",
		withTestUser(uid, domain.RoleViewer),
		ResponseCache(c, time.Minute, discardLogger()),
		handler,
	)
	return r
}

func doGet(r http.Handler, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	r.ServeHTTP(rec, req)
	return rec
}

// First request misses (handler runs, response is stored); second request
// is served from the cache without invoking the handler.
func TestResponseCache_MissThenHit(t *testing.T) {
	fc := newFakeCache()
	uid := uuid.New()
	var calls int32
	handler := func(c *gin.Context) {
		atomic.AddInt32(&calls, 1)
		c.JSON(http.StatusOK, gin.H{"id": uid.String(), "n": atomic.LoadInt32(&calls)})
	}
	r := newCacheRouter(fc, uid, handler)

	rec1 := doGet(r, "/auth/me")
	if rec1.Code != http.StatusOK {
		t.Fatalf("miss status: got %d, want 200", rec1.Code)
	}
	if got := rec1.Header().Get("X-Cache"); got != "MISS" {
		t.Errorf("miss header: got %q, want MISS", got)
	}
	if !strings.Contains(rec1.Body.String(), uid.String()) {
		t.Errorf("miss body missing uid: %s", rec1.Body.String())
	}
	if c := atomic.LoadInt32(&calls); c != 1 {
		t.Fatalf("handler calls after miss: got %d, want 1", c)
	}

	rec2 := doGet(r, "/auth/me")
	if rec2.Code != http.StatusOK {
		t.Fatalf("hit status: got %d, want 200", rec2.Code)
	}
	if got := rec2.Header().Get("X-Cache"); got != "HIT" {
		t.Errorf("hit header: got %q, want HIT", got)
	}
	if rec2.Body.String() != rec1.Body.String() {
		t.Errorf("hit body differs from miss body\nhit:  %s\nmiss: %s", rec2.Body.String(), rec1.Body.String())
	}
	if c := atomic.LoadInt32(&calls); c != 1 {
		t.Errorf("handler must not run on hit: calls=%d, want 1", c)
	}
	if ct := rec2.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("hit content-type: got %q, want application/json...", ct)
	}
}

// Non-200 responses must not be cached.
func TestResponseCache_NonOKNotCached(t *testing.T) {
	fc := newFakeCache()
	uid := uuid.New()
	handler := func(c *gin.Context) {
		c.JSON(http.StatusInternalServerError, gin.H{"oops": true})
	}
	r := newCacheRouter(fc, uid, handler)

	rec := doGet(r, "/auth/me")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status: got %d, want 500", rec.Code)
	}
	if n := atomic.LoadInt32(&fc.setN); n != 0 {
		t.Errorf("non-200 must not call Set; setN=%d", n)
	}
}

// Per-user keying: distinct users get distinct cache entries, so one user
// never sees another user's cached payload.
func TestResponseCache_PerUserKey(t *testing.T) {
	fc := newFakeCache()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/auth/me",
		func(c *gin.Context) {
			id, _ := uuid.Parse(c.Query("u"))
			c.Set(ctxUserID, id)
			c.Next()
		},
		ResponseCache(fc, time.Minute, discardLogger()),
		func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"uid": CurrentUserID(c).String()})
		},
	)

	uid1, uid2 := uuid.New(), uuid.New()
	rec1 := doGet(r, "/auth/me?u="+uid1.String())
	rec2 := doGet(r, "/auth/me?u="+uid2.String())

	if !strings.Contains(rec1.Body.String(), uid1.String()) {
		t.Errorf("user1 body: %s", rec1.Body.String())
	}
	if !strings.Contains(rec2.Body.String(), uid2.String()) {
		t.Errorf("user2 body: %s", rec2.Body.String())
	}
	if rec1.Header().Get("X-Cache") != "MISS" || rec2.Header().Get("X-Cache") != "MISS" {
		t.Errorf("different users must both miss: rec1=%s rec2=%s",
			rec1.Header().Get("X-Cache"), rec2.Header().Get("X-Cache"))
	}
	// Re-hit user1 only — must come from cache and still contain uid1, not uid2.
	rec3 := doGet(r, "/auth/me?u="+uid1.String())
	if rec3.Header().Get("X-Cache") != "HIT" {
		t.Errorf("user1 second request should hit: %s", rec3.Header().Get("X-Cache"))
	}
	if !strings.Contains(rec3.Body.String(), uid1.String()) || strings.Contains(rec3.Body.String(), uid2.String()) {
		t.Errorf("cross-user leak: rec3 body=%s", rec3.Body.String())
	}
}

// A Redis (cache backend) outage must not break the request — the middleware
// transparently falls back to running the handler.
func TestResponseCache_BypassWhenCacheErrors(t *testing.T) {
	fc := newFakeCache()
	fc.getErr = errors.New("redis down")
	uid := uuid.New()
	var calls int32
	handler := func(c *gin.Context) {
		atomic.AddInt32(&calls, 1)
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
	r := newCacheRouter(fc, uid, handler)

	rec := doGet(r, "/auth/me")
	if rec.Code != http.StatusOK {
		t.Fatalf("status on cache error: got %d, want 200", rec.Code)
	}
	if c := atomic.LoadInt32(&calls); c != 1 {
		t.Errorf("handler must still run on cache.Get error; calls=%d", c)
	}
}

// Defensive: if no authenticated user is in the context, skip the cache
// entirely rather than collapsing every anonymous response into one entry.
func TestResponseCache_NoUserBypass(t *testing.T) {
	fc := newFakeCache()
	var calls int32
	handler := func(c *gin.Context) {
		atomic.AddInt32(&calls, 1)
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/auth/me", ResponseCache(fc, time.Minute, discardLogger()), handler)

	_ = doGet(r, "/auth/me")
	if n := atomic.LoadInt32(&fc.getN); n != 0 {
		t.Errorf("must not call Get without a user; getN=%d", n)
	}
	if c := atomic.LoadInt32(&calls); c != 1 {
		t.Errorf("handler must run; calls=%d", c)
	}
}
