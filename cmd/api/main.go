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
	"auth/pkg/database"
	"auth/pkg/logger"
	pkgredis "auth/pkg/redis"
	"auth/pkg/ratelimit"
)

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
	defer rdb.Close()
	limiter := ratelimit.NewRedis(rdb, log)

	userRepo := repository.NewUserRepository(db)
	tokenRepo := repository.NewTokenRepository(db)
	attemptRepo := repository.NewLoginAttemptRepository(db)

	jwtSvc := service.NewJWTService(cfg.JWT)
	authSvc := service.NewAuthService(userRepo, tokenRepo, attemptRepo, jwtSvc, cfg.Security)
	authHandler := handler.NewAuthHandler(authSvc)

	engine := router.New(cfg, log, authHandler, jwtSvc, limiter)
	srv := server.New(cfg.App.Port, engine)

	if err := srv.Run(ctx, log); err != nil {
		log.Error("server error", "error", err)
		os.Exit(1)
	}
}
