package service

import (
	"context"
	"errors"
	"time"

	"auth/config"
	"auth/internal/domain"
	"auth/pkg/hash"

	"github.com/google/uuid"
)

// TokenPair is an access token plus its companion refresh token.
type TokenPair struct {
	AccessToken  string
	RefreshToken string
}

// AuthService implements registration, login, token refresh and logout.
type AuthService struct {
	users    domain.UserRepository
	tokens   domain.TokenRepository
	attempts domain.LoginAttemptRepository
	jwt      *JWTService
	security config.SecurityConfig
}

func NewAuthService(
	users domain.UserRepository,
	tokens domain.TokenRepository,
	attempts domain.LoginAttemptRepository,
	jwt *JWTService,
	security config.SecurityConfig,
) *AuthService {
	return &AuthService{
		users:    users,
		tokens:   tokens,
		attempts: attempts,
		jwt:      jwt,
		security: security,
	}
}

// Register creates a new user with the default Viewer role.
func (s *AuthService) Register(ctx context.Context, email, password string) (*domain.User, error) {
	_, err := s.users.FindByEmail(ctx, email)
	if err == nil {
		return nil, domain.ErrEmailTaken
	}
	if !errors.Is(err, domain.ErrUserNotFound) {
		return nil, err
	}

	passwordHash, err := hash.HashPassword(password)
	if err != nil {
		return nil, err
	}
	user := &domain.User{
		Email:        email,
		PasswordHash: passwordHash,
		Role:         domain.RoleViewer,
	}
	if err := s.users.Create(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

// Login verifies credentials, enforces brute-force protection, and issues tokens.
func (s *AuthService) Login(ctx context.Context, email, password, ip string) (*TokenPair, error) {
	since := time.Now().Add(-s.security.BruteForceWindow)
	failed, err := s.attempts.CountRecentFailed(ctx, email, since)
	if err != nil {
		return nil, err
	}
	if failed >= s.security.BruteForceMaxAttempts {
		return nil, domain.ErrAccountLocked
	}

	user, err := s.users.FindByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			s.recordAttempt(ctx, email, ip, false)
			return nil, domain.ErrInvalidCredentials
		}
		return nil, err
	}
	if err := hash.CheckPassword(user.PasswordHash, password); err != nil {
		s.recordAttempt(ctx, email, ip, false)
		return nil, domain.ErrInvalidCredentials
	}

	s.recordAttempt(ctx, email, ip, true)
	return s.issueTokens(ctx, user)
}

// Refresh rotates a refresh token: the old one is revoked and a new pair issued.
func (s *AuthService) Refresh(ctx context.Context, refreshToken string) (*TokenPair, error) {
	rt, err := s.tokens.FindByHash(ctx, hash.SHA256Hex(refreshToken))
	if err != nil {
		return nil, err
	}
	if rt.RevokedAt != nil {
		return nil, domain.ErrTokenRevoked
	}
	if time.Now().After(rt.ExpiresAt) {
		return nil, domain.ErrTokenExpired
	}

	user, err := s.users.FindByID(ctx, rt.UserID)
	if err != nil {
		return nil, err
	}
	if err := s.tokens.Revoke(ctx, rt.ID); err != nil {
		return nil, err
	}
	return s.issueTokens(ctx, user)
}

// Logout revokes a refresh token. It is idempotent: an unknown token is a no-op.
func (s *AuthService) Logout(ctx context.Context, refreshToken string) error {
	rt, err := s.tokens.FindByHash(ctx, hash.SHA256Hex(refreshToken))
	if err != nil {
		if errors.Is(err, domain.ErrTokenNotFound) {
			return nil
		}
		return err
	}
	return s.tokens.Revoke(ctx, rt.ID)
}

// GetUser returns a single user by ID.
func (s *AuthService) GetUser(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	return s.users.FindByID(ctx, id)
}

// ListUsers returns all users (used by the Admin-only example route).
func (s *AuthService) ListUsers(ctx context.Context) ([]domain.User, error) {
	return s.users.List(ctx)
}

func (s *AuthService) issueTokens(ctx context.Context, user *domain.User) (*TokenPair, error) {
	access, err := s.jwt.GenerateAccessToken(user.ID, user.Role)
	if err != nil {
		return nil, err
	}
	refresh, err := s.jwt.GenerateRefreshToken()
	if err != nil {
		return nil, err
	}
	record := &domain.RefreshToken{
		UserID:    user.ID,
		TokenHash: hash.SHA256Hex(refresh),
		ExpiresAt: time.Now().Add(s.jwt.RefreshTTL()),
	}
	if err := s.tokens.Create(ctx, record); err != nil {
		return nil, err
	}
	return &TokenPair{AccessToken: access, RefreshToken: refresh}, nil
}

func (s *AuthService) recordAttempt(ctx context.Context, email, ip string, successful bool) {
	// Best-effort: a failure to record an attempt must not block the login flow.
	_ = s.attempts.Create(ctx, &domain.LoginAttempt{
		Email:      email,
		IPAddress:  ip,
		Successful: successful,
	})
}
