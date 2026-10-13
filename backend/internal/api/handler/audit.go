package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"news-admin/backend/internal/domain"
	"news-admin/backend/internal/service"
)

// AuditHandler exposes the read-only admin audit-log listing.
type AuditHandler struct {
	svc *service.AuditService
}

// NewAuditHandler builds an AuditHandler.
func NewAuditHandler(svc *service.AuditService) *AuditHandler {
	return &AuditHandler{svc: svc}
}

// List handles GET /api/v1/admin/audit-logs with actor/action/resource and
// time-window filters plus pagination, newest first.
func (h *AuditHandler) List(c *gin.Context) {
	q, err := parseAuditQuery(c)
	if err != nil {
		responseValidation(c, err)
		return
	}
	page, err := h.svc.List(c.Request.Context(), q)
	if err != nil {
		responseInternal(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"items":    auditLogsJSON(page.Items),
		"total":    page.Total,
		"page":     page.Page,
		"pageSize": page.PageSize,
	})
}

// parseAuditQuery reads the optional filters; from/to accept ISO8601
// (RFC3339). A malformed timestamp is a 400.
func parseAuditQuery(c *gin.Context) (*domain.AuditQuery, error) {
	q := &domain.AuditQuery{Page: 1, PageSize: 10}
	if v := strings.TrimSpace(c.Query("actorId")); v != "" {
		q.ActorID = &v
	}
	if v := strings.TrimSpace(c.Query("action")); v != "" {
		q.Action = &v
	}
	if v := strings.TrimSpace(c.Query("resourceType")); v != "" {
		q.ResourceType = &v
	}
	if v := strings.TrimSpace(c.Query("resourceId")); v != "" {
		q.ResourceID = &v
	}
	if v := strings.TrimSpace(c.Query("from")); v != "" {
		ts, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return nil, errors.New("from must be an ISO8601 timestamp")
		}
		q.From = &ts
	}
	if v := strings.TrimSpace(c.Query("to")); v != "" {
		ts, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return nil, errors.New("to must be an ISO8601 timestamp")
		}
		q.To = &ts
	}
	var err error
	if page := strings.TrimSpace(c.Query("page")); page != "" {
		if q.Page, err = strconv.Atoi(page); err != nil || q.Page < 1 {
			return nil, errors.New("page must be a positive integer")
		}
	}
	if size := strings.TrimSpace(c.Query("pageSize")); size != "" {
		if q.PageSize, err = strconv.Atoi(size); err != nil || q.PageSize < 1 || q.PageSize > 100 {
			return nil, errors.New("pageSize must be between 1 and 100")
		}
	}
	return q, nil
}

func auditLogJSON(e *domain.AuditLog) gin.H {
	return gin.H{
		"id":           e.ID,
		"actorId":      e.Actor,
		"actorName":    e.ActorName,
		"action":       e.Action,
		"resourceType": e.ResourceType,
		"resourceId":   e.ResourceID,
		"before":       e.Before,
		"after":        e.After,
		"ip":           e.IP,
		"createdAt":    e.CreatedAt,
	}
}

func auditLogsJSON(entries []*domain.AuditLog) []gin.H {
	out := make([]gin.H, 0, len(entries))
	for _, e := range entries {
		out = append(out, auditLogJSON(e))
	}
	return out
}
