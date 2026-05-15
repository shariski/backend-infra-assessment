package ratelimit

import (
	"context"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

// tokenBucketScript implements an atomic token-bucket check-and-decrement.
//
// State is a small Redis hash:
//   t  = current token count (float)
//   ts = last refill timestamp in unix milliseconds
//
// Each call refills tokens proportional to elapsed time (capped at burst),
// then either consumes one token (return 1) or denies (return 0). The key
// gets a TTL of "time to refill a full bucket" so idle keys evict cleanly.
//
// KEYS[1] = bucket key
// ARGV[1] = rate (tokens/sec, float)
// ARGV[2] = burst (max tokens, int)
// ARGV[3] = now (unix ms, int)
var tokenBucketScript = redis.NewScript(`
local key   = KEYS[1]
local rate  = tonumber(ARGV[1])
local burst = tonumber(ARGV[2])
local now   = tonumber(ARGV[3])

local tokens = tonumber(redis.call('HGET', key, 't'))
local last   = tonumber(redis.call('HGET', key, 'ts'))
if tokens == nil then
  tokens = burst
  last   = now
end

local elapsed = math.max(0, now - last) / 1000.0
tokens = math.min(burst, tokens + elapsed * rate)

local allowed = 0
if tokens >= 1 then
  tokens = tokens - 1
  allowed = 1
end

redis.call('HSET', key, 't', tokens, 'ts', now)
-- expire after the time it would take to refill the full bucket from empty,
-- plus a small slack; idle keys evict on their own.
local ttl_ms = math.ceil((burst / rate) * 1000) + 1000
redis.call('PEXPIRE', key, ttl_ms)
return allowed
`)

// RedisLimiter is a Redis-backed token-bucket Limiter.
//
// On Redis errors it fails OPEN (returns true) and logs at WARN level. This is
// a deliberate availability bias: a Redis outage should not break /auth/login
// or /admin/*. Brute-force protection on login is also enforced at the DB
// layer (login_attempt_repository), so rate-limiting here is defense-in-depth.
type RedisLimiter struct {
	rdb *redis.Client
	log *slog.Logger
}

func NewRedis(rdb *redis.Client, log *slog.Logger) *RedisLimiter {
	return &RedisLimiter{rdb: rdb, log: log}
}

func (l *RedisLimiter) Allow(ctx context.Context, key string, ratePerSec float64, burst int) (bool, error) {
	if ratePerSec <= 0 || burst <= 0 {
		return false, nil
	}
	now := time.Now().UnixMilli()
	res, err := tokenBucketScript.Run(ctx, l.rdb, []string{key}, ratePerSec, burst, now).Int()
	if err != nil {
		l.log.Warn("rate limiter unavailable, failing open", "key", key, "error", err)
		return true, err
	}
	return res == 1, nil
}
