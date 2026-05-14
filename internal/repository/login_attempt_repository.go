package repository

import (
	"context"
	"time"

	"auth/internal/domain"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type loginAttemptRepository struct {
	db *gorm.DB
}

// NewLoginAttemptRepository returns a Gorm-backed domain.LoginAttemptRepository.
func NewLoginAttemptRepository(db *gorm.DB) domain.LoginAttemptRepository {
	return &loginAttemptRepository{db: db}
}

func (r *loginAttemptRepository) Create(ctx context.Context, a *domain.LoginAttempt) error {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	return r.db.WithContext(ctx).Create(a).Error
}

func (r *loginAttemptRepository) CountRecentFailed(ctx context.Context, email string, since time.Time) (int, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&domain.LoginAttempt{}).
		Where("email = ? AND successful = ? AND created_at >= ?", email, false, since).
		Count(&count).Error
	return int(count), err
}
