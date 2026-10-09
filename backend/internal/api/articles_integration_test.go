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

func (f *faArticleRepo) Transition(_ context.Context, id, from, to string, extra map[string]any, actorID string, now time.Time) (*domain.Article, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a, ok := f.articles[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	if from == to || a.Status != from {
		return nil, domain.ErrIllegalTransition
	}
	a.Status = to
	for k, v := range extra {
		switch k {
		case "submitted_at":
			at := v.(time.Time)
			a.SubmittedAt = &at
		case "published_at":
			at := v.(time.Time)
			a.PublishedAt = &at
		case "unpublished_at":
			at := v.(time.Time)
			a.UnpublishedAt = &at
		case "reject_reason":
			reason := v.(string)
			a.RejectReason = &reason
		case "rejected_at":
			at := v.(time.Time)
			a.RejectedAt = &at
		}
	}
	a.Version++
	actor := actorID
	a.UpdatedBy = &actor
	a.UpdatedAt = now
	cp := *a
	return &cp, nil
}

func (f *faArticleRepo) SetPinned(_ context.Context, id string, pinned bool, actorID string, now time.Time) (*domain.Article, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a, ok := f.articles[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	if a.Status != domain.ArticleStatusPublished {
		return nil, domain.ErrIllegalTransition
	}
	a.Pinned = pinned
	if pinned {
		a.PinnedAt = &now
	} else {
		a.PinnedAt = nil
	}
	a.Version++
	actor := actorID
	a.UpdatedBy = &actor
	a.UpdatedAt = now
	cp := *a
	return &cp, nil
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

func (f *faArticleRepo) ListPublic(_ context.Context, q *domain.PublicArticleQuery) (*domain.ArticlePage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.publicPageLocked(q)
}

func (f *faArticleRepo) SearchPublic(_ context.Context, q *domain.PublicArticleQuery) (*domain.ArticlePage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.publicPageLocked(q)
}

func (f *faArticleRepo) publicPageLocked(q *domain.PublicArticleQuery) (*domain.ArticlePage, error) {
	var items []*domain.Article
	for _, a := range f.articles {
		if a.Status != domain.ArticleStatusPublished {
			continue
		}
		if q.CategoryID != nil && *q.CategoryID != "" && a.CategoryID != *q.CategoryID {
			continue
		}
		if q.Keyword != nil && *q.Keyword != "" {
			kw := strings.ToLower(*q.Keyword)
			if !strings.Contains(strings.ToLower(a.Title), kw) &&
				!strings.Contains(strings.ToLower(a.Summary), kw) &&
				!strings.Contains(strings.ToLower(a.BodyText), kw) {
				continue
			}
		}
		cp := *a
		items = append(items, &cp)
	}
	return &domain.ArticlePage{Items: items, Total: int64(len(items)), Page: q.Page, PageSize: q.PageSize}, nil
}

func (f *faArticleRepo) FindPublic(_ context.Context, id string) (*domain.Article, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a, ok := f.articles[id]
	if !ok || a.Status != domain.ArticleStatusPublished {
		return nil, domain.ErrNotFound
	}
	cp := *a
	return &cp, nil
}

func articlesRouter(t *testing.T, users *rbacUserRepo, cats *fkCategoryRepo, arts *faArticleRepo) http.Handler {
	t.Helper()
	return articlesRouterWithAudit(t, users, cats, arts, newTFakeAudit())
}

func articlesRouterWithAudit(t *testing.T, users *rbacUserRepo, cats *fkCategoryRepo, arts *faArticleRepo, audit *tFakeAudit) http.Handler {
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
		Audit:      audit,
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

// createArticleVia posts an article and returns its id.
func createArticleVia(t *testing.T, router http.Handler, token string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/articles", articlePayload("标题", "<p>正文</p>", nil))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	return created.ID
}

// postWorkflow performs one workflow POST and returns the decoded article
// plus the raw status code.
func postWorkflow(t *testing.T, router http.Handler, token, path string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin"+path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return rec.Code, body
}

func seedWorkflowUsers(users *rbacUserRepo) {
	seedRBACUsers(users) // admin/editor/reviewer
}

func TestArticleWorkflowLifecycleViaHTTP(t *testing.T) {
	users := newRBACUsers()
	seedWorkflowUsers(users)
	cats := newFkCategoryRepo()
	cats.add("cat-1", "技术", "tech", 0)
	arts := newFaArticleRepo()
	router := articlesRouter(t, users, cats, arts)

	editorID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaa2"
	reviewerID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaa3"
	editorToken := rbacToken(t, editorID, domain.RoleEditor)
	reviewerToken := rbacToken(t, reviewerID, domain.RoleReviewer)

	id := createArticleVia(t, router, editorToken)

	// submit (author) → pending_review.
	code, pending := postWorkflow(t, router, editorToken, "/articles/"+id+"/submit")
	if code != http.StatusOK || pending["status"] != "pending_review" {
		t.Fatalf("submit: code=%d body=%v", code, pending)
	}
	if pending["submittedAt"] == nil {
		t.Error("submit did not stamp submittedAt")
	}

	// approve (reviewer) → published.
	code, published := postWorkflow(t, router, reviewerToken, "/articles/"+id+"/approve")
	if code != http.StatusOK || published["status"] != "published" {
		t.Fatalf("approve: code=%d body=%v", code, published)
	}
	if published["publishedAt"] == nil {
		t.Error("approve did not stamp publishedAt")
	}

	// pin then unpin (reviewer).
	pinBody, _ := json.Marshal(map[string]any{"pinned": true})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/articles/"+id+"/pin", bytes.NewReader(pinBody))
	req.Header.Set("Authorization", "Bearer "+reviewerToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("pin: status = %d, body=%s", rec.Code, rec.Body.String())
	}

	// unpublish (reviewer) → unpublished.
	code, unpublished := postWorkflow(t, router, reviewerToken, "/articles/"+id+"/unpublish")
	if code != http.StatusOK || unpublished["status"] != "unpublished" {
		t.Fatalf("unpublish: code=%d body=%v", code, unpublished)
	}
	if unpublished["unpublishedAt"] == nil {
		t.Error("unpublish did not stamp unpublishedAt")
	}

	// re-submit from unpublished (author) → pending_review.
	code, repending := postWorkflow(t, router, editorToken, "/articles/"+id+"/submit")
	if code != http.StatusOK || repending["status"] != "pending_review" {
		t.Fatalf("re-submit: code=%d body=%v", code, repending)
	}

	// reject (reviewer) with reason → draft + rejectReason/rejectedAt.
	rejectBody, _ := json.Marshal(map[string]any{"reason": "缺少来源标注"})
	req = httptest.NewRequest(http.MethodPost, "/api/v1/admin/articles/"+id+"/reject", bytes.NewReader(rejectBody))
	req.Header.Set("Authorization", "Bearer "+reviewerToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("reject: status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var rejected struct {
		Status       string  `json:"status"`
		RejectReason *string `json:"rejectReason"`
		RejectedAt   any     `json:"rejectedAt"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &rejected)
	if rejected.Status != "draft" || rejected.RejectReason == nil || *rejected.RejectReason != "缺少来源标注" || rejected.RejectedAt == nil {
		t.Errorf("reject result = %+v", rejected)
	}
}

func TestArticleWorkflowRejectRequiresReasonViaHTTP(t *testing.T) {
	users := newRBACUsers()
	seedWorkflowUsers(users)
	cats := newFkCategoryRepo()
	cats.add("cat-1", "技术", "tech", 0)
	router := articlesRouter(t, users, cats, newFaArticleRepo())
	editorToken := rbacToken(t, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaa2", domain.RoleEditor)
	reviewerToken := rbacToken(t, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaa3", domain.RoleReviewer)

	id := createArticleVia(t, router, editorToken)
	postWorkflow(t, router, editorToken, "/articles/"+id+"/submit")

	// Missing reason → 400 validation.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/articles/"+id+"/reject", nil)
	req.Header.Set("Authorization", "Bearer "+reviewerToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("reject without body: status = %d, want 400", rec.Code)
	}
	emptyBody, _ := json.Marshal(map[string]any{"reason": ""})
	req = httptest.NewRequest(http.MethodPost, "/api/v1/admin/articles/"+id+"/reject", bytes.NewReader(emptyBody))
	req.Header.Set("Authorization", "Bearer "+reviewerToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("reject with empty reason: status = %d, want 400", rec.Code)
	}
}

func TestArticleWorkflowIllegalTransitionsViaHTTP(t *testing.T) {
	users := newRBACUsers()
	seedWorkflowUsers(users)
	cats := newFkCategoryRepo()
	cats.add("cat-1", "技术", "tech", 0)
	router := articlesRouter(t, users, cats, newFaArticleRepo())
	editorToken := rbacToken(t, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaa2", domain.RoleEditor)
	reviewerToken := rbacToken(t, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaa3", domain.RoleReviewer)

	id := createArticleVia(t, router, editorToken)

	// approving a fresh draft directly → 409.
	code, _ := postWorkflow(t, router, reviewerToken, "/articles/"+id+"/approve")
	if code != http.StatusConflict {
		t.Errorf("approve draft: status = %d, want 409", code)
	}
	// unpublishing a fresh draft directly → 409.
	code, _ = postWorkflow(t, router, reviewerToken, "/articles/"+id+"/unpublish")
	if code != http.StatusConflict {
		t.Errorf("unpublish draft: status = %d, want 409", code)
	}
	// reject a fresh draft directly → 409.
	rejectBody, _ := json.Marshal(map[string]any{"reason": "r"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/articles/"+id+"/reject", bytes.NewReader(rejectBody))
	req.Header.Set("Authorization", "Bearer "+reviewerToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Errorf("reject draft: status = %d, want 409", rec.Code)
	}

	// publish it, then submit again → 409 (published can not re-submit).
	postWorkflow(t, router, editorToken, "/articles/"+id+"/submit")
	postWorkflow(t, router, reviewerToken, "/articles/"+id+"/approve")
	code, _ = postWorkflow(t, router, editorToken, "/articles/"+id+"/submit")
	if code != http.StatusConflict {
		t.Errorf("submit published: status = %d, want 409", code)
	}
}

func TestArticleSubmitIncompleteViaHTTP(t *testing.T) {
	users := newRBACUsers()
	seedWorkflowUsers(users)
	cats := newFkCategoryRepo()
	cats.add("cat-1", "技术", "tech", 0)
	router := articlesRouter(t, users, cats, newFaArticleRepo())
	editorToken := rbacToken(t, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaa2", domain.RoleEditor)

	// Create a draft without a summary, then submit → 422.
	payload, _ := json.Marshal(map[string]any{"title": "标题", "summary": "", "bodyHtml": "<p>正文</p>", "categoryId": "cat-1"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/articles", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+editorToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	var created struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &created)

	code, _ := postWorkflow(t, router, editorToken, "/articles/"+created.ID+"/submit")
	if code != http.StatusUnprocessableEntity {
		t.Errorf("submit incomplete: status = %d, want 422", code)
	}
}

func TestArticleWorkflowRBACViaHTTP(t *testing.T) {
	users := newRBACUsers()
	seedWorkflowUsers(users)
	cats := newFkCategoryRepo()
	cats.add("cat-1", "技术", "tech", 0)
	router := articlesRouter(t, users, cats, newFaArticleRepo())
	editorToken := rbacToken(t, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaa2", domain.RoleEditor)

	id := createArticleVia(t, router, editorToken)

	// editor cannot approve/reject/unpublish/pin → 403.
	for _, path := range []string{
		"/articles/" + id + "/approve",
		"/articles/" + id + "/unpublish",
	} {
		code, _ := postWorkflow(t, router, editorToken, path)
		if code != http.StatusForbidden {
			t.Errorf("editor on %s: status = %d, want 403", path, code)
		}
	}
	rejectBody, _ := json.Marshal(map[string]any{"reason": "r"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/articles/"+id+"/reject", bytes.NewReader(rejectBody))
	req.Header.Set("Authorization", "Bearer "+editorToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("editor reject: status = %d, want 403", rec.Code)
	}
	pinBody, _ := json.Marshal(map[string]any{"pinned": true})
	req = httptest.NewRequest(http.MethodPut, "/api/v1/admin/articles/"+id+"/pin", bytes.NewReader(pinBody))
	req.Header.Set("Authorization", "Bearer "+editorToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("editor pin: status = %d, want 403", rec.Code)
	}

	// editor cannot submit an article it did not create → 403.
	adminToken := rbacToken(t, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaa1", domain.RoleAdmin)
	otherID := createArticleVia(t, router, adminToken)
	code, _ := postWorkflow(t, router, editorToken, "/articles/"+otherID+"/submit")
	if code != http.StatusForbidden {
		t.Errorf("editor submit foreign article: status = %d, want 403", code)
	}
}

func TestArticleWorkflowAuditTrail(t *testing.T) {
	users := newRBACUsers()
	seedWorkflowUsers(users)
	cats := newFkCategoryRepo()
	cats.add("cat-1", "技术", "tech", 0)
	arts := newFaArticleRepo()
	audit := newTFakeAudit()
	router := articlesRouterWithAudit(t, users, cats, arts, audit)
	editorToken := rbacToken(t, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaa2", domain.RoleEditor)
	reviewerToken := rbacToken(t, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaa3", domain.RoleReviewer)

	id := createArticleVia(t, router, editorToken)
	postWorkflow(t, router, editorToken, "/articles/"+id+"/submit")
	postWorkflow(t, router, reviewerToken, "/articles/"+id+"/approve")
	postWorkflow(t, router, reviewerToken, "/articles/"+id+"/unpublish")

	got := audit.actions()
	want := map[string]int{"article_submit": 1, "article_approve": 1, "article_unpublish": 1}
	for _, action := range got {
		want[action]--
	}
	for action, left := range want {
		if left > 0 {
			t.Errorf("audit trail missing %s (got %v)", action, got)
		}
	}
}
