package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"

	"news-admin/backend/internal/api/response"
)

// Re-exported response codes so handlers stay terse.
const (
	responseCodeValidation      = response.CodeValidation
	responseCodeUnauthenticated = response.CodeUnauthenticated
	responseCodeForbidden       = response.CodeForbidden
	responseCodeNotFound        = response.CodeNotFound
	responseCodeConflict        = response.CodeConflict
	responseCodeUnprocessable   = response.CodeUnprocessable
	responseCodeRateLimited     = response.CodeRateLimited
)

// context keys for values set by the auth middleware.
const (
	ctxUserKey     = "auth.user.id"
	ctxUserRoleKey = "auth.user.role"
)

// UserIDKey returns the gin context key under which the authenticated user id
// is stored (exported for the middleware/handler boundary).
func UserIDKey() string { return ctxUserKey }

// UserRoleKey returns the gin context key under which the authenticated user
// role is stored.
func UserRoleKey() string { return ctxUserRoleKey }

// validationMessage extracts a human-readable message from a binding error.
func validationMessage(err error) string {
	var verr validator.ValidationErrors
	if errors.As(err, &verr) && len(verr) > 0 {
		return "validation failed: " + verr[0].Field() + " " + verr[0].Tag()
	}
	return "validation failed"
}

// responseValidation maps binding errors to the contract's ValidationError
// envelope using the first validation message.
func responseValidation(c *gin.Context, err error) {
	response.WriteError(c, http.StatusBadRequest, response.CodeValidation, validationMessage(err), nil)
}

// responseWriteError is a thin alias over the shared envelope writer.
func responseWriteError(c *gin.Context, status int, code, message string, details map[string]any) {
	response.WriteError(c, status, code, message, details)
}

// responseInternal logs and emits the internal error envelope without leaking
// details to the client.
func responseInternal(c *gin.Context, err error) {
	response.FromError(c, err)
}

// currentUserID returns the user id placed in the context by the auth
// middleware; the caller must be behind that middleware.
func currentUserID(c *gin.Context) string {
	return c.GetString(ctxUserKey)
}
