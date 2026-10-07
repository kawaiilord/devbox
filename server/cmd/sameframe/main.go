package main

import (
	"context"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/kawaiilord/devbox/server/internal/app"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx := context.Background()
	var repository app.Repository = app.NewMemoryRepository()
	var initialRooms []app.Room
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL != "" {
		postgres, err := app.OpenPostgres(ctx, databaseURL)
		if err != nil {
			logger.Error("connect postgres", "error", err)
			os.Exit(1)
		}
		defer postgres.Close()
		if err := postgres.Migrate(ctx); err != nil {
			logger.Error("migrate postgres", "error", err)
			os.Exit(1)
		}
		initialRooms, err = postgres.LoadRooms(ctx)
		if err != nil {
			logger.Error("load rooms", "error", err)
			os.Exit(1)
		}
		repository = postgres
		logger.Info("postgres persistence enabled", "restored_rooms", len(initialRooms))
	}
	var redisCoordinator *app.RedisCoordinator
	if redisURL := os.Getenv("REDIS_URL"); redisURL != "" {
		coordinator, redisErr := app.OpenRedis(ctx, redisURL, os.Getenv("SAMEFRAME_NODE_ID"))
		if redisErr != nil {
			logger.Error("connect redis", "error", redisErr)
			os.Exit(1)
		}
		redisCoordinator = coordinator
		defer redisCoordinator.Close()
		for _, room := range initialRooms {
			if err := redisCoordinator.InitializeRoom(ctx, room); err != nil {
				logger.Error("initialize Redis room", "error", err, "room", room.Code)
				os.Exit(1)
			}
		}
		logger.Info("redis realtime coordination enabled")
	}
	jwtSecret := os.Getenv("SAMEFRAME_JWT_SECRET")
	if jwtSecret == "" {
		if databaseURL != "" {
			logger.Error("SAMEFRAME_JWT_SECRET is required when persistence is enabled")
			os.Exit(1)
		}
		jwtSecret = "development-only-secret-change-me-now"
		logger.Warn("using development JWT secret because persistence is disabled")
	}
	tokens, err := app.NewTokenManager(jwtSecret, envOr("SAMEFRAME_JWT_ISSUER", "sameframe"))
	if err != nil {
		logger.Error("configure tokens", "error", err)
		os.Exit(1)
	}
	addr := envOr("SAMEFRAME_ADDR", ":8080")
	origins := strings.Split(envOr("SAMEFRAME_ALLOWED_ORIGINS", "http://localhost:*"), ",")
	server := app.NewServer(app.Options{
		Address:        addr,
		AllowedOrigins: origins,
		Logger:         logger,
		Repository:     repository,
		Auth:           app.NewAuthService(repository, tokens),
		InitialRooms:   initialRooms,
		AllowDemoAuth:  envBool("SAMEFRAME_ALLOW_DEMO_AUTH", false),
		Redis:          redisCoordinator,
	})
	if err := server.ListenAndServe(); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func envBool(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
