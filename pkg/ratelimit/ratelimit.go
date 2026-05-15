// Package ratelimit provides a shared rate-limiting primitive backed by Redis.
//
// Limiter is intentionally minimal: Allow(ctx, key, rate, burst) returns
// whether the given key is allowed one token right now. Callers own key naming
// (e.g. "rl:login:1.2.3.4", "rl:role:admin:<uid>") and quota selection.
package ratelimit

import "context"

// Limiter decides whether one unit of work for `key` is allowed under a
// token-bucket configured by ratePerSec (refill rate) and burst (max tokens).
//
// Implementations MUST be safe for concurrent use.
type Limiter interface {
	Allow(ctx context.Context, key string, ratePerSec float64, burst int) (bool, error)
}
