package service_test

import (
	"context"
	"errors"
	"strings"
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

func (f *fakeArticleRepo) Transition(_ context.Context, id, from, to string, extra map[string]any, actorID string, now time.Time) (*domain.Article, error) {
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

func (f *fakeArticleRepo) SetPinned(_ context.Context, id string, pinned bool, actorID string, now time.Time) (*domain.Article, error) {
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

// publicMatch reports whether the article is visible on the public site.
func publicMatch(a *domain.Article, q *domain.PublicArticleQuery) bool {
	if a.Status != domain.ArticleStatusPublished {
		return false
	}
	if q.CategoryID != nil && *q.CategoryID != "" && a.CategoryID != *q.CategoryID {
		return false
	}
	if q.Keyword != nil && *q.Keyword != "" {
		kw := strings.ToLower(*q.Keyword)
		if !strings.Contains(strings.ToLower(a.Title), kw) &&
			!strings.Contains(strings.ToLower(a.Summary), kw) &&
			!strings.Contains(strings.ToLower(a.BodyText), kw) {
			return false
		}
	}
	return true
}

func (f *fakeArticleRepo) ListPublic(ctx context.Context, q *domain.PublicArticleQuery) (*domain.ArticlePage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.publicPageLocked(q)
}

func (f *fakeArticleRepo) SearchPublic(ctx context.Context, q *domain.PublicArticleQuery) (*domain.ArticlePage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.publicPageLocked(q)
}

// publicPageLocked filters published articles; the caller holds the lock.
func (f *fakeArticleRepo) publicPageLocked(q *domain.PublicArticleQuery) (*domain.ArticlePage, error) {
	var items []*domain.Article
	for _, a := range f.articles {
		if !publicMatch(a, q) {
			continue
		}
		cp := *a
		items = append(items, &cp)
	}
	return &domain.ArticlePage{Items: items, Total: int64(len(items)), Page: q.Page, PageSize: q.PageSize}, nil
}

func (f *fakeArticleRepo) FindPublic(ctx context.Context, id string) (*domain.Article, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a, ok := f.articles[id]
	if !ok || a.Status != domain.ArticleStatusPublished {
		return nil, domain.ErrNotFound
	}
	cp := *a
	return &cp, nil
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

// validArticle builds an article that passes the submission completeness
// check.
func validArticle(t *testing.T, svc *service.ArticleService, actor string) *domain.Article {
	t.Helper()
	article, err := svc.Create(context.Background(), &domain.ArticleInput{
		Title: "标题", Summary: "摘要", BodyHTML: "<p>正文</p>", CategoryID: "cat-1",
	}, actor, "ip")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	return article
}

func TestArticleSubmitRejectsNonOwner(t *testing.T) {
	repo := newFakeArticleRepo()
	svc := service.NewArticleService(repo, newFakeUserRepo(), newFakeCategoryRepo(), newFakeAuditRepo())
	article := validArticle(t, svc, "u1")

	_, err := svc.Submit(context.Background(), article.ID, "u2", "ip")
	if !errors.Is(err, domain.ErrNotArticleOwner) {
		t.Fatalf("Submit(by non-owner) error = %v, want ErrNotArticleOwner", err)
	}
}

func TestArticleSubmitRequiresCompleteFields(t *testing.T) {
	repo := newFakeArticleRepo()
	svc := service.NewArticleService(repo, newFakeUserRepo(), newFakeCategoryRepo(), newFakeAuditRepo())
	// Create with an empty summary, then try to submit.
	article, err := svc.Create(context.Background(), &domain.ArticleInput{
		Title: "t", Summary: "", BodyHTML: "<p>x</p>", CategoryID: "cat-1",
	}, "u1", "ip")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	_, err = svc.Submit(context.Background(), article.ID, "u1", "ip")
	if !errors.Is(err, service.ErrArticleIncomplete) {
		t.Fatalf("Submit(incomplete) error = %v, want ErrArticleIncomplete", err)
	}
}

func TestArticleRejectRequiresReason(t *testing.T) {
	repo := newFakeArticleRepo()
	svc := service.NewArticleService(repo, newFakeUserRepo(), newFakeCategoryRepo(), newFakeAuditRepo())
	article := validArticle(t, svc, "u1")
	if _, err := svc.Submit(context.Background(), article.ID, "u1", "ip"); err != nil {
		t.Fatalf("Submit() error = %v", err)
	}

	for _, reason := range []string{"", "   "} {
		_, err := svc.Reject(context.Background(), article.ID, reason, "reviewer", "ip")
		if !errors.Is(err, service.ErrRejectReasonInvalid) {
			t.Errorf("Reject(reason=%q) error = %v, want ErrRejectReasonInvalid", reason, err)
		}
	}
	long := strings.Repeat("很", 501)
	_, err := svc.Reject(context.Background(), article.ID, long, "reviewer", "ip")
	if !errors.Is(err, service.ErrRejectReasonInvalid) {
		t.Errorf("Reject(too long) error = %v, want ErrRejectReasonInvalid", err)
	}
}

func TestArticleWorkflowLegalChain(t *testing.T) {
	repo := newFakeArticleRepo()
	svc := service.NewArticleService(repo, newFakeUserRepo(), newFakeCategoryRepo(), newFakeAuditRepo())
	article := validArticle(t, svc, "u1")

	// draft -> pending_review -> published -> unpublished -> pending_review.
	pending, err := svc.Submit(context.Background(), article.ID, "u1", "ip")
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	if pending.Status != domain.ArticleStatusPendingReview || pending.SubmittedAt == nil {
		t.Errorf("after submit: status=%q submittedAt=%v", pending.Status, pending.SubmittedAt)
	}
	if pending.Version != 2 {
		t.Errorf("version after submit = %d, want 2", pending.Version)
	}

	published, err := svc.Approve(context.Background(), article.ID, "reviewer", "ip")
	if err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	if published.Status != domain.ArticleStatusPublished || published.PublishedAt == nil {
		t.Errorf("after approve: status=%q publishedAt=%v", published.Status, published.PublishedAt)
	}

	unpublished, err := svc.Unpublish(context.Background(), article.ID, nil, "reviewer", "ip")
	if err != nil {
		t.Fatalf("Unpublish() error = %v", err)
	}
	if unpublished.Status != domain.ArticleStatusUnpublished || unpublished.UnpublishedAt == nil {
		t.Errorf("after unpublish: status=%q unpublishedAt=%v", unpublished.Status, unpublished.UnpublishedAt)
	}

	// Re-submit from unpublished.
	repending, err := svc.Submit(context.Background(), article.ID, "u1", "ip")
	if err != nil {
		t.Fatalf("Submit(unpublished) error = %v", err)
	}
	if repending.Status != domain.ArticleStatusPendingReview || repending.SubmittedAt == nil {
		t.Errorf("after re-submit: status=%q", repending.Status)
	}
}

func TestArticleRejectKeepsLatestReason(t *testing.T) {
	repo := newFakeArticleRepo()
	svc := service.NewArticleService(repo, newFakeUserRepo(), newFakeCategoryRepo(), newFakeAuditRepo())
	article := validArticle(t, svc, "u1")
	if _, err := svc.Submit(context.Background(), article.ID, "u1", "ip"); err != nil {
		t.Fatalf("Submit() error = %v", err)
	}

	first, err := svc.Reject(context.Background(), article.ID, "缺少来源", "reviewer", "ip")
	if err != nil {
		t.Fatalf("Reject() error = %v", err)
	}
	if first.Status != domain.ArticleStatusDraft || first.RejectReason == nil || *first.RejectReason != "缺少来源" || first.RejectedAt == nil {
		t.Errorf("after reject: status=%q reason=%v rejectedAt=%v", first.Status, first.RejectReason, first.RejectedAt)
	}

	// Submit again and reject with a new reason; the latest reason is kept.
	if _, err := svc.Submit(context.Background(), article.ID, "u1", "ip"); err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	second, err := svc.Reject(context.Background(), article.ID, "图片侵权", "reviewer", "ip")
	if err != nil {
		t.Fatalf("Reject() error = %v", err)
	}
	if second.RejectReason == nil || *second.RejectReason != "图片侵权" {
		t.Errorf("reject reason not replaced: %v", second.RejectReason)
	}
}

func TestArticleWorkflowIllegalTransitions(t *testing.T) {
	repo := newFakeArticleRepo()
	svc := service.NewArticleService(repo, newFakeUserRepo(), newFakeCategoryRepo(), newFakeAuditRepo())
	article := validArticle(t, svc, "u1")

	// approving a draft directly is illegal.
	if _, err := svc.Approve(context.Background(), article.ID, "reviewer", "ip"); !errors.Is(err, domain.ErrIllegalTransition) {
		t.Errorf("Approve(draft) error = %v, want ErrIllegalTransition", err)
	}
	// unpublishing a draft directly is illegal.
	if _, err := svc.Unpublish(context.Background(), article.ID, nil, "reviewer", "ip"); !errors.Is(err, domain.ErrIllegalTransition) {
		t.Errorf("Unpublish(draft) error = %v, want ErrIllegalTransition", err)
	}

	if _, err := svc.Submit(context.Background(), article.ID, "u1", "ip"); err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	// submitting twice in a row (pending_review -> pending_review) is illegal.
	if _, err := svc.Submit(context.Background(), article.ID, "u1", "ip"); !errors.Is(err, domain.ErrIllegalTransition) {
		t.Errorf("Submit(pending_review) error = %v, want ErrIllegalTransition", err)
	}

	if _, err := svc.Approve(context.Background(), article.ID, "reviewer", "ip"); err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	// approving an already-published article is illegal, as is pinning
	// anything that is not published. unpublishing twice is also illegal.
	if _, err := svc.Approve(context.Background(), article.ID, "reviewer", "ip"); !errors.Is(err, domain.ErrIllegalTransition) {
		t.Errorf("Approve(published) error = %v, want ErrIllegalTransition", err)
	}
	if _, err := svc.Unpublish(context.Background(), article.ID, nil, "reviewer", "ip"); err != nil {
		t.Fatalf("Unpublish() error = %v", err)
	}
	if _, err := svc.Unpublish(context.Background(), article.ID, nil, "reviewer", "ip"); !errors.Is(err, domain.ErrIllegalTransition) {
		t.Errorf("Unpublish(unpublished) error = %v, want ErrIllegalTransition", err)
	}
}

func TestArticlePinOnlyPublished(t *testing.T) {
	repo := newFakeArticleRepo()
	svc := service.NewArticleService(repo, newFakeUserRepo(), newFakeCategoryRepo(), newFakeAuditRepo())
	article := validArticle(t, svc, "u1")

	if _, err := svc.Pin(context.Background(), article.ID, true, "reviewer", "ip"); !errors.Is(err, domain.ErrIllegalTransition) {
		t.Errorf("Pin(draft) error = %v, want ErrIllegalTransition", err)
	}

	if _, err := svc.Submit(context.Background(), article.ID, "u1", "ip"); err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	if _, err := svc.Approve(context.Background(), article.ID, "reviewer", "ip"); err != nil {
		t.Fatalf("Approve() error = %v", err)
	}

	pinned, err := svc.Pin(context.Background(), article.ID, true, "reviewer", "ip")
	if err != nil {
		t.Fatalf("Pin() error = %v", err)
	}
	if !pinned.Pinned || pinned.PinnedAt == nil {
		t.Errorf("after pin: pinned=%v pinnedAt=%v", pinned.Pinned, pinned.PinnedAt)
	}
	unpinned, err := svc.Pin(context.Background(), article.ID, false, "reviewer", "ip")
	if err != nil {
		t.Fatalf("Pin(false) error = %v", err)
	}
	if unpinned.Pinned || unpinned.PinnedAt != nil {
		t.Errorf("after unpin: pinned=%v pinnedAt=%v", unpinned.Pinned, unpinned.PinnedAt)
	}
	if unpinned.Version != 5 {
		t.Errorf("version after create+submit+approve+pin+unpin = %d, want 5", unpinned.Version)
	}
}

func TestArticleUnpublishReasonLimit(t *testing.T) {
	repo := newFakeArticleRepo()
	svc := service.NewArticleService(repo, newFakeUserRepo(), newFakeCategoryRepo(), newFakeAuditRepo())
	article := validArticle(t, svc, "u1")
	if _, err := svc.Submit(context.Background(), article.ID, "u1", "ip"); err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	if _, err := svc.Approve(context.Background(), article.ID, "reviewer", "ip"); err != nil {
		t.Fatalf("Approve() error = %v", err)
	}

	long := strings.Repeat("长", 501)
	_, err := svc.Unpublish(context.Background(), article.ID, &long, "reviewer", "ip")
	if !errors.Is(err, service.ErrReasonTooLong) {
		t.Errorf("Unpublish(long reason) error = %v, want ErrReasonTooLong", err)
	}
	reason := "内容不合规"
	unpublished, err := svc.Unpublish(context.Background(), article.ID, &reason, "reviewer", "ip")
	if err != nil {
		t.Fatalf("Unpublish(with reason) error = %v", err)
	}
	if unpublished.Status != domain.ArticleStatusUnpublished {
		t.Errorf("status = %q, want unpublished", unpublished.Status)
	}
}

func TestArticlePublishedUpdateAndDeleteRefused(t *testing.T) {
	repo := newFakeArticleRepo()
	svc := service.NewArticleService(repo, newFakeUserRepo(), newFakeCategoryRepo(), newFakeAuditRepo())
	article := validArticle(t, svc, "u1")
	if _, err := svc.Submit(context.Background(), article.ID, "u1", "ip"); err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	if _, err := svc.Approve(context.Background(), article.ID, "reviewer", "ip"); err != nil {
		t.Fatalf("Approve() error = %v", err)
	}

	if _, err := svc.Update(context.Background(), article.ID, &domain.ArticleInput{Title: "改"}, 5, "u1", "ip"); !errors.Is(err, domain.ErrArticleNotEditable) {
		t.Errorf("Update(published) error = %v, want ErrArticleNotEditable", err)
	}
	if err := svc.SoftDelete(context.Background(), article.ID, "u1", "ip"); !errors.Is(err, domain.ErrArticlePublished) {
		t.Errorf("SoftDelete(published) error = %v, want ErrArticlePublished", err)
	}
}

// seedPublishedArticle drives a draft through approve so it is public.
func seedPublishedArticle(t *testing.T, repo *fakeArticleRepo, svc *service.ArticleService, actor, title, body string) *domain.Article {
	t.Helper()
	article, err := svc.Create(context.Background(), &domain.ArticleInput{
		Title: title, Summary: "摘要 " + title, BodyHTML: "<p>" + body + "</p>", CategoryID: "cat-1",
	}, actor, "ip")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, err := svc.Submit(context.Background(), article.ID, actor, "ip"); err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	published, err := svc.Approve(context.Background(), article.ID, "reviewer", "ip")
	if err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	return published
}

func TestPublicListOnlyPublished(t *testing.T) {
	repo := newFakeArticleRepo()
	svc := service.NewArticleService(repo, newFakeUserRepo(), newFakeCategoryRepo(), newFakeAuditRepo())

	pub := seedPublishedArticle(t, repo, svc, "u1", "公开文章", "visible")
	// A draft that never reaches pending_review must stay hidden.
	draft, err := svc.Create(context.Background(), &domain.ArticleInput{
		Title: "草稿", Summary: "s", BodyHTML: "<p>x</p>", CategoryID: "cat-1",
	}, "u1", "ip")
	if err != nil {
		t.Fatalf("Create(draft) error = %v", err)
	}
	// A published-then-unpublished article must also stay hidden.
	unpublished, err := svc.Unpublish(context.Background(), pub.ID, nil, "reviewer", "ip")
	if err != nil {
		t.Fatalf("Unpublish() error = %v", err)
	}

	page, err := svc.ListPublic(context.Background(), &domain.PublicArticleQuery{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListPublic() error = %v", err)
	}
	ids := map[string]bool{}
	for _, a := range page.Items {
		ids[a.ID] = true
	}
	if ids[draft.ID] || ids[unpublished.ID] {
		t.Errorf("hidden articles leaked into public list: %v", ids)
	}
	if page.Total != 0 {
		t.Errorf("total = %d, want 0 (nothing published left)", page.Total)
	}
}

func TestPublicListCategoryFilter(t *testing.T) {
	repo := newFakeArticleRepo()
	svc := service.NewArticleService(repo, newFakeUserRepo(), newFakeCategoryRepo(), newFakeAuditRepo())

	seedPublishedArticle(t, repo, svc, "u1", "甲", "a")
	_ = repo
	// cat-2 articles are created directly with another category id.
	article2, err := svc.Create(context.Background(), &domain.ArticleInput{
		Title: "乙", Summary: "s", BodyHTML: "<p>b</p>", CategoryID: "cat-2",
	}, "u1", "ip")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, err := svc.Submit(context.Background(), article2.ID, "u1", "ip"); err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	if _, err := svc.Approve(context.Background(), article2.ID, "reviewer", "ip"); err != nil {
		t.Fatalf("Approve() error = %v", err)
	}

	catID := "cat-2"
	page, err := svc.ListPublic(context.Background(), &domain.PublicArticleQuery{CategoryID: &catID, Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListPublic(category) error = %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].Title != "乙" {
		t.Errorf("category filter got %d items, want only 乙", len(page.Items))
	}
}

func TestPublicSearchMatchesTitleAndBody(t *testing.T) {
	repo := newFakeArticleRepo()
	svc := service.NewArticleService(repo, newFakeUserRepo(), newFakeCategoryRepo(), newFakeAuditRepo())

	seedPublishedArticle(t, repo, svc, "u1", "Go 并发编程", "goroutine 详解")
	seedPublishedArticle(t, repo, svc, "u1", "前端工程化", "vite 构建")

	kw := "goroutine"
	page, err := svc.SearchPublic(context.Background(), &domain.PublicArticleQuery{Keyword: &kw, Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("SearchPublic() error = %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].Title != "Go 并发编程" {
		t.Errorf("search body hit got %d items: %+v", len(page.Items), page.Items)
	}

	kw = "Go"
	page, err = svc.SearchPublic(context.Background(), &domain.PublicArticleQuery{Keyword: &kw, Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("SearchPublic(title) error = %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].Title != "Go 并发编程" {
		t.Errorf("search title hit got %d items", len(page.Items))
	}

	kw = "不存在的词"
	page, err = svc.SearchPublic(context.Background(), &domain.PublicArticleQuery{Keyword: &kw, Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("SearchPublic(no hit) error = %v", err)
	}
	if len(page.Items) != 0 || page.Total != 0 {
		t.Errorf("search no-hit got %d items", len(page.Items))
	}
}

func TestPublicGetHiddenArticleReturnsNotFound(t *testing.T) {
	repo := newFakeArticleRepo()
	svc := service.NewArticleService(repo, newFakeUserRepo(), newFakeCategoryRepo(), newFakeAuditRepo())

	draft, err := svc.Create(context.Background(), &domain.ArticleInput{
		Title: "草稿", Summary: "s", BodyHTML: "<p>x</p>", CategoryID: "cat-1",
	}, "u1", "ip")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, err := svc.GetPublic(context.Background(), draft.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("GetPublic(draft) error = %v, want ErrNotFound", err)
	}
	if _, err := svc.GetPublic(context.Background(), "missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("GetPublic(missing) error = %v, want ErrNotFound", err)
	}

	published := seedPublishedArticle(t, repo, svc, "u1", "公开", "x")
	got, err := svc.GetPublic(context.Background(), published.ID)
	if err != nil {
		t.Fatalf("GetPublic(published) error = %v", err)
	}
	if got.ID != published.ID {
		t.Errorf("got article %s, want %s", got.ID, published.ID)
	}
}
