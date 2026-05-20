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
	t.Setenv("OLLAMA_URL", "http://ollama.example:11434")
	t.Setenv("LLM_MAX_ATTEMPTS", "3")

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
	if cfg.LLM.BaseURL != "http://ollama.example:11434" {
		t.Errorf("LLM.BaseURL = %q, want override", cfg.LLM.BaseURL)
	}
	if cfg.LLM.MaxAttempts != 3 {
		t.Errorf("LLM.MaxAttempts = %d, want 3", cfg.LLM.MaxAttempts)
	}
}

func TestLoad_LLMDefaults(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.LLM.Model != "llama3.2:1b" {
		t.Errorf("default LLM.Model = %q, want %q", cfg.LLM.Model, "llama3.2:1b")
	}
	if cfg.LLM.Timeout != 60*time.Second {
		t.Errorf("default LLM.Timeout = %v, want %v", cfg.LLM.Timeout, 60*time.Second)
	}
	if cfg.LLM.SummaryTTL != 5*time.Minute {
		t.Errorf("default LLM.SummaryTTL = %v, want %v", cfg.LLM.SummaryTTL, 5*time.Minute)
	}
	if cfg.LLM.MaxAttempts != 20 {
		t.Errorf("default LLM.MaxAttempts = %d, want 20", cfg.LLM.MaxAttempts)
	}
	if cfg.LLM.MaxEvents != 20 {
		t.Errorf("default LLM.MaxEvents = %d, want 20", cfg.LLM.MaxEvents)
	}
	if cfg.LLM.BaseURL != "" {
		t.Errorf("default LLM.BaseURL = %q, want empty (disabled)", cfg.LLM.BaseURL)
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
