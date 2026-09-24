package service

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"news-admin/backend/internal/domain"
	"news-admin/backend/internal/sanitize"
)

// Errors the article workflow service reports to handlers.
var (
	// ErrArticleIncomplete is returned when a draft is missing required
	// fields at submission time.
	ErrArticleIncomplete = errors.New("article is missing required fields")
	// ErrRejectReasonInvalid is returned when a reject reason is empty or
	// longer than the 500-character limit.
	ErrRejectReasonInvalid = errors.New("reject reason must not be empty and is limited to 500 characters")
	// ErrReasonTooLong is returned when an optional unpublish reason exceeds
	// the 500-character limit.
	ErrReasonTooLong = errors.New("reason is limited to 500 characters")
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

// Submit moves a draft (including a rejected draft) or an unpublished article
// into pending_review. Only the author may submit; the article must carry the
// required fields (title/summary/bodyHtml/categoryId).
func (s *ArticleService) Submit(ctx context.Context, id, actorID, ip string) (*domain.Article, error) {
	current, err := s.articles.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if current.CreatedBy != actorID {
		return nil, domain.ErrNotArticleOwner
	}
	// Only draft (including rejected) and unpublished articles can enter the
	// review queue on their own; any other source state is not submittable.
	switch current.Status {
	case domain.ArticleStatusDraft, domain.ArticleStatusUnpublished:
	default:
		return nil, domain.ErrIllegalTransition
	}
	if err := s.checkSubmittable(current); err != nil {
		return nil, err
	}
	now := s.now()
	article, err := s.articles.Transition(ctx, id, current.Status, domain.ArticleStatusPendingReview,
		map[string]any{"submitted_at": now}, actorID, now)
	if err != nil {
		return nil, err
	}
	_ = s.writeAudit(ctx, &domain.AuditLog{
		Actor: actorID, Action: domain.ActionArticleSubmit,
		ResourceType: "article", ResourceID: &article.ID, IP: ip,
	})
	return article, nil
}

// Approve publishes a pending_review article and stamps published_at. There
// is no separate publish action: approval is publication.
func (s *ArticleService) Approve(ctx context.Context, id, actorID, ip string) (*domain.Article, error) {
	now := s.now()
	article, err := s.articles.Transition(ctx, id, domain.ArticleStatusPendingReview, domain.ArticleStatusPublished,
		map[string]any{"published_at": now}, actorID, now)
	if err != nil {
		return nil, err
	}
	_ = s.writeAudit(ctx, &domain.AuditLog{
		Actor: actorID, Action: domain.ActionArticleApprove,
		ResourceType: "article", ResourceID: &article.ID, IP: ip,
	})
	return article, nil
}

// Reject returns a pending_review article to draft with a mandatory reason,
// stamping reject_reason and rejected_at (kept as the latest rejection).
func (s *ArticleService) Reject(ctx context.Context, id, reason, actorID, ip string) (*domain.Article, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" || utf8.RuneCountInString(reason) > 500 {
		return nil, ErrRejectReasonInvalid
	}
	now := s.now()
	article, err := s.articles.Transition(ctx, id, domain.ArticleStatusPendingReview, domain.ArticleStatusDraft,
		map[string]any{"reject_reason": reason, "rejected_at": now}, actorID, now)
	if err != nil {
		return nil, err
	}
	_ = s.writeAudit(ctx, &domain.AuditLog{
		Actor: actorID, Action: domain.ActionArticleReject,
		ResourceType: "article", ResourceID: &article.ID, IP: ip,
	})
	return article, nil
}

// Unpublish takes a published article down and stamps unpublished_at. The
// reason is optional and capped at 500 characters.
func (s *ArticleService) Unpublish(ctx context.Context, id string, reason *string, actorID, ip string) (*domain.Article, error) {
	if reason != nil {
		trimmed := strings.TrimSpace(*reason)
		if utf8.RuneCountInString(trimmed) > 500 {
			return nil, ErrReasonTooLong
		}
		reason = &trimmed
	}
	now := s.now()
	article, err := s.articles.Transition(ctx, id, domain.ArticleStatusPublished, domain.ArticleStatusUnpublished,
		map[string]any{"unpublished_at": now}, actorID, now)
	if err != nil {
		return nil, err
	}
	audit := &domain.AuditLog{
		Actor: actorID, Action: domain.ActionArticleUnpub,
		ResourceType: "article", ResourceID: &article.ID, IP: ip,
	}
	if reason != nil {
		audit.After = map[string]any{"reason": *reason}
	}
	_ = s.writeAudit(ctx, audit)
	return article, nil
}

// Pin sets or clears the pinned flag on a published article.
func (s *ArticleService) Pin(ctx context.Context, id string, pinned bool, actorID, ip string) (*domain.Article, error) {
	article, err := s.articles.SetPinned(ctx, id, pinned, actorID, s.now())
	if err != nil {
		return nil, err
	}
	_ = s.writeAudit(ctx, &domain.AuditLog{
		Actor: actorID, Action: domain.ActionArticlePin,
		ResourceType: "article", ResourceID: &article.ID, IP: ip,
	})
	return article, nil
}

// checkSubmittable enforces that a draft carries everything the public reader
// needs before it can enter the review queue.
func (s *ArticleService) checkSubmittable(a *domain.Article) error {
	ok := a.Title != "" && a.Summary != "" && a.BodyHTML != "" && a.CategoryID != ""
	if !ok {
		return ErrArticleIncomplete
	}
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
