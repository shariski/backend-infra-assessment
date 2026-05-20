package repository

import (
	"context"
	"errors"

	"auth/internal/domain"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type userRepository struct {
	db *gorm.DB
}

// NewUserRepository returns a Gorm-backed domain.UserRepository.
func NewUserRepository(db *gorm.DB) domain.UserRepository {
	return &userRepository{db: db}
}

func (r *userRepository) Create(ctx context.Context, u *domain.User) error {
	if u.ID == uuid.Nil {
		u.ID = uuid.New()
	}
	return r.db.WithContext(ctx).Create(u).Error
}

func (r *userRepository) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	var u domain.User
	err := r.db.WithContext(ctx).Where("email = ?", email).First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *userRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	var u domain.User
	err := r.db.WithContext(ctx).First(&u, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *userRepository) List(ctx context.Context) ([]domain.User, error) {
	var users []domain.User
	err := r.db.WithContext(ctx).Order("created_at desc").Find(&users).Error
	return users, err
}

func (r *userRepository) UpdateRole(ctx context.Context, id uuid.UUID, role domain.Role) error {
	return r.db.WithContext(ctx).
		Model(&domain.User{}).
		Where("id = ?", id).
		Update("role", role).Error
}
