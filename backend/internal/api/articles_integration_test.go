// Package api contains HTTP-level integration tests for the article
// endpoints using in-memory fake repositories.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"news-admin/backend/internal/config"
	"news-admin/backend/internal/domain"
)

// faArticleRepo is an in-memory domain.ArticleRepository for handler tests.
type faArticleRepo struct {
	mu       sync.Mutex
	next     int
	articles map[string]*domain.Article
}

func newFaArticleRepo() *faArticleRepo {
	return &faArticleRepo{articles: map[string]*domain.Article{}}
}

func (f *faArticleRepo) Create(_ context.Context, in *domain.ArticleInput, actorID string, now time.Time) (*domain.Article, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.next++
	id := string(rune('a'+f.next)) + "-article"
	actor := actorID
	article := &domain.Article{
		ID: id, Title: in.Title, Summary: in.Summary,
		BodyHTML: in.BodyHTML, BodyText: in.BodyText,
		CategoryID: in.CategoryID, CoverURL: in.CoverURL,
		Status:    domain.ArticleStatusDraft,
		CreatedBy: actorID, CreatedByName: "someone", UpdatedBy: &actor,
		Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	f.articles[id] = article
	cp := *article
	return &cp, nil
}

func (f *faArticleRepo) FindByID(_ context.Context, id string) (*domain.Article, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if a, ok := f.articles[id]; ok {
		cp := *a
		return &cp, nil
	}
	return nil, domain.ErrNotFound
}

func (f *faArticleRepo) Update(_ context.Context, id string, in *domain.ArticleInput, expectedVersion int, actorID string, now time.Time) (*domain.Article, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a, ok := f.articles[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	if a.Status != domain.ArticleStatusDraft {
		return nil, domain.ErrArticleNotEditable
	}
	if a.Version != expectedVersion {
		return nil, domain.ErrVersionConflict
	}
	if in.Title != "" {
		a.Title = in.Title
	}
	if in.Summary != "" {
		a.Summary = in.Summary
	}
	if in.BodyHTML != "" {
		a.BodyHTML = in.BodyHTML
		a.BodyText = in.BodyText
	}
	if in.CoverURL != nil {
		a.CoverURL = in.CoverURL
	}
	a.Version++
	actor := actorID
	a.UpdatedBy = &actor
	cp := *a
	return &cp, nil
}

func (f *faArticleRepo) SoftDelete(_ context.Context, id string, _ time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	a, ok := f.articles[id]
	if !ok {
		return domain.ErrNotFound
	}
	if a.Status == domain.ArticleStatusPublished {
		return domain.ErrArticlePublished
	}
	if a.Status != domain.ArticleStatusDraft {
		return domain.ErrArticleNotEditable
	}
	delete(f.articles, id)
	return nil
}

func (f *faArticleRepo) List(_ context.Context, q *domain.ArticleQuery) (*domain.ArticlePage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var items []*domain.Article
	for _, a := range f.articles {
		if q.Status != nil && a.Status != *q.Status {
			continue
		}
		if q.Role == domain.RoleEditor && a.CreatedBy != q.UserID && a.Status != domain.ArticleStatusPublished {
			continue
		}
		cp := *a
		items = append(items, &cp)
	}
	return &domain.ArticlePage{Items: items, Total: int64(len(items)), Page: q.Page, PageSize: q.PageSize}, nil
}

func articlesRouter(t *testing.T, users *rbacUserRepo, cats *fkCategoryRepo, arts *faArticleRepo) http.Handler {
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
		Secret:     cfg.JWTSecret,
		Users:      users,
		Sessions:   newTFakeSessions(),
		Audit:      newTFakeAudit(),
		Categories: cats,
		Articles:   arts,
	})
}

