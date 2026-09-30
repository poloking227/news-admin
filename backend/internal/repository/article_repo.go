package repository

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
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

// Transition applies a guarded status change: the UPDATE only matches rows in
// the expected source status, so an illegal or stale-state transition updates
// zero rows and is reported as ErrIllegalTransition. The API check and the
// database trigger (check_article_status_transition) double-guard this: a
// violation that reaches the trigger is mapped to the same domain error.
func (r *ArticleRepository) Transition(ctx context.Context, id, from, to string, extra map[string]any, actorID string, now time.Time) (*domain.Article, error) {
	updates := map[string]any{
		"status":     to,
		"version":    gorm.Expr("version + 1"),
		"updated_by": actorID,
		"updated_at": now,
	}
	for k, v := range extra {
		updates[k] = v
	}
	if from == to {
		// A same-status "transition" is not one of the legal moves.
		return nil, domain.ErrIllegalTransition
	}
	res := r.db.WithContext(ctx).Model(&articleRow{}).
		Where("id = ? AND deleted_at IS NULL AND status = ?", id, from).
		Updates(updates)
	if res.Error != nil {
		var pgErr *pgconn.PgError
		if errors.As(res.Error, &pgErr) && pgErr.Code == "23514" {
			// check_violation from the status-transition trigger.
			return nil, domain.ErrIllegalTransition
		}
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		// No row matched: either the article is gone or it is not in the
		// expected source status, both of which are conflict semantics.
		if _, err := r.FindByID(ctx, id); err != nil {
			return nil, err
		}
		return nil, domain.ErrIllegalTransition
	}
	return r.FindByID(ctx, id)
}

// SetPinned flips the pin flag on published articles only. The status guard
// keeps the operation a no-op for non-published rows, mirroring the API 409.
func (r *ArticleRepository) SetPinned(ctx context.Context, id string, pinned bool, actorID string, now time.Time) (*domain.Article, error) {
	updates := map[string]any{
		"pinned":     pinned,
		"updated_by": actorID,
		"updated_at": now,
		"version":    gorm.Expr("version + 1"),
	}
	if pinned {
		updates["pinned_at"] = now
	} else {
		updates["pinned_at"] = nil
	}
	res := r.db.WithContext(ctx).Model(&articleRow{}).
		Where("id = ? AND deleted_at IS NULL AND status = ?", id, domain.ArticleStatusPublished).
		Updates(updates)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		current, err := r.FindByID(ctx, id)
		if err != nil {
			return nil, err
		}
		if current.Status != domain.ArticleStatusPublished {
			return nil, domain.ErrIllegalTransition
		}
		return r.FindByID(ctx, id)
	}
	return r.FindByID(ctx, id)
}

// ListPublic returns published, non-deleted articles, pinned first then
// newest publishedAt. Hidden states never enter the result set.
func (r *ArticleRepository) ListPublic(ctx context.Context, q *domain.PublicArticleQuery) (*domain.ArticlePage, error) {
	base := r.db.WithContext(ctx).
		Table("articles a").
		Joins("LEFT JOIN categories c ON c.id = a.category_id").
		Where("a.deleted_at IS NULL AND a.status = ?", domain.ArticleStatusPublished)
	if q.CategoryID != nil && *q.CategoryID != "" {
		base = base.Where("a.category_id = ?", *q.CategoryID)
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
		Order("a.pinned DESC, a.published_at DESC").
		Offset((q.Page - 1) * q.PageSize).
		Limit(q.PageSize).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return r.toPublicPage(ctx, rows, total, q.Page, q.PageSize)
}

// SearchPublic matches published, non-deleted articles against title,
// summary, and body text using pg_trgm ILIKE (GIN index in 001_init).
func (r *ArticleRepository) SearchPublic(ctx context.Context, q *domain.PublicArticleQuery) (*domain.ArticlePage, error) {
	base := r.db.WithContext(ctx).
		Table("articles a").
		Joins("LEFT JOIN categories c ON c.id = a.category_id").
		Where("a.deleted_at IS NULL AND a.status = ?", domain.ArticleStatusPublished)
	if q.Keyword != nil && *q.Keyword != "" {
		base = base.Where("(a.title ILIKE ? OR a.summary ILIKE ? OR a.body_text ILIKE ?)",
			"%"+*q.Keyword+"%", "%"+*q.Keyword+"%", "%"+*q.Keyword+"%")
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
		Order("a.pinned DESC, a.published_at DESC").
		Offset((q.Page - 1) * q.PageSize).
		Limit(q.PageSize).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return r.toPublicPage(ctx, rows, total, q.Page, q.PageSize)
}

// FindPublic returns a published, non-deleted article by id; any other state
// maps to ErrNotFound so drafts stay undiscoverable.
func (r *ArticleRepository) FindPublic(ctx context.Context, id string) (*domain.Article, error) {
	var row articleRow
	err := r.db.WithContext(ctx).
		Where("id = ? AND deleted_at IS NULL AND status = ?", id, domain.ArticleStatusPublished).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	article := row.toDomain()
	r.db.WithContext(ctx).Table("categories").Select("name").
		Where("id = ?", article.CategoryID).Scan(&article.CategoryName)
	return article, nil
}

// toPublicPage assembles a page and enriches display names for the reader.
func (r *ArticleRepository) toPublicPage(ctx context.Context, rows []articleRow, total int64, page, pageSize int) (*domain.ArticlePage, error) {
	items := make([]*domain.Article, 0, len(rows))
	for i := range rows {
		items = append(items, rows[i].toDomain())
	}
	if len(items) > 0 {
		r.attachDisplayNames(ctx, items)
	}
	return &domain.ArticlePage{Items: items, Total: total, Page: page, PageSize: pageSize}, nil
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
