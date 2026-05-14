package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"auth/config"
	"auth/internal/domain"
	"auth/pkg/hash"

	"github.com/google/uuid"
)

type mockUserRepo struct {
	CreateFn      func(ctx context.Context, u *domain.User) error
	FindByEmailFn func(ctx context.Context, email string) (*domain.User, error)
	FindByIDFn    func(ctx context.Context, id uuid.UUID) (*domain.User, error)
	ListFn        func(ctx context.Context) ([]domain.User, error)
}

func (m *mockUserRepo) Create(ctx context.Context, u *domain.User) error {
	return m.CreateFn(ctx, u)
}
func (m *mockUserRepo) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	return m.FindByEmailFn(ctx, email)
}
func (m *mockUserRepo) FindByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	return m.FindByIDFn(ctx, id)
}
func (m *mockUserRepo) List(ctx context.Context) ([]domain.User, error) {
	return m.ListFn(ctx)
}

type mockTokenRepo struct {
	CreateFn     func(ctx context.Context, t *domain.RefreshToken) error
	FindByHashFn func(ctx context.Context, hash string) (*domain.RefreshToken, error)
	RevokeFn     func(ctx context.Context, id uuid.UUID) error
}

func (m *mockTokenRepo) Create(ctx context.Context, t *domain.RefreshToken) error {
	return m.CreateFn(ctx, t)
}
func (m *mockTokenRepo) FindByHash(ctx context.Context, h string) (*domain.RefreshToken, error) {
	return m.FindByHashFn(ctx, h)
}
func (m *mockTokenRepo) Revoke(ctx context.Context, id uuid.UUID) error {
	return m.RevokeFn(ctx, id)
}

type mockAttemptRepo struct {
	CreateFn            func(ctx context.Context, a *domain.LoginAttempt) error
	CountRecentFailedFn func(ctx context.Context, email string, since time.Time) (int, error)
}

func (m *mockAttemptRepo) Create(ctx context.Context, a *domain.LoginAttempt) error {
	return m.CreateFn(ctx, a)
}
func (m *mockAttemptRepo) CountRecentFailed(ctx context.Context, email string, since time.Time) (int, error) {
	return m.CountRecentFailedFn(ctx, email, since)
}

func newTestAuthService(users *mockUserRepo, tokens *mockTokenRepo, attempts *mockAttemptRepo) *AuthService {
	jwtSvc := NewJWTService(config.JWTConfig{
		Secret:     "test-secret",
		AccessTTL:  time.Minute,
		RefreshTTL: time.Hour,
	})
	return NewAuthService(users, tokens, attempts, jwtSvc, config.SecurityConfig{
		BruteForceMaxAttempts: 3,
		BruteForceWindow:      15 * time.Minute,
	})
}

func TestAuthService_Register_Success(t *testing.T) {
	var created *domain.User
	users := &mockUserRepo{
		FindByEmailFn: func(ctx context.Context, email string) (*domain.User, error) {
			return nil, domain.ErrUserNotFound
		},
		CreateFn: func(ctx context.Context, u *domain.User) error {
			u.ID = uuid.New()
			created = u
			return nil
		},
	}
	svc := newTestAuthService(users, &mockTokenRepo{}, &mockAttemptRepo{})

	user, err := svc.Register(context.Background(), "new@example.com", "password123")
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if user.Email != "new@example.com" {
		t.Errorf("user.Email = %q, want %q", user.Email, "new@example.com")
	}
	if user.Role != domain.RoleViewer {
		t.Errorf("user.Role = %q, want %q", user.Role, domain.RoleViewer)
	}
	if created.PasswordHash == "password123" {
		t.Error("password stored in plaintext")
	}
}

func TestAuthService_Register_EmailTaken(t *testing.T) {
	users := &mockUserRepo{
		FindByEmailFn: func(ctx context.Context, email string) (*domain.User, error) {
			return &domain.User{ID: uuid.New(), Email: email}, nil
		},
	}
	svc := newTestAuthService(users, &mockTokenRepo{}, &mockAttemptRepo{})
	if _, err := svc.Register(context.Background(), "taken@example.com", "password123"); !errors.Is(err, domain.ErrEmailTaken) {
		t.Fatalf("Register() error = %v, want ErrEmailTaken", err)
	}
}

func TestAuthService_Login_Success(t *testing.T) {
	ph, _ := hash.HashPassword("correct-password")
	userID := uuid.New()
	users := &mockUserRepo{
		FindByEmailFn: func(ctx context.Context, email string) (*domain.User, error) {
			return &domain.User{ID: userID, Email: email, PasswordHash: ph, Role: domain.RoleAnalyst}, nil
		},
	}
	tokens := &mockTokenRepo{
		CreateFn: func(ctx context.Context, tk *domain.RefreshToken) error { return nil },
	}
	attempts := &mockAttemptRepo{
		CountRecentFailedFn: func(ctx context.Context, email string, since time.Time) (int, error) { return 0, nil },
		CreateFn:            func(ctx context.Context, a *domain.LoginAttempt) error { return nil },
	}
	svc := newTestAuthService(users, tokens, attempts)

	pair, err := svc.Login(context.Background(), "user@example.com", "correct-password", "127.0.0.1")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if pair.AccessToken == "" || pair.RefreshToken == "" {
		t.Error("Login() returned empty tokens")
	}
}

