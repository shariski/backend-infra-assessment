package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"auth/config"
	"auth/internal/domain"
	"auth/pkg/llm"

	"github.com/google/uuid"
)

func TestBuildPrompt_ContainsSignals(t *testing.T) {
	user := &domain.User{ID: uuid.New(), Email: "victim@example.com", Role: domain.RoleViewer}
	attempts := []domain.LoginAttempt{
		{Email: user.Email, IPAddress: "1.2.3.4", Successful: false, CreatedAt: time.Now()},
		{Email: user.Email, IPAddress: "5.6.7.8", Successful: true, CreatedAt: time.Now()},
	}
	events := []domain.AuditEvent{
		{Action: "admin.users.list", Method: "GET", Path: "/admin/users", StatusCode: 403, IPAddress: "5.6.7.8", CreatedAt: time.Now()},
	}

	p := buildPrompt(user, attempts, events)

	for _, want := range []string{"victim@example.com", "1.2.3.4", "FAIL", "5.6.7.8", "SUCCESS", "/admin/users", "403"} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt missing %q\n---\n%s", want, p)
		}
	}
}

func TestBuildPrompt_EmptyHistory(t *testing.T) {
	user := &domain.User{ID: uuid.New(), Email: "quiet@example.com", Role: domain.RoleViewer}
	p := buildPrompt(user, nil, nil)
	if !strings.Contains(p, "(none)") {
		t.Errorf("empty prompt should note (none):\n%s", p)
	}
}

// Small models (llama3.2:1b) over-refuse when the task reads as "attack an
// account". The prompt must frame the work as an authorized, defensive review
// and tell the model not to refuse.
func TestBuildPrompt_DefensiveFraming(t *testing.T) {
	user := &domain.User{ID: uuid.New(), Email: "u@example.com", Role: domain.RoleViewer}
	p := strings.ToLower(buildPrompt(user, nil, nil))
	for _, want := range []string{"authorized", "do not refuse"} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt missing defensive framing %q\n---\n%s", want, p)
		}
	}
}

// mockUserRepo and mockAttemptRepo are defined in auth_service_test.go (same package).

type mockAuditRepo struct {
	CreateFn            func(ctx context.Context, e *domain.AuditEvent) error
	ListRecentByActorFn func(ctx context.Context, actorID uuid.UUID, limit int) ([]domain.AuditEvent, error)
}

func (m *mockAuditRepo) Create(ctx context.Context, e *domain.AuditEvent) error {
	return m.CreateFn(ctx, e)
}
func (m *mockAuditRepo) ListRecentByActor(ctx context.Context, actorID uuid.UUID, limit int) ([]domain.AuditEvent, error) {
	return m.ListRecentByActorFn(ctx, actorID, limit)
}

type mockLLM struct {
	GenerateFn func(ctx context.Context, prompt string) (string, error)
}

func (m *mockLLM) Generate(ctx context.Context, prompt string) (string, error) {
	return m.GenerateFn(ctx, prompt)
}

type memCache struct {
	store  map[string][]byte
	getErr error
	setErr error
	setN   int
}

func newMemCache() *memCache { return &memCache{store: map[string][]byte{}} }

func (c *memCache) Get(_ context.Context, key string) ([]byte, bool, error) {
	if c.getErr != nil {
		return nil, false, c.getErr
	}
	v, ok := c.store[key]
	return v, ok, nil
}
func (c *memCache) Set(_ context.Context, key string, value []byte, _ time.Duration) error {
	c.setN++
	if c.setErr != nil {
		return c.setErr
	}
	c.store[key] = value
	return nil
}

func testLLMConfig() config.LLMConfig {
	return config.LLMConfig{Model: "llama3.2:1b", Timeout: time.Second, SummaryTTL: time.Minute, MaxAttempts: 20, MaxEvents: 20}
}

func discardServiceLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func newThreatService(users *mockUserRepo, attempts *mockAttemptRepo, audits *mockAuditRepo, l llm.Client, c *memCache) *ThreatService {
	return NewThreatService(users, attempts, audits, l, c, testLLMConfig(), discardServiceLogger())
}

