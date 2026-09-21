package service

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"

	"news-admin/backend/internal/domain"
	"news-admin/backend/internal/sanitize"
)

// ArticleService orchestrates the article draft CRUD use-cases.
type ArticleService struct {
	articles   domain.ArticleRepository
	users      domain.UserRepository
	categories domain.CategoryRepository
	audit      domain.AuditRepository
	now        func() time.Time
}

// NewArticleService builds an ArticleService.
func NewArticleService(articles domain.ArticleRepository, users domain.UserRepository, categories domain.CategoryRepository, audit domain.AuditRepository) *ArticleService {
	return &ArticleService{articles: articles, users: users, categories: categories, audit: audit, now: time.Now}
}

// Create validates the input, sanitizes the HTML, and stores a draft.
func (s *ArticleService) Create(ctx context.Context, in *domain.ArticleInput, actorID, ip string) (*domain.Article, error) {
	if err := validateArticle(in); err != nil {
		return nil, err
	}
	clean := sanitize.BodyHTML(in.BodyHTML)
	if strings.TrimSpace(clean) == "" {
		return nil, ErrInvalidArticleBody
	}
	in.BodyHTML = clean
	in.BodyText = sanitize.ToText(clean)

	article, err := s.articles.Create(ctx, in, actorID, s.now())
	if err != nil {
		return nil, err
	}
	_ = s.writeAudit(ctx, &domain.AuditLog{
		Actor: actorID, Action: domain.ActionArticleCreate,
		ResourceType: "article", ResourceID: &article.ID, IP: ip,
	})
	return article, nil
}

// Get returns one article by id.
func (s *ArticleService) Get(ctx context.Context, id string) (*domain.Article, error) {
	return s.articles.FindByID(ctx, id)
}

// Update applies a partial update guarded by the If-Match version. Only
// drafts (including rejected drafts) are editable.
func (s *ArticleService) Update(ctx context.Context, id string, in *domain.ArticleInput, expectedVersion int, actorID, ip string) (*domain.Article, error) {
	if err := validateArticle(in); err != nil {
		return nil, err
	}
	if in.BodyHTML != "" {
		in.BodyHTML = sanitize.BodyHTML(in.BodyHTML)
		in.BodyText = sanitize.ToText(in.BodyHTML)
	}
	article, err := s.articles.Update(ctx, id, in, expectedVersion, actorID, s.now())
	if err != nil {
		return nil, err
	}
	_ = s.writeAudit(ctx, &domain.AuditLog{
		Actor: actorID, Action: domain.ActionArticleUpdate,
		ResourceType: "article", ResourceID: &article.ID, IP: ip,
	})
	return article, nil
}

// SoftDelete marks an article deleted; only drafts (including rejected) and
// unpublished are deletable, published is refused.
func (s *ArticleService) SoftDelete(ctx context.Context, id, actorID, ip string) error {
	if err := s.articles.SoftDelete(ctx, id, s.now()); err != nil {
		return err
	}
	_ = s.writeAudit(ctx, &domain.AuditLog{
		Actor: actorID, Action: domain.ActionArticleSoftDel,
		ResourceType: "article", ResourceID: &id, IP: ip,
	})
	return nil
}

// List returns a paged admin listing with role-based visibility.
func (s *ArticleService) List(ctx context.Context, q *domain.ArticleQuery) (*domain.ArticlePage, error) {
	return s.articles.List(ctx, q)
}

func (s *ArticleService) writeAudit(ctx context.Context, entry *domain.AuditLog) error {
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = s.now()
	}
	if err := s.audit.Insert(ctx, entry); err != nil {
		return errors.Join(errors.New("write audit"), err)
	}
	return nil
}

// validateArticle enforces size limits and the http(s) cover URL scheme.
func validateArticle(in *domain.ArticleInput) error {
	if len(in.Title) == 0 || len(in.Title) > 200 {
		return ErrArticleValidation
	}
	if len(in.Summary) > 500 {
		return ErrArticleValidation
	}
	if len(in.BodyHTML) > 2*1024*1024 {
		return ErrArticleValidation
	}
	if in.CoverURL != nil && *in.CoverURL != "" {
		u, err := url.Parse(*in.CoverURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
			return ErrArticleValidation
		}
	}
	return nil
}
