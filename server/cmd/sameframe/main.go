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
	requireVerifiedEmail := envBool("SAMEFRAME_REQUIRE_VERIFIED_EMAIL", false)
	var mailer app.Mailer = app.NoopMailer{}
	if webhookURL := os.Getenv("SAMEFRAME_MAIL_WEBHOOK_URL"); webhookURL != "" {
		webhookMailer, mailErr := app.NewWebhookMailer(
			webhookURL, os.Getenv("SAMEFRAME_MAIL_WEBHOOK_SECRET"),
		)
		if mailErr != nil {
			logger.Error("configure mail webhook", "error", mailErr)
			os.Exit(1)
		}
		mailer = webhookMailer
	} else if requireVerifiedEmail {
		logger.Error("mail webhook is required when verified email is enforced")
		os.Exit(1)
	}
	var sourceManager *app.MediaSourceManager
	var credentialVault *app.CredentialVault
	if vaultKey := os.Getenv("SAMEFRAME_VAULT_KEY"); vaultKey != "" {
		vault, vaultErr := app.NewCredentialVault(vaultKey)
		if vaultErr != nil {
			logger.Error("configure credential vault", "error", vaultErr)
			os.Exit(1)
		}
		sourceManager = app.NewMediaSourceManager(
			repository,
			vault,
			envBool("SAMEFRAME_ALLOW_PRIVATE_SOURCES", false),
		)
		credentialVault = vault
		logger.Info("media source vault enabled")
	}
	addr := envOr("SAMEFRAME_ADDR", ":8080")
	origins := strings.Split(envOr("SAMEFRAME_ALLOWED_ORIGINS", "http://localhost:*"), ",")
	adminEmails := splitList(os.Getenv("SAMEFRAME_ADMIN_EMAILS"))
	authService := app.NewAuthService(repository, tokens, mailer)
	authService.SetAdminEmails(adminEmails)
	for _, email := range adminEmails {
		account, lookupErr := repository.UserByEmail(ctx, email)
		if lookupErr != nil {
			continue
		}
		if !account.IsAdmin {
			if setErr := repository.SetUserAdmin(ctx, account.ID, true); setErr != nil {
				logger.Error("provision administrator", "error", setErr)
				os.Exit(1)
			}
		}
	}
	var metadataClient *app.MetadataClient
	if tmdbToken := os.Getenv("SAMEFRAME_TMDB_TOKEN"); tmdbToken != "" {
		metadataClient, err = app.NewMetadataClient(tmdbToken)
		if err != nil {
			logger.Error("configure metadata search", "error", err)
			os.Exit(1)
		}
	}
	var rtcManager *app.RTCConfigManager
	if turnURLs := os.Getenv("SAMEFRAME_TURN_URLS"); turnURLs != "" {
		rtcManager, err = app.NewRTCConfigManager(strings.Split(turnURLs, ","), os.Getenv("SAMEFRAME_TURN_SECRET"))
		if err != nil {
			logger.Error("configure voice TURN", "error", err)
			os.Exit(1)
		}
	}
	var objectStore app.ObjectStore
	if endpoint := os.Getenv("SAMEFRAME_S3_ENDPOINT"); endpoint != "" {
		store, storeErr := app.NewS3ObjectStore(endpoint, os.Getenv("SAMEFRAME_S3_ACCESS_KEY"), os.Getenv("SAMEFRAME_S3_SECRET_KEY"), os.Getenv("SAMEFRAME_S3_BUCKET"))
		if storeErr != nil {
			logger.Error("configure object storage", "error", storeErr)
			os.Exit(1)
		}
		objectStore = store
	}
	var payment app.PaymentProvider
	if checkoutURL := os.Getenv("SAMEFRAME_PAYMENT_CHECKOUT_URL"); checkoutURL != "" {
		provider, paymentErr := app.NewHMACPaymentProvider(envOr("SAMEFRAME_PAYMENT_PROVIDER", "hmac"), checkoutURL, os.Getenv("SAMEFRAME_PAYMENT_CALLBACK_SECRET"))
		if paymentErr != nil {
			logger.Error("configure payment provider", "error", paymentErr)
			os.Exit(1)
		}
		payment = provider
	}
	server := app.NewServer(app.Options{
		Address:              addr,
		AllowedOrigins:       origins,
		Logger:               logger,
		Repository:           repository,
		Auth:                 authService,
		InitialRooms:         initialRooms,
		AllowDemoAuth:        envBool("SAMEFRAME_ALLOW_DEMO_AUTH", false),
		Redis:                redisCoordinator,
		RequireVerifiedEmail: requireVerifiedEmail,
		Sources:              sourceManager,
		PublicBaseURL:        os.Getenv("SAMEFRAME_PUBLIC_BASE_URL"),
		Metadata:             metadataClient,
		RTC:                  rtcManager,
		Objects:              objectStore,
		Payment:              payment,
		Vault:                credentialVault,
		VIPAnnouncement:      os.Getenv("SAMEFRAME_VIP_ANNOUNCEMENT"),
		PointsPerCheckIn:     envInt("SAMEFRAME_POINTS_PER_CHECK_IN", 1),
		PointsPerVIPDay:      envInt("SAMEFRAME_POINTS_PER_VIP_DAY", 0),
		MetricsToken:         os.Getenv("SAMEFRAME_METRICS_TOKEN"),
	})
	if err := server.ListenAndServe(); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func splitList(value string) []string {
	items := make([]string, 0)
	for _, item := range strings.Split(value, ",") {
		if item = strings.ToLower(strings.TrimSpace(item)); item != "" {
			items = append(items, item)
		}
	}
	return items
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

func envInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
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
