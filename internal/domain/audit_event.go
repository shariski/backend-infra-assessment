package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// AuditEvent is a single, persisted record of a user-initiated action.
// ActorID and ActorRole are nullable to allow recording anonymous events
// (e.g. failed logins, unauthenticated 4xx responses).
type AuditEvent struct {
	ID         uuid.UUID  `gorm:"type:uuid;primaryKey"`
	ActorID    *uuid.UUID `gorm:"type:uuid;index"`
	ActorRole  string
	Action     string `gorm:"not null"`
	Method     string `gorm:"not null"`
	Path       string `gorm:"not null"`
	StatusCode int    `gorm:"not null"`
	IPAddress  string
	UserAgent  string
	RequestID  string
	CreatedAt  time.Time
}

// AuditEventRepository persists audit events.
type AuditEventRepository interface {
	Create(ctx context.Context, e *AuditEvent) error
}
