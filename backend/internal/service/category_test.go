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

// fakeCategoryRepo is an in-memory domain.CategoryRepository.
type fakeCategoryRepo struct {
	mu         sync.Mutex
	nextID     int
	categories map[string]*domain.Category
	linked     map[string]bool // categoryID -> has linked articles
	slugIndex  map[string]string
}

func newFakeCategoryRepo() *fakeCategoryRepo {
	return &fakeCategoryRepo{
		categories: map[string]*domain.Category{},
		linked:     map[string]bool{},
		slugIndex:  map[string]string{},
	}
}

func (f *fakeCategoryRepo) seed(id, name, slug string, count int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	now := time.Now()
	f.categories[id] = &domain.Category{
		ID: id, Name: name, Slug: slug,
		Description:  nil,
		SortOrder:    0,
		ArticleCount: count,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	f.slugIndex[slug] = id
	if count > 0 {
		f.linked[id] = true
	}
}

func (f *fakeCategoryRepo) List(_ context.Context) ([]*domain.Category, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*domain.Category
	for _, c := range f.categories {
		cp := *c
		out = append(out, &cp)
	}
	return out, nil
}

func (f *fakeCategoryRepo) ListPublished(_ context.Context) ([]*domain.Category, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*domain.Category
	for _, c := range f.categories {
		if f.linked[c.ID] {
			cp := *c
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (f *fakeCategoryRepo) Create(_ context.Context, in *domain.CategoryInput) (*domain.Category, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	id := string(rune('a'+f.nextID)) + "-id"
	now := time.Now()
	f.categories[id] = &domain.Category{
		ID: id, Name: in.Name, Slug: in.Slug,
		Description: in.Description, SortOrder: in.SortOrder,
		CreatedAt: now, UpdatedAt: now,
	}
	f.slugIndex[in.Slug] = id
	out := *f.categories[id]
	return &out, nil
}

func (f *fakeCategoryRepo) Update(_ context.Context, id string, in *domain.CategoryInput) (*domain.Category, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.categories[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	delete(f.slugIndex, c.Slug)
	c.Name, c.Slug = in.Name, in.Slug
	c.Description, c.SortOrder = in.Description, in.SortOrder
	c.UpdatedAt = time.Now()
	f.slugIndex[in.Slug] = id
	out := *c
	return &out, nil
}

func (f *fakeCategoryRepo) SoftDelete(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.categories[id]
	if !ok {
		return domain.ErrNotFound
	}
	delete(f.slugIndex, c.Slug)
	delete(f.categories, id)
	delete(f.linked, id)
	return nil
}

func (f *fakeCategoryRepo) ExistsSlug(_ context.Context, slug, exceptID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id, ok := f.slugIndex[slug]
	return ok && id != exceptID, nil
}

func (f *fakeCategoryRepo) HasLinkedArticles(_ context.Context, categoryID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.linked[categoryID], nil
}

func (f *fakeCategoryRepo) FindByID(_ context.Context, id string) (*domain.Category, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if c, ok := f.categories[id]; ok {
		cp := *c
		return &cp, nil
	}
	return nil, domain.ErrNotFound
}

func TestCategoryCreateAndList(t *testing.T) {
	repo := newFakeCategoryRepo()
	svc := service.NewCategoryService(repo, newFakeAuditRepo())

	cat, err := svc.Create(context.Background(), &domain.CategoryInput{Name: "技术", Slug: "tech"}, "u1", "10.0.0.1")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if cat.Name != "技术" || cat.Slug != "tech" {
		t.Errorf("created category = %+v", cat)
	}

	list, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 1 || list[0].Slug != "tech" {
		t.Errorf("list = %+v, want 1 category tech", list)
	}
}

func TestCategorySlugConflict(t *testing.T) {
	repo := newFakeCategoryRepo()
	repo.seed("cat-1", "技术", "tech", 0)
	svc := service.NewCategoryService(repo, newFakeAuditRepo())

	_, err := svc.Create(context.Background(), &domain.CategoryInput{Name: "另一个", Slug: "tech"}, "u1", "ip")
	if !errors.Is(err, domain.ErrSlugConflict) {
		t.Fatalf("Create(dup slug) error = %v, want ErrSlugConflict", err)
	}

	// Update to the same slug as itself is allowed.
	if _, err := svc.Update(context.Background(), "cat-1", &domain.CategoryInput{Name: "技术", Slug: "tech"}, "u1", "ip"); err != nil {
		t.Errorf("Update(same slug) error = %v, want nil", err)
	}
	// Update to another category's slug conflicts.
	repo.seed("cat-2", "体育", "sports", 0)
	_, err = svc.Update(context.Background(), "cat-1", &domain.CategoryInput{Name: "技术", Slug: "sports"}, "u1", "ip")
	if !errors.Is(err, domain.ErrSlugConflict) {
		t.Fatalf("Update(conflict slug) error = %v, want ErrSlugConflict", err)
	}
}

func TestCategoryDeleteInUseRejected(t *testing.T) {
	repo := newFakeCategoryRepo()
	repo.seed("cat-1", "技术", "tech", 3)
	svc := service.NewCategoryService(repo, newFakeAuditRepo())

	err := svc.SoftDelete(context.Background(), "cat-1", "u1", "ip")
	if !errors.Is(err, domain.ErrCategoryInUse) {
		t.Fatalf("SoftDelete(in use) error = %v, want ErrCategoryInUse", err)
	}
}

func TestCategoryDeleteOK(t *testing.T) {
	repo := newFakeCategoryRepo()
	repo.seed("cat-1", "技术", "tech", 0)
	svc := service.NewCategoryService(repo, newFakeAuditRepo())

	if err := svc.SoftDelete(context.Background(), "cat-1", "u1", "ip"); err != nil {
		t.Fatalf("SoftDelete() error = %v", err)
	}
	_, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
}

func TestCategoryListPublic(t *testing.T) {
	repo := newFakeCategoryRepo()
	repo.seed("cat-1", "技术", "tech", 3)
	repo.seed("cat-2", "空分类", "empty", 0)
	svc := service.NewCategoryService(repo, newFakeAuditRepo())

	list, err := svc.ListPublic(context.Background())
	if err != nil {
		t.Fatalf("ListPublic() error = %v", err)
	}
	if len(list) != 1 || list[0].Slug != "tech" {
		t.Errorf("public list = %+v, want only tech", list)
	}
}

func TestCategoryUpdateNotFound(t *testing.T) {
	repo := newFakeCategoryRepo()
	svc := service.NewCategoryService(repo, newFakeAuditRepo())

	_, err := svc.Update(context.Background(), "nope", &domain.CategoryInput{Name: "x", Slug: "x"}, "u1", "ip")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Update(missing) error = %v, want ErrNotFound", err)
	}
}
