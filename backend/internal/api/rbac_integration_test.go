package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"news-admin/backend/internal/api/authorize"
	"news-admin/backend/internal/auth"
	"news-admin/backend/internal/config"
	"news-admin/backend/internal/domain"
)

// rbacUserRepo is a minimal in-memory user store for RBAC tests.
type rbacUserRepo struct {
	mu    map[string]*domain.User // by id
	byUid map[string]*domain.User // by username
}

func newRBACUsers() *rbacUserRepo {
	return &rbacUserRepo{mu: map[string]*domain.User{}, byUid: map[string]*domain.User{}}
}

func (r *rbacUserRepo) FindByUsername(_ context.Context, username string) (*domain.User, error) {
	if u, ok := r.byUid[username]; ok {
		return u, nil
	}
	return nil, domain.ErrNotFound
}

func (r *rbacUserRepo) FindByID(_ context.Context, id string) (*domain.User, error) {
	if u, ok := r.mu[id]; ok {
		return u, nil
	}
	return nil, domain.ErrNotFound
}

func (r *rbacUserRepo) UpdatePassword(_ context.Context, id, hash string, at time.Time) error {
	if u, ok := r.mu[id]; ok {
		u.PasswordHash = hash
		u.MustChangePassword = false
		u.PasswordChangedAt = &at
		return nil
	}
	return domain.ErrNotFound
}

func (r *rbacUserRepo) add(u *domain.User) {
	r.mu[u.ID] = u
	r.byUid[u.Username] = u
}

