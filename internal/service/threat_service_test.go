package service

import (
	"strings"
	"testing"
	"time"

	"auth/internal/domain"

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
