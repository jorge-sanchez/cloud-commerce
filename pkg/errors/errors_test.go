package apperrors_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	apperrors "github.com/jorge-sanchez/cloud-commerce/pkg/errors"
)

func TestSentinels_ErrorMethod(t *testing.T) {
	tests := []struct {
		name     string
		err      *apperrors.AppError
		wantCode string
	}{
		{"not found", apperrors.ErrNotFound, "NOT_FOUND"},
		{"forbidden", apperrors.ErrForbidden, "FORBIDDEN"},
		{"unauthorized", apperrors.ErrUnauthorized, "UNAUTHORIZED"},
		{"validation", apperrors.ErrValidation, "VALIDATION_ERROR"},
		{"internal", apperrors.ErrInternal, "INTERNAL_ERROR"},
		{"quota exceeded", apperrors.ErrQuotaExceeded, "QUOTA_EXCEEDED"},
		{"payment required", apperrors.ErrPaymentRequired, "PAYMENT_REQUIRED"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Code != tt.wantCode {
				t.Errorf("expected code %s, got %s", tt.wantCode, tt.err.Code)
			}
			if tt.err.Error() == "" {
				t.Error("Error() should return non-empty string")
			}
		})
	}
}

func TestAppError_Wrap_ErrorsIs(t *testing.T) {
	cause := errors.New("db connection failed")
	wrapped := apperrors.ErrNotFound.Wrap(cause)

	if !errors.Is(wrapped, cause) {
		t.Error("errors.Is should find the wrapped cause")
	}
	if wrapped.Code != "NOT_FOUND" {
		t.Errorf("expected NOT_FOUND code, got %s", wrapped.Code)
	}
}

func TestAppError_ErrorsAs(t *testing.T) {
	var appErr *apperrors.AppError
	if !errors.As(apperrors.ErrForbidden, &appErr) {
		t.Error("errors.As should unwrap to *AppError")
	}
	if appErr.Code != "FORBIDDEN" {
		t.Errorf("expected FORBIDDEN, got %s", appErr.Code)
	}
}

func TestErrorHandler_AppError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(apperrors.ErrorHandler())
	r.GET("/test", func(c *gin.Context) {
		_ = c.Error(apperrors.ErrNotFound)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "NOT_FOUND") {
		t.Errorf("expected NOT_FOUND in body, got: %s", body)
	}
}

func TestErrorHandler_UnknownError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(apperrors.ErrorHandler())
	r.GET("/test", func(c *gin.Context) {
		_ = c.Error(errors.New("something unexpected"))
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "INTERNAL_ERROR") {
		t.Errorf("expected INTERNAL_ERROR in body, got: %s", body)
	}
	// Must not leak the raw error message
	if strings.Contains(body, "something unexpected") {
		t.Error("response must not leak internal error message")
	}
}
