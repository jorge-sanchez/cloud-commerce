// The inventory service wires the layers together: repository → service →
// handler. It verifies platform tokens (ADR-006) and consumes catalog
// events via Pub/Sub push (ADR-002 amendment).
package main

import (
	"context"
	"database/sql"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
	"google.golang.org/api/idtoken"

	"go.uber.org/zap"

	"github.com/jorge-sanchez/cloud-commerce/pkg/auth"
	apperrors "github.com/jorge-sanchez/cloud-commerce/pkg/errors"
	"github.com/jorge-sanchez/cloud-commerce/pkg/events"
	"github.com/jorge-sanchez/cloud-commerce/pkg/logger"
	"github.com/jorge-sanchez/cloud-commerce/pkg/outbox"
	"github.com/jorge-sanchez/cloud-commerce/services/inventory/internal/handler"
	"github.com/jorge-sanchez/cloud-commerce/services/inventory/internal/repository"
	"github.com/jorge-sanchez/cloud-commerce/services/inventory/internal/service"
)

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// idtokenValidator validates Google-signed OIDC tokens on Pub/Sub pushes.
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

	verifier, err := auth.NewVerifier(os.Getenv("JWT_PUBLIC_KEY"))
	if err != nil {
		log.Fatal("JWT_PUBLIC_KEY must hold the platform public key", zap.Error(err))
	}

	repo := repository.NewPostgresStockRepository(db,
		repository.WithEventRecorder(outbox.NewRecorder()))
	svc := service.NewStockService(repo)
	h := handler.NewStockHandler(svc)

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
	// poller keeps `make run-inventory` zero-config.
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

	h.RegisterRoutes(router.Group("/v1", auth.Middleware(verifier)))

	internal := router.Group("/internal")
	internal.POST("/outbox/drain",
		outbox.DrainHandler(relay, os.Getenv("OUTBOX_DRAIN_TOKEN")))
	// Pub/Sub push: only PUBSUB_PUSH_SA may deliver, proven by a Google
	// OIDC token with this endpoint as audience. Fails closed when unset.
	handler.NewPubSubHandler(svc, idtokenValidator{},
		os.Getenv("PUBSUB_AUDIENCE"), os.Getenv("PUBSUB_PUSH_SA")).
		RegisterRoutes(internal)

	addr := ":" + envOr("PORT", "8080")
	log.Info("inventory service listening", zap.String("addr", addr))
	if err := router.Run(addr); err != nil {
		log.Fatal("server exited", zap.Error(err))
	}
}
