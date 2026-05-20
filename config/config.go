package config

import (
	"errors"
	"fmt"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	App      AppConfig
	DB       DBConfig
	Redis    RedisConfig
	JWT      JWTConfig
	Security SecurityConfig
	Log      LogConfig
	Cache    CacheConfig
	LLM      LLMConfig
}

type AppConfig struct {
	Port string
	Env  string
}

type DBConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	Name     string
	SSLMode  string
}

type RedisConfig struct {
	Host     string
	Port     string
	Password string
	DB       int
}

// Addr returns the host:port string accepted by redis.Options.
func (r RedisConfig) Addr() string {
	return fmt.Sprintf("%s:%s", r.Host, r.Port)
}

type JWTConfig struct {
	Secret     string
	AccessTTL  time.Duration
	RefreshTTL time.Duration
}

type SecurityConfig struct {
	BruteForceMaxAttempts int
	BruteForceWindow      time.Duration
}

type LogConfig struct {
	Level string
}

// CacheConfig controls HTTP response caching. TTL applies to every cached
// entry; the middleware does not yet support per-route overrides.
type CacheConfig struct {
	TTL time.Duration
}

// LLMConfig configures the local LLM (Ollama) used for threat summaries.
// BaseURL empty => feature disabled (the endpoint returns 503); OLLAMA_URL,
// CF_ACCESS_CLIENT_ID and CF_ACCESS_CLIENT_SECRET intentionally have no
// defaults. MaxAttempts/MaxEvents cap how many recent login attempts / audit
// events are fed to the model.
type LLMConfig struct {
	BaseURL              string
	Model                string
	Timeout              time.Duration
	CFAccessClientID     string
	CFAccessClientSecret string
	SummaryTTL           time.Duration
	MaxAttempts          int
	MaxEvents            int
}

// DSN builds a PostgreSQL connection string for Gorm.
func (d DBConfig) DSN() string {
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		d.Host, d.Port, d.User, d.Password, d.Name, d.SSLMode)
}

// Load reads configuration from environment variables, falling back to a
// local .env file when present. JWT_SECRET is required.
func Load() (*Config, error) {
	v := viper.New()
	v.SetConfigName(".env")
	v.SetConfigType("env")
	v.AddConfigPath(".")
	v.AutomaticEnv()

	v.SetDefault("APP_PORT", "8080")
	v.SetDefault("APP_ENV", "development")
	v.SetDefault("DB_HOST", "localhost")
	v.SetDefault("DB_PORT", "5432")
	v.SetDefault("DB_USER", "postgres")
	v.SetDefault("DB_PASSWORD", "postgres")
	v.SetDefault("DB_NAME", "auth")
	v.SetDefault("DB_SSLMODE", "disable")
	v.SetDefault("REDIS_HOST", "localhost")
	v.SetDefault("REDIS_PORT", "6379")
	v.SetDefault("REDIS_PASSWORD", "")
	v.SetDefault("REDIS_DB", 0)
	v.SetDefault("JWT_ACCESS_TTL", "15m")
	v.SetDefault("JWT_REFRESH_TTL", "168h")
	v.SetDefault("BRUTE_FORCE_MAX_ATTEMPTS", 5)
	v.SetDefault("BRUTE_FORCE_WINDOW", "15m")
	v.SetDefault("LOG_LEVEL", "info")
	v.SetDefault("CACHE_TTL", "60s")
	v.SetDefault("OLLAMA_MODEL", "llama3.2:1b")
	v.SetDefault("OLLAMA_TIMEOUT", "30s")
	v.SetDefault("LLM_SUMMARY_TTL", "5m")
	v.SetDefault("LLM_MAX_ATTEMPTS", 20)
	v.SetDefault("LLM_MAX_EVENTS", 20)

	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) {
			return nil, err
		}
	}

	cfg := &Config{
		App: AppConfig{
			Port: v.GetString("APP_PORT"),
			Env:  v.GetString("APP_ENV"),
		},
		DB: DBConfig{
			Host:     v.GetString("DB_HOST"),
			Port:     v.GetString("DB_PORT"),
			User:     v.GetString("DB_USER"),
			Password: v.GetString("DB_PASSWORD"),
			Name:     v.GetString("DB_NAME"),
			SSLMode:  v.GetString("DB_SSLMODE"),
		},
		Redis: RedisConfig{
			Host:     v.GetString("REDIS_HOST"),
			Port:     v.GetString("REDIS_PORT"),
			Password: v.GetString("REDIS_PASSWORD"),
			DB:       v.GetInt("REDIS_DB"),
		},
		JWT: JWTConfig{
			Secret:     v.GetString("JWT_SECRET"),
			AccessTTL:  v.GetDuration("JWT_ACCESS_TTL"),
			RefreshTTL: v.GetDuration("JWT_REFRESH_TTL"),
		},
		Security: SecurityConfig{
			BruteForceMaxAttempts: v.GetInt("BRUTE_FORCE_MAX_ATTEMPTS"),
			BruteForceWindow:      v.GetDuration("BRUTE_FORCE_WINDOW"),
		},
		Log: LogConfig{
			Level: v.GetString("LOG_LEVEL"),
		},
		Cache: CacheConfig{
			TTL: v.GetDuration("CACHE_TTL"),
		},
		LLM: LLMConfig{
			BaseURL:              v.GetString("OLLAMA_URL"),
			Model:                v.GetString("OLLAMA_MODEL"),
			Timeout:              v.GetDuration("OLLAMA_TIMEOUT"),
			CFAccessClientID:     v.GetString("CF_ACCESS_CLIENT_ID"),
			CFAccessClientSecret: v.GetString("CF_ACCESS_CLIENT_SECRET"),
			SummaryTTL:           v.GetDuration("LLM_SUMMARY_TTL"),
			MaxAttempts:          v.GetInt("LLM_MAX_ATTEMPTS"),
			MaxEvents:            v.GetInt("LLM_MAX_EVENTS"),
		},
	}

	if cfg.JWT.Secret == "" {
		return nil, errors.New("JWT_SECRET is required")
	}
	return cfg, nil
}
