// The storefront renders buyer pages from the public APIs (ADR-009). No
// database, no JWT, no outbox — a pure presentation client of the platform.
package main

import (
	"net/http"
	"os"

	"github.com/gin-gonic/gin"

	"go.uber.org/zap"

	apperrors "github.com/jorge-sanchez/cloud-commerce/pkg/errors"
	"github.com/jorge-sanchez/cloud-commerce/pkg/logger"
	"github.com/jorge-sanchez/cloud-commerce/services/storefront/internal/gateway"
	"github.com/jorge-sanchez/cloud-commerce/services/storefront/internal/handler"
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

	platform := gateway.NewHTTPPlatform(
		envOr("MERCHANTS_URL", "http://localhost:8081"),
		envOr("CATALOG_URL", "http://localhost:8082"),
	)
	h, err := handler.NewStorefrontHandler(platform, handler.Config{
		OrdersURL:    envOr("ORDERS_URL", "http://localhost:8083"),
		MerchantsURL: envOr("MERCHANTS_URL", "http://localhost:8081"),
		StripePubKey: os.Getenv("STRIPE_PUBLISHABLE_KEY"),
	})
	if err != nil {
		log.Fatal("parse templates", zap.Error(err))
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
	h.RegisterRoutes(router)

	addr := ":" + envOr("PORT", "8080")
	log.Info("storefront listening", zap.String("addr", addr))
	if err := router.Run(addr); err != nil {
		log.Fatal("server exited", zap.Error(err))
	}
}
