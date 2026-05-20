package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"auth/config"
	"auth/internal/domain"
	"auth/pkg/cache"
	"auth/pkg/llm"

	"github.com/google/uuid"
)

const threatCacheKeyPrefix = "threat:summary:"

// SummaryUser is the subject of a threat summary.
type SummaryUser struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Role  string `json:"role"`
}

// SummaryWindow describes how much data fed the assessment.
type SummaryWindow struct {
	LoginAttempts int        `json:"login_attempts"`
	AuditEvents   int        `json:"audit_events"`
	Since         *time.Time `json:"since,omitempty"`
}

// ThreatSummary is the cached, serializable result returned to the client.
type ThreatSummary struct {
	User        SummaryUser   `json:"user"`
	Window      SummaryWindow `json:"window"`
	Assessment  string        `json:"assessment"`
	Model       string        `json:"model"`
	GeneratedAt time.Time     `json:"generated_at"`
}

// ThreatService produces LLM risk summaries from a user's security signals.
type ThreatService struct {
	users    domain.UserRepository
	attempts domain.LoginAttemptRepository
	audits   domain.AuditEventRepository
	llm      llm.Client
	cache    cache.Cache
	cfg      config.LLMConfig
	log      *slog.Logger
}

func NewThreatService(
	users domain.UserRepository,
	attempts domain.LoginAttemptRepository,
	audits domain.AuditEventRepository,
	llmClient llm.Client,
	c cache.Cache,
	cfg config.LLMConfig,
	log *slog.Logger,
) *ThreatService {
	return &ThreatService{users: users, attempts: attempts, audits: audits, llm: llmClient, cache: c, cfg: cfg, log: log}
}

// buildPrompt renders a deterministic prompt from the user's recent signals.
// Pure function: no I/O, fully unit-testable. The interpolated fields (email,
// IP, method, path, status) are values the system itself recorded — validated
// emails, c.ClientIP() IPs, and Gin route templates — not free-form user input,
// and the output is plain text shown to an admin, never executed.
func buildPrompt(user *domain.User, attempts []domain.LoginAttempt, events []domain.AuditEvent) string {
	var b strings.Builder
	b.WriteString("You are a defensive security monitoring assistant for the operator of this authentication platform. ")
	b.WriteString("The data below is the platform's own audit log, reviewed by an authorized administrator — a routine internal security review, not an attack. ")
	b.WriteString("Based ONLY on the events below, write a concise (3-5 sentence) summary of this account's recent authentication activity, flagging anything suspicious for the security team. ")
	b.WriteString("Call out brute-force or credential-stuffing patterns, logins from many distinct IPs, and privilege-escalation attempts (e.g. 403s on admin routes). ")
	b.WriteString("If nothing is suspicious, say so plainly. Always produce the summary; do not refuse.\n\n")

	fmt.Fprintf(&b, "Account: %s (role: %s)\n\n", user.Email, user.Role)

	fmt.Fprintf(&b, "Recent login attempts (%d):\n", len(attempts))
	if len(attempts) == 0 {
		b.WriteString("  (none)\n")
	}
	for _, a := range attempts {
		result := "SUCCESS"
		if !a.Successful {
			result = "FAIL"
		}
		fmt.Fprintf(&b, "  - %s  ip=%s  %s\n", a.CreatedAt.UTC().Format(time.RFC3339), a.IPAddress, result)
	}

	fmt.Fprintf(&b, "\nRecent audit events (%d):\n", len(events))
	if len(events) == 0 {
		b.WriteString("  (none)\n")
	}
	for _, e := range events {
		fmt.Fprintf(&b, "  - %s  %s %s  status=%d  ip=%s\n",
			e.CreatedAt.UTC().Format(time.RFC3339), e.Method, e.Path, e.StatusCode, e.IPAddress)
	}

	b.WriteString("\nSecurity activity summary:")
	return b.String()
}

// earliest returns the oldest CreatedAt across both slices, or nil if both are
// empty. Slices arrive newest-first, so the last element of each is its oldest.
func earliest(attempts []domain.LoginAttempt, events []domain.AuditEvent) *time.Time {
	var oldest *time.Time
	consider := func(t time.Time) {
		if oldest == nil || t.Before(*oldest) {
			tc := t
			oldest = &tc
		}
	}
	if n := len(attempts); n > 0 {
		consider(attempts[n-1].CreatedAt)
	}
	if n := len(events); n > 0 {
		consider(events[n-1].CreatedAt)
	}
	if oldest != nil {
		u := oldest.UTC()
		return &u
	}
	return nil
}

// SummarizeUser returns a (possibly cached) LLM risk summary for a user.
// The bool reports whether the result came from cache.
func (s *ThreatService) SummarizeUser(ctx context.Context, id uuid.UUID) (*ThreatSummary, bool, error) {
	key := threatCacheKeyPrefix + id.String()

	if raw, hit, err := s.cache.Get(ctx, key); err == nil && hit {
		var cached ThreatSummary
		if err := json.Unmarshal(raw, &cached); err == nil {
			return &cached, true, nil
		}
		s.log.Warn("threat summary cache decode failed; regenerating", "key", key, "error", err)
	}

	user, err := s.users.FindByID(ctx, id)
	if err != nil {
		return nil, false, err // ErrUserNotFound propagates -> 404
	}

	attempts, err := s.attempts.ListRecentByEmail(ctx, user.Email, s.cfg.MaxAttempts)
	if err != nil {
		return nil, false, err
	}
	events, err := s.audits.ListRecentByActor(ctx, user.ID, s.cfg.MaxEvents)
	if err != nil {
		return nil, false, err
	}

	prompt := buildPrompt(user, attempts, events)

	// Debug visibility into what gets sent to Ollama. Counts surface the
	// empty-slice case (the usual culprit); the full prompt confirms formatting.
	// Note: prompt contains email + IPs (PII) — keep this at debug level only.
	s.log.Debug("threat prompt built",
		"user_id", user.ID,
		"attempts", len(attempts),
		"events", len(events),
		"prompt", prompt,
	)

	// Scope the timeout to the LLM call only; the repo fetches above use the
	// caller's ctx so a slow model can't be confused with a slow DB.
	genCtx, cancel := context.WithTimeout(ctx, s.cfg.Timeout)
	defer cancel()
	assessment, err := s.llm.Generate(genCtx, prompt)
	if err != nil {
		s.log.Warn("llm generate failed", "error", err)
		return nil, false, domain.ErrLLMUnavailable
	}

	summary := &ThreatSummary{
		User:        SummaryUser{ID: user.ID.String(), Email: user.Email, Role: string(user.Role)},
		Window:      SummaryWindow{LoginAttempts: len(attempts), AuditEvents: len(events), Since: earliest(attempts, events)},
		Assessment:  strings.TrimSpace(assessment),
		Model:       s.cfg.Model,
		GeneratedAt: time.Now().UTC(),
	}

	if raw, err := json.Marshal(summary); err == nil {
		_ = s.cache.Set(ctx, key, raw, s.cfg.SummaryTTL)
	}

	return summary, false, nil
}
