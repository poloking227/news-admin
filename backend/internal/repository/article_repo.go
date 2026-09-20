package repository

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"news-admin/backend/internal/domain"
)

// articleRow maps the articles table.
type articleRow struct {
	ID            string     `gorm:"column:id;type:uuid;primaryKey"`
	Title         string     `gorm:"column:title"`
	Summary       string     `gorm:"column:summary"`
	BodyHTML      string     `gorm:"column:body_html"`
	BodyText      string     `gorm:"column:body_text"`
	CategoryID    string     `gorm:"column:category_id;type:uuid"`
	CoverURL      *string    `gorm:"column:cover_url"`
	Status        string     `gorm:"column:status"`
	RejectReason  *string    `gorm:"column:reject_reason"`
	RejectedAt    *time.Time `gorm:"column:rejected_at"`
	Pinned        bool       `gorm:"column:pinned"`
	PinnedAt      *time.Time `gorm:"column:pinned_at"`
	SubmittedAt   *time.Time `gorm:"column:submitted_at"`
	PublishedAt   *time.Time `gorm:"column:published_at"`
	UnpublishedAt *time.Time `gorm:"column:unpublished_at"`
	CreatedBy     string     `gorm:"column:created_by;type:uuid"`
	UpdatedBy     *string    `gorm:"column:updated_by;type:uuid"`
	Version       int        `gorm:"column:version"`
	CreatedAt     time.Time  `gorm:"column:created_at"`
	UpdatedAt     time.Time  `gorm:"column:updated_at"`
	DeletedAt     *time.Time `gorm:"column:deleted_at"`
}

func (articleRow) TableName() string { return "articles" }

func (r articleRow) toDomain() *domain.Article {
	return &domain.Article{
		ID:            r.ID,
		Title:         r.Title,
		Summary:       r.Summary,
		BodyHTML:      r.BodyHTML,
		BodyText:      r.BodyText,
		CategoryID:    r.CategoryID,
		CoverURL:      r.CoverURL,
		Status:        r.Status,
		RejectReason:  r.RejectReason,
		RejectedAt:    r.RejectedAt,
		Pinned:        r.Pinned,
		PinnedAt:      r.PinnedAt,
		SubmittedAt:   r.SubmittedAt,
		PublishedAt:   r.PublishedAt,
		UnpublishedAt: r.UnpublishedAt,
		CreatedBy:     r.CreatedBy,
		UpdatedBy:     r.UpdatedBy,
		Version:       r.Version,
		CreatedAt:     r.CreatedAt,
		UpdatedAt:     r.UpdatedAt,
	}
}

// ArticleRepository is the GORM-backed article repository.
type ArticleRepository struct {
	db *gorm.DB
}

// NewArticleRepository builds an ArticleRepository.
func NewArticleRepository(db *gorm.DB) *ArticleRepository {
	return &ArticleRepository{db: db}
}

func (r *ArticleRepository) Create(ctx context.Context, in *domain.ArticleInput, actorID string, now time.Time) (*domain.Article, error) {
	row := articleRow{
		ID:         newCategoryID(),
		Title:      in.Title,
		Summary:    in.Summary,
		BodyHTML:   in.BodyHTML,
		BodyText:   "",
		CategoryID: in.CategoryID,
		CoverURL:   in.CoverURL,
		Status:     domain.ArticleStatusDraft,
		CreatedBy:  actorID,
		UpdatedBy:  &actorID,
		Version:    1,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	// Enrich with category name for the admin listing display.
	var categoryName string
	err := r.db.WithContext(ctx).Table("categories").
		Select("name").Where("id = ? AND deleted_at IS NULL", in.CategoryID).
		Scan(&categoryName).Error
	if err != nil {
		return nil, err
	}
	if categoryName == "" {
		return nil, domain.ErrNotFound
	}
	row.CategoryID = in.CategoryID
	row.BodyText = in.BodyText
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return nil, err
	}
	article := row.toDomain()
	article.CategoryName = categoryName
	return article, nil
}

