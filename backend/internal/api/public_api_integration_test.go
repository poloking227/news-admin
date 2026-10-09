// Package api contains HTTP-level integration tests for the anonymous public
// reader endpoints: published-only listing, hidden-state 404, and search.
package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"news-admin/backend/internal/domain"
)

// seedPublicArticle creates and approves an article through the API, then
// returns its id. The author must be able to submit and a reviewer to approve.
func seedPublicArticle(t *testing.T, router http.Handler, catID string, title, summary, body string) string {
	t.Helper()
	editorToken := rbacToken(t, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaa2", domain.RoleEditor)
	reviewerToken := rbacToken(t, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaa3", domain.RoleReviewer)

	payload, _ := json.Marshal(map[string]any{
		"title": title, "summary": summary, "bodyHtml": "<p>" + body + "</p>", "categoryId": catID,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/articles", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+editorToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &created)

	code, _ := postWorkflow(t, router, editorToken, "/articles/"+created.ID+"/submit")
	if code != http.StatusOK {
		t.Fatalf("submit: status = %d", code)
	}
	code, _ = postWorkflow(t, router, reviewerToken, "/articles/"+created.ID+"/approve")
	if code != http.StatusOK {
		t.Fatalf("approve: status = %d", code)
	}
	return created.ID
}

func TestPublicListOnlyPublishedAnonymous(t *testing.T) {
	users := newRBACUsers()
	seedRBACUsers(users)
	cats := newFkCategoryRepo()
	cats.add("cat-1", "技术", "tech", 0)
	router := articlesRouter(t, users, cats, newFaArticleRepo())

	pubID := seedPublicArticle(t, router, "cat-1", "已发布", "s", "x")

	// A draft stays hidden on the public list.
	editorToken := rbacToken(t, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaa2", domain.RoleEditor)
	payload, _ := json.Marshal(map[string]any{"title": "草稿", "summary": "s", "bodyHtml": "<p>x</p>", "categoryId": "cat-1"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/articles", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+editorToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	var draft struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &draft)

	// Anonymous list: only the published article is visible.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/public/articles", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("public list: status = %d", rec.Code)
	}
	var page struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
		Total int64 `json:"total"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &page)
	if page.Total != 1 || page.Items[0].ID != pubID {
		t.Errorf("public list: total=%d items=%v, want only published", page.Total, page.Items)
	}
	if draft.ID == pubID {
		t.Fatalf("draft id == published id, test setup broken")
	}
}

func TestPublicDetailHiddenStates404(t *testing.T) {
	users := newRBACUsers()
	seedRBACUsers(users)
	cats := newFkCategoryRepo()
	cats.add("cat-1", "技术", "tech", 0)
	router := articlesRouter(t, users, cats, newFaArticleRepo())

	pubID := seedPublicArticle(t, router, "cat-1", "已发布", "s", "x")

	// Published detail is reachable.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/public/articles/"+pubID, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("public detail published: status = %d", rec.Code)
	}

	// Unknown id is 404.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/public/articles/does-not-exist", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("public detail unknown: status = %d, want 404", rec.Code)
	}
}

func TestPublicSearchAndEmptyQ(t *testing.T) {
	users := newRBACUsers()
	seedRBACUsers(users)
	cats := newFkCategoryRepo()
	cats.add("cat-1", "技术", "tech", 0)
	router := articlesRouter(t, users, cats, newFaArticleRepo())

	pubID := seedPublicArticle(t, router, "cat-1", "Gopher 语言", "并发模型", "goroutine channel")

	// Body-text hit.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/public/search?q=goroutine", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("search: status = %d", rec.Code)
	}
	var page struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
		Total int64 `json:"total"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &page)
	if page.Total != 1 || page.Items[0].ID != pubID {
		t.Errorf("search hit: total=%d items=%v", page.Total, page.Items)
	}

	// Title hit with different case.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/public/search?q=gopHer", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	_ = json.Unmarshal(rec.Body.Bytes(), &page)
	if page.Total != 1 {
		t.Errorf("search case-insensitive: total=%d, want 1", page.Total)
	}

	// No hit → empty page.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/public/search?q=zzz-no-such-term", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	_ = json.Unmarshal(rec.Body.Bytes(), &page)
	if page.Total != 0 {
		t.Errorf("search no-hit: total=%d, want 0", page.Total)
	}

	// Empty q → 400 (contract minLength 1 on q).
	for _, q := range []string{"/api/v1/public/search", "/api/v1/public/search?q="} {
		req = httptest.NewRequest(http.MethodGet, q, nil)
		rec = httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("search empty q %q: status = %d, want 400", q, rec.Code)
		}
	}
}

func TestPublicPaginationDefaults(t *testing.T) {
	users := newRBACUsers()
	seedRBACUsers(users)
	cats := newFkCategoryRepo()
	cats.add("cat-1", "技术", "tech", 0)
	router := articlesRouter(t, users, cats, newFaArticleRepo())

	for range 3 {
		seedPublicArticle(t, router, "cat-1", "文章", "s", "body")
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/public/articles", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	var page struct {
		Total    int64 `json:"total"`
		Page     int   `json:"page"`
		PageSize int   `json:"pageSize"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &page)
	if page.Total != 3 || page.Page != 1 || page.PageSize != 10 {
		t.Errorf("defaults wrong: %+v", page)
	}

	// pageSize beyond cap is clamped to the default.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/public/articles?pageSize=999", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	_ = json.Unmarshal(rec.Body.Bytes(), &page)
	if page.PageSize != 10 {
		t.Errorf("pageSize clamp: got %d, want 10", page.PageSize)
	}
}

func TestPublicCategoriesOnlyWithPublished(t *testing.T) {
	users := newRBACUsers()
	seedRBACUsers(users)
	cats := newFkCategoryRepo()
	// count>0 marks a category as having published articles, matching the
	// repository contract of ListPublished.
	cats.add("cat-1", "技术", "tech", 1)
	cats.add("cat-2", "商业", "business", 0)
	router := articlesRouter(t, users, cats, newFaArticleRepo())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/public/categories", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("public categories: status = %d", rec.Code)
	}
	var categories []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &categories)
	seen := map[string]bool{}
	for _, c := range categories {
		seen[c.ID] = true
	}
	if !seen["cat-1"] || seen["cat-2"] {
		t.Errorf("public categories = %v, want only cat-1", categories)
	}
}
