// Package api contains HTTP-level integration tests for the category
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
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"news-admin/backend/internal/config"
	"news-admin/backend/internal/domain"
)

// fkCategoryRepo is an in-memory domain.CategoryRepository for handler tests.
type fkCategoryRepo struct {
	mu     sync.Mutex
	next   int
	byID   map[string]*domain.Category
	bySlug map[string]string
	linked map[string]bool
}

func newFkCategoryRepo() *fkCategoryRepo {
	return &fkCategoryRepo{
		byID:   map[string]*domain.Category{},
		bySlug: map[string]string{},
		linked: map[string]bool{},
	}
}

func (f *fkCategoryRepo) add(id, name, slug string, count int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.next++
	f.byID[id] = &domain.Category{
		ID: id, Name: name, Slug: slug,
		Description:  nil,
		SortOrder:    0,
		ArticleCount: count,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	f.bySlug[slug] = id
	if count > 0 {
		f.linked[id] = true
	}
}

func (f *fkCategoryRepo) List(_ context.Context) ([]*domain.Category, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*domain.Category
	for _, c := range f.byID {
		cp := *c
		out = append(out, &cp)
	}
	return out, nil
}

func (f *fkCategoryRepo) ListPublished(_ context.Context) ([]*domain.Category, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*domain.Category
	for _, c := range f.byID {
		if f.linked[c.ID] {
			cp := *c
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (f *fkCategoryRepo) Create(_ context.Context, in *domain.CategoryInput) (*domain.Category, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.next++
	id := string(rune('a'+f.next)) + "-id"
	now := time.Now()
	f.byID[id] = &domain.Category{
		ID: id, Name: in.Name, Slug: in.Slug,
		Description: in.Description, SortOrder: in.SortOrder,
		CreatedAt: now, UpdatedAt: now,
	}
	f.bySlug[in.Slug] = id
	cp := *f.byID[id]
	return &cp, nil
}

func (f *fkCategoryRepo) Update(_ context.Context, id string, in *domain.CategoryInput) (*domain.Category, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.byID[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	delete(f.bySlug, c.Slug)
	c.Name, c.Slug = in.Name, in.Slug
	c.Description, c.SortOrder = in.Description, in.SortOrder
	c.UpdatedAt = time.Now()
	f.bySlug[in.Slug] = id
	cp := *c
	return &cp, nil
}

func (f *fkCategoryRepo) SoftDelete(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.byID[id]
	if !ok {
		return domain.ErrNotFound
	}
	if f.linked[id] {
		return domain.ErrCategoryInUse
	}
	delete(f.bySlug, c.Slug)
	delete(f.byID, id)
	return nil
}

func (f *fkCategoryRepo) ExistsSlug(_ context.Context, slug, exceptID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id, ok := f.bySlug[slug]
	return ok && id != exceptID, nil
}

func (f *fkCategoryRepo) HasLinkedArticles(_ context.Context, categoryID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.linked[categoryID], nil
}

func (f *fkCategoryRepo) FindByID(_ context.Context, id string) (*domain.Category, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if c, ok := f.byID[id]; ok {
		cp := *c
		return &cp, nil
	}
	return nil, domain.ErrNotFound
}

func categoriesRouter(t *testing.T, users *rbacUserRepo, cats *fkCategoryRepo, audit *tFakeAudit) http.Handler {
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
	})
}

func catsJSON(t *testing.T, v any) *bytes.Reader {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return bytes.NewReader(b)
}

func TestCategoryCRUDViaHTTP(t *testing.T) {
	users := newRBACUsers()
	seedRBACUsers(users)
	cats := newFkCategoryRepo()
	audit := newTFakeAudit()
	router := categoriesRouter(t, users, cats, audit)
	token := rbacToken(t, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaa1", domain.RoleAdmin)

	// Create.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/categories", catsJSON(t, map[string]any{"name": "技术", "slug": "tech", "sortOrder": 1}))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var created struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	if created.ID == "" || created.Name != "技术" {
		t.Fatalf("created = %+v", created)
	}

	// Duplicate slug -> 409.
	req = httptest.NewRequest(http.MethodPost, "/api/v1/admin/categories", catsJSON(t, map[string]any{"name": "其他", "slug": "tech"}))
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("dup slug: status = %d, want 409", rec.Code)
	}

	// List.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/categories", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: status = %d", rec.Code)
	}
	var list []map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list) != 1 {
		t.Fatalf("list length = %d, want 1", len(list))
	}

	// Update.
	req = httptest.NewRequest(http.MethodPut, "/api/v1/admin/categories/"+created.ID, catsJSON(t, map[string]any{"name": "技术中心", "slug": "tech-hub"}))
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("update: status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var updated map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &updated)
	if updated["slug"] != "tech-hub" {
		t.Errorf("updated slug = %v, want tech-hub", updated["slug"])
	}

	// Delete (no linked articles).
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/admin/categories/"+created.ID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete: status = %d, want 204", rec.Code)
	}
}

