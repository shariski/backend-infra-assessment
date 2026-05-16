package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	HeaderRequestID = "X-Request-Id"
	ctxRequestID    = "ctx.request.id"
)

// RequestID returns the correlation id set by RequestLogger, or "" if absent.
func RequestID(c *gin.Context) string {
	if v, ok := c.Get(ctxRequestID); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// RequestLogger emits one structured slog entry per HTTP request. Must be
// installed before Auth so unauthenticated traffic is also recorded.
func RequestLogger(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		reqID := c.GetHeader(HeaderRequestID)
		if reqID == "" {
			reqID = uuid.NewString()
		}
		c.Set(ctxRequestID, reqID)
		c.Header(HeaderRequestID, reqID)

		start := time.Now()
		c.Next()

		status := c.Writer.Status()

		var userID string
		if uid := CurrentUserID(c); uid != uuid.Nil {
			userID = uid.String()
		}

		attrs := []slog.Attr{
			slog.String("request_id", reqID),
			slog.String("method", c.Request.Method),
			slog.String("path", c.Request.URL.Path),
			slog.String("query", c.Request.URL.RawQuery),
			slog.Int("status", status),
			slog.Int("bytes_out", c.Writer.Size()),
			slog.Duration("latency", time.Since(start)),
			slog.String("client_ip", c.ClientIP()),
			slog.String("user_agent", c.Request.UserAgent()),
			slog.String("user_id", userID),
		}
		if status >= 500 && len(c.Errors) > 0 {
			attrs = append(attrs, slog.Any("errors", c.Errors.Errors()))
		}

		log.LogAttrs(c.Request.Context(), levelForStatus(status), "http_request", attrs...)
	}
}

func levelForStatus(status int) slog.Level {
	switch {
	case status >= 500:
		return slog.LevelError
	case status >= 400:
		return slog.LevelWarn
	default:
		return slog.LevelInfo
	}
}
