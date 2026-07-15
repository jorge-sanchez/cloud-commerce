// The orders service wires the layers together: repository → service →
// handler. It owns carts, checkout, and the order lifecycle; buyer routes
// are public (capability-based cart IDs), merchant routes verify platform
// tokens (ADR-006).
package main

import (
	"context"
	"database/sql"
	"net/http"
	"os"
	"strings"

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
	"github.com/jorge-sanchez/cloud-commerce/services/orders/internal/gateway"
	"github.com/jorge-sanchez/cloud-commerce/services/orders/internal/handler"
	"github.com/jorge-sanchez/cloud-commerce/services/orders/internal/producer"
	"github.com/jorge-sanchez/cloud-commerce/services/orders/internal/repository"
	"github.com/jorge-sanchez/cloud-commerce/services/orders/internal/service"
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

	platform := gateway.NewHTTPPlatform(
		envOr("MERCHANTS_URL", "http://localhost:8081"),
		envOr("CATALOG_URL", "http://localhost:8082"),
	)
	repo := repository.NewPostgresOrderRepository(db,
		repository.WithEventRecorder(outbox.NewRecorder()))
	svc := service.NewOrderService(repo, platform)

	// PaymentGateway (ADR-008): stripe in production, fake for local dev.
	// Fail fast on unknown providers — never silently fake.
	var gw service.PaymentGateway
	switch provider := envOr("PAYMENT_PROVIDER", "fake"); provider {
	case "fake":
		gw = gateway.NewFakeGateway(envOr("FAKE_PAYMENT_SECRET", "local-dev-secret"))
		log.Info("payment gateway: fake (no real money)")
	case "stripe":
		key := os.Getenv("STRIPE_SECRET_KEY")
		if key == "" {
			log.Fatal("STRIPE_SECRET_KEY must be set for PAYMENT_PROVIDER=stripe")
		}
		gw = gateway.NewStripeGateway(key)
		log.Info("payment gateway: stripe")
	default:
		log.Fatal("unknown PAYMENT_PROVIDER", zap.String("provider", provider))
	}
	payments := service.NewPaymentService(repo, gw)
	h := handler.NewOrderHandler(svc, payments)

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
	// poller keeps `make run-orders` zero-config.
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

	// Path-aware CORS: buyer surfaces are public (any origin, and this
	// global middleware answers their preflights even for unmatched
	// OPTIONS); everything else uses the explicit allowlist.
	publicCORS := cors.Public()
	allowlistCORS := cors.Middleware(os.Getenv("CORS_ALLOWED_ORIGINS"))
	router.Use(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/v1/public/") {
			publicCORS(c)
			return
		}
		allowlistCORS(c)
	})

	router.GET("/health", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	h.RegisterBuyerRoutes(router.Group("/v1"))
	h.RegisterMerchantRoutes(router.Group("/v1", auth.Middleware(verifier)))

	internal := router.Group("/internal")
	internal.POST("/outbox/drain",
		outbox.DrainHandler(relay, os.Getenv("OUTBOX_DRAIN_TOKEN")))
	// Stripe signs deliveries with the endpoint secret; empty fails closed.
	handler.NewStripeWebhookHandler(payments, os.Getenv("STRIPE_WEBHOOK_SECRET")).
		RegisterRoutes(internal)

	addr := ":" + envOr("PORT", "8080")
	log.Info("orders service listening", zap.String("addr", addr))
	if err := router.Run(addr); err != nil {
		log.Fatal("server exited", zap.Error(err))
	}
}
