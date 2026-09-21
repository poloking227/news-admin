package service_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"news-admin/backend/internal/domain"
	"news-admin/backend/internal/service"
)

// fakeArticleRepo is an in-memory domain.ArticleRepository.
type fakeArticleRepo struct {
	mu       sync.Mutex
	next     int
	articles map[string]*domain.Article
}

func newFakeArticleRepo() *fakeArticleRepo {
	return &fakeArticleRepo{articles: map[string]*domain.Article{}}
}

func (f *fakeArticleRepo) Create(_ context.Context, in *domain.ArticleInput, actorID string, now time.Time) (*domain.Article, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.next++
	id := string(rune('a'+f.next)) + "-article"
	actor := actorID
	f.articles[id] = &domain.Article{
		ID: id, Title: in.Title, Summary: in.Summary,
		BodyHTML: in.BodyHTML, BodyText: in.BodyText,
		CategoryID: in.CategoryID, CoverURL: in.CoverURL,
		Status:    domain.ArticleStatusDraft,
		CreatedBy: actorID, CreatedByName: "someone", UpdatedBy: &actor,
		Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	cp := *f.articles[id]
	return &cp, nil
}

func (f *fakeArticleRepo) FindByID(_ context.Context, id string) (*domain.Article, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if a, ok := f.articles[id]; ok {
		cp := *a
		return &cp, nil
	}
	return nil, domain.ErrNotFound
}

func (f *fakeArticleRepo) Update(_ context.Context, id string, in *domain.ArticleInput, expectedVersion int, actorID string, now time.Time) (*domain.Article, error) {
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
	if in.CategoryID != "" {
		a.CategoryID = in.CategoryID
	}
	if in.CoverURL != nil {
		a.CoverURL = in.CoverURL
	}
	a.Version++
	a.UpdatedAt = now
	actor := actorID
	a.UpdatedBy = &actor
	cp := *a
	return &cp, nil
}

func (f *fakeArticleRepo) SoftDelete(_ context.Context, id string, now time.Time) error {
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

func (f *fakeArticleRepo) List(_ context.Context, q *domain.ArticleQuery) (*domain.ArticlePage, error) {
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

func ptr[T any](v T) *T { return &v }

func TestArticleCreateSanitizesAndSetsDraft(t *testing.T) {
	repo := newFakeArticleRepo()
	svc := service.NewArticleService(repo, newFakeUserRepo(), newFakeCategoryRepo(), newFakeAuditRepo())

	article, err := svc.Create(context.Background(), &domain.ArticleInput{
		Title: "标题", Summary: "摘要", BodyHTML: "<p>好内容</p><script>alert(1)</script>",
		CategoryID: "cat-1",
	}, "u1", "ip")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if article.Status != domain.ArticleStatusDraft {
		t.Errorf("status = %q, want draft", article.Status)
	}
	if article.Version != 1 {
		t.Errorf("version = %d, want 1", article.Version)
	}
	if article.BodyHTML != "<p>好内容</p>" {
		t.Errorf("body not sanitized: %q", article.BodyHTML)
	}
}

func TestArticleCreateRejectsBadCover(t *testing.T) {
	repo := newFakeArticleRepo()
	svc := service.NewArticleService(repo, newFakeUserRepo(), newFakeCategoryRepo(), newFakeAuditRepo())

	for _, bad := range []string{"ftp://x", "javascript:alert(1)", "not-a-url"} {
		_, err := svc.Create(context.Background(), &domain.ArticleInput{
			Title: "t", Summary: "s", BodyHTML: "<p>x</p>", CategoryID: "cat-1",
			CoverURL: ptr(bad),
		}, "u1", "ip")
		if !errors.Is(err, service.ErrArticleValidation) {
			t.Errorf("cover %q: error = %v, want ErrArticleValidation", bad, err)
		}
	}
}

func TestArticleUpdateOptimisticLock(t *testing.T) {
	repo := newFakeArticleRepo()
	svc := service.NewArticleService(repo, newFakeUserRepo(), newFakeCategoryRepo(), newFakeAuditRepo())
	created, _ := svc.Create(context.Background(), &domain.ArticleInput{
		Title: "old", Summary: "s", BodyHTML: "<p>x</p>", CategoryID: "cat-1",
	}, "u1", "ip")

	// Stale version -> conflict.
	_, err := svc.Update(context.Background(), created.ID, &domain.ArticleInput{Title: "new"}, 1, "u1", "ip")
	if err != nil {
		t.Fatalf("Update(version ok) error = %v", err)
	}
	_, err = svc.Update(context.Background(), created.ID, &domain.ArticleInput{Title: "newer"}, 1, "u1", "ip")
	if !errors.Is(err, domain.ErrVersionConflict) {
		t.Fatalf("Update(stale version) error = %v, want ErrVersionConflict", err)
	}
}

func TestArticleUpdatePartialSemantics(t *testing.T) {
	repo := newFakeArticleRepo()
	svc := service.NewArticleService(repo, newFakeUserRepo(), newFakeCategoryRepo(), newFakeAuditRepo())
	created, _ := svc.Create(context.Background(), &domain.ArticleInput{
		Title: "title", Summary: "summary", BodyHTML: "<p>body</p>", CategoryID: "cat-1",
	}, "u1", "ip")

	updated, err := svc.Update(context.Background(), created.ID, &domain.ArticleInput{Title: "new-title"}, 1, "u1", "ip")
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated.Title != "new-title" || updated.Summary != "summary" || updated.BodyHTML != "<p>body</p>" {
		t.Errorf("partial update broke fields: %+v", updated)
	}
	if updated.Version != 2 {
		t.Errorf("version = %d, want 2", updated.Version)
	}
}

func TestArticleSoftDeletePublishedRejected(t *testing.T) {
	repo := newFakeArticleRepo()
	svc := service.NewArticleService(repo, newFakeUserRepo(), newFakeCategoryRepo(), newFakeAuditRepo())
	created, _ := svc.Create(context.Background(), &domain.ArticleInput{
		Title: "t", Summary: "s", BodyHTML: "<p>x</p>", CategoryID: "cat-1",
	}, "u1", "ip")
	repo.mu.Lock()
	repo.articles[created.ID].Status = domain.ArticleStatusPublished
	repo.mu.Unlock()

	err := svc.SoftDelete(context.Background(), created.ID, "u1", "ip")
	if !errors.Is(err, domain.ErrArticlePublished) {
		t.Fatalf("SoftDelete(published) error = %v, want ErrArticlePublished", err)
	}
}

func TestArticleListEditorVisibility(t *testing.T) {
	repo := newFakeArticleRepo()
	repo.mu.Lock()
	now := time.Now()
	repo.articles["a1"] = &domain.Article{ID: "a1", Title: "mine-draft", Status: domain.ArticleStatusDraft, CreatedBy: "u1", CreatedAt: now, UpdatedAt: now}
	repo.articles["a2"] = &domain.Article{ID: "a2", Title: "mine-published", Status: domain.ArticleStatusPublished, CreatedBy: "u1", CreatedAt: now, UpdatedAt: now}
	repo.articles["a3"] = &domain.Article{ID: "a3", Title: "other-published", Status: domain.ArticleStatusPublished, CreatedBy: "u2", CreatedAt: now, UpdatedAt: now}
	repo.articles["a4"] = &domain.Article{ID: "a4", Title: "other-draft", Status: domain.ArticleStatusDraft, CreatedBy: "u2", CreatedAt: now, UpdatedAt: now}
	repo.mu.Unlock()

	svc := service.NewArticleService(repo, newFakeUserRepo(), newFakeCategoryRepo(), newFakeAuditRepo())
	page, err := svc.List(context.Background(), &domain.ArticleQuery{UserID: "u1", Role: domain.RoleEditor, Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(page.Items) != 3 {
		t.Errorf("editor sees %d articles, want 3 (own 2 + published 1)", len(page.Items))
	}
}
