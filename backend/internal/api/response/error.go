// Package response provides the unified error envelope used by every API
// response, matching the ErrorEnvelope schema in docs/openapi.yaml.
package response

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Stable error codes, kept in sync with docs/openapi.yaml Error.code.
const (
	CodeValidation      = "VALIDATION_FAILED"
	CodeUnauthenticated = "UNAUTHENTICATED"
	CodeForbidden       = "FORBIDDEN"
	CodeNotFound        = "NOT_FOUND"
	CodeConflict        = "CONFLICT"
	CodeUnprocessable   = "UNPROCESSABLE"
	CodeRateLimited     = "RATE_LIMITED"
	CodeInternal        = "INTERNAL"
)

// Error is the payload of the error envelope.
type Error struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

// Envelope is the top-level error response, as defined by ErrorEnvelope in
// docs/openapi.yaml: { "error": { "code", "message", "details" } }.
type Envelope struct {
	Error Error `json:"error"`
}

// WriteError aborts the request with the given status code and an envelope
// whose code follows the stable code list above.
func WriteError(c *gin.Context, status int, code, message string, details map[string]any) {
	c.AbortWithStatusJSON(status, Envelope{Error: Error{Code: code, Message: message, Details: details}})
}

// WriteErrorf is WriteError with a formatted message.
func WriteErrorf(c *gin.Context, status int, code, format string, args ...any) {
	WriteError(c, status, code, fmt.Sprintf(format, args...), nil)
}

// Sentinels that middleware and handlers can return; FromError translates
// them into the envelope.
var (
	ErrNotFound        = errors.New("not found")
	ErrUnauthenticated = errors.New("unauthenticated")
	ErrConflict        = errors.New("conflict")
)

// FromError converts a known sentinel error into the envelope. Unknown
// errors are reported as an internal server error without leaking details.
func FromError(c *gin.Context, err error) {
	if err == nil {
		return
	}
	switch {
	case errors.Is(err, ErrNotFound):
		WriteError(c, http.StatusNotFound, CodeNotFound, err.Error(), nil)
	case errors.Is(err, ErrUnauthenticated):
		WriteError(c, http.StatusUnauthorized, CodeUnauthenticated, err.Error(), nil)
	case errors.Is(err, ErrConflict):
		WriteError(c, http.StatusConflict, CodeConflict, err.Error(), nil)
	default:
		WriteError(c, http.StatusInternalServerError, CodeInternal, "internal server error", nil)
	}
}
