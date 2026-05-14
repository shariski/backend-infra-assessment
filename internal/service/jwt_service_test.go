package service

import (
	"testing"
	"time"

	"auth/config"
	"auth/internal/domain"

	"github.com/google/uuid"
)

func newTestJWTService(accessTTL time.Duration) *JWTService {
	return NewJWTService(config.JWTConfig{
		Secret:     "test-secret",
		AccessTTL:  accessTTL,
		RefreshTTL: time.Hour,
	})
}

func TestJWTService_AccessTokenRoundTrip(t *testing.T) {
	svc := newTestJWTService(time.Minute)
	id := uuid.New()

	token, err := svc.GenerateAccessToken(id, domain.RoleAdmin)
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}
	claims, err := svc.ParseAccessToken(token)
	if err != nil {
		t.Fatalf("ParseAccessToken() error = %v", err)
	}
	if claims.UserID != id.String() {
		t.Errorf("claims.UserID = %q, want %q", claims.UserID, id.String())
	}
	if claims.Role != string(domain.RoleAdmin) {
		t.Errorf("claims.Role = %q, want %q", claims.Role, domain.RoleAdmin)
	}
}

func TestJWTService_ParseExpiredToken(t *testing.T) {
	svc := newTestJWTService(-time.Minute) // already expired
	token, err := svc.GenerateAccessToken(uuid.New(), domain.RoleViewer)
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}
	if _, err := svc.ParseAccessToken(token); err == nil {
		t.Error("ParseAccessToken() expected error for expired token, got nil")
	}
}

func TestJWTService_GenerateRefreshToken(t *testing.T) {
	svc := newTestJWTService(time.Minute)
	a, err := svc.GenerateRefreshToken()
	if err != nil {
		t.Fatalf("GenerateRefreshToken() error = %v", err)
	}
	b, _ := svc.GenerateRefreshToken()
	if a == "" || a == b {
		t.Errorf("GenerateRefreshToken() not random: a=%q b=%q", a, b)
	}
}