func TestSummarizeUser_MissGeneratesAndCaches(t *testing.T) {
	uid := uuid.New()
	users := &mockUserRepo{FindByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.User, error) {
		return &domain.User{ID: uid, Email: "u@example.com", Role: domain.RoleViewer}, nil
	}}
	attempts := &mockAttemptRepo{ListRecentByEmailFn: func(ctx context.Context, email string, limit int) ([]domain.LoginAttempt, error) {
		return []domain.LoginAttempt{{Email: email, IPAddress: "1.1.1.1", Successful: false, CreatedAt: time.Now()}}, nil
	}}
	audits := &mockAuditRepo{ListRecentByActorFn: func(ctx context.Context, actorID uuid.UUID, limit int) ([]domain.AuditEvent, error) {
		return nil, nil
	}}
	llmc := &mockLLM{GenerateFn: func(ctx context.Context, prompt string) (string, error) {
		return "  looks risky  ", nil
	}}
	cache := newMemCache()
	svc := newThreatService(users, attempts, audits, llmc, cache)

	got, hit, err := svc.SummarizeUser(context.Background(), uid)
	if err != nil {
		t.Fatalf("SummarizeUser() error = %v", err)
	}
	if hit {
		t.Error("first call should be a MISS")
	}
	if got.Assessment != "looks risky" {
		t.Errorf("Assessment = %q, want trimmed 'looks risky'", got.Assessment)
	}
	if got.Window.LoginAttempts != 1 || got.Window.AuditEvents != 0 {
		t.Errorf("Window = %+v", got.Window)
	}
	if got.Model != "llama3.2:1b" {
		t.Errorf("Model = %q", got.Model)
	}
	if cache.setN != 1 {
		t.Errorf("expected one cache Set, got %d", cache.setN)
	}

	llmc.GenerateFn = func(ctx context.Context, prompt string) (string, error) {
		t.Fatal("LLM must not be called on cache hit")
		return "", nil
	}
	got2, hit2, err := svc.SummarizeUser(context.Background(), uid)
	if err != nil || !hit2 {
		t.Fatalf("second call: hit=%v err=%v, want hit=true", hit2, err)
	}
	if got2.Assessment != "looks risky" {
		t.Errorf("cached Assessment = %q", got2.Assessment)
	}
}

func TestSummarizeUser_UserNotFound(t *testing.T) {
	users := &mockUserRepo{FindByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.User, error) {
		return nil, domain.ErrUserNotFound
	}}
	svc := newThreatService(users, &mockAttemptRepo{}, &mockAuditRepo{}, &mockLLM{}, newMemCache())
	if _, _, err := svc.SummarizeUser(context.Background(), uuid.New()); !errors.Is(err, domain.ErrUserNotFound) {
		t.Fatalf("error = %v, want ErrUserNotFound", err)
	}
}

func TestSummarizeUser_LLMError(t *testing.T) {
	uid := uuid.New()
	users := &mockUserRepo{FindByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.User, error) {
		return &domain.User{ID: uid, Email: "u@example.com", Role: domain.RoleViewer}, nil
	}}
	attempts := &mockAttemptRepo{ListRecentByEmailFn: func(ctx context.Context, email string, limit int) ([]domain.LoginAttempt, error) { return nil, nil }}
	audits := &mockAuditRepo{ListRecentByActorFn: func(ctx context.Context, actorID uuid.UUID, limit int) ([]domain.AuditEvent, error) { return nil, nil }}
	llmc := &mockLLM{GenerateFn: func(ctx context.Context, prompt string) (string, error) { return "", llm.ErrUnavailable }}
	svc := newThreatService(users, attempts, audits, llmc, newMemCache())

	if _, _, err := svc.SummarizeUser(context.Background(), uid); !errors.Is(err, domain.ErrLLMUnavailable) {
		t.Fatalf("error = %v, want ErrLLMUnavailable", err)
	}
}

func TestSummarizeUser_RedisDownFailOpen(t *testing.T) {
	uid := uuid.New()
	users := &mockUserRepo{FindByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.User, error) {
		return &domain.User{ID: uid, Email: "u@example.com", Role: domain.RoleViewer}, nil
	}}
	attempts := &mockAttemptRepo{ListRecentByEmailFn: func(ctx context.Context, email string, limit int) ([]domain.LoginAttempt, error) { return nil, nil }}
	audits := &mockAuditRepo{ListRecentByActorFn: func(ctx context.Context, actorID uuid.UUID, limit int) ([]domain.AuditEvent, error) { return nil, nil }}
	llmc := &mockLLM{GenerateFn: func(ctx context.Context, prompt string) (string, error) { return "ok", nil }}
	cache := newMemCache()
	cache.getErr = errors.New("redis down")
	svc := newThreatService(users, attempts, audits, llmc, cache)

	got, hit, err := svc.SummarizeUser(context.Background(), uid)
	if err != nil {
		t.Fatalf("fail-open expected, got error %v", err)
	}
	if hit {
		t.Error("cache error must be treated as miss")
	}
	if got.Assessment != "ok" {
		t.Errorf("Assessment = %q", got.Assessment)
	}
}

