package repository

import (
	"context"

	"auth/internal/domain"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type auditEventRepository struct {
	db *gorm.DB
}

// NewAuditEventRepository returns a Gorm-backed domain.AuditEventRepository.
func NewAuditEventRepository(db *gorm.DB) domain.AuditEventRepository {
	return &auditEventRepository{db: db}
}

func (r *auditEventRepository) Create(ctx context.Context, e *domain.AuditEvent) error {
	if e.ID == uuid.Nil {
		e.ID = uuid.New()
	}
	return r.db.WithContext(ctx).Create(e).Error
}
