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

// PublicArticleHandler exposes the anonymous reader endpoints: published
// listing, published detail (hidden states stay 404), and full-text search.
type PublicArticleHandler struct {
	svc *service.ArticleService
}

// NewPublicArticleHandler builds a PublicArticleHandler.
func NewPublicArticleHandler(svc *service.ArticleService) *PublicArticleHandler {
	return &PublicArticleHandler{svc: svc}
}

// List handles GET /api/v1/public/articles (published only, pinned first,
// newest publishedAt first).
func (h *PublicArticleHandler) List(c *gin.Context) {
	q, err := parsePublicQuery(c)
	if err != nil {
		responseValidation(c, err)
		return
	}
	page, err := h.svc.ListPublic(c.Request.Context(), q)
	if err != nil {
		responseInternal(c, err)
		return
	}
	c.JSON(http.StatusOK, publicArticlePageJSON(page))
}

// Get handles GET /api/v1/public/articles/{id}; any non-published state is
// reported as not found so drafts stay undiscoverable.
func (h *PublicArticleHandler) Get(c *gin.Context) {
	article, err := h.svc.GetPublic(c.Request.Context(), c.Param("id"))
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			responseWriteError(c, http.StatusNotFound, responseCodeNotFound, "article not found", nil)
			return
		}
		responseInternal(c, err)
		return
	}
	c.JSON(http.StatusOK, publicArticleJSON(article))
}

// Search handles GET /api/v1/public/search?q=; an empty keyword is rejected
// with 400 so the reader never gets an unconstrained scan.
func (h *PublicArticleHandler) Search(c *gin.Context) {
	q, err := parsePublicQuery(c)
	if err != nil {
		responseValidation(c, err)
		return
	}
	keyword := strings.TrimSpace(c.Query("q"))
	if keyword == "" {
		responseWriteError(c, http.StatusBadRequest, responseCodeValidation, "q is required", nil)
		return
	}
	q.Keyword = &keyword
	page, err := h.svc.SearchPublic(c.Request.Context(), q)
	if err != nil {
		responseInternal(c, err)
		return
	}
	c.JSON(http.StatusOK, publicArticlePageJSON(page))
}

// parsePublicQuery reads categoryId and pagination knobs (defaults page=1,
// pageSize=10, max 100) shared by the public list and search endpoints.
func parsePublicQuery(c *gin.Context) (*domain.PublicArticleQuery, error) {
	q := &domain.PublicArticleQuery{Page: 1, PageSize: 10}
	if categoryID := strings.TrimSpace(c.Query("categoryId")); categoryID != "" {
		q.CategoryID = &categoryID
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

// publicArticleJSON renders the reader-facing DTO: a strict subset of the
// admin article, aligned with the shared PublicArticle contract type.
func publicArticleJSON(a *domain.Article) gin.H {
	return gin.H{
		"id":           a.ID,
		"title":        a.Title,
		"summary":      a.Summary,
		"bodyHtml":     a.BodyHTML,
		"categoryId":   a.CategoryID,
		"categoryName": a.CategoryName,
		"coverUrl":     a.CoverURL,
		"publishedAt":  a.PublishedAt,
		"pinned":       a.Pinned,
	}
}

func publicArticlePageJSON(page *domain.ArticlePage) gin.H {
	items := make([]gin.H, 0, len(page.Items))
	for _, a := range page.Items {
		items = append(items, publicArticleJSON(a))
	}
	return gin.H{
		"items":    items,
		"total":    page.Total,
		"page":     page.Page,
		"pageSize": page.PageSize,
	}
}
