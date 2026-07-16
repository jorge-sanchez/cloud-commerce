// The notifications service turns order events into buyer email (issue
// #29). Pure consumer: Pub/Sub push in, provider email out, sent log as
// its only state. No merchant API, no outbox, no JWT.
package main

import (
	"context"
	"database/sql"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"

	"google.golang.org/api/idtoken"

	"github.com/jorge-sanchez/cloud-commerce/pkg/auth"

	"go.uber.org/zap"

	apperrors "github.com/jorge-sanchez/cloud-commerce/pkg/errors"
	"github.com/jorge-sanchez/cloud-commerce/pkg/logger"
	"github.com/jorge-sanchez/cloud-commerce/services/notifications/internal/gateway"
	"github.com/jorge-sanchez/cloud-commerce/services/notifications/internal/handler"
	"github.com/jorge-sanchez/cloud-commerce/services/notifications/internal/repository"
	"github.com/jorge-sanchez/cloud-commerce/services/notifications/internal/service"
)

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

type idtokenValidator struct{}

func (idtokenValidator) Validate(ctx context.Context, token, audience string) (string, error) {
	payload, err := idtoken.Validate(ctx, token, audience)
	if err != nil {
		return "", err
	}
	email, _ := payload.Claims["email"].(string)
	return email, nil
}

func main() {
	env := envOr("APP_ENV", "local")

	log, err := logger.New(logger.Config{Env: env, Level: envOr("LOG_LEVEL", "info")})
	if err != nil {
		panic(err)
	}
	defer func() { _ = log.Sync() }()

	dsn := envOr("DATABASE_URL", "postgres://app:app@localhost:5432/app?sslmode=disable")
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatal("open database", zap.Error(err))
	}
	if err := db.Ping(); err != nil {
		log.Fatal("ping database", zap.Error(err))
	}

	apiKey := os.Getenv("RESEND_API_KEY")
	if apiKey == "" {
		log.Fatal("RESEND_API_KEY must be set")
	}
	sender := gateway.NewResendSender(apiKey, envOr("EMAIL_FROM", "onboarding@resend.dev"))
	svc := service.NewNotificationService(repository.NewPostgresSentLog(db), sender)
	webhooks := service.NewWebhookService(
		repository.NewPostgresWebhookRepo(db), gateway.NewHTTPWebhookPoster())

	verifier, err := auth.NewVerifier(os.Getenv("JWT_PUBLIC_KEY"))
	if err != nil {
		log.Fatal("JWT_PUBLIC_KEY must hold the platform public key", zap.Error(err))
	}

	if env != "local" {
		gin.SetMode(gin.ReleaseMode)
	}
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(logger.GinMiddleware(log))
	router.Use(apperrors.ErrorHandler())

	router.GET("/health", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	handler.NewPubSubHandler(svc, idtokenValidator{},
		os.Getenv("PUBSUB_AUDIENCE"), os.Getenv("PUBSUB_PUSH_SA")).
		WithWebhooks(webhooks).
		RegisterRoutes(router.Group("/internal"))
	handler.NewWebhookAdminHandler(webhooks).
		RegisterRoutes(router.Group("/v1", auth.Middleware(verifier)))

	addr := ":" + envOr("PORT", "8080")
	log.Info("notifications service listening", zap.String("addr", addr))
	if err := router.Run(addr); err != nil {
		log.Fatal("server exited", zap.Error(err))
	}
}
