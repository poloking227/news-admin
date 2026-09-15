// Package middleware contains the Gin middlewares shared by the API router:
// panic recovery with the error envelope, structured request logging, CORS,
// bearer authentication, and the forced-password-change guard.
package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"news-admin/backend/internal/api/handler"
	"news-admin/backend/internal/api/response"
	"news-admin/backend/internal/auth"
	"news-admin/backend/internal/domain"
)

// RequireAuth validates the bearer access token and stores the user id and
// role in the context. It returns 401 with the contract envelope on failure.
func RequireAuth(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		token, ok := strings.CutPrefix(header, "Bearer ")
		if !ok || token == "" {
			response.WriteError(c, http.StatusUnauthorized, response.CodeUnauthenticated, "missing bearer token", nil)
			c.Abort()
			return
		}
		claims, err := auth.ParseAccessToken(secret, token)
		if err != nil {
			response.WriteError(c, http.StatusUnauthorized, response.CodeUnauthenticated, "invalid or expired token", nil)
			c.Abort()
			return
		}
		c.Set(handler.UserIDKey(), claims.UserID)
		c.Set(handler.UserRoleKey(), claims.Role)
		c.Next()
	}
}

// UserGetter loads a user by id (the repository method).
type UserGetter func(ctx context.Context, id string) (*domain.User, error)

// RequireUserActive reloads the user from the store and rejects disabled
// accounts with 403.
func RequireUserActive(getUser UserGetter) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, err := getUser(c.Request.Context(), c.GetString(handler.UserIDKey()))
		if err != nil {
			response.WriteError(c, http.StatusForbidden, response.CodeForbidden, "account unavailable", nil)
			c.Abort()
			return
		}
		if user.Status != domain.UserStatusActive {
			response.WriteError(c, http.StatusForbidden, response.CodeForbidden, "account disabled", nil)
			c.Abort()
			return
		}
		c.Next()
	}
}

// RequirePasswordChanged blocks business endpoints while the M0 forced change
// is pending: during mustChangePassword only /auth/me, /auth/change-password,
// and /auth/logout are allowed.
func RequirePasswordChanged(getUser UserGetter) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, err := getUser(c.Request.Context(), c.GetString(handler.UserIDKey()))
		if err != nil {
			response.WriteError(c, http.StatusForbidden, response.CodeForbidden, "account unavailable", nil)
			c.Abort()
			return
		}
		if user.MustChangePassword {
			response.WriteError(c, http.StatusForbidden, response.CodeForbidden, "password change required", nil)
			c.Abort()
			return
		}
		c.Next()
	}
}
