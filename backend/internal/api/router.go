// Package api assembles the Gin engine: middleware chain, routes, and the
// catch-all 404/405 behavior.
package api

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"news-admin/backend/internal/api/authorize"
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
	// Categories persists categories.
	Categories domain.CategoryRepository
	// Articles persists articles.
	Articles domain.ArticleRepository
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

	registerAdminRoutes(router, deps)
	registerPublicRoutes(router, deps)

	router.NoRoute(func(c *gin.Context) {
		response.WriteError(c, http.StatusNotFound, response.CodeNotFound, "route not found", nil)
	})
	return router
}

// registerPublicRoutes mounts the anonymous public endpoints.
func registerPublicRoutes(router *gin.Engine, deps Deps) {
	if deps.Categories == nil {
		return
	}
	catHandler := handler.NewCategoryHandler(service.NewCategoryService(deps.Categories, deps.Audit))

	pub := router.Group("/api/v1/public")
	pub.GET("/categories", catHandler.ListPublic)
}

// registerAdminRoutes mounts the /api/v1/admin group behind the full RBAC
// chain (RequireAuth → RequireUserActive → RequirePasswordChanged →
// RequirePermission) and binds every contract route to its permission point.
// Categories carry real handlers; the remaining routes keep a 501 placeholder
// until their owning changes land.
func registerAdminRoutes(router *gin.Engine, deps Deps) {
	getUser := func(ctx context.Context, id string) (*domain.User, error) {
		return deps.Users.FindByID(ctx, id)
	}

	admin := router.Group("/api/v1/admin",
		middleware.RequireAuth(deps.Secret),
		middleware.RequireUserActive(getUser),
		middleware.RequirePasswordChanged(getUser),
	)

	handlers := map[[2]string]gin.HandlerFunc{}
	if deps.Categories != nil {
		catHandler := handler.NewCategoryHandler(service.NewCategoryService(deps.Categories, deps.Audit))
		handlers[[2]string{http.MethodGet, "/categories"}] = catHandler.List
		handlers[[2]string{http.MethodPost, "/categories"}] = catHandler.Create
		handlers[[2]string{http.MethodPut, "/categories/:id"}] = catHandler.Update
		handlers[[2]string{http.MethodDelete, "/categories/:id"}] = catHandler.Delete
	}
	if deps.Articles != nil {
		articleHandler := handler.NewArticleHandler(service.NewArticleService(deps.Articles, deps.Users, deps.Categories, deps.Audit))
		handlers[[2]string{http.MethodGet, "/articles"}] = articleHandler.List
		handlers[[2]string{http.MethodPost, "/articles"}] = articleHandler.Create
		handlers[[2]string{http.MethodGet, "/articles/:id"}] = articleHandler.Get
		handlers[[2]string{http.MethodPut, "/articles/:id"}] = articleHandler.Update
		handlers[[2]string{http.MethodDelete, "/articles/:id"}] = articleHandler.Delete
		handlers[[2]string{http.MethodPost, "/articles/:id/submit"}] = articleHandler.Submit
		handlers[[2]string{http.MethodPost, "/articles/:id/approve"}] = articleHandler.Approve
		handlers[[2]string{http.MethodPost, "/articles/:id/reject"}] = articleHandler.Reject
		handlers[[2]string{http.MethodPost, "/articles/:id/unpublish"}] = articleHandler.Unpublish
		handlers[[2]string{http.MethodPut, "/articles/:id/pin"}] = articleHandler.Pin
	}

	for _, route := range authorize.AdminRoutes {
		h, ok := handlers[[2]string{route.Method, route.Path}]
		if !ok {
			h = handler.NotImplemented()
		}
		admin.Handle(route.Method, route.Path,
			middleware.RequirePermission(route.Permission),
			h,
		)
	}
}