func TestSummarizeUser_CacheSetErrorBestEffort(t *testing.T) {
	uid := uuid.New()
	users := &mockUserRepo{FindByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.User, error) {
		return &domain.User{ID: uid, Email: "u@example.com", Role: domain.RoleViewer}, nil
	}}
	attempts := &mockAttemptRepo{ListRecentByEmailFn: func(ctx context.Context, email string, limit int) ([]domain.LoginAttempt, error) { return nil, nil }}
	audits := &mockAuditRepo{ListRecentByActorFn: func(ctx context.Context, actorID uuid.UUID, limit int) ([]domain.AuditEvent, error) { return nil, nil }}
	llmc := &mockLLM{GenerateFn: func(ctx context.Context, prompt string) (string, error) { return "fine", nil }}
	cache := newMemCache()
	cache.setErr = errors.New("redis write failed")
	svc := newThreatService(users, attempts, audits, llmc, cache)

	got, hit, err := svc.SummarizeUser(context.Background(), uid)
	if err != nil {
		t.Fatalf("cache Set failure must be best-effort, got error %v", err)
	}
	if hit {
		t.Error("hit should be false on fresh generation")
	}
	if got.Assessment != "fine" {
		t.Errorf("Assessment = %q, want 'fine'", got.Assessment)
	}
}

func TestEarliest(t *testing.T) {
	base := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	// slices arrive newest-first, so the LAST element is the oldest.
	att := []domain.LoginAttempt{{CreatedAt: base}, {CreatedAt: base.Add(-1 * time.Hour)}}
	evtOlder := []domain.AuditEvent{{CreatedAt: base.Add(-2 * time.Hour)}, {CreatedAt: base.Add(-3 * time.Hour)}}

	// both populated, events older than attempts -> base-3h
	if got := earliest(att, evtOlder); got == nil || !got.Equal(base.Add(-3*time.Hour)) {
		t.Errorf("earliest(events older) = %v, want %v", got, base.Add(-3*time.Hour))
	}

	// both populated, attempts older than events -> base-5h
	attOlder := []domain.LoginAttempt{{CreatedAt: base}, {CreatedAt: base.Add(-5 * time.Hour)}}
	evt := []domain.AuditEvent{{CreatedAt: base.Add(-1 * time.Hour)}, {CreatedAt: base.Add(-2 * time.Hour)}}
	if got := earliest(attOlder, evt); got == nil || !got.Equal(base.Add(-5*time.Hour)) {
		t.Errorf("earliest(attempts older) = %v, want %v", got, base.Add(-5*time.Hour))
	}

	// both empty -> nil
	if got := earliest(nil, nil); got != nil {
		t.Errorf("earliest(nil,nil) = %v, want nil", got)
	}
}

func TestSummarizeUser_AttemptsRepoError(t *testing.T) {
	uid := uuid.New()
	users := &mockUserRepo{FindByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.User, error) {
		return &domain.User{ID: uid, Email: "u@example.com", Role: domain.RoleViewer}, nil
	}}
	attempts := &mockAttemptRepo{ListRecentByEmailFn: func(ctx context.Context, email string, limit int) ([]domain.LoginAttempt, error) {
		return nil, errors.New("db boom")
	}}
	svc := newThreatService(users, attempts, &mockAuditRepo{}, &mockLLM{}, newMemCache())
	if _, _, err := svc.SummarizeUser(context.Background(), uid); err == nil {
		t.Fatal("expected error from attempts repo, got nil")
	}
}

func TestSummarizeUser_AuditsRepoError(t *testing.T) {
	uid := uuid.New()
	users := &mockUserRepo{FindByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.User, error) {
		return &domain.User{ID: uid, Email: "u@example.com", Role: domain.RoleViewer}, nil
	}}
	attempts := &mockAttemptRepo{ListRecentByEmailFn: func(ctx context.Context, email string, limit int) ([]domain.LoginAttempt, error) {
		return nil, nil
	}}
	audits := &mockAuditRepo{ListRecentByActorFn: func(ctx context.Context, actorID uuid.UUID, limit int) ([]domain.AuditEvent, error) {
		return nil, errors.New("db boom")
	}}
	svc := newThreatService(users, attempts, audits, &mockLLM{}, newMemCache())
	if _, _, err := svc.SummarizeUser(context.Background(), uid); err == nil {
		t.Fatal("expected error from audits repo, got nil")
	}
}
