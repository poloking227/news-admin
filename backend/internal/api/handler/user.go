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

// UserHandler exposes the admin user-management endpoints: list, create,
// update, status toggle, and password reset.
type UserHandler struct {
	svc *service.UserService
}

// NewUserHandler builds a UserHandler.
func NewUserHandler(svc *service.UserService) *UserHandler {
	return &UserHandler{svc: svc}
}

type userCreateRequest struct {
	Username    string `json:"username" binding:"required,max=50"`
	Password    string `json:"password" binding:"required,min=8,max=72"`
	DisplayName string `json:"displayName" binding:"required,max=50"`
	Role        string `json:"role" binding:"required"`
}

type userUpdateRequest struct {
	DisplayName *string `json:"displayName" binding:"omitempty,max=50"`
	Role        *string `json:"role"`
}

type userStatusRequest struct {
	Status string `json:"status" binding:"required"`
}

// List handles GET /api/v1/admin/users (role/status/keyword + pagination).
func (h *UserHandler) List(c *gin.Context) {
	q, err := parseUserQuery(c)
	if err != nil {
		responseValidation(c, err)
		return
	}
	page, err := h.svc.List(c.Request.Context(), q)
	if err != nil {
		responseInternal(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"items":    usersJSON(page.Items),
		"total":    page.Total,
		"page":     page.Page,
		"pageSize": page.PageSize,
	})
}

// Create handles POST /api/v1/admin/users; new accounts are opened with a
// temporary password and forced change (M0).
func (h *UserHandler) Create(c *gin.Context) {
	var req userCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		responseValidation(c, err)
		return
	}
	user, err := h.svc.Create(c.Request.Context(), &domain.UserInput{
		Username:     req.Username,
		PasswordHash: req.Password,
		DisplayName:  req.DisplayName,
		Role:         req.Role,
	}, currentUserID(c), c.ClientIP())
	if err != nil {
		h.writeUserError(c, err)
		return
	}
	c.JSON(http.StatusCreated, userJSON(user))
}

// Update handles PUT /api/v1/admin/users/{id} (partial, self-demotion 409).
func (h *UserHandler) Update(c *gin.Context) {
	var req userUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		responseValidation(c, err)
		return
	}
	user, err := h.svc.Update(c.Request.Context(), c.Param("id"), &domain.UserUpdateInput{
		DisplayName: req.DisplayName,
		Role:        req.Role,
	}, currentUserID(c), c.ClientIP())
	if err != nil {
		h.writeUserError(c, err)
		return
	}
	c.JSON(http.StatusOK, userJSON(user))
}

// SetStatus handles PATCH /api/v1/admin/users/{id}/status; disabling
// yourself is rejected and disabling revokes the user's sessions.
func (h *UserHandler) SetStatus(c *gin.Context) {
	var req userStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		responseValidation(c, err)
		return
	}
	user, err := h.svc.SetStatus(c.Request.Context(), c.Param("id"), req.Status, currentUserID(c), c.ClientIP())
	if err != nil {
		h.writeUserError(c, err)
		return
	}
	c.JSON(http.StatusOK, userJSON(user))
}

// ResetPassword handles POST /api/v1/admin/users/{id}/reset-password and
// returns the temporary password exactly once.
func (h *UserHandler) ResetPassword(c *gin.Context) {
	temp, _, err := h.svc.ResetPassword(c.Request.Context(), c.Param("id"), currentUserID(c), c.ClientIP())
	if err != nil {
		h.writeUserError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"temporaryPassword": temp})
}

func (h *UserHandler) writeUserError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrUsernameInvalid), errors.Is(err, service.ErrPasswordPolicy),
		errors.Is(err, service.ErrDisplayNameInvalid), errors.Is(err, service.ErrInvalidRole),
		errors.Is(err, service.ErrInvalidStatus):
		responseWriteError(c, http.StatusBadRequest, responseCodeValidation, responseMessage(err), nil)
	case errors.Is(err, domain.ErrUsernameTaken):
		responseWriteError(c, http.StatusConflict, responseCodeConflict, "username already exists", nil)
	case errors.Is(err, domain.ErrSelfRoleChange):
		responseWriteError(c, http.StatusConflict, responseCodeConflict, "cannot change your own role", nil)
	case errors.Is(err, domain.ErrSelfStatusChange):
		responseWriteError(c, http.StatusConflict, responseCodeConflict, "cannot disable your own account", nil)
	case errors.Is(err, domain.ErrNotFound):
		responseWriteError(c, http.StatusNotFound, responseCodeNotFound, "user not found", nil)
	default:
		responseInternal(c, err)
	}
}

// parseUserQuery reads role/status/keyword filters and pagination knobs.
func parseUserQuery(c *gin.Context) (*domain.UserQuery, error) {
	q := &domain.UserQuery{Page: 1, PageSize: 10}
	if role := strings.TrimSpace(c.Query("role")); role != "" {
		q.Role = &role
	}
	if status := strings.TrimSpace(c.Query("status")); status != "" {
		q.Status = &status
	}
	if keyword := strings.TrimSpace(c.Query("keyword")); keyword != "" {
		q.Keyword = &keyword
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

func userJSON(u *domain.User) gin.H {
	return gin.H{
		"id":                 u.ID,
		"username":           u.Username,
		"displayName":        u.DisplayName,
		"role":               u.Role,
		"status":             u.Status,
		"mustChangePassword": u.MustChangePassword,
		"passwordChangedAt":  u.PasswordChangedAt,
		"createdAt":          u.CreatedAt,
		"updatedAt":          u.UpdatedAt,
	}
}

func usersJSON(users []*domain.User) []gin.H {
	out := make([]gin.H, 0, len(users))
	for _, u := range users {
		out = append(out, userJSON(u))
	}
	return out
}
