package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type LoginAttempt struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey"`
	Email      string    `gorm:"not null;index"`
	IPAddress  string    `gorm:"not null"`
	Successful bool      `gorm:"not null"`
	CreatedAt  time.Time
}

// LoginAttemptRepository records login attempts and supports brute-force checks.
type LoginAttemptRepository interface {
	Create(ctx context.Context, a *LoginAttempt) error
	CountRecentFailed(ctx context.Context, email string, since time.Time) (int, error)
	ListRecentByEmail(ctx context.Context, email string, limit int) ([]LoginAttempt, error)
}
