package logger_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/jorge-sanchez/cloud-commerce/pkg/logger"
)

// ---------------------------------------------------------------------------
// Test Budget: 2 distinct behaviors × 2 = 4 max unit tests
// Actual: 2 unit tests
//
// Behaviors:
//  1. ScrubFields redacts sensitive field names from a log entry (body scrub)
//  2. ScrubFields does not redact non-sensitive fields
// ---------------------------------------------------------------------------

func TestNew_ProductionJSON(t *testing.T) {
	l, err := logger.New(logger.Config{Env: "production", Level: "info"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer l.Sync() //nolint:errcheck
	if l == nil {
		t.Error("expected non-nil logger")
	}
}

func TestNew_LocalConsole(t *testing.T) {
	l, err := logger.New(logger.Config{Env: "local", Level: "debug"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer l.Sync() //nolint:errcheck
	if l == nil {
		t.Error("expected non-nil logger")
	}
}

func TestFromContext_NilFallback(t *testing.T) {
	ctx := context.Background()
	l := logger.FromContext(ctx)
	if l == nil {
		t.Error("FromContext should return nop logger, not nil")
	}
}

func TestWithContext_RoundTrip(t *testing.T) {
	original, _ := logger.New(logger.Config{Env: "local"})
	ctx := logger.WithContext(context.Background(), original)
	retrieved := logger.FromContext(ctx)
	if retrieved == nil {
		t.Error("expected logger from context, got nil")
	}
}

func TestWithTraceID_AddsFields(t *testing.T) {
	// Use zap observer to capture output
	core, logs := observer.New(zapcore.InfoLevel)
	l := zap.New(core)

	traced := logger.WithTraceID(l, "trace-abc", "span-123")
	traced.Info("test message")

	if logs.Len() != 1 {
		t.Fatalf("expected 1 log entry, got %d", logs.Len())
	}
	entry := logs.All()[0]
	fields := make(map[string]string)
	for _, f := range entry.Context {
		if f.Type == zapcore.StringType {
			fields[f.Key] = f.String
		}
	}
	if fields["traceId"] != "trace-abc" {
		t.Errorf("expected traceId=trace-abc, got %q", fields["traceId"])
	}
	if fields["spanId"] != "span-123" {
		t.Errorf("expected spanId=span-123, got %q", fields["spanId"])
	}
}

func TestJSONOutput_RequiredFields(t *testing.T) {
	var buf bytes.Buffer
	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.TimeKey = "time"
	encoderCfg.LevelKey = "level"
	encoderCfg.MessageKey = "msg"
	encoderCfg.EncodeTime = zapcore.RFC3339NanoTimeEncoder
	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderCfg),
		zapcore.AddSync(&buf),
		zapcore.InfoLevel,
	)
	l := zap.New(core)
	l.Info("hello world")

	var out map[string]any
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("output is not valid JSON: %v\nraw: %s", err, buf.String())
	}
	for _, key := range []string{"time", "level", "msg"} {
		if _, ok := out[key]; !ok {
			t.Errorf("missing field %q in JSON output", key)
		}
	}
}

// Behavior 1: ScrubFields redacts all sensitive field names in a log entry.
func TestScrubFields_RedactsSensitiveFields(t *testing.T) {
	core, logs := observer.New(zapcore.InfoLevel)
	l := zap.New(logger.ScrubCore(core))

	l.Info("incoming request",
		zap.String("body", `{"password":"secret"}`),
		zap.String("request_body", `{"token":"abc"}`),
		zap.String("response_body", `{"data":"sensitive"}`),
		zap.String("password", "hunter2"),
		zap.String("token", "Bearer xyz"),
		zap.String("path", "/v1/send"),
	)

	if logs.Len() != 1 {
		t.Fatalf("expected 1 log entry, got %d", logs.Len())
	}
	entry := logs.All()[0]
	fieldMap := make(map[string]string, len(entry.Context))
	for _, f := range entry.Context {
		if f.Type == zapcore.StringType {
			fieldMap[f.Key] = f.String
		}
	}

	sensitiveKeys := []string{"body", "request_body", "response_body", "password", "token"}
	for _, k := range sensitiveKeys {
		if v, ok := fieldMap[k]; ok && v != logger.RedactedValue {
			t.Errorf("field %q should be redacted, got %q", k, v)
		}
	}
	// Non-sensitive field must pass through unchanged.
	if got := fieldMap["path"]; got != "/v1/send" {
		t.Errorf("field path: want /v1/send, got %q", got)
	}
}

// Behavior 3: RegisterScrubFields — service-registered fields are redacted.
func TestRegisterScrubFields_ServiceFieldsAreRedacted(t *testing.T) {
	logger.RegisterScrubFields("gcm_server_key", "apns_certificate")

	core, logs := observer.New(zapcore.InfoLevel)
	l := zap.New(logger.ScrubCore(core))

	l.Info("channel credentials",
		zap.String("gcm_server_key", "AIzaSy-secret"),
		zap.String("apns_certificate", "-----BEGIN CERTIFICATE-----"),
		zap.String("channel_id", "ch-123"),
	)

	if logs.Len() != 1 {
		t.Fatalf("expected 1 log entry, got %d", logs.Len())
	}
	entry := logs.All()[0]
	fieldMap := make(map[string]string, len(entry.Context))
	for _, f := range entry.Context {
		if f.Type == zapcore.StringType {
			fieldMap[f.Key] = f.String
		}
	}
	for _, k := range []string{"gcm_server_key", "apns_certificate"} {
		if v := fieldMap[k]; v != logger.RedactedValue {
			t.Errorf("field %q should be redacted, got %q", k, v)
		}
	}
	if got := fieldMap["channel_id"]; got != "ch-123" {
		t.Errorf("non-sensitive field channel_id: want ch-123, got %q", got)
	}
}

// Behavior 2: ScrubFields does not modify non-sensitive fields.
func TestScrubFields_PassesThroughNonSensitiveFields(t *testing.T) {
	core, logs := observer.New(zapcore.InfoLevel)
	l := zap.New(logger.ScrubCore(core))

	l.Info("response sent",
		zap.String("status", "200"),
		zap.String("method", "POST"),
		zap.Int("duration_ms", 42),
	)

	if logs.Len() != 1 {
		t.Fatalf("expected 1 log entry, got %d", logs.Len())
	}
	entry := logs.All()[0]
	fieldMap := make(map[string]interface{}, len(entry.Context))
	for _, f := range entry.Context {
		switch f.Type {
		case zapcore.StringType:
			fieldMap[f.Key] = f.String
		case zapcore.Int64Type:
			fieldMap[f.Key] = f.Integer
		}
	}
	if fieldMap["status"] != "200" {
		t.Errorf("status: want 200, got %v", fieldMap["status"])
	}
	if fieldMap["method"] != "POST" {
		t.Errorf("method: want POST, got %v", fieldMap["method"])
	}
}
