package middleware

import (
	"context"
	"log/slog"
	"time"

	"auth/internal/domain"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Audit persists one durable event per audited request after it completes.
// Actor identity is read from the Auth middleware (when present); requests that
// never authenticated are recorded with a nil ActorID so failed logins and
// unauthorized attempts still appear in the trail.
func Audit(repo domain.AuditEventRepository, log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		if !shouldRecord(c) {
			return
		}

		event := buildAuditEvent(c)

		// Detach from the request context so the audit write survives client
		// disconnects, and cap it at 2s to bound latency on a slow DB.
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		if err := repo.Create(ctx, event); err != nil {
			log.Error("audit: persist failed",
				"error", err,
				"action", event.Action,
				"actor_id", event.ActorID,
				"request_id", event.RequestID,
			)
		}
	}
}

// buildAuditEvent extracts request metadata into a domain.AuditEvent.
// Path is Gin's matched route pattern (e.g. "/admin/users") rather than the
// raw URL so high-cardinality IDs don't blow up index size or analytics.
func buildAuditEvent(c *gin.Context) *domain.AuditEvent {
	var actorID *uuid.UUID
	if id := CurrentUserID(c); id != uuid.Nil {
		actorID = &id
	}

	path := c.FullPath()
	if path == "" {
		path = c.Request.URL.Path
	}

	return &domain.AuditEvent{
		ActorID:    actorID,
		ActorRole:  string(CurrentRole(c)),
		Action:     c.Request.Method + " " + path,
		Method:     c.Request.Method,
		Path:       path,
		StatusCode: c.Writer.Status(),
		IPAddress:  c.ClientIP(),
		UserAgent:  c.Request.UserAgent(),
		RequestID:  RequestID(c),
	}
}

// shouldRecord decides whether this request is worth persisting to the audit log.
//
// Policy:
//   - Skip /livez and /readyz — kubelet and Cloudflare's edge probe these
//     every few seconds, so recording them would flood the table with no
//     security signal.
//   - Skip /swagger/* — the docs UI is a read-only static surface with no
//     authentication context.
//   - Everything else is recorded, including:
//   - Pre-auth events (failed /auth/login, /auth/refresh) — actor is nil
//     but ip_address + user_agent + status_code answer "who tried what".
//   - Authenticated mutations on /auth/* and /admin/* — mandatory for
//     compliance.
//   - /auth/me reads — kept on the assumption that "who looked at their
//     own profile when" is useful baseline activity. Drop if cost matters.
//   - 404 / unmatched routes — useful for spotting endpoint scans.
func shouldRecord(c *gin.Context) bool {
	switch c.FullPath() {
	case "/livez", "/readyz", "/swagger/*any":
		return false
	}
	return true
}
