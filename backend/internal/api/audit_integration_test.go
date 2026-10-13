// Package api contains HTTP-level integration tests for the admin audit-log
// listing endpoint using in-memory fake repositories.
package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"news-admin/backend/internal/config"
	"news-admin/backend/internal/domain"
)

const auditAdminID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaa1"

// auditRouter wires users+sessions+audit so the audit-logs route is real.
func auditRouter(t *testing.T, users *rbacUserRepo, audit *tFakeAudit) http.Handler {
	t.Helper()
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		AppEnv:      "development",
		Port:        "8080",
		LogLevel:    "info",
		CORSOrigins: []string{"http://localhost:5173"},
		JWTSecret:   "rbac-test-secret",
	}
	return NewRouter(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), Deps{
		Secret:   cfg.JWTSecret,
		Users:    users,
		Sessions: newTFakeSessions(),
		Audit:    audit,
	})
}

func TestAuditLogsAdminOnly(t *testing.T) {
	users := newRBACUsers()
	seedRBACUsers(users)
	router := auditRouter(t, users, newTFakeAudit())
	editorToken := rbacToken(t, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaa2", domain.RoleEditor)

	rec := call(router, http.MethodGet, "/api/v1/admin/audit-logs", editorToken)
	if rec.Code != http.StatusForbidden {
		t.Errorf("editor on audit-logs: status = %d, want 403", rec.Code)
	}
}

func TestAuditLogsListViaHTTP(t *testing.T) {
	users := newRBACUsers()
	seedRBACUsers(users)
	audit := newTFakeAudit()
	base := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	for _, e := range []*domain.AuditLog{
		{ID: 1, Actor: auditAdminID, ActorName: "老大", Action: "login", ResourceType: "user", IP: "1.1.1.1", CreatedAt: base},
		{ID: 2, Actor: auditAdminID, ActorName: "老大", Action: "article_create", ResourceType: "article", IP: "2.2.2.2", CreatedAt: base.Add(10 * time.Minute)},
		{ID: 3, Actor: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaa2", ActorName: "编辑", Action: "article_approve", ResourceType: "article", IP: "3.3.3.3", CreatedAt: base.Add(30 * time.Minute)},
	} {
		if err := audit.Insert(context.Background(), e); err != nil {
			t.Fatalf("Insert() error = %v", err)
		}
	}
	router := auditRouter(t, users, audit)
	adminToken := rbacToken(t, auditAdminID, domain.RoleAdmin)

	// Full list, newest first.
	rec := call(router, http.MethodGet, "/api/v1/admin/audit-logs", adminToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var page struct {
		Items []struct {
			ID        int64  `json:"id"`
			ActorID   string `json:"actorId"`
			ActorName string `json:"actorName"`
			Action    string `json:"action"`
			CreatedAt string `json:"createdAt"`
		} `json:"items"`
		Total int64 `json:"total"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &page)
	if page.Total != 3 || len(page.Items) != 3 {
		t.Fatalf("list: total=%d len=%d", page.Total, len(page.Items))
	}
	if page.Items[0].ID != 3 {
		t.Errorf("not newest-first: first id = %d, want 3", page.Items[0].ID)
	}
	if page.Items[0].ActorName != "编辑" {
		t.Errorf("actorName not enriched: %q", page.Items[0].ActorName)
	}

	// Action filter.
	rec = call(router, http.MethodGet, "/api/v1/admin/audit-logs?action=article_create", adminToken)
	_ = json.Unmarshal(rec.Body.Bytes(), &page)
	if page.Total != 1 || page.Items[0].ID != 2 {
		t.Errorf("action filter: total=%d id=%d", page.Total, page.Items[0].ID)
	}

	// Time window where only entry 2 qualifies.
	rec = call(router, http.MethodGet, "/api/v1/admin/audit-logs?from=2026-10-01T08:05:00Z&to=2026-10-01T08:15:00Z", adminToken)
	_ = json.Unmarshal(rec.Body.Bytes(), &page)
	if page.Total != 1 || page.Items[0].ID != 2 {
		t.Errorf("time window: total=%d", page.Total)
	}

	// Bad timestamp -> 400.
	rec = call(router, http.MethodGet, "/api/v1/admin/audit-logs?from=not-a-date", adminToken)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad from: status = %d, want 400", rec.Code)
	}

	// Empty result.
	rec = call(router, http.MethodGet, "/api/v1/admin/audit-logs?action=no_such", adminToken)
	_ = json.Unmarshal(rec.Body.Bytes(), &page)
	if page.Total != 0 {
		t.Errorf("empty result: total = %d, want 0", page.Total)
	}
}

func TestAuditLogsPagination(t *testing.T) {
	users := newRBACUsers()
	seedRBACUsers(users)
	audit := newTFakeAudit()
	base := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	for i := 0; i < 12; i++ {
		_ = audit.Insert(context.Background(), &domain.AuditLog{
			ID: int64(i + 1), Actor: auditAdminID, Action: "login",
			ResourceType: "user", CreatedAt: base.Add(time.Duration(i) * time.Minute),
		})
	}
	router := auditRouter(t, users, audit)
	adminToken := rbacToken(t, auditAdminID, domain.RoleAdmin)

	// Defaults page=1 pageSize=10.
	rec := call(router, http.MethodGet, "/api/v1/admin/audit-logs", adminToken)
	var page struct {
		Total    int64 `json:"total"`
		Page     int   `json:"page"`
		PageSize int   `json:"pageSize"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &page)
	if page.Total != 12 || page.Page != 1 || page.PageSize != 10 {
		t.Errorf("defaults: total=%d page=%d pageSize=%d", page.Total, page.Page, page.PageSize)
	}

	// Page 2 bound.
	rec = call(router, http.MethodGet, "/api/v1/admin/audit-logs?page=2", adminToken)
	_ = json.Unmarshal(rec.Body.Bytes(), &page)
	if page.Page != 2 || page.Total != 12 {
		t.Errorf("page 2: total=%d page=%d", page.Total, page.Page)
	}

	// pageSize beyond cap -> 400 (contract max 100).
	rec = call(router, http.MethodGet, "/api/v1/admin/audit-logs?pageSize=999", adminToken)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("pageSize=999: status = %d, want 400", rec.Code)
	}
}