func TestCategoryDeleteInUse409ViaHTTP(t *testing.T) {
	users := newRBACUsers()
	seedRBACUsers(users)
	cats := newFkCategoryRepo()
	cats.add("cat-in-use", "技术", "tech", 5)
	router := categoriesRouter(t, users, cats, newTFakeAudit())
	token := rbacToken(t, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaa1", domain.RoleAdmin)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/categories/cat-in-use", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("delete in-use: status = %d, want 409", rec.Code)
	}
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	if env.Error.Code != "CONFLICT" {
		t.Errorf("error.code = %q, want CONFLICT", env.Error.Code)
	}
}

func TestCategoryEditorForbidden403(t *testing.T) {
	users := newRBACUsers()
	seedRBACUsers(users)
	router := categoriesRouter(t, users, newFkCategoryRepo(), newTFakeAudit())
	token := rbacToken(t, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaa2", domain.RoleEditor)

	// The taxonomy read is shared by every authoring role (the article editor
	// lists categories while authoring); mutations remain admin-only.
	if rec := categoriesAuthorizedRequest(router, http.MethodGet, "/api/v1/admin/categories", token); rec.Code != http.StatusOK {
		t.Errorf("editor GET categories: status = %d, want 200 (taxonomy read)", rec.Code)
	}
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		rec := categoriesAuthorizedRequest(router, method, "/api/v1/admin/categories", token)
		if method == http.MethodPut || method == http.MethodDelete {
			rec = categoriesAuthorizedRequest(router, method, "/api/v1/admin/categories/x", token)
		}
		if rec.Code != http.StatusForbidden {
			t.Errorf("editor %s categories: status = %d, want 403", method, rec.Code)
		}
	}
}

func categoriesAuthorizedRequest(router http.Handler, method, path, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestCategoryPublicListAnonymous(t *testing.T) {
	users := newRBACUsers()
	seedRBACUsers(users)
	cats := newFkCategoryRepo()
	cats.add("cat-1", "技术", "tech", 3)
	cats.add("cat-2", "空", "empty", 0)
	router := categoriesRouter(t, users, cats, newTFakeAudit())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/public/categories", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("public list: status = %d", rec.Code)
	}
	var list []map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list) != 1 || list[0]["slug"] != "tech" {
		t.Errorf("public list = %+v, want only tech (published-only)", list)
	}
}

func TestCategoryValidationMissingNameOrSlug(t *testing.T) {
	users := newRBACUsers()
	seedRBACUsers(users)
	router := categoriesRouter(t, users, newFkCategoryRepo(), newTFakeAudit())
	token := rbacToken(t, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaa1", domain.RoleAdmin)

	for _, body := range []map[string]any{{"name": "x"}, {"slug": "x"}, {}} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/categories", catsJSON(t, body))
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("payload %+v: status = %d, want 400", body, rec.Code)
		}
	}
}
