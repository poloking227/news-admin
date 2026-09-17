// Package service implements the application use-cases: category CRUD with
// slug uniqueness and in-use protection.
package service

import (
	"context"
	"errors"
	"time"

	"news-admin/backend/internal/domain"
)

// CategoryService orchestrates category management use-cases.
type CategoryService struct {
	categories domain.CategoryRepository
	audit      domain.AuditRepository
	now        func() time.Time
}

// NewCategoryService builds a CategoryService.
func NewCategoryService(categories domain.CategoryRepository, audit domain.AuditRepository) *CategoryService {
	return &CategoryService{categories: categories, audit: audit, now: time.Now}
}

// List returns all non-deleted categories with article counts.
func (s *CategoryService) List(ctx context.Context) ([]*domain.Category, error) {
	return s.categories.List(ctx)
}

// ListPublic returns categories that have published articles (public site).
func (s *CategoryService) ListPublic(ctx context.Context) ([]*domain.Category, error) {
	return s.categories.ListPublished(ctx)
}

// Create validates slug uniqueness then inserts the category.
func (s *CategoryService) Create(ctx context.Context, in *domain.CategoryInput, actorID, ip string) (*domain.Category, error) {
	exists, err := s.categories.ExistsSlug(ctx, in.Slug, "")
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, domain.ErrSlugConflict
	}
	cat, err := s.categories.Create(ctx, in)
	if err != nil {
		return nil, err
	}
	_ = s.writeAudit(ctx, &domain.AuditLog{
		Actor: actorID, Action: domain.ActionCategoryCreate,
		ResourceType: "category", ResourceID: &cat.ID, IP: ip,
	})
	return cat, nil
}

// Update validates slug uniqueness (excluding the category itself) then saves.
func (s *CategoryService) Update(ctx context.Context, id string, in *domain.CategoryInput, actorID, ip string) (*domain.Category, error) {
	exists, err := s.categories.ExistsSlug(ctx, in.Slug, id)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, domain.ErrSlugConflict
	}
	cat, err := s.categories.Update(ctx, id, in)
	if err != nil {
		return nil, err
	}
	_ = s.writeAudit(ctx, &domain.AuditLog{
		Actor: actorID, Action: domain.ActionCategoryUpdate,
		ResourceType: "category", ResourceID: &cat.ID, IP: ip,
	})
	return cat, nil
}

// SoftDelete refuses when linked non-deleted articles still exist.
func (s *CategoryService) SoftDelete(ctx context.Context, id, actorID, ip string) error {
	inUse, err := s.categories.HasLinkedArticles(ctx, id)
	if err != nil {
		return err
	}
	if inUse {
		return domain.ErrCategoryInUse
	}
	if err := s.categories.SoftDelete(ctx, id); err != nil {
		return err
	}
	_ = s.writeAudit(ctx, &domain.AuditLog{
		Actor: actorID, Action: domain.ActionCategorySoftDel,
		ResourceType: "category", ResourceID: &id, IP: ip,
	})
	return nil
}

func (s *CategoryService) writeAudit(ctx context.Context, entry *domain.AuditLog) error {
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = s.now()
	}
	if err := s.audit.Insert(ctx, entry); err != nil {
		return errors.Join(errors.New("write audit"), err)
	}
	return nil
}
