package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"auth/config"
	"auth/internal/handler"
	"auth/internal/repository"
	"auth/internal/router"
	"auth/internal/server"
	"auth/internal/service"
	"auth/pkg/cache"
	"auth/pkg/database"
	"auth/pkg/logger"
	"auth/pkg/ratelimit"
	pkgredis "auth/pkg/redis"
)

// @title                       Auth API
// @version                     1.0
// @description                 Auth API for backend infra assessment
//
// @contact.name                Falahudin Halim Shariski
// @contact.email               falahudin6@gmail.com
//
// @BasePath                    https://auth.shariski.com
//
// @securityDefinitions.apikey  BearerAuth
// @in                          header
// @name                        Authorization
// @description                 Paste the access_token from /auth/login as: "Bearer <token>".
func main() {
	cfg, err := config.Load()
	if err != nil {
		panic("failed to load config: " + err.Error())
	}

	log := logger.New(cfg.Log.Level)

	db, err := database.New(cfg.DB.DSN())
	if err != nil {
		log.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	rdb, err := pkgredis.New(ctx, cfg.Redis)
	if err != nil {
		log.Error("failed to connect to redis", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := rdb.Close(); err != nil {
			log.Warn("redis close failed", "error", err)
		}
	}()
	limiter := ratelimit.NewRedis(rdb, log)
	respCache := cache.NewRedis(rdb, log)

	userRepo := repository.NewUserRepository(db)
	tokenRepo := repository.NewTokenRepository(db)
	attemptRepo := repository.NewLoginAttemptRepository(db)
	auditRepo := repository.NewAuditEventRepository(db)

	jwtSvc := service.NewJWTService(cfg.JWT)
	authSvc := service.NewAuthService(userRepo, tokenRepo, attemptRepo, jwtSvc, cfg.Security)
	authHandler := handler.NewAuthHandler(authSvc)

	engine := router.New(cfg, log, authHandler, jwtSvc, limiter, respCache, db, rdb, auditRepo)
	srv := server.New(cfg.App.Port, engine)

	if err := srv.Run(ctx, log); err != nil {
		log.Error("server error", "error", err)
		os.Exit(1)
	}
}
