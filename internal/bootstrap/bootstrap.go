// Package bootstrap performs one-time startup setup that can't be expressed as a
// SQL migration — currently, ensuring an administrator account exists so the
// Admin-only routes can be exercised on a fresh deployment.
package bootstrap

import (
	"context"
	"errors"
	"fmt"

	"auth/internal/domain"
	"auth/pkg/hash"

	"github.com/google/uuid"
)

// AdminStore is the subset of the user repository EnsureAdmin needs.
type AdminStore interface {
	FindByEmail(ctx context.Context, email string) (*domain.User, error)
	Create(ctx context.Context, u *domain.User) error
	UpdateRole(ctx context.Context, id uuid.UUID, role domain.Role) error
}

// Outcome reports what EnsureAdmin did, so the caller can log it.
type Outcome string

const (
	OutcomeSkipped      Outcome = "skipped"       // not configured
	OutcomeCreated      Outcome = "created"       // admin account created
	OutcomePromoted     Outcome = "promoted"      // existing user promoted to admin
	OutcomeAlreadyAdmin Outcome = "already_admin" // nothing to do
)

// EnsureAdmin makes sure a user with the given email exists and has the Admin
// role. It is idempotent: safe to run on every startup. When email or password
// is empty the bootstrap is disabled and the call is a no-op.
func EnsureAdmin(ctx context.Context, store AdminStore, email, password string) (Outcome, error) {
	if email == "" || password == "" {
		return OutcomeSkipped, nil
	}

	existing, err := store.FindByEmail(ctx, email)
	switch {
	case err == nil:
		if existing.Role == domain.RoleAdmin {
			return OutcomeAlreadyAdmin, nil
		}
		if err := store.UpdateRole(ctx, existing.ID, domain.RoleAdmin); err != nil {
			return "", fmt.Errorf("promote %q to admin: %w", email, err)
		}
		return OutcomePromoted, nil

	case errors.Is(err, domain.ErrUserNotFound):
		passwordHash, err := hash.HashPassword(password)
		if err != nil {
			return "", fmt.Errorf("hash bootstrap admin password: %w", err)
		}
		if err := store.Create(ctx, &domain.User{
			Email:        email,
			PasswordHash: passwordHash,
			Role:         domain.RoleAdmin,
		}); err != nil {
			return "", fmt.Errorf("create bootstrap admin %q: %w", email, err)
		}
		return OutcomeCreated, nil

	default:
		return "", fmt.Errorf("look up bootstrap admin %q: %w", email, err)
	}
}