func (r *ArticleRepository) FindByID(ctx context.Context, id string) (*domain.Article, error) {
	var row articleRow
	err := r.db.WithContext(ctx).Where("id = ? AND deleted_at IS NULL", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	article := row.toDomain()
	// Enrich display names with two small lookups (avoids GORM JOIN aliasing
	// quirks when scanning into embedded rows).
	r.db.WithContext(ctx).Table("categories").Select("name").
		Where("id = ?", article.CategoryID).Scan(&article.CategoryName)
	r.db.WithContext(ctx).Table("users").Select("display_name").
		Where("id = ?", article.CreatedBy).Scan(&article.CreatedByName)
	return article, nil
}

func (r *ArticleRepository) Update(ctx context.Context, id string, in *domain.ArticleInput, expectedVersion int, actorID string, now time.Time) (*domain.Article, error) {
	updates := map[string]any{
		"updated_by": actorID,
		"updated_at": now,
		"version":    gorm.Expr("version + 1"),
	}
	// Partial-update semantics: nil leaves the field untouched.
	if in.Title != "" {
		updates["title"] = in.Title
	}
	if in.Summary != "" {
		updates["summary"] = in.Summary
	}
	if in.BodyHTML != "" {
		updates["body_html"] = in.BodyHTML
		updates["body_text"] = in.BodyText
	}
	if in.CategoryID != "" {
		updates["category_id"] = in.CategoryID
	}
	if in.CoverURL != nil {
		updates["cover_url"] = *in.CoverURL
	}

	res := r.db.WithContext(ctx).Model(&articleRow{}).
		Where("id = ? AND version = ? AND deleted_at IS NULL AND status = ?",
			id, expectedVersion, domain.ArticleStatusDraft).
		Updates(updates)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		// Distinguish version mismatch from not-found / not-editable.
		current, err := r.FindByID(ctx, id)
		if err != nil {
			return nil, err
		}
		if current.Version != expectedVersion {
			return nil, domain.ErrVersionConflict
		}
		return nil, domain.ErrArticleNotEditable
	}
	return r.FindByID(ctx, id)
}

func (r *ArticleRepository) SoftDelete(ctx context.Context, id string, now time.Time) error {
	// Only drafts (including rejected drafts) can be soft-deleted.
	res := r.db.WithContext(ctx).Model(&articleRow{}).
		Where("id = ? AND deleted_at IS NULL AND status = ?", id, domain.ArticleStatusDraft).
		Update("deleted_at", now)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		current, err := r.FindByID(ctx, id)
		if err != nil {
			return err
		}
		if current.Status == domain.ArticleStatusPublished {
			return domain.ErrArticlePublished
		}
		return domain.ErrArticleNotEditable
	}
	return nil
}

func (r *ArticleRepository) List(ctx context.Context, q *domain.ArticleQuery) (*domain.ArticlePage, error) {
	base := r.db.WithContext(ctx).
		Table("articles a").
		Joins("LEFT JOIN categories c ON c.id = a.category_id").
		Joins("LEFT JOIN users u ON u.id = a.created_by").
		Where("a.deleted_at IS NULL")

	// Admin/reviewer see everything; editor sees own + all published.
	if q.Role == domain.RoleEditor {
		base = base.Where("(a.created_by = ? OR a.status = ?)", q.UserID, domain.ArticleStatusPublished)
	}
	if q.Status != nil {
		base = base.Where("a.status = ?", *q.Status)
	}
	if q.CategoryID != nil && *q.CategoryID != "" {
		base = base.Where("a.category_id = ?", *q.CategoryID)
	}
	if q.Keyword != nil && *q.Keyword != "" {
		base = base.Where("(a.title ILIKE ? OR a.summary ILIKE ? OR a.body_text ILIKE ?)",
			"%"+*q.Keyword+"%", "%"+*q.Keyword+"%", "%"+*q.Keyword+"%")
	}
	if q.Pinned != nil {
		base = base.Where("a.pinned = ?", *q.Pinned)
	}

	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, err
	}

	if q.Page < 1 {
		q.Page = 1
	}
	if q.PageSize < 1 || q.PageSize > 100 {
		q.PageSize = 10
	}

	var rows []articleRow
	err := base.
		Select("a.*").
		Order("a.pinned DESC, a.created_at DESC").
		Offset((q.Page - 1) * q.PageSize).
		Limit(q.PageSize).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	// Batch-enrich category and author display names to avoid N+1 JOINs.
	items := make([]*domain.Article, 0, len(rows))
	for i := range rows {
		items = append(items, rows[i].toDomain())
	}
	if len(items) > 0 {
		r.attachDisplayNames(ctx, items)
	}
	return &domain.ArticlePage{Items: items, Total: total, Page: q.Page, PageSize: q.PageSize}, nil
}

// attachDisplayNames fills categoryName and createdByName for a batch of
// articles using two grouped queries.
func (r *ArticleRepository) attachDisplayNames(ctx context.Context, articles []*domain.Article) {
	categoryIDs := make([]string, 0, len(articles))
	creatorIDs := make([]string, 0, len(articles))
	for _, a := range articles {
		categoryIDs = append(categoryIDs, a.CategoryID)
		creatorIDs = append(creatorIDs, a.CreatedBy)
	}

	var cats []struct {
		ID   string
		Name string
	}
	r.db.WithContext(ctx).Table("categories").Select("id, name").
		Where("id IN ?", categoryIDs).Scan(&cats)
	names := map[string]string{}
	for _, c := range cats {
		names[c.ID] = c.Name
	}

	var creators []struct {
		ID          string
		DisplayName string
	}
	r.db.WithContext(ctx).Table("users").Select("id, display_name").
		Where("id IN ?", creatorIDs).Scan(&creators)
	creatorNames := map[string]string{}
	for _, u := range creators {
		creatorNames[u.ID] = u.DisplayName
	}

	for _, a := range articles {
		a.CategoryName = names[a.CategoryID]
		a.CreatedByName = creatorNames[a.CreatedBy]
	}
}
