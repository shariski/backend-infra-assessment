package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"auth/internal/domain"
	"auth/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type mockSummarizer struct {
	fn func(ctx context.Context, id uuid.UUID) (*service.ThreatSummary, bool, error)
}

func (m *mockSummarizer) SummarizeUser(ctx context.Context, id uuid.UUID) (*service.ThreatSummary, bool, error) {
	return m.fn(ctx, id)
}

func newThreatRouter(s threatSummarizer) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/admin/users/:id/threat-summary", NewThreatHandler(s).Summarize)
	return r
}

func doGet(r http.Handler, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	r.ServeHTTP(rec, req)
	return rec
}

func TestThreatHandler_OK_MissHeader(t *testing.T) {
	uid := uuid.New()
	s := &mockSummarizer{fn: func(ctx context.Context, id uuid.UUID) (*service.ThreatSummary, bool, error) {
		return &service.ThreatSummary{
			User:        service.SummaryUser{ID: id.String(), Email: "u@example.com", Role: "viewer"},
			Assessment:  "all clear",
			Model:       "llama3.2:1b",
			GeneratedAt: time.Now().UTC(),
		}, false, nil
	}}
	rec := doGet(newThreatRouter(s), "/admin/users/"+uid.String()+"/threat-summary")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if h := rec.Header().Get("X-Cache"); h != "MISS" {
		t.Errorf("X-Cache = %q, want MISS", h)
	}
	if !strings.Contains(rec.Body.String(), "all clear") {
		t.Errorf("body = %s", rec.Body.String())
	}
}

func TestThreatHandler_HitHeader(t *testing.T) {
	uid := uuid.New()
	s := &mockSummarizer{fn: func(ctx context.Context, id uuid.UUID) (*service.ThreatSummary, bool, error) {
		return &service.ThreatSummary{Assessment: "cached", Model: "m"}, true, nil
	}}
	rec := doGet(newThreatRouter(s), "/admin/users/"+uid.String()+"/threat-summary")
	if h := rec.Header().Get("X-Cache"); h != "HIT" {
		t.Errorf("X-Cache = %q, want HIT", h)
	}
}

func TestThreatHandler_InvalidUUID(t *testing.T) {
	s := &mockSummarizer{fn: func(ctx context.Context, id uuid.UUID) (*service.ThreatSummary, bool, error) {
		t.Fatal("service must not be called for an invalid id")
		return nil, false, nil
	}}
	rec := doGet(newThreatRouter(s), "/admin/users/not-a-uuid/threat-summary")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestThreatHandler_NotFound(t *testing.T) {
	uid := uuid.New()
	s := &mockSummarizer{fn: func(ctx context.Context, id uuid.UUID) (*service.ThreatSummary, bool, error) {
		return nil, false, domain.ErrUserNotFound
	}}
	rec := doGet(newThreatRouter(s), "/admin/users/"+uid.String()+"/threat-summary")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestThreatHandler_LLMUnavailable(t *testing.T) {
	uid := uuid.New()
	s := &mockSummarizer{fn: func(ctx context.Context, id uuid.UUID) (*service.ThreatSummary, bool, error) {
		return nil, false, domain.ErrLLMUnavailable
	}}
	rec := doGet(newThreatRouter(s), "/admin/users/"+uid.String()+"/threat-summary")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}