func seedArticleUsers(users *rbacUserRepo) string {
	users.add(&domain.User{ID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaa1", Username: "admin", Role: domain.RoleAdmin, Status: domain.UserStatusActive})
	users.add(&domain.User{ID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaa2", Username: "editor", Role: domain.RoleEditor, Status: domain.UserStatusActive})
	return "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaa1"
}

func articlePayload(title, body string, cover *string) *bytes.Reader {
	payload := map[string]any{"title": title, "summary": "摘要", "bodyHtml": body, "categoryId": "cat-1"}
	if cover != nil {
		payload["coverUrl"] = *cover
	}
	b, _ := json.Marshal(payload)
	return bytes.NewReader(b)
}

func TestArticleCreateGetUpdateDeleteViaHTTP(t *testing.T) {
	users := newRBACUsers()
	adminID := seedArticleUsers(users)
	cats := newFkCategoryRepo()
	cats.add("cat-1", "技术", "tech", 0)
	arts := newFaArticleRepo()
	router := articlesRouter(t, users, cats, arts)
	token := rbacToken(t, adminID, domain.RoleAdmin)

	// Create (with XSS payload — should be sanitized).
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/articles", articlePayload("标题", "<p>正文</p><script>alert(1)</script>", nil))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var created struct {
		ID       string `json:"id"`
		Status   string `json:"status"`
		Version  int    `json:"version"`
		BodyHTML string `json:"bodyHtml"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	if created.Status != "draft" || created.Version != 1 {
		t.Fatalf("created = %+v, want draft v1", created)
	}
	if created.BodyHTML != "<p>正文</p>" {
		t.Errorf("body not sanitized: %q", created.BodyHTML)
	}

	// Get.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/articles/"+created.ID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get: status = %d", rec.Code)
	}

	// Update with correct If-Match → 200, version bump.
	body, _ := json.Marshal(map[string]any{"title": "新标题"})
	req = httptest.NewRequest(http.MethodPut, "/api/v1/admin/articles/"+created.ID, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("If-Match", "1")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("update: status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var updated struct {
		Title   string `json:"title"`
		Version int    `json:"version"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &updated)
	if updated.Title != "新标题" || updated.Version != 2 {
		t.Errorf("updated = %+v, want title 新标题 version 2", updated)
	}

	// Delete → 204.
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/admin/articles/"+created.ID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete: status = %d, want 204", rec.Code)
	}
}

func TestArticleVersionConflictAndMissingIfMatch(t *testing.T) {
	users := newRBACUsers()
	adminID := seedArticleUsers(users)
	cats := newFkCategoryRepo()
	cats.add("cat-1", "技术", "tech", 0)
	arts := newFaArticleRepo()
	router := articlesRouter(t, users, cats, arts)
	token := rbacToken(t, adminID, domain.RoleAdmin)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/articles", articlePayload("t", "<p>x</p>", nil))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	var created struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &created)

	// No If-Match → 400.
	body, _ := json.Marshal(map[string]any{"title": "x"})
	req = httptest.NewRequest(http.MethodPut, "/api/v1/admin/articles/"+created.ID, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("update without If-Match: status = %d, want 400", rec.Code)
	}

	// Correct version bumps; stale/bogus versions conflict.
	req = httptest.NewRequest(http.MethodPut, "/api/v1/admin/articles/"+created.ID, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("If-Match", "1")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("update If-Match=1 first time: status = %d, want 200", rec.Code)
	}
	for _, version := range []string{"1", "99"} {
		req = httptest.NewRequest(http.MethodPut, "/api/v1/admin/articles/"+created.ID, bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("If-Match", version)
		rec = httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusConflict {
			t.Errorf("update If-Match=%s: status = %d, want 409", version, rec.Code)
		}
	}
}

func TestArticleInvalidCoverRejected(t *testing.T) {
	users := newRBACUsers()
	adminID := seedArticleUsers(users)
	cats := newFkCategoryRepo()
	cats.add("cat-1", "技术", "tech", 0)
	router := articlesRouter(t, users, cats, newFaArticleRepo())
	token := rbacToken(t, adminID, domain.RoleAdmin)

	bad := "javascript:alert(1)"
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/articles", articlePayload("t", "<p>x</p>", &bad))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad cover: status = %d, want 400", rec.Code)
	}
}

