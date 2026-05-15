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
	}

	if cfg.JWT.Secret == "" {
		return nil, errors.New("JWT_SECRET is required")
	}
	return cfg, nil
}