func (r *rbacUserRepo) Create(_ context.Context, in *domain.UserInput, now time.Time) (*domain.User, error) {
	for _, u := range r.mu {
		if u.Username == in.Username {
			return nil, domain.ErrUsernameTaken
		}
	}
	u := &domain.User{
		ID:                 "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb" + fmt.Sprintf("%d", len(r.mu)+1),
		Username:           in.Username,
		PasswordHash:       in.PasswordHash,
		DisplayName:        in.DisplayName,
		Role:               in.Role,
		Status:             domain.UserStatusActive,
		MustChangePassword: true,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	r.mu[u.ID] = u
	r.byUid[u.Username] = u
	return u, nil
}

func (r *rbacUserRepo) Update(_ context.Context, id string, in *domain.UserUpdateInput, now time.Time) (*domain.User, error) {
	u, ok := r.mu[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	if in.DisplayName != nil {
		u.DisplayName = *in.DisplayName
	}
	if in.Role != nil {
		u.Role = *in.Role
	}
	u.UpdatedAt = now
	return u, nil
}

func (r *rbacUserRepo) SetStatus(_ context.Context, id, status string, now time.Time) (*domain.User, error) {
	u, ok := r.mu[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	u.Status = status
	u.UpdatedAt = now
	return u, nil
}

func (r *rbacUserRepo) SetPasswordHash(_ context.Context, id, passwordHash string, now time.Time) error {
	u, ok := r.mu[id]
	if !ok {
		return domain.ErrNotFound
	}
	u.PasswordHash = passwordHash
	u.MustChangePassword = true
	u.PasswordChangedAt = nil
	u.UpdatedAt = now
	return nil
}

func (r *rbacUserRepo) List(_ context.Context, q *domain.UserQuery) (*domain.UserPage, error) {
	var items []*domain.User
	for _, u := range r.mu {
		if q.Role != nil && *q.Role != "" && u.Role != *q.Role {
			continue
		}
		if q.Status != nil && *q.Status != "" && u.Status != *q.Status {
			continue
		}
		if q.Keyword != nil && *q.Keyword != "" &&
			!strings.Contains(strings.ToLower(u.Username), strings.ToLower(*q.Keyword)) &&
			!strings.Contains(strings.ToLower(u.DisplayName), strings.ToLower(*q.Keyword)) {
			continue
		}
		cp := *u
		items = append(items, &cp)
	}
	return &domain.UserPage{Items: items, Total: int64(len(items)), Page: q.Page, PageSize: q.PageSize}, nil
}

func rbacRouter(t *testing.T, users *rbacUserRepo, allMustChange bool) http.Handler {
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
		Secret: cfg.JWTSecret,
		Users:  users,
	})
}

func rbacToken(t *testing.T, userID, role string) string {
	t.Helper()
	tok, err := auth.SignAccessToken("rbac-test-secret", userID, role, time.Now())
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return tok
}

// call performs one authenticated request against the router.
func call(h http.Handler, method, path, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func seedRBACUsers(users *rbacUserRepo) {
	users.add(&domain.User{ID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaa1", Username: "admin", Role: domain.RoleAdmin, Status: domain.UserStatusActive})
	users.add(&domain.User{ID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaa2", Username: "editor", Role: domain.RoleEditor, Status: domain.UserStatusActive})
	users.add(&domain.User{ID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaa3", Username: "reviewer", Role: domain.RoleReviewer, Status: domain.UserStatusActive})
}

func TestRBACUnauthenticatedIs401(t *testing.T) {
	users := newRBACUsers()
	seedRBACUsers(users)
	router := rbacRouter(t, users, false)

	rec := call(router, http.MethodGet, "/api/v1/admin/articles", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no token: status = %d, want 401", rec.Code)
	}
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	if env.Error.Code != "UNAUTHENTICATED" {
		t.Errorf("error.code = %q, want UNAUTHENTICATED", env.Error.Code)
	}
}

func TestRBACAdminCanCallEverything(t *testing.T) {
	users := newRBACUsers()
	seedRBACUsers(users)
	router := rbacRouter(t, users, false)
	token := rbacToken(t, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaa1", domain.RoleAdmin)

	// Every route in the matrix must reach the placeholder handler (501),
	// never be blocked by permission checks (403).
	for _, route := range authorize.AdminRoutes {
		rec := call(router, route.Method, "/api/v1/admin"+route.Path, token)
		if rec.Code == http.StatusForbidden {
			t.Errorf("admin blocked on %s %s with permission %s", route.Method, route.Path, route.Permission)
		}
		if rec.Code != http.StatusNotImplemented {
			t.Errorf("admin on %s %s: status = %d, want 501 placeholder", route.Method, route.Path, rec.Code)
		}
	}
}

func TestRBACEditorCannotApprove(t *testing.T) {
	users := newRBACUsers()
	seedRBACUsers(users)
	router := rbacRouter(t, users, false)
	token := rbacToken(t, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaa2", domain.RoleEditor)

	for _, path := range []string{"/api/v1/admin/articles/x/approve", "/api/v1/admin/articles/x/reject", "/api/v1/admin/articles/x/unpublish"} {
		rec := call(router, http.MethodPost, path, token)
		if rec.Code != http.StatusForbidden {
			t.Errorf("editor on %s: status = %d, want 403", path, rec.Code)
		}
	}
	if rec := call(router, http.MethodPut, "/api/v1/admin/articles/x/pin", token); rec.Code != http.StatusForbidden {
		t.Errorf("editor on PUT pin: status = %d, want 403", rec.Code)
	}
	// Edit (PUT) and submit are allowed for editor on placeholder.
	if rec := call(router, http.MethodPut, "/api/v1/admin/articles/x", token); rec.Code == http.StatusForbidden {
		t.Error("editor should be able to update its own articles")
	}
	if rec := call(router, http.MethodPost, "/api/v1/admin/articles/x/submit", token); rec.Code == http.StatusForbidden {
		t.Error("editor should be able to submit articles")
	}
}

func TestRBACReviewerCannotEditContent(t *testing.T) {
	users := newRBACUsers()
	seedRBACUsers(users)
	router := rbacRouter(t, users, false)
	token := rbacToken(t, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaa3", domain.RoleReviewer)

	// Editing or submitting content is not in the reviewer role.
	if rec := call(router, http.MethodPut, "/api/v1/admin/articles/x", token); rec.Code != http.StatusForbidden {
		t.Errorf("reviewer on PUT /articles/x: status = %d, want 403", rec.Code)
	}
	if rec := call(router, http.MethodPost, "/api/v1/admin/articles/x/submit", token); rec.Code != http.StatusForbidden {
		t.Errorf("reviewer on POST /articles/x/submit: status = %d, want 403", rec.Code)
	}
	// Approve allowed for reviewer.
	if rec := call(router, http.MethodPost, "/api/v1/admin/articles/x/approve", token); rec.Code == http.StatusForbidden {
		t.Error("reviewer blocked on approve")
	}
}

func TestRBACDisabledUserRejected(t *testing.T) {
	users := newRBACUsers()
	seedRBACUsers(users)
	users.mu["aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaa1"].Status = domain.UserStatusDisabled
	router := rbacRouter(t, users, false)
	token := rbacToken(t, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaa1", domain.RoleAdmin)

	rec := call(router, http.MethodGet, "/api/v1/admin/articles", token)
	if rec.Code != http.StatusForbidden {
		t.Errorf("disabled admin: status = %d, want 403", rec.Code)
	}
}

func TestRBACMustChangePasswordBlocksBusiness(t *testing.T) {
	users := newRBACUsers()
	seedRBACUsers(users)
	users.mu["aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaa2"].MustChangePassword = true
	router := rbacRouter(t, users, false)
	token := rbacToken(t, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaa2", domain.RoleEditor)

	// Even though editor can update, mustChangePassword forces 403 on business.
	rec := call(router, http.MethodPut, "/api/v1/admin/articles/x", token)
	if rec.Code != http.StatusForbidden {
		t.Errorf("mustChangePassword on business: status = %d, want 403", rec.Code)
	}
}

func TestRolePermissionMatrix(t *testing.T) {
	admin := domain.PermissionsFor(domain.RoleAdmin)
	for _, p := range domain.ArticlePermissions {
		if !slicesContains(admin, p) {
			t.Errorf("admin missing article permission %s", p)
		}
	}
	for _, p := range []string{domain.PermCategoriesManage, domain.PermUsersManage, domain.PermAuditRead} {
		if !slicesContains(admin, p) {
			t.Errorf("admin missing %s", p)
		}
	}
	editor := domain.PermissionsFor(domain.RoleEditor)
	for _, forbidden := range []string{domain.PermArticleApprove, domain.PermArticleReject, domain.PermArticleUnpublish, domain.PermArticlePin, domain.PermCategoriesManage} {
		if slicesContains(editor, forbidden) {
			t.Errorf("editor should not have %s", forbidden)
		}
	}
	reviewer := domain.PermissionsFor(domain.RoleReviewer)
	for _, forbidden := range []string{domain.PermArticleCreate, domain.PermArticleUpdate, domain.PermArticleSoftDelete, domain.PermUsersManage} {
		if slicesContains(reviewer, forbidden) {
			t.Errorf("reviewer should not have %s", forbidden)
		}
	}
	if len(domain.PermissionsFor(domain.RoleOperator)) != 0 {
		t.Error("operator should be empty in M0")
	}
}

func slicesContains(s []string, want string) bool {
	for _, v := range s {
		if v == want {
			return true
		}
	}
	return false
}
