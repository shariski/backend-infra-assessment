package bootstrap

import (
	"context"
	"errors"
	"testing"

	"auth/internal/domain"
	"auth/pkg/hash"

	"github.com/google/uuid"
)

// mockStore records the calls EnsureAdmin makes against the user store.
type mockStore struct {
	findByEmail func(ctx context.Context, email string) (*domain.User, error)

	createErr   error
	created     *domain.User
	createCalls int

	updateErr   error
	updatedID   uuid.UUID
	updatedRole domain.Role
	updateCalls int
}

func (m *mockStore) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	return m.findByEmail(ctx, email)
}

func (m *mockStore) Create(ctx context.Context, u *domain.User) error {
	m.createCalls++
	if m.createErr != nil {
		return m.createErr
	}
	m.created = u
	return nil
}

func (m *mockStore) UpdateRole(ctx context.Context, id uuid.UUID, role domain.Role) error {
	m.updateCalls++
	if m.updateErr != nil {
		return m.updateErr
	}
	m.updatedID = id
	m.updatedRole = role
	return nil
}

func TestEnsureAdmin_SkippedWhenUnconfigured(t *testing.T) {
	cases := map[string]struct{ email, password string }{
		"both empty":     {"", ""},
		"empty email":    {"", "secretpw123"},
		"empty password": {"admin@example.com", ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			store := &mockStore{
				findByEmail: func(context.Context, string) (*domain.User, error) {
					t.Fatal("FindByEmail should not be called when unconfigured")
					return nil, nil
				},
			}
			out, err := EnsureAdmin(context.Background(), store, tc.email, tc.password)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if out != OutcomeSkipped {
				t.Errorf("outcome = %q, want %q", out, OutcomeSkipped)
			}
		})
	}
}

func TestEnsureAdmin_CreatesAdminWhenAbsent(t *testing.T) {
	store := &mockStore{
		findByEmail: func(context.Context, string) (*domain.User, error) {
			return nil, domain.ErrUserNotFound
		},
	}

	out, err := EnsureAdmin(context.Background(), store, "admin@example.com", "secretpw123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != OutcomeCreated {
		t.Errorf("outcome = %q, want %q", out, OutcomeCreated)
	}
	if store.createCalls != 1 {
		t.Fatalf("Create called %d times, want 1", store.createCalls)
	}
	if store.created.Email != "admin@example.com" {
		t.Errorf("created email = %q, want admin@example.com", store.created.Email)
	}
	if store.created.Role != domain.RoleAdmin {
		t.Errorf("created role = %q, want %q", store.created.Role, domain.RoleAdmin)
	}
	if err := hash.CheckPassword(store.created.PasswordHash, "secretpw123"); err != nil {
		t.Errorf("stored hash does not verify against the password: %v", err)
	}
	if store.updateCalls != 0 {
		t.Errorf("UpdateRole called %d times, want 0", store.updateCalls)
	}
}

func TestEnsureAdmin_PromotesExistingNonAdmin(t *testing.T) {
	id := uuid.New()
	store := &mockStore{
		findByEmail: func(context.Context, string) (*domain.User, error) {
			return &domain.User{ID: id, Email: "admin@example.com", Role: domain.RoleViewer}, nil
		},
	}

	out, err := EnsureAdmin(context.Background(), store, "admin@example.com", "secretpw123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != OutcomePromoted {
		t.Errorf("outcome = %q, want %q", out, OutcomePromoted)
	}
	if store.updateCalls != 1 {
		t.Fatalf("UpdateRole called %d times, want 1", store.updateCalls)
	}
	if store.updatedID != id {
		t.Errorf("promoted id = %v, want %v", store.updatedID, id)
	}
	if store.updatedRole != domain.RoleAdmin {
		t.Errorf("promoted role = %q, want %q", store.updatedRole, domain.RoleAdmin)
	}
	if store.createCalls != 0 {
		t.Errorf("Create called %d times, want 0", store.createCalls)
	}
}

func TestEnsureAdmin_NoopWhenAlreadyAdmin(t *testing.T) {
	store := &mockStore{
		findByEmail: func(context.Context, string) (*domain.User, error) {
			return &domain.User{ID: uuid.New(), Email: "admin@example.com", Role: domain.RoleAdmin}, nil
		},
	}

	out, err := EnsureAdmin(context.Background(), store, "admin@example.com", "secretpw123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != OutcomeAlreadyAdmin {
		t.Errorf("outcome = %q, want %q", out, OutcomeAlreadyAdmin)
	}
	if store.createCalls != 0 || store.updateCalls != 0 {
		t.Errorf("create=%d update=%d, want both 0", store.createCalls, store.updateCalls)
	}
}

func TestEnsureAdmin_PropagatesLookupError(t *testing.T) {
	sentinel := errors.New("db down")
	store := &mockStore{
		findByEmail: func(context.Context, string) (*domain.User, error) {
			return nil, sentinel
		},
	}

	_, err := EnsureAdmin(context.Background(), store, "admin@example.com", "secretpw123")
	if !errors.Is(err, sentinel) {
		t.Fatalf("error = %v, want it to wrap %v", err, sentinel)
	}
}
