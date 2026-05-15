package middleware

import (
	"log/slog"

	"github.com/gin-gonic/gin"
)

// RequestLogger is a STUB for compliance request/response logging (Security
// Analytics). TODO: capture the request method, path, status, latency, client
// IP, and (where compliance requires) request/response bodies, then emit a
// structured log entry to a dedicated compliance sink.
func RequestLogger(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		// TODO: emit a structured compliance log entry here.
		_ = log
	}
}
