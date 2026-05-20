package middleware

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"auth/pkg/cache"
)

const (
	cacheKeyPrefix  = "cache:GET:"
	cacheHeader     = "X-Cache"
	cacheHeaderHit  = "HIT"
	cacheHeaderMiss = "MISS"
)

// cacheEntry is the serialized envelope stored in Redis. We persist enough
// to faithfully replay the response: status, Content-Type, and body bytes.
// Short JSON tags keep the stored payload small.
type cacheEntry struct {
	Status      int    `json:"s"`
	ContentType string `json:"ct"`
	Body        []byte `json:"b"`
}

// ResponseCache caches successful (200) GET responses keyed by the route's
// path template and the authenticated user ID. MUST be installed AFTER
// middleware.Auth so CurrentUserID is populated; without a user it bypasses
// the cache to avoid collapsing every anonymous response into one entry.
//
// Cache backend failures degrade to direct handler invocation — Redis must
// never break a request. Non-200 responses are not cached.
func ResponseCache(c cache.Cache, ttl time.Duration, log *slog.Logger) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		uid := CurrentUserID(ctx)
		if uid == uuid.Nil {
			ctx.Next()
			return
		}

		key := cacheKeyPrefix + ctx.FullPath() + ":" + uid.String()

		if raw, hit, err := c.Get(ctx.Request.Context(), key); err == nil && hit {
			var entry cacheEntry
			if err := json.Unmarshal(raw, &entry); err == nil {
				writeCachedResponse(ctx, entry)
				return
			}
			log.Warn("response cache decode failed; treating as miss", "key", key, "error", err)
		}

		cw := &captureWriter{ResponseWriter: ctx.Writer, body: &bytes.Buffer{}}
		ctx.Writer = cw
		ctx.Writer.Header().Set(cacheHeader, cacheHeaderMiss)

		ctx.Next()

		if ctx.IsAborted() || cw.Status() != http.StatusOK {
			return
		}
		entry := cacheEntry{
			Status:      cw.Status(),
			ContentType: cw.Header().Get("Content-Type"),
			Body:        cw.body.Bytes(),
		}
		raw, err := json.Marshal(entry)
		if err != nil {
			log.Warn("response cache encode failed", "key", key, "error", err)
			return
		}
		_ = c.Set(ctx.Request.Context(), key, raw, ttl)
	}
}

func writeCachedResponse(ctx *gin.Context, entry cacheEntry) {
	if entry.ContentType != "" {
		ctx.Writer.Header().Set("Content-Type", entry.ContentType)
	}
	ctx.Writer.Header().Set(cacheHeader, cacheHeaderHit)
	ctx.Writer.WriteHeader(entry.Status)
	_, _ = ctx.Writer.Write(entry.Body)
	ctx.Abort()
}

// captureWriter tees response bytes into an internal buffer so the cache
// middleware can serialize the body after the handler runs, without changing
// what the client sees.
type captureWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w *captureWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

func (w *captureWriter) WriteString(s string) (int, error) {
	w.body.WriteString(s)
	return w.ResponseWriter.WriteString(s)
}
