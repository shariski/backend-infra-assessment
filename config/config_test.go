package config

import (
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret")
	t.Setenv("APP_PORT", "9999")
	t.Setenv("JWT_ACCESS_TTL", "30m")
	t.Setenv("BRUTE_FORCE_MAX_ATTEMPTS", "7")
	t.Setenv("CACHE_TTL", "30s")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.App.Port != "9999" {
		t.Errorf("App.Port = %q, want %q", cfg.App.Port, "9999")
	}
	if cfg.JWT.Secret != "test-secret" {
		t.Errorf("JWT.Secret = %q, want %q", cfg.JWT.Secret, "test-secret")
	}
	if cfg.JWT.AccessTTL != 30*time.Minute {
		t.Errorf("JWT.AccessTTL = %v, want %v", cfg.JWT.AccessTTL, 30*time.Minute)
	}
	if cfg.Security.BruteForceMaxAttempts != 7 {
		t.Errorf("BruteForceMaxAttempts = %d, want 7", cfg.Security.BruteForceMaxAttempts)
	}
	if cfg.Cache.TTL != 30*time.Second {
		t.Errorf("Cache.TTL = %v, want %v", cfg.Cache.TTL, 30*time.Second)
	}
}

func TestLoad_CacheTTLDefault(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Cache.TTL != 60*time.Second {
		t.Errorf("default Cache.TTL = %v, want %v", cfg.Cache.TTL, 60*time.Second)
	}
}

func TestLoad_MissingJWTSecret(t *testing.T) {
	t.Setenv("JWT_SECRET", "")
	if _, err := Load(); err == nil {
		t.Fatal("Load() expected error when JWT_SECRET is missing, got nil")
	}
}
