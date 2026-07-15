package apperrors

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

// AppError is the canonical error type for the service.
// All service errors should be wrapped in or returned as an AppError.
type AppError struct {
	Code       string // machine-readable error code, e.g. "NOT_FOUND"
	Message    string // human-readable message safe to return to clients
	HTTPStatus int    // HTTP status code to use in responses
	Err        error  // underlying cause (not exposed to clients)
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return e.Code + ": " + e.Message + ": " + e.Err.Error()
	}
	return e.Code + ": " + e.Message
}

// Unwrap enables errors.Is / errors.As to traverse the chain.
func (e *AppError) Unwrap() error {
	return e.Err
}

// Is reports whether e matches target by Code equality.
// This allows errors.Is(wrappedErr, ErrNotFound) to return true when both
// share the same Code, even though Wrap returns a distinct *AppError pointer.
func (e *AppError) Is(target error) bool {
	var t *AppError
	if errors.As(target, &t) {
		return e.Code == t.Code
	}
	return false
}

// Wrap returns a new AppError wrapping cause with the same Code/Message/HTTPStatus.
func (e *AppError) Wrap(cause error) *AppError {
	return &AppError{
		Code:       e.Code,
		Message:    e.Message,
		HTTPStatus: e.HTTPStatus,
		Err:        cause,
	}
}

// Sentinel errors — use errors.Is(err, ErrNotFound) to test.
var (
	ErrNotFound = &AppError{
		Code:       "NOT_FOUND",
		Message:    "the requested resource was not found",
		HTTPStatus: http.StatusNotFound,
	}
	ErrForbidden = &AppError{
		Code:       "FORBIDDEN",
		Message:    "you do not have permission to perform this action",
		HTTPStatus: http.StatusForbidden,
	}
	ErrTenantSuspended = &AppError{
		Code:       "TENANT_SUSPENDED",
		Message:    "this tenant account has been suspended",
		HTTPStatus: http.StatusForbidden,
	}
	ErrTenantDeleted = &AppError{
		Code:       "TENANT_DELETED",
		Message:    "this tenant account has been closed",
		HTTPStatus: http.StatusForbidden,
	}
	ErrUnauthorized = &AppError{
		Code:       "UNAUTHORIZED",
		Message:    "authentication is required",
		HTTPStatus: http.StatusUnauthorized,
	}
	ErrValidation = &AppError{
		Code:       "VALIDATION_ERROR",
		Message:    "the request body is invalid",
		HTTPStatus: http.StatusUnprocessableEntity,
	}
	ErrInternal = &AppError{
		Code:       "INTERNAL_ERROR",
		Message:    "an internal error occurred",
		HTTPStatus: http.StatusInternalServerError,
	}
	ErrConflict = &AppError{
		Code:       "CONFLICT",
		Message:    "a resource with the provided identifier already exists",
		HTTPStatus: http.StatusConflict,
	}
	ErrQuotaExceeded = &AppError{
		Code:       "QUOTA_EXCEEDED",
		Message:    "you have exceeded your plan quota",
		HTTPStatus: http.StatusTooManyRequests,
	}
	ErrPaymentRequired = &AppError{
		Code:       "PAYMENT_REQUIRED",
		Message:    "quota limit reached for this billing period",
		HTTPStatus: http.StatusPaymentRequired,
	}
	ErrBadRequest = &AppError{
		Code:       "BAD_REQUEST",
		Message:    "the request was invalid",
		HTTPStatus: http.StatusBadRequest,
	}
	ErrServiceUnavailable = &AppError{
		Code:       "SERVICE_UNAVAILABLE",
		Message:    "a dependent service is unavailable",
		HTTPStatus: http.StatusServiceUnavailable,
	}
	ErrTemplateNotFound = &AppError{
		Code:       "TEMPLATE_NOT_FOUND",
		Message:    "the referenced template was not found",
		HTTPStatus: http.StatusUnprocessableEntity,
	}
	ErrFeatureNotAvailable = &AppError{
		Code:       "FEATURE_NOT_AVAILABLE",
		Message:    "this feature is not available on your current plan",
		HTTPStatus: http.StatusForbidden,
	}
)

// errorResponse is the JSON shape returned to clients.
type errorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// RespondError maps an error to a JSON HTTP response. If err is (or wraps) an
// *AppError, the response uses the AppError's status code and structured body.
// Any other error falls back to 500 INTERNAL_ERROR.
func RespondError(c *gin.Context, err error) {
	var appErr *AppError
	if errors.As(err, &appErr) {
		c.JSON(appErr.HTTPStatus, errorResponse{
			Code:    appErr.Code,
			Message: appErr.Message,
		})
		return
	}
	c.JSON(http.StatusInternalServerError, errorResponse{
		Code:    ErrInternal.Code,
		Message: ErrInternal.Message,
	})
}

// ErrorHandler is a Gin middleware that converts AppError (or unknown errors)
// into JSON responses. Must be registered before route handlers.
func ErrorHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		if len(c.Errors) == 0 {
			return
		}

		// Use the last error attached via c.Error(err)
		err := c.Errors.Last().Err

		var appErr *AppError
		if errors.As(err, &appErr) {
			c.JSON(appErr.HTTPStatus, errorResponse{
				Code:    appErr.Code,
				Message: appErr.Message,
			})
			return
		}

		// Unknown error — do not leak internals
		c.JSON(http.StatusInternalServerError, errorResponse{
			Code:    ErrInternal.Code,
			Message: ErrInternal.Message,
		})
	}
}