func TestArticleListPaginationDefaults(t *testing.T) {
	users := newRBACUsers()
	adminID := seedArticleUsers(users)
	cats := newFkCategoryRepo()
	cats.add("cat-1", "技术", "tech", 0)
	arts := newFaArticleRepo()
	router := articlesRouter(t, users, cats, arts)
	token := rbacToken(t, adminID, domain.RoleAdmin)

	for range 3 {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/articles", articlePayload("t", "<p>x</p>", nil))
		req.Header.Set("Authorization", "Bearer "+token)
		router.ServeHTTP(httptest.NewRecorder(), req)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/articles", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	var page struct {
		Items    []any `json:"items"`
		Total    int64 `json:"total"`
		Page     int   `json:"page"`
		PageSize int   `json:"pageSize"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &page)
	if page.Total != 3 || page.Page != 1 || page.PageSize != 10 {
		t.Errorf("defaults wrong: %+v", page)
	}

	// Status filter draft.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/articles?status=draft", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	_ = json.Unmarshal(rec.Body.Bytes(), &page)
	if page.Total != 3 {
		t.Errorf("draft filter total = %d, want 3", page.Total)
	}
}

func TestArticleXSSStrippedAtAPI(t *testing.T) {
	users := newRBACUsers()
	adminID := seedArticleUsers(users)
	cats := newFkCategoryRepo()
	cats.add("cat-1", "技术", "tech", 0)
	router := articlesRouter(t, users, cats, newFaArticleRepo())
	token := rbacToken(t, adminID, domain.RoleAdmin)

	payload, _ := json.Marshal(map[string]any{
		"title": "xss", "summary": "s",
		"bodyHtml":   `<p onclick="alert(1)">safe</p><img src=x onerror=alert(1)>`,
		"categoryId": "cat-1",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/articles", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	var created struct {
		BodyHTML string `json:"bodyHtml"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	if created.BodyHTML != "<p>safe</p>" {
		t.Errorf("XSS not stripped: %q", created.BodyHTML)
	}
	if strings.Contains(created.BodyHTML, "onerror") || strings.Contains(created.BodyHTML, "onclick") {
		t.Errorf("event handlers survived: %q", created.BodyHTML)
	}
}

func TestArticleEditorCannotSeeOthersDrafts(t *testing.T) {
	users := newRBACUsers()
	seedArticleUsers(users)
	cats := newFkCategoryRepo()
	cats.add("cat-1", "技术", "tech", 0)
	arts := newFaArticleRepo()
	router := articlesRouter(t, users, cats, arts)
	adminToken := rbacToken(t, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaa1", domain.RoleAdmin)

	// admin creates 2 drafts, editor creates 1.
	for range 2 {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/articles", articlePayload("t", "<p>x</p>", nil))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		router.ServeHTTP(httptest.NewRecorder(), req)
	}
	editorToken := rbacToken(t, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaa2", domain.RoleEditor)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/articles", articlePayload("mine", "<p>x</p>", nil))
	req.Header.Set("Authorization", "Bearer "+editorToken)
	router.ServeHTTP(httptest.NewRecorder(), req)

	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/articles", nil)
	req.Header.Set("Authorization", "Bearer "+editorToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	var page struct {
		Total int64 `json:"total"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &page)
	if page.Total != 1 {
		t.Errorf("editor sees %d articles, want 1 (own draft)", page.Total)
	}
}

func TestArticleUnauthenticated401(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/articles", nil)
	rec := httptest.NewRecorder()
	router := articlesRouter(t, nil, nil, nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("no token: status = %d, want 401", rec.Code)
	}
}
