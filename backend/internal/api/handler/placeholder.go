package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"news-admin/backend/internal/api/response"
)

// NotImplemented is the placeholder handler for admin routes whose business
// handlers land in dedicated changes. It exists so the RBAC layer can be
// exercised end to end before the endpoints carry real behavior.
func NotImplemented() gin.HandlerFunc {
	return func(c *gin.Context) {
		response.WriteError(c, http.StatusNotImplemented, "NOT_IMPLEMENTED", "endpoint not implemented yet", nil)
	}
}
