// Package api assembles the Gin engine: middleware chain, routes, and the
// catch-all 404/405 behavior.
package api

import (
	"log/slog"

	"github.com/gin-gonic/gin"

	"news-admin/backend/internal/api/handler"
	"news-admin/backend/internal/api/middleware"
	"news-admin/backend/internal/api/response"
	"news-admin/backend/internal/config"
)

// NewRouter builds the HTTP router for the config/logging provided.
func NewRouter(cfg *config.Config, logger *slog.Logger) *gin.Engine {
	if cfg.AppEnv == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	router.Use(
		middleware.RequestID(),
		middleware.RequestLogger(logger),
		middleware.Recovery(logger),
		middleware.CORS(cfg.CORSOrigins),
	)

	router.GET("/healthz", handler.Health)

	router.NoRoute(func(c *gin.Context) {
		response.WriteError(c, 404, response.CodeNotFound, "route not found", nil)
	})
	return router
}
