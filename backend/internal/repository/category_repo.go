package repository

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"news-admin/backend/internal/domain"
)

// categoryRow maps the categories table.
type categoryRow struct {
	ID          string     `gorm:"column:id;type:uuid;primaryKey"`
	Name        string     `gorm:"column:name"`
	Slug        string     `gorm:"column:slug"`
	Description *string    `gorm:"column:description"`
	SortOrder   int        `gorm:"column:sort_order"`
	CreatedAt   time.Time  `gorm:"column:created_at"`
	UpdatedAt   time.Time  `gorm:"column:updated_at"`
	DeletedAt   *time.Time `gorm:"column:deleted_at"`
}

func (categoryRow) TableName() string { return "categories" }

func (r categoryRow) toDomain() *domain.Category {
	return &domain.Category{
		ID:          r.ID,
		Name:        r.Name,
		Slug:        r.Slug,
		Description: r.Description,
		SortOrder:   r.SortOrder,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}
}

// CategoryRepository is the GORM-backed category repository.
type CategoryRepository struct {
	db *gorm.DB
}

// NewCategoryRepository builds a CategoryRepository.
func NewCategoryRepository(db *gorm.DB) *CategoryRepository {
	return &CategoryRepository{db: db}
}

func (r *CategoryRepository) List(ctx context.Context) ([]*domain.Category, error) {
	return r.listWithCount(ctx, false)
}

func (r *CategoryRepository) ListPublished(ctx context.Context) ([]*domain.Category, error) {
	return r.listWithCount(ctx, true)
}

// listWithCount loads categories then attaches each one's article count in a
// second grouped query, avoiding GORM aliasing quirks with raw selects.
func (r *CategoryRepository) listWithCount(ctx context.Context, publishedOnly bool) ([]*domain.Category, error) {
	var rows []categoryRow
	if err := r.db.WithContext(ctx).
		Where("deleted_at IS NULL").
		Order("sort_order ASC, created_at DESC").
		Find(&rows).Error; err != nil {
		return nil, err
	}

	counts := make(map[string]int64, len(rows))
	if len(rows) > 0 {
		q := r.db.WithContext(ctx).
			Table("articles").
			Select("category_id, COUNT(*) AS n").
			Where("deleted_at IS NULL").
			Group("category_id")
		if publishedOnly {
			q = q.Where("status = 'published'")
		}
		type countRow struct {
			CategoryID string
			N          int64
		}
		var counted []countRow
		if err := q.Scan(&counted).Error; err != nil {
			return nil, err
		}
		for _, c := range counted {
			counts[c.CategoryID] = c.N
		}
	}

	out := make([]*domain.Category, 0, len(rows))
	for _, row := range rows {
		cat := row.toDomain()
		cat.ArticleCount = counts[row.ID]
		// Public listing exposes only categories that carry published content.
		if publishedOnly && cat.ArticleCount == 0 {
			continue
		}
		out = append(out, cat)
	}
	return out, nil
}

func (r *CategoryRepository) Create(ctx context.Context, in *domain.CategoryInput) (*domain.Category, error) {
	now := time.Now()
	row := categoryRow{
		ID:          newCategoryID(),
		Name:        in.Name,
		Slug:        in.Slug,
		Description: emptyWhenNil(in.Description),
		SortOrder:   in.SortOrder,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return nil, err
	}
	return row.toDomain(), nil
}

// emptyWhenNil maps a nil description to an empty string because the schema
// defines the column as NOT NULL DEFAULT ”.
func emptyWhenNil(s *string) *string {
	if s == nil {
		v := ""
		return &v
	}
	return s
}

// newCategoryID returns a random v4-style UUID string for primary keys.
func newCategoryID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		panic("uuid: " + err.Error())
	}
	buf[6] = (buf[6] & 0x0f) | 0x40
	buf[8] = (buf[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", buf[0:4], buf[4:6], buf[6:8], buf[8:10], buf[10:16])
}

func (r *CategoryRepository) Update(ctx context.Context, id string, in *domain.CategoryInput) (*domain.Category, error) {
	updates := map[string]any{
		"name":       in.Name,
		"slug":       in.Slug,
		"sort_order": in.SortOrder,
		"updated_at": time.Now(),
	}
	if in.Description != nil {
		updates["description"] = *in.Description
	}
	res := r.db.WithContext(ctx).Model(&categoryRow{}).
		Where("id = ? AND deleted_at IS NULL", id).
		Updates(updates)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, domain.ErrNotFound
	}
	return r.FindByID(ctx, id)
}

func (r *CategoryRepository) SoftDelete(ctx context.Context, id string) error {
	now := time.Now()
	res := r.db.WithContext(ctx).Model(&categoryRow{}).
		Where("id = ? AND deleted_at IS NULL", id).
		Update("deleted_at", now)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *CategoryRepository) ExistsSlug(ctx context.Context, slug, exceptID string) (bool, error) {
	var count int64
	q := r.db.WithContext(ctx).Model(&categoryRow{}).
		Where("slug = ? AND deleted_at IS NULL", slug)
	if exceptID != "" {
		q = q.Where("id <> ?", exceptID)
	}
	if err := q.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *CategoryRepository) HasLinkedArticles(ctx context.Context, categoryID string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Table("articles").
		Where("category_id = ? AND deleted_at IS NULL", categoryID).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *CategoryRepository) FindByID(ctx context.Context, id string) (*domain.Category, error) {
	var row categoryRow
	err := r.db.WithContext(ctx).Where("id = ? AND deleted_at IS NULL", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return row.toDomain(), nil
}
