// The merchants service wires the layers together: repository → service →
// handler. It is the platform identity issuer (ADR-006): the only service
// holding the JWT private key.
package main

import (
	"context"
	"database/sql"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"

	"go.uber.org/zap"

	"github.com/jorge-sanchez/cloud-commerce/pkg/auth"
	apperrors "github.com/jorge-sanchez/cloud-commerce/pkg/errors"
	"github.com/jorge-sanchez/cloud-commerce/pkg/events"
	"github.com/jorge-sanchez/cloud-commerce/pkg/logger"
	"github.com/jorge-sanchez/cloud-commerce/pkg/outbox"
	"github.com/jorge-sanchez/cloud-commerce/services/merchants/internal/handler"
	"github.com/jorge-sanchez/cloud-commerce/services/merchants/internal/repository"
	"github.com/jorge-sanchez/cloud-commerce/services/merchants/internal/service"
)

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
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

	// This service mints platform identity; everyone else only verifies.
	issuer, err := auth.NewIssuer(os.Getenv("JWT_PRIVATE_KEY"))
	if err != nil {
		log.Fatal("JWT_PRIVATE_KEY must hold the platform private seed", zap.Error(err))
	}
	verifier, err := auth.NewVerifier(os.Getenv("JWT_PUBLIC_KEY"))
	if err != nil {
		log.Fatal("JWT_PUBLIC_KEY must hold the platform public key", zap.Error(err))
	}

	repo := repository.NewPostgresMerchantRepository(db,
		repository.WithEventRecorder(outbox.NewRecorder()))
	svc := service.NewMerchantService(repo, issuer)
	h := handler.NewMerchantHandler(svc)

	relay := outbox.NewRelay(db, outbox.DelivererFunc(
		func(_ context.Context, env events.Envelope) error {
			log.Info("event delivered",
				zap.String("event_id", env.ID),
				zap.String("type", env.Type),
				zap.String("tenant_id", env.TenantID))
			return nil
		}), outbox.WithLogger(log))

	// On Cloud Run the relay must not be a background goroutine (ADR-003);
	// Cloud Scheduler POSTs /internal/outbox/drain instead. Locally, a
	// poller keeps `make run-merchants` zero-config.
	if env == "local" {
		relayCtx, stopRelay := context.WithCancel(context.Background())
		defer stopRelay()
		go relay.Run(relayCtx)
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

	v1 := router.Group("/v1")
	h.RegisterPublicRoutes(v1)
	h.RegisterAuthedRoutes(router.Group("/v1", auth.Middleware(verifier)))

	router.POST("/internal/outbox/drain",
		outbox.DrainHandler(relay, os.Getenv("OUTBOX_DRAIN_TOKEN")))

	addr := ":" + envOr("PORT", "8080")
	log.Info("merchants service listening", zap.String("addr", addr))
	if err := router.Run(addr); err != nil {
		log.Fatal("server exited", zap.Error(err))
	}
}
