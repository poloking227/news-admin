// Package api assembles the Gin engine: middleware chain, routes, and the
// catch-all 404/405 behavior.
package api

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"news-admin/backend/internal/api/handler"
	"news-admin/backend/internal/api/middleware"
	"news-admin/backend/internal/api/response"
	"news-admin/backend/internal/config"
	"news-admin/backend/internal/domain"
	"news-admin/backend/internal/service"
)

// Deps bundles the components NewRouter needs beyond config/logging.
type Deps struct {
	// Secret signs access tokens.
	Secret string
	// Secure marks auth cookies Secure (production only).
	Secure bool
	// Users implements user persistence for auth.
	Users domain.UserRepository
	// Sessions persists refresh sessions.
	Sessions domain.SessionRepository
	// Audit writes audit entries.
	Audit domain.AuditRepository
}

// NewRouter builds the HTTP router for the config/logging provided.
func NewRouter(cfg *config.Config, logger *slog.Logger, deps Deps) *gin.Engine {
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

	authSvc := service.NewAuthService(deps.Users, deps.Sessions, deps.Audit, deps.Secret)
	authHandler := handler.NewAuthHandler(authSvc, deps.Secure)

	auth := router.Group("/api/v1/auth")
	{
		auth.POST("/login", authHandler.Login)
		auth.POST("/refresh", authHandler.Refresh)

		authed := auth.Group("", middleware.RequireAuth(deps.Secret))
		authed.POST("/logout", authHandler.Logout)
		authed.POST("/change-password", authHandler.ChangePassword)
		authed.GET("/me", authHandler.Me)
	}

	router.NoRoute(func(c *gin.Context) {
		response.WriteError(c, http.StatusNotFound, response.CodeNotFound, "route not found", nil)
	})
	return router
}
