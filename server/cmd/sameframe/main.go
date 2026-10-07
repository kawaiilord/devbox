package main

import (
	"log/slog"
	"os"
	"strings"

	"github.com/kawaiilord/devbox/server/internal/app"
)

func main() {
	addr := envOr("SAMEFRAME_ADDR", ":8080")
	origins := strings.Split(envOr("SAMEFRAME_ALLOWED_ORIGINS", "http://localhost:*"), ",")
	server := app.NewServer(app.Options{
		Address:        addr,
		AllowedOrigins: origins,
		Logger:         slog.New(slog.NewJSONHandler(os.Stdout, nil)),
	})
	if err := server.ListenAndServe(); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
