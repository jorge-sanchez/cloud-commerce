package logger

import (
	"context"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type contextKey struct{}

// Config controls logger behaviour.
type Config struct {
	// Env should be "local" for console output, anything else for JSON.
	// Typically set from APP_ENV.
	Env string
	// Level is the minimum log level: "debug", "info", "warn", "error".
	// Defaults to "info" when empty.
	Level string
}

// New builds a *zap.Logger. Local env gets a human-readable console logger;
// all other envs get production JSON.
func New(cfg Config) (*zap.Logger, error) {
	level := zapcore.InfoLevel
	if cfg.Level != "" {
		if err := level.UnmarshalText([]byte(cfg.Level)); err != nil {
			return nil, err
		}
	}

	if cfg.Env == "local" {
		devCfg := zap.NewDevelopmentConfig()
		devCfg.Level = zap.NewAtomicLevelAt(level)
		return devCfg.Build()
	}

	prodCfg := zap.NewProductionConfig()
	prodCfg.Level = zap.NewAtomicLevelAt(level)
	// Ensure standard field names expected by Cloud Logging
	prodCfg.EncoderConfig.TimeKey = "time"
	prodCfg.EncoderConfig.LevelKey = "level"
	prodCfg.EncoderConfig.MessageKey = "msg"
	prodCfg.EncoderConfig.EncodeTime = zapcore.RFC3339NanoTimeEncoder
	return prodCfg.Build()
}

// WithContext returns a new context carrying l.
func WithContext(ctx context.Context, l *zap.Logger) context.Context {
	return context.WithValue(ctx, contextKey{}, l)
}

// FromContext retrieves the logger stored by WithContext.
// Returns a no-op logger if none is present so callers never get a nil pointer.
func FromContext(ctx context.Context) *zap.Logger {
	if l, ok := ctx.Value(contextKey{}).(*zap.Logger); ok && l != nil {
		return l
	}
	return zap.NewNop()
}

// WithTraceID returns a child logger with traceId and spanId fields attached.
func WithTraceID(l *zap.Logger, traceID, spanID string) *zap.Logger {
	if traceID == "" && spanID == "" {
		return l
	}
	fields := make([]zap.Field, 0, 2)
	if traceID != "" {
		fields = append(fields, zap.String("traceId", traceID))
	}
	if spanID != "" {
		fields = append(fields, zap.String("spanId", spanID))
	}
	return l.With(fields...)
}

// GinMiddleware injects a request-scoped logger into the Gin context.
// It reads X-Cloud-Trace-Context or traceparent headers for trace IDs.
func GinMiddleware(l *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		reqLogger := l
		// Try W3C traceparent first, fall back to X-Cloud-Trace-Context
		traceID, spanID := extractTraceIDs(c)
		if traceID != "" || spanID != "" {
			reqLogger = WithTraceID(l, traceID, spanID)
		}
		ctx := WithContext(c.Request.Context(), reqLogger)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

// extractTraceIDs parses W3C traceparent or X-Cloud-Trace-Context headers.
func extractTraceIDs(c *gin.Context) (traceID, spanID string) {
	// W3C traceparent: version-traceId-spanId-flags
	if tp := c.GetHeader("traceparent"); tp != "" {
		parts := strings.SplitN(tp, "-", 4)
		if len(parts) == 4 {
			return parts[1], parts[2]
		}
	}
	// GCP: X-Cloud-Trace-Context: traceId/spanId;o=1
	if gcp := c.GetHeader("X-Cloud-Trace-Context"); gcp != "" {
		parts := strings.SplitN(gcp, "/", 2)
		if len(parts) >= 1 {
			traceID = parts[0]
		}
		if len(parts) == 2 {
			spanParts := strings.SplitN(parts[1], ";", 2)
			spanID = spanParts[0]
		}
	}
	return
}
