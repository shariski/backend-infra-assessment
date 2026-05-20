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
// Pure function: no I/O, fully unit-testable.
func buildPrompt(user *domain.User, attempts []domain.LoginAttempt, events []domain.AuditEvent) string {
	var b strings.Builder
	b.WriteString("You are a security analyst for an authentication platform. ")
	b.WriteString("Based ONLY on the events below, write a concise (3-5 sentence) risk assessment for this user account. ")
	b.WriteString("Call out brute-force or credential-stuffing patterns, logins from many distinct IPs, and privilege-escalation attempts (e.g. 403s on admin routes). ")
	b.WriteString("If nothing is suspicious, say so plainly.\n\n")

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

	b.WriteString("\nRisk assessment:")
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

// SummarizeUser is implemented in Task 7. This stub keeps the package building
// (imports used) until the full body lands.
func (s *ThreatService) SummarizeUser(ctx context.Context, id uuid.UUID) (*ThreatSummary, bool, error) {
	_ = json.Marshal // placeholder use; replaced in Task 7
	_ = threatCacheKeyPrefix
	_ = earliest
	return nil, false, domain.ErrLLMUnavailable
}
