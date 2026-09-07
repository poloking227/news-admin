// Package middleware contains the Gin middlewares shared by the API router:
// panic recovery with the error envelope, structured request logging, and
// CORS handling.
package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"

	"news-admin/backend/internal/api/response"
)

// RequestID assigns a random per-request ID and echoes it in the
// X-Request-ID header so logs can be correlated across components.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Request-ID")
		if id == "" {
			id = newID()
		}
		c.Set("request_id", id)
		c.Header("X-Request-ID", id)
		c.Next()
	}
}

func newID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(b[:])
}

// Recovery converts panics into a 500 error envelope and logs the stack via
// slog, so the client always receives the contract-shaped body.
func Recovery(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("panic recovered",
					"path", c.Request.URL.Path,
					"method", c.Request.Method,
					"panic", r,
				)
				response.WriteError(c, 500, response.CodeInternal, "internal server error", nil)
			}
		}()
		c.Next()
	}
}

// RequestLogger emits one structured log line per request with status and
// duration; request bodies and headers are never logged.
func RequestLogger(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		logger.Info("request",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"duration_ms", time.Since(start).Milliseconds(),
			"client_ip", c.ClientIP(),
			"request_id", c.GetString("request_id"),
		)
	}
}
