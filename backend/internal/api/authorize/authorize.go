// Package authorize holds the admin endpoint permission matrix: each contract
// route is mapped to the permission point required to call it. Business
// handlers attach RequirePermission via this table so RBAC stays centralized.
package authorize

import (
	"net/http"

	"news-admin/backend/internal/domain"
)

// AdminRoute binds an admin endpoint to the permission point guarding it.
// Path uses the contract's {id} placeholder style.
type AdminRoute struct {
	Method     string
	Path       string
	Permission string
}

// AdminRoutes lists every admin endpoint and its required permission,
// following the shared contract matrix:
//
//	articles:   create|update|soft_delete|submit (editor+admin),
//	            approve|reject|unpublish|pin (reviewer+admin)
//	categories: manage (admin)
//	users:      manage (admin)
//	audit-logs: read (admin)
var AdminRoutes = []AdminRoute{
	// Articles: production and lifecycle.
	{Method: http.MethodGet, Path: "/articles", Permission: domain.PermArticleRead},
	{Method: http.MethodPost, Path: "/articles", Permission: domain.PermArticleCreate},
	{Method: http.MethodGet, Path: "/articles/:id", Permission: domain.PermArticleRead},
	{Method: http.MethodPut, Path: "/articles/:id", Permission: domain.PermArticleUpdate},
	{Method: http.MethodDelete, Path: "/articles/:id", Permission: domain.PermArticleSoftDelete},
	{Method: http.MethodPost, Path: "/articles/:id/submit", Permission: domain.PermArticleSubmit},
	{Method: http.MethodPost, Path: "/articles/:id/approve", Permission: domain.PermArticleApprove},
	{Method: http.MethodPost, Path: "/articles/:id/reject", Permission: domain.PermArticleReject},
	{Method: http.MethodPost, Path: "/articles/:id/unpublish", Permission: domain.PermArticleUnpublish},
	{Method: http.MethodPut, Path: "/articles/:id/pin", Permission: domain.PermArticlePin},

	// Categories: admin only.
	{Method: http.MethodGet, Path: "/categories", Permission: domain.PermCategoriesManage},
	{Method: http.MethodPost, Path: "/categories", Permission: domain.PermCategoriesManage},
	{Method: http.MethodPut, Path: "/categories/:id", Permission: domain.PermCategoriesManage},
	{Method: http.MethodDelete, Path: "/categories/:id", Permission: domain.PermCategoriesManage},

	// Users: admin only.
	{Method: http.MethodGet, Path: "/users", Permission: domain.PermUsersManage},
	{Method: http.MethodPost, Path: "/users", Permission: domain.PermUsersManage},
	{Method: http.MethodPut, Path: "/users/:id", Permission: domain.PermUsersManage},
	{Method: http.MethodPatch, Path: "/users/:id/status", Permission: domain.PermUsersManage},
	{Method: http.MethodPost, Path: "/users/:id/reset-password", Permission: domain.PermUsersManage},

	// Audit logs: admin only.
	{Method: http.MethodGet, Path: "/audit-logs", Permission: domain.PermAuditRead},
}

// PermissionForRoute returns the permission guarding the given admin route, or
// "" when the route is not part of the contract matrix.
func PermissionForRoute(method, path string) string {
	for _, r := range AdminRoutes {
		if r.Method == method && r.Path == path {
			return r.Permission
		}
	}
	return ""
}
