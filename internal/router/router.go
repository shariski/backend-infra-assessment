package router

import (
	"net/http"

	"auth/config"
	"auth/internal/domain"
	"auth/internal/handler"
	"auth/internal/middleware"
	"auth/internal/service"
	"auth/pkg/ratelimit"

	"github.com/gin-gonic/gin"
	"log/slog"
)

// New builds the Gin engine with all routes and middleware wired up.
func New(
	cfg *config.Config,
	log *slog.Logger,
	authHandler *handler.AuthHandler,
	jwtSvc *service.JWTService,
	limiter ratelimit.Limiter,
) *gin.Engine {
	if cfg.App.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(middleware.Recovery(log))
	r.Use(middleware.RequestLogger(log)) // STUB
	r.Use(middleware.Audit(log))         // STUB

	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	auth := r.Group("/auth")
	{
		auth.POST("/register", authHandler.Register)
		auth.POST("/login",
			middleware.LoginRateLimit(limiter, cfg.Security.BruteForceMaxAttempts, cfg.Security.BruteForceMaxAttempts),
			authHandler.Login,
		)
		auth.POST("/refresh", authHandler.Refresh)
		auth.POST("/logout", middleware.Auth(jwtSvc), authHandler.Logout)
		auth.GET("/me", middleware.Auth(jwtSvc), authHandler.Me)
	}

	admin := r.Group("/admin")
	admin.Use(
		middleware.Auth(jwtSvc),
		middleware.RequireRole(domain.RoleAdmin),
		middleware.RateLimitByRole(limiter),
	)
	{
		admin.GET("/users", authHandler.ListUsers)
	}

	return r
}