func TestAuthService_Login_WrongPassword(t *testing.T) {
	ph, _ := hash.HashPassword("correct-password")
	var recorded *domain.LoginAttempt
	users := &mockUserRepo{
		FindByEmailFn: func(ctx context.Context, email string) (*domain.User, error) {
			return &domain.User{ID: uuid.New(), Email: email, PasswordHash: ph, Role: domain.RoleViewer}, nil
		},
	}
	attempts := &mockAttemptRepo{
		CountRecentFailedFn: func(ctx context.Context, email string, since time.Time) (int, error) { return 0, nil },
		CreateFn: func(ctx context.Context, a *domain.LoginAttempt) error {
			recorded = a
			return nil
		},
	}
	svc := newTestAuthService(users, &mockTokenRepo{}, attempts)

	if _, err := svc.Login(context.Background(), "user@example.com", "wrong-password", "127.0.0.1"); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("Login() error = %v, want ErrInvalidCredentials", err)
	}
	if recorded == nil || recorded.Successful {
		t.Error("Login() did not record a failed attempt")
	}
}

func TestAuthService_Login_AccountLocked(t *testing.T) {
	attempts := &mockAttemptRepo{
		CountRecentFailedFn: func(ctx context.Context, email string, since time.Time) (int, error) {
			return 3, nil // == BruteForceMaxAttempts
		},
	}
	svc := newTestAuthService(&mockUserRepo{}, &mockTokenRepo{}, attempts)
	if _, err := svc.Login(context.Background(), "user@example.com", "whatever", "127.0.0.1"); !errors.Is(err, domain.ErrAccountLocked) {
		t.Fatalf("Login() error = %v, want ErrAccountLocked", err)
	}
}

func TestAuthService_Refresh_Success(t *testing.T) {
	userID := uuid.New()
	tokenID := uuid.New()
	var revoked uuid.UUID
	refreshPlain := "some-refresh-token"
	tokens := &mockTokenRepo{
		FindByHashFn: func(ctx context.Context, h string) (*domain.RefreshToken, error) {
			if h != hash.SHA256Hex(refreshPlain) {
				return nil, domain.ErrTokenNotFound
			}
			return &domain.RefreshToken{
				ID: tokenID, UserID: userID, TokenHash: h,
				ExpiresAt: time.Now().Add(time.Hour),
			}, nil
		},
		RevokeFn: func(ctx context.Context, id uuid.UUID) error { revoked = id; return nil },
		CreateFn: func(ctx context.Context, tk *domain.RefreshToken) error { return nil },
	}
	users := &mockUserRepo{
		FindByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.User, error) {
			return &domain.User{ID: id, Email: "user@example.com", Role: domain.RoleViewer}, nil
		},
	}
	svc := newTestAuthService(users, tokens, &mockAttemptRepo{})

	pair, err := svc.Refresh(context.Background(), refreshPlain)
	if err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	if revoked != tokenID {
		t.Error("Refresh() did not revoke the old token")
	}
	if pair.AccessToken == "" || pair.RefreshToken == "" {
		t.Error("Refresh() returned empty tokens")
	}
}

func TestAuthService_Refresh_Expired(t *testing.T) {
	tokens := &mockTokenRepo{
		FindByHashFn: func(ctx context.Context, h string) (*domain.RefreshToken, error) {
			return &domain.RefreshToken{
				ID: uuid.New(), UserID: uuid.New(), TokenHash: h,
				ExpiresAt: time.Now().Add(-time.Hour),
			}, nil
		},
	}
	svc := newTestAuthService(&mockUserRepo{}, tokens, &mockAttemptRepo{})
	if _, err := svc.Refresh(context.Background(), "expired"); !errors.Is(err, domain.ErrTokenExpired) {
		t.Fatalf("Refresh() error = %v, want ErrTokenExpired", err)
	}
}

func TestAuthService_Logout(t *testing.T) {
	tokenID := uuid.New()
	var revoked uuid.UUID
	tokens := &mockTokenRepo{
		FindByHashFn: func(ctx context.Context, h string) (*domain.RefreshToken, error) {
			return &domain.RefreshToken{ID: tokenID, ExpiresAt: time.Now().Add(time.Hour)}, nil
		},
		RevokeFn: func(ctx context.Context, id uuid.UUID) error { revoked = id; return nil },
	}
	svc := newTestAuthService(&mockUserRepo{}, tokens, &mockAttemptRepo{})
	if err := svc.Logout(context.Background(), "token"); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
	if revoked != tokenID {
		t.Error("Logout() did not revoke the token")
	}
}
