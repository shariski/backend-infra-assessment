// Package cache provides a small key/value cache abstraction used by HTTP
// response-caching middleware. Implementations are byte-oriented; callers
// own value encoding (JSON, msgpack, etc.).
//
// The Cache interface intentionally distinguishes "miss" from "error": a
// miss returns (nil, false, nil), an error returns (nil, false, err). The
// response-cache middleware treats both as cache misses so a Redis outage
// degrades gracefully to direct handler invocation rather than failing the
// request.
package cache

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

// Cache is a byte-oriented key/value store with per-entry TTLs.
//
// Implementations MUST be safe for concurrent use.
type Cache interface {
	// Get returns the value for key. The bool reports whether the key was
	// present. A returned error means the backend was unreachable; callers
	// may treat this as a miss to preserve availability.
	Get(ctx context.Context, key string) ([]byte, bool, error)

	// Set stores value under key with the given TTL. A zero TTL stores the
	// value without expiry (avoid in caching paths).
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
}

// RedisCache is a Cache backed by a Redis client. Errors are logged at WARN
// and surfaced to the caller, which is expected to treat them as misses.
type RedisCache struct {
	rdb *redis.Client
	log *slog.Logger
}

func NewRedis(rdb *redis.Client, log *slog.Logger) *RedisCache {
	return &RedisCache{rdb: rdb, log: log}
}

func (c *RedisCache) Get(ctx context.Context, key string) ([]byte, bool, error) {
	val, err := c.rdb.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, false, nil
	}
	if err != nil {
		c.log.Warn("response cache get failed", "key", key, "error", err)
		return nil, false, err
	}
	return val, true, nil
}

func (c *RedisCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if err := c.rdb.Set(ctx, key, value, ttl).Err(); err != nil {
		c.log.Warn("response cache set failed", "key", key, "error", err)
		return err
	}
	return nil
}
