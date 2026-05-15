package middleware

import (
	"log/slog"

	"github.com/gin-gonic/gin"
)

// Audit is a STUB for user activity logging and audit trails (Security Analytics).
// TODO: after the request completes, persist an audit event capturing the actor
// (CurrentUserID), action (method + path), result status, and timestamp to a
// durable audit store.
func Audit(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		// TODO: record audit event here.
		_ = log
	}
}
