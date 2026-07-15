// The catalog service wires the layers together: repository → service →
// handler. It verifies platform tokens (ADR-006) but never issues them.
package main

import (
	"context"
	"database/sql"
	"net/http"
	"os"

	"cloud.google.com/go/pubsub/v2"
	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"

	"go.uber.org/zap"

	"github.com/jorge-sanchez/cloud-commerce/pkg/auth"
	"github.com/jorge-sanchez/cloud-commerce/pkg/cors"
	apperrors "github.com/jorge-sanchez/cloud-commerce/pkg/errors"
	"github.com/jorge-sanchez/cloud-commerce/pkg/events"
	"github.com/jorge-sanchez/cloud-commerce/pkg/logger"
	"github.com/jorge-sanchez/cloud-commerce/pkg/outbox"
	"github.com/jorge-sanchez/cloud-commerce/services/catalog/internal/handler"
	"github.com/jorge-sanchez/cloud-commerce/services/catalog/internal/producer"
	"github.com/jorge-sanchez/cloud-commerce/services/catalog/internal/repository"
	"github.com/jorge-sanchez/cloud-commerce/services/catalog/internal/service"
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

	verifier, err := auth.NewVerifier(os.Getenv("JWT_PUBLIC_KEY"))
	if err != nil {
		log.Fatal("JWT_PUBLIC_KEY must hold the platform public key", zap.Error(err))
	}

	repo := repository.NewPostgresProductRepository(db,
		repository.WithEventRecorder(outbox.NewRecorder()))
	svc := service.NewProductService(repo)
	h := handler.NewProductHandler(svc)
	collections := handler.NewCollectionHandler(
		service.NewCollectionService(repository.NewPostgresCollectionRepository(db)))

	// Relay transport (ADR-002 amendment): Pub/Sub when PUBSUB_TOPIC is
	// set; a log-only deliverer keeps local development broker-free.
	var deliverer outbox.Deliverer = outbox.DelivererFunc(
		func(_ context.Context, env events.Envelope) error {
			log.Info("event delivered (log transport)",
				zap.String("event_id", env.ID),
				zap.String("type", env.Type),
				zap.String("tenant_id", env.TenantID))
			return nil
		})
	if topicID := os.Getenv("PUBSUB_TOPIC"); topicID != "" {
		psClient, err := pubsub.NewClient(context.Background(), pubsub.DetectProjectID)
		if err != nil {
			log.Fatal("create pubsub client", zap.Error(err))
		}
		defer func() { _ = psClient.Close() }()
		deliverer = producer.NewPubSubDeliverer(psClient, topicID)
		log.Info("relay transport: pubsub", zap.String("topic", topicID))
	}
	relay := outbox.NewRelay(db, deliverer, outbox.WithLogger(log))

	// On Cloud Run the relay must not be a background goroutine (ADR-003);
	// Cloud Scheduler POSTs /internal/outbox/drain instead. Locally, a
	// poller keeps `make run-catalog` zero-config.
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
	router.Use(cors.Middleware(os.Getenv("CORS_ALLOWED_ORIGINS")))

	router.GET("/health", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	v1 := router.Group("/v1", auth.Middleware(verifier))
	h.RegisterRoutes(v1)
	collections.RegisterRoutes(v1)

	router.POST("/internal/outbox/drain",
		outbox.DrainHandler(relay, os.Getenv("OUTBOX_DRAIN_TOKEN")))

	addr := ":" + envOr("PORT", "8080")
	log.Info("catalog service listening", zap.String("addr", addr))
	if err := router.Run(addr); err != nil {
		log.Fatal("server exited", zap.Error(err))
	}
}
