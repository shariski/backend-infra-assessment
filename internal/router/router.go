package router

import (
	"context"
	"net/http"
	"time"

	"auth/config"
	_ "auth/docs" // swag-generated OpenAPI spec
	"auth/internal/domain"
	"auth/internal/handler"
	"auth/internal/middleware"
	"auth/internal/service"
	"auth/pkg/ratelimit"

	"log/slog"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	swaggerfiles "github.com/swaggo/files"
	ginswagger "github.com/swaggo/gin-swagger"
	"gorm.io/gorm"
)

// New builds the Gin engine with all routes and middleware wired up.
func New(
	cfg *config.Config,
	log *slog.Logger,
	authHandler *handler.AuthHandler,
	jwtSvc *service.JWTService,
	limiter ratelimit.Limiter,
	db *gorm.DB,
	rdb *redis.Client,
) *gin.Engine {
	if cfg.App.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(middleware.Recovery(log))
	r.Use(middleware.RequestLogger(log)) // STUB
	r.Use(middleware.Audit(log))         // STUB

	r.GET("/livez", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	r.GET("/readyz", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()

		checks := gin.H{}
		ready := true

		sqlDB, err := db.DB()
		if err != nil {
			checks["db"] = err.Error()
			ready = false
		} else if err := sqlDB.PingContext(ctx); err != nil {
			checks["db"] = err.Error()
			ready = false
		} else {
			checks["db"] = "ok"
		}

		if err := rdb.Ping(ctx).Err(); err != nil {
			checks["redis"] = err.Error()
			ready = false
		} else {
			checks["redis"] = "ok"
		}

		status := http.StatusOK
		if !ready {
			status = http.StatusServiceUnavailable
		}
		c.JSON(status, gin.H{"ready": ready, "checks": checks})
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

	// Swagger UI is intentionally disabled in production to avoid leaking the
	// API surface (endpoint catalog, schemas, error codes) to unauthenticated
	// callers. Staging keeps it on so reviewers and developers can explore.
	if cfg.App.Env != "production" {
		r.GET("/swagger/*any", ginswagger.WrapHandler(swaggerfiles.Handler))
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
