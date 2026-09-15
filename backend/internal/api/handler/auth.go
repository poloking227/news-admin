// Package handler contains the HTTP handlers for the API server.
package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"news-admin/backend/internal/auth"
	"news-admin/backend/internal/domain"
	"news-admin/backend/internal/service"
)

// Cookie names and attributes per the contract (refresh cookie Path scoped to
// the auth routes).
const (
	refreshCookie = "refresh_token"
	csrfCookie    = "csrf_token"
	csrfHeader    = "X-CSRF-Token"
	authPath      = "/api/v1/auth"
)

// AuthHandler exposes the /auth/* endpoints.
type AuthHandler struct {
	svc *service.AuthService
	// secure marks the refresh cookie Secure (production only).
	secure bool
}

// NewAuthHandler builds an AuthHandler.
func NewAuthHandler(svc *service.AuthService, secure bool) *AuthHandler {
	return &AuthHandler{svc: svc, secure: secure}
}

type loginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// Login handles POST /auth/login.
func (h *AuthHandler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		responseValidation(c, err)
		return
	}

	session, err := h.svc.Login(c.Request.Context(), req.Username, req.Password, c.ClientIP())
	if err != nil {
		switch {
		case errors.Is(err, service.ErrRateLimited):
			responseWriteError(c, http.StatusTooManyRequests, responseCodeRateLimited, "too many failed attempts, retry later", nil)
		case errors.Is(err, service.ErrAccountDisabled):
			responseWriteError(c, http.StatusForbidden, responseCodeForbidden, "account disabled", nil)
		case errors.Is(err, service.ErrInvalidCredentials):
			responseWriteError(c, http.StatusUnauthorized, responseCodeUnauthenticated, "invalid username or password", nil)
		default:
			responseInternal(c, err)
		}
		return
	}

	h.setAuthCookies(c, session)
	c.JSON(http.StatusOK, gin.H{
		"accessToken": session.AccessToken,
		"expiresIn":   session.ExpiresIn,
		"user":        currentUserJSON(c, session.User),
	})
}

// Refresh handles POST /auth/refresh (double-submit CSRF check).
func (h *AuthHandler) Refresh(c *gin.Context) {
	cookie, err := c.Cookie(refreshCookie)
	if err != nil || cookie == "" {
		responseWriteError(c, http.StatusUnauthorized, responseCodeUnauthenticated, "refresh token missing", nil)
		return
	}
	// Double-submit CSRF: the X-CSRF-Token header must match the csrf cookie
	// issued alongside the refresh token.
	csrfVal := csrfFromCookies(c)
	if h := c.GetHeader(csrfHeader); h == "" || csrfVal == "" || h != csrfVal {
		responseWriteError(c, http.StatusUnauthorized, responseCodeUnauthenticated, "CSRF token mismatch", nil)
		return
	}

	session, err := h.svc.Refresh(c.Request.Context(), cookie, csrfVal, c.ClientIP())
	if err != nil {
		if errors.Is(err, auth.ErrInvalidToken) {
			h.clearAuthCookies(c)
			responseWriteError(c, http.StatusUnauthorized, responseCodeUnauthenticated, "invalid or expired refresh token", nil)
			return
		}
		responseInternal(c, err)
		return
	}
	h.setAuthCookies(c, session)
	c.JSON(http.StatusOK, gin.H{
		"accessToken": session.AccessToken,
		"expiresIn":   session.ExpiresIn,
		"user":        currentUserJSON(c, session.User),
	})
}

// Logout handles POST /auth/logout (204).
func (h *AuthHandler) Logout(c *gin.Context) {
	cookie, err := c.Cookie(refreshCookie)
	if err == nil && cookie != "" {
		_ = h.svc.Logout(c.Request.Context(), cookie, c.ClientIP(), currentUserID(c))
	}
	h.clearAuthCookies(c)
	c.Status(http.StatusNoContent)
}

type changePasswordRequest struct {
	OldPassword string `json:"oldPassword" binding:"required"`
	NewPassword string `json:"newPassword" binding:"required,min=8,max=72"`
}

// ChangePassword handles POST /auth/change-password (204).
func (h *AuthHandler) ChangePassword(c *gin.Context) {
	var req changePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		responseValidation(c, err)
		return
	}

	err := h.svc.ChangePassword(c.Request.Context(), currentUserID(c), req.OldPassword, req.NewPassword, c.ClientIP())
	if err != nil {
		switch {
		case errors.Is(err, service.ErrWrongPassword):
			responseWriteError(c, http.StatusUnprocessableEntity, responseCodeUnprocessable, "old password is incorrect", nil)
		case errors.Is(err, service.ErrPasswordPolicy):
			responseWriteError(c, http.StatusBadRequest, responseCodeValidation, "new password must be 8-72 characters", nil)
		default:
			responseInternal(c, err)
		}
		return
	}
	h.clearAuthCookies(c)
	c.Status(http.StatusNoContent)
}

// Me handles GET /auth/me.
func (h *AuthHandler) Me(c *gin.Context) {
	user, err := h.svc.Me(c.Request.Context(), currentUserID(c))
	if err != nil {
		responseInternal(c, err)
		return
	}
	c.JSON(http.StatusOK, currentUserJSON(c, user))
}

// currentUserJSON builds the CurrentUser contract shape: user fields
// (camelCase) plus permissions and mustChangePassword.
func currentUserJSON(c *gin.Context, u *domain.User) gin.H {
	return gin.H{
		"id":                 u.ID,
		"username":           u.Username,
		"displayName":        u.DisplayName,
		"role":               u.Role,
		"status":             u.Status,
		"permissions":        domain.PermissionsFor(u.Role),
		"mustChangePassword": u.MustChangePassword,
		"passwordChangedAt":  u.PasswordChangedAt,
		"createdAt":          u.CreatedAt,
		"updatedAt":          u.UpdatedAt,
	}
}

func (h *AuthHandler) setAuthCookies(c *gin.Context, session *service.Session) {
	maxAge := int(auth.RefreshTTL.Seconds())
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(refreshCookie, session.RefreshJTI, maxAge, authPath, "", h.secure, true)
	c.SetCookie(csrfCookie, session.CSRF, maxAge, authPath, "", h.secure, false)
}

func (h *AuthHandler) clearAuthCookies(c *gin.Context) {
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(refreshCookie, "", -1, authPath, "", h.secure, true)
	c.SetCookie(csrfCookie, "", -1, authPath, "", h.secure, false)
}

func csrfFromCookies(c *gin.Context) string {
	v, err := c.Cookie(csrfCookie)
	if err != nil {
		return ""
	}
	return v
}
