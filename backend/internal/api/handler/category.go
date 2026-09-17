package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"news-admin/backend/internal/domain"
	"news-admin/backend/internal/service"
)

// CategoryHandler exposes the admin and public category endpoints.
type CategoryHandler struct {
	svc *service.CategoryService
}

// NewCategoryHandler builds a CategoryHandler.
func NewCategoryHandler(svc *service.CategoryService) *CategoryHandler {
	return &CategoryHandler{svc: svc}
}

type categoryRequest struct {
	Name        string  `json:"name" binding:"required,max=50"`
	Slug        string  `json:"slug" binding:"required,max=50"`
	Description *string `json:"description" binding:"omitempty,max=200"`
	SortOrder   *int    `json:"sortOrder"`
}

// List handles GET /api/v1/admin/categories.
func (h *CategoryHandler) List(c *gin.Context) {
	categories, err := h.svc.List(c.Request.Context())
	if err != nil {
		responseInternal(c, err)
		return
	}
	c.JSON(http.StatusOK, categoriesJSON(categories))
}

// ListPublic handles GET /api/v1/public/categories (anonymous, published ok).
func (h *CategoryHandler) ListPublic(c *gin.Context) {
	categories, err := h.svc.ListPublic(c.Request.Context())
	if err != nil {
		responseInternal(c, err)
		return
	}
	c.JSON(http.StatusOK, categoriesJSON(categories))
}

// Create handles POST /api/v1/admin/categories.
func (h *CategoryHandler) Create(c *gin.Context) {
	in, ok := bindCategory(c)
	if !ok {
		return
	}
	cat, err := h.svc.Create(c.Request.Context(), in, currentUserID(c), c.ClientIP())
	if err != nil {
		if errors.Is(err, domain.ErrSlugConflict) {
			responseWriteError(c, http.StatusConflict, responseCodeConflict, "category slug already exists", nil)
			return
		}
		responseInternal(c, err)
		return
	}
	c.JSON(http.StatusCreated, categoryJSON(cat))
}

// Update handles PUT /api/v1/admin/categories/{id}.
func (h *CategoryHandler) Update(c *gin.Context) {
	in, ok := bindCategory(c)
	if !ok {
		return
	}
	cat, err := h.svc.Update(c.Request.Context(), c.Param("id"), in, currentUserID(c), c.ClientIP())
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrSlugConflict):
			responseWriteError(c, http.StatusConflict, responseCodeConflict, "category slug already exists", nil)
		case errors.Is(err, domain.ErrNotFound):
			responseWriteError(c, http.StatusNotFound, responseCodeNotFound, "category not found", nil)
		default:
			responseInternal(c, err)
		}
		return
	}
	c.JSON(http.StatusOK, categoryJSON(cat))
}

// Delete handles DELETE /api/v1/admin/categories/{id} (soft delete, 204).
func (h *CategoryHandler) Delete(c *gin.Context) {
	err := h.svc.SoftDelete(c.Request.Context(), c.Param("id"), currentUserID(c), c.ClientIP())
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrCategoryInUse):
			responseWriteError(c, http.StatusConflict, responseCodeConflict, "category still has linked articles, migrate content first", nil)
		case errors.Is(err, domain.ErrNotFound):
			responseWriteError(c, http.StatusNotFound, responseCodeNotFound, "category not found", nil)
		default:
			responseInternal(c, err)
		}
		return
	}
	c.Status(http.StatusNoContent)
}

func bindCategory(c *gin.Context) (*domain.CategoryInput, bool) {
	var req categoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		responseValidation(c, err)
		return nil, false
	}
	sortOrder := 0
	if req.SortOrder != nil {
		sortOrder = *req.SortOrder
	}
	return &domain.CategoryInput{
		Name:        req.Name,
		Slug:        req.Slug,
		Description: req.Description,
		SortOrder:   sortOrder,
	}, true
}

func categoriesJSON(categories []*domain.Category) []gin.H {
	out := make([]gin.H, 0, len(categories))
	for _, cat := range categories {
		out = append(out, categoryJSON(cat))
	}
	return out
}

func categoryJSON(cat *domain.Category) gin.H {
	return gin.H{
		"id":           cat.ID,
		"name":         cat.Name,
		"slug":         cat.Slug,
		"description":  cat.Description,
		"sortOrder":    cat.SortOrder,
		"articleCount": cat.ArticleCount,
		"createdAt":    cat.CreatedAt,
		"updatedAt":    cat.UpdatedAt,
	}
}
