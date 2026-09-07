// Package handler contains the HTTP handlers for the API server.
package handler

import (
	"net/http"
	"runtime"
	"time"

	"github.com/gin-gonic/gin"
)

// Health is the /healthz handler. It reports that the process is up plus a
// few cheap runtime facts for load balancers and ops tooling.
func Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"version": runtime.Version(),
		"time":    time.Now().UTC().Format(time.RFC3339),
	})
}
