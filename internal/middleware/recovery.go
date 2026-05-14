package middleware

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Recovery converts panics into a 500 JSON response and logs them.
func Recovery(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				log.Error("panic recovered",
					"error", r,
					"path", c.Request.URL.Path,
					"method", c.Request.Method,
				)
				abortError(c, http.StatusInternalServerError, "INTERNAL", "internal server error")
			}
		}()
		c.Next()
	}
}
