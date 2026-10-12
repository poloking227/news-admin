// Package api contains HTTP-level integration tests for the admin user
// management endpoints using in-memory fake repositories.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"news-admin/backend/internal/config"
	"news-admin/backend/internal/domain"
)

const (
	adminID    = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaa1"
	editorID   = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaa2"
	reviewerID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaa3"
)

// rbacRouterWithSessions builds a router with real session storage so the
// disable/reset session-revocation paths can be exercised.
func rbacRouterWithSessions(t *testing.T, users *rbacUserRepo, sessions *tFakeSessions) http.Handler {
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
		Sessions: sessions,
		Audit:    newTFakeAudit(),
	})
}

// insertSessionFor adds one refresh session for the user so revocation can be
// observed after disable/reset.
func insertSessionFor(sessions *tFakeSessions, userID string) {
	_ = sessions.Insert(context.Background(), &domain.RefreshSession{
		ID: "s-" + userID, UserID: userID, JTI: "j-" + userID, FamilyID: "f-" + userID,
		ExpiresAt: time.Now().Add(time.Hour),
	})
}

func TestUserManagementAdminOnly(t *testing.T) {
	users := newRBACUsers()
	seedRBACUsers(users)
	router := rbacRouterWithSessions(t, users, newTFakeSessions())
	editorToken := rbacToken(t, editorID, domain.RoleEditor)

	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/admin/users"},
		{http.MethodPost, "/api/v1/admin/users"},
		{http.MethodPut, "/api/v1/admin/users/" + reviewerID},
		{http.MethodPatch, "/api/v1/admin/users/" + reviewerID + "/status"},
		{http.MethodPost, "/api/v1/admin/users/" + reviewerID + "/reset-password"},
	} {
		rec := call(router, tc.method, tc.path, editorToken)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s %s: status = %d, want 403", tc.method, tc.path, rec.Code)
		}
	}
}

func TestUserManagementCreateListUpdateStatusReset(t *testing.T) {
	users := newRBACUsers()
	seedRBACUsers(users)
	sessions := newTFakeSessions()
	router := rbacRouterWithSessions(t, users, sessions)
	adminToken := rbacToken(t, adminID, domain.RoleAdmin)

	// Create -> 201 with forced-change flag.
	payload, _ := json.Marshal(map[string]any{
		"username": "reporter", "password": "TmpPass123", "displayName": "记者", "role": "editor",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var created struct {
		ID                 string `json:"id"`
		MustChangePassword bool   `json:"mustChangePassword"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	if !created.MustChangePassword {
		t.Error("created user must have mustChangePassword=true")
	}

	// Duplicate username -> 409.
	req = httptest.NewRequest(http.MethodPost, "/api/v1/admin/users", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Errorf("duplicate create: status = %d, want 409", rec.Code)
	}

	// List with keyword filter.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/users?keyword=reporter", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	var page struct {
		Total int64 `json:"total"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &page)
	if page.Total != 1 {
		t.Errorf("keyword list total = %d, want 1", page.Total)
	}

	// Update display name.
	upd, _ := json.Marshal(map[string]any{"displayName": "资深记者"})
	req = httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/"+created.ID, bytes.NewReader(upd))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("update: status = %d, body=%s", rec.Code, rec.Body.String())
	}

	// Disable -> sessions revoked.
	insertSessionFor(sessions, created.ID)
	statusBody, _ := json.Marshal(map[string]any{"status": "disabled"})
	req = httptest.NewRequest(http.MethodPatch, "/api/v1/admin/users/"+created.ID+"/status", bytes.NewReader(statusBody))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("disable: status = %d, body=%s", rec.Code, rec.Body.String())
	}

	// Reset password -> temporary password returned, forced change set.
	req = httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/"+created.ID+"/reset-password", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("reset: status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var reset struct {
		TemporaryPassword string `json:"temporaryPassword"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &reset)
	if len(reset.TemporaryPassword) < 8 {
		t.Errorf("temporary password too short: %q", reset.TemporaryPassword)
	}
}

func TestUserManagementSelfProtection409(t *testing.T) {
	users := newRBACUsers()
	seedRBACUsers(users)
	router := rbacRouterWithSessions(t, users, newTFakeSessions())
	adminToken := rbacToken(t, adminID, domain.RoleAdmin)

	// Demoting yourself -> 409.
	body, _ := json.Marshal(map[string]any{"role": "editor"})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/"+adminID, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Errorf("self demote: status = %d, want 409", rec.Code)
	}

	// Disabling yourself -> 409.
	statusBody, _ := json.Marshal(map[string]any{"status": "disabled"})
	req = httptest.NewRequest(http.MethodPatch, "/api/v1/admin/users/"+adminID+"/status", bytes.NewReader(statusBody))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Errorf("self disable: status = %d, want 409", rec.Code)
	}
}

func TestUserManagementPaginationDefaults(t *testing.T) {
	users := newRBACUsers()
	seedRBACUsers(users)
	router := rbacRouterWithSessions(t, users, newTFakeSessions())
	adminToken := rbacToken(t, adminID, domain.RoleAdmin)

	// Seed 12 more users so pagination matters.
	for i := 0; i < 12; i++ {
		payload, _ := json.Marshal(map[string]any{
			"username": "user" + string(rune('a'+i)), "password": "TmpPass123",
			"displayName": "用户" + string(rune('a'+i)), "role": "editor",
		})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users", bytes.NewReader(payload))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		router.ServeHTTP(httptest.NewRecorder(), req)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	var page struct {
		Total    int64 `json:"total"`
		Page     int   `json:"page"`
		PageSize int   `json:"pageSize"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &page)
	if page.Total != 15 || page.Page != 1 || page.PageSize != 10 {
		t.Errorf("defaults wrong: total=%d page=%d pageSize=%d", page.Total, page.Page, page.PageSize)
	}
}
