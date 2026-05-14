package repository

import (
	"context"
	"errors"
	"time"

	"auth/internal/domain"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type tokenRepository struct {
	db *gorm.DB
}

// NewTokenRepository returns a Gorm-backed domain.TokenRepository.
func NewTokenRepository(db *gorm.DB) domain.TokenRepository {
	return &tokenRepository{db: db}
}

func (r *tokenRepository) Create(ctx context.Context, t *domain.RefreshToken) error {
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	return r.db.WithContext(ctx).Create(t).Error
}

func (r *tokenRepository) FindByHash(ctx context.Context, hash string) (*domain.RefreshToken, error) {
	var t domain.RefreshToken
	err := r.db.WithContext(ctx).Where("token_hash = ?", hash).First(&t).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrTokenNotFound
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *tokenRepository) Revoke(ctx context.Context, id uuid.UUID) error {
	now := time.Now()
	return r.db.WithContext(ctx).
		Model(&domain.RefreshToken{}).
		Where("id = ?", id).
		Update("revoked_at", now).Error
}
