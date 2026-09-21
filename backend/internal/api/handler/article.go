package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"news-admin/backend/internal/domain"
	"news-admin/backend/internal/service"
)

// ArticleHandler exposes the admin article draft endpoints.
type ArticleHandler struct {
	svc *service.ArticleService
}

// NewArticleHandler builds an ArticleHandler.
func NewArticleHandler(svc *service.ArticleService) *ArticleHandler {
	return &ArticleHandler{svc: svc}
}

type articleRequest struct {
	Title      string  `json:"title"`
	Summary    string  `json:"summary"`
	BodyHTML   string  `json:"bodyHtml"`
	CategoryID string  `json:"categoryId"`
	CoverURL   *string `json:"coverUrl"`
}

// List handles GET /api/v1/admin/articles.
func (h *ArticleHandler) List(c *gin.Context) {
	q, err := parseArticleQuery(c)
	if err != nil {
		responseValidation(c, err)
		return
	}
	q.UserID = currentUserID(c)
	q.Role = currentUserRole(c)

	page, err := h.svc.List(c.Request.Context(), q)
	if err != nil {
		responseInternal(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"items":    articlesJSON(page.Items),
		"total":    page.Total,
		"page":     page.Page,
		"pageSize": page.PageSize,
	})
}

// Get handles GET /api/v1/admin/articles/{id}.
func (h *ArticleHandler) Get(c *gin.Context) {
	article, err := h.svc.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			responseWriteError(c, http.StatusNotFound, responseCodeNotFound, "article not found", nil)
			return
		}
		responseInternal(c, err)
		return
	}
	c.JSON(http.StatusOK, articleJSON(article))
}

// Create handles POST /api/v1/admin/articles (draft, version=1).
func (h *ArticleHandler) Create(c *gin.Context) {
	var req articleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		responseValidation(c, err)
		return
	}
	article, err := h.svc.Create(c.Request.Context(), &domain.ArticleInput{
		Title:      req.Title,
		Summary:    req.Summary,
		BodyHTML:   req.BodyHTML,
		CategoryID: req.CategoryID,
		CoverURL:   req.CoverURL,
	}, currentUserID(c), c.ClientIP())
	if err != nil {
		h.writeArticleError(c, err)
		return
	}
	c.JSON(http.StatusCreated, articleJSON(article))
}

// Update handles PUT /api/v1/admin/articles/{id} (If-Match optimistic lock).
func (h *ArticleHandler) Update(c *gin.Context) {
	version, ok := parseIfMatch(c)
	if !ok {
		responseWriteError(c, http.StatusBadRequest, responseCodeValidation, "If-Match header required", nil)
		return
	}
	var req articleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		responseValidation(c, err)
		return
	}
	article, err := h.svc.Update(c.Request.Context(), c.Param("id"), &domain.ArticleInput{
		Title:      req.Title,
		Summary:    req.Summary,
		BodyHTML:   req.BodyHTML,
		CategoryID: req.CategoryID,
		CoverURL:   req.CoverURL,
	}, version, currentUserID(c), c.ClientIP())
	if err != nil {
		h.writeArticleError(c, err)
		return
	}
	c.JSON(http.StatusOK, articleJSON(article))
}

// Delete handles DELETE /api/v1/admin/articles/{id} (soft delete).
func (h *ArticleHandler) Delete(c *gin.Context) {
	err := h.svc.SoftDelete(c.Request.Context(), c.Param("id"), currentUserID(c), c.ClientIP())
	if err != nil {
		h.writeArticleError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *ArticleHandler) writeArticleError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrArticleValidation), errors.Is(err, service.ErrInvalidArticleBody):
		responseWriteError(c, http.StatusBadRequest, responseCodeValidation, responseMessage(err), nil)
	case errors.Is(err, domain.ErrVersionConflict):
		responseWriteError(c, http.StatusConflict, responseCodeConflict, "version conflict, article was modified", nil)
	case errors.Is(err, domain.ErrArticlePublished):
		responseWriteError(c, http.StatusConflict, responseCodeConflict, "published article cannot be deleted", nil)
	case errors.Is(err, domain.ErrArticleNotEditable):
		responseWriteError(c, http.StatusConflict, responseCodeConflict, "article not editable in its current state", nil)
	case errors.Is(err, domain.ErrNotFound):
		responseWriteError(c, http.StatusNotFound, responseCodeNotFound, "article not found", nil)
	default:
		responseInternal(c, err)
	}
}

// parseArticleQuery reads filters and pagination (defaults page=1, pageSize=10,
// max 100) from query params.
func parseArticleQuery(c *gin.Context) (*domain.ArticleQuery, error) {
	q := &domain.ArticleQuery{Page: 1, PageSize: 10}
	if status := strings.TrimSpace(c.Query("status")); status != "" {
		q.Status = &status
	}
	if categoryID := strings.TrimSpace(c.Query("categoryId")); categoryID != "" {
		q.CategoryID = &categoryID
	}
	if keyword := strings.TrimSpace(c.Query("keyword")); keyword != "" {
		q.Keyword = &keyword
	}
	if pinned := strings.TrimSpace(c.Query("pinned")); pinned != "" {
		v, err := strconv.ParseBool(pinned)
		if err != nil {
			return nil, errors.New("pinned must be a boolean")
		}
		q.Pinned = &v
	}
	var err error
	if page := strings.TrimSpace(c.Query("page")); page != "" {
		if q.Page, err = strconv.Atoi(page); err != nil || q.Page < 1 {
			return nil, errors.New("page must be a positive integer")
		}
	}
	if size := strings.TrimSpace(c.Query("pageSize")); size != "" {
		if q.PageSize, err = strconv.Atoi(size); err != nil || q.PageSize < 1 || q.PageSize > 100 {
			return nil, errors.New("pageSize must be between 1 and 100")
		}
	}
	return q, nil
}

func parseIfMatch(c *gin.Context) (int, bool) {
	raw := strings.TrimSpace(c.GetHeader("If-Match"))
	if raw == "" {
		return 0, false
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < 1 {
		return 0, false
	}
	return v, true
}

func responseMessage(err error) string {
	if err == nil {
		return "invalid request"
	}
	return err.Error()
}

func articlesJSON(articles []*domain.Article) []gin.H {
	out := make([]gin.H, 0, len(articles))
	for _, a := range articles {
		out = append(out, articleJSON(a))
	}
	return out
}

func articleJSON(a *domain.Article) gin.H {
	return gin.H{
		"id":            a.ID,
		"title":         a.Title,
		"summary":       a.Summary,
		"bodyHtml":      a.BodyHTML,
		"bodyText":      a.BodyText,
		"categoryId":    a.CategoryID,
		"categoryName":  a.CategoryName,
		"coverUrl":      a.CoverURL,
		"status":        a.Status,
		"rejectReason":  a.RejectReason,
		"rejectedAt":    a.RejectedAt,
		"pinned":        a.Pinned,
		"pinnedAt":      a.PinnedAt,
		"submittedAt":   a.SubmittedAt,
		"publishedAt":   a.PublishedAt,
		"unpublishedAt": a.UnpublishedAt,
		"createdBy":     a.CreatedBy,
		"createdByName": a.CreatedByName,
		"updatedBy":     a.UpdatedBy,
		"version":       a.Version,
		"createdAt":     a.CreatedAt,
		"updatedAt":     a.UpdatedAt,
	}
}
