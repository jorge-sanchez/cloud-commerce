// The example service wires the layers together: repository → service →
// handler. Configuration comes from the environment; sensible local defaults
// keep `make run-example` zero-config against the docker-compose Postgres.
package main

import (
	"context"
	"database/sql"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"

	"go.uber.org/zap"

	apperrors "github.com/jorge-sanchez/cloud-commerce/pkg/errors"
	"github.com/jorge-sanchez/cloud-commerce/pkg/events"
	"github.com/jorge-sanchez/cloud-commerce/pkg/logger"
	"github.com/jorge-sanchez/cloud-commerce/services/example/internal/handler"
	"github.com/jorge-sanchez/cloud-commerce/services/example/internal/producer"
	"github.com/jorge-sanchez/cloud-commerce/services/example/internal/repository"
	"github.com/jorge-sanchez/cloud-commerce/services/example/internal/service"
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

	// Events are recorded to the outbox inside repository transactions
	// (ADR-002); the relay drains them. The initial transport just logs —
	// swap the deliverer for a broker client when one exists.
	repo := repository.NewPostgresWidgetRepository(db,
		repository.WithEventRecorder(producer.NewOutboxRecorder()))
	svc := service.NewWidgetService(repo)
	h := handler.NewWidgetHandler(svc)

	relay := producer.NewRelay(db, producer.DelivererFunc(
		func(_ context.Context, env events.Envelope) error {
			log.Info("event delivered",
				zap.String("event_id", env.ID),
				zap.String("type", env.Type),
				zap.String("tenant_id", env.TenantID))
			return nil
		}), producer.WithLogger(log))

	// On Cloud Run the relay must not be a background goroutine — CPU is
	// throttled outside requests and instances scale to zero (ADR-003).
	// Cloud Scheduler POSTs /internal/outbox/drain instead, authenticated
	// with OUTBOX_DRAIN_TOKEN. Locally, a poller keeps `make run-example`
	// zero-config.
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
	h.RegisterRoutes(router.Group("/v1"))
	// Internal surface — not part of the public API. The drain endpoint
	// fails closed when OUTBOX_DRAIN_TOKEN is unset.
	handler.NewOutboxHandler(relay, os.Getenv("OUTBOX_DRAIN_TOKEN")).
		RegisterRoutes(router.Group("/internal"))

	addr := ":" + envOr("PORT", "8080")
	log.Info("example service listening", zap.String("addr", addr))
	if err := router.Run(addr); err != nil {
		log.Fatal("server exited", zap.Error(err))
	}
}
