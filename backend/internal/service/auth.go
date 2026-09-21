// Package service implements the application use-cases: authentication and
// session management with rate limiting, refresh rotation, reuse detection,
// and forced password change (M0).
package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"news-admin/backend/internal/auth"
	"news-admin/backend/internal/domain"
)

// Errors mapped to contract codes by the API layer.
var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrAccountDisabled    = errors.New("account disabled")
	ErrRateLimited        = errors.New("too many failed attempts")
	ErrMustChangePassword = errors.New("password change required")
	ErrPasswordPolicy     = errors.New("new password does not meet requirements")
	ErrWrongPassword      = errors.New("old password is incorrect")
	// ErrArticleValidation covers size/scheme rule violations; ErrInvalidArticleBody
	// fires when the sanitized body is empty (e.g. only script tags).
	ErrArticleValidation  = errors.New("article validation failed")
	ErrInvalidArticleBody = errors.New("article body required")
)

// AuthService coordinates login, token refresh, logout, password change, and
// the current-user projection.
type AuthService struct {
	users    domain.UserRepository
	sessions domain.SessionRepository
	audit    domain.AuditRepository
	limiter  *LoginLimiter
	secret   string
	now      func() time.Time
}

// NewAuthService builds an AuthService.
func NewAuthService(users domain.UserRepository, sessions domain.SessionRepository, audit domain.AuditRepository, jwtSecret string) *AuthService {
	return &AuthService{
		users:    users,
		sessions: sessions,
		audit:    audit,
		limiter:  newLoginLimiter(),
		secret:   jwtSecret,
		now:      time.Now,
	}
}

// Session is the outcome of a successful login or refresh.
type Session struct {
	AccessToken string
	ExpiresIn   int64
	RefreshJTI  string
	FamilyID    string
	CSRF        string
	User        *domain.User
}

// Login authenticates with username/password, enforcing per-IP and per-account
// failure limits, and starts a new refresh family.
func (s *AuthService) Login(ctx context.Context, username, password, ip string) (*Session, error) {
	if s.limiter.IsBlockedIP(ip) || s.limiter.IsBlockedUser(username) {
		return nil, ErrRateLimited
	}

	user, err := s.users.FindByUsername(ctx, username)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}
	if err != nil || !auth.VerifyPassword(user.PasswordHash, password) {
		s.limiter.RecordFailure(ip, username)
		if user != nil {
			_ = s.writeAudit(ctx, &domain.AuditLog{
				Actor: user.ID, Action: domain.ActionFailedLogin,
				ResourceType: "user", ResourceID: &user.ID, IP: ip,
			})
		}
		return nil, ErrInvalidCredentials
	}
	if user.Status != domain.UserStatusActive {
		s.limiter.RecordFailure(ip, username)
		return nil, ErrAccountDisabled
	}

	s.limiter.ResetUser(username)
	session, err := s.issueSession(ctx, user, ip)
	if err != nil {
		return nil, err
	}
	return session, nil
}

// Refresh rotates a refresh token: the presented jti is revoked and a new one
// joins the same family. Presenting an already-revoked jti is treated as token
// reuse and revokes the whole family.
func (s *AuthService) Refresh(ctx context.Context, jti, csrf, ip string) (*Session, error) {
	sess, err := s.sessions.FindByJTI(ctx, jti)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, auth.ErrInvalidToken
	}
	if err != nil {
		return nil, err
	}
	if sess.RevokedAt != nil {
		if revokeErr := s.sessions.RevokeFamily(ctx, sess.FamilyID); revokeErr != nil {
			return nil, revokeErr
		}
		return nil, auth.ErrInvalidToken
	}
	if sess.ExpiresAt.Before(s.now()) {
		return nil, auth.ErrInvalidToken
	}

	user, err := s.users.FindByID(ctx, sess.UserID)
	if err != nil {
		return nil, err
	}
	if user.Status != domain.UserStatusActive {
		return nil, ErrAccountDisabled
	}

	// Rotate: revoke the presented jti, issue a fresh one in the same family.
	if err := s.sessions.Revoke(ctx, jti); err != nil {
		return nil, err
	}
	newFamily := sess.FamilyID
	newJTI, err := auth.NewRefreshJTI()
	if err != nil {
		return nil, err
	}
	next := &domain.RefreshSession{
		ID:        newUUID(),
		UserID:    user.ID,
		JTI:       newJTI,
		FamilyID:  newFamily,
		ExpiresAt: s.now().Add(auth.RefreshTTL),
		CreatedAt: s.now(),
	}
	if err := s.sessions.Insert(ctx, next); err != nil {
		return nil, err
	}

	token, err := auth.SignAccessToken(s.secret, user.ID, user.Role, s.now())
	if err != nil {
		return nil, err
	}
	return &Session{
		AccessToken: token,
		ExpiresIn:   int64(auth.AccessTokenTTL.Seconds()),
		RefreshJTI:  newJTI,
		FamilyID:    newFamily,
		CSRF:        csrf,
		User:        user,
	}, nil
}

// Logout revokes the family of the presented refresh token.
func (s *AuthService) Logout(ctx context.Context, jti, ip string, actorID string) error {
	sess, err := s.sessions.FindByJTI(ctx, jti)
	if errors.Is(err, domain.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := s.sessions.RevokeFamily(ctx, sess.FamilyID); err != nil {
		return err
	}
	return s.writeAudit(ctx, &domain.AuditLog{
		Actor: actorID, Action: domain.ActionLogout,
		ResourceType: "session", ResourceID: &sess.FamilyID, IP: ip,
	})
}

// Me returns the current user with permissions and the forced-change flag.
func (s *AuthService) Me(ctx context.Context, userID string) (*domain.User, error) {
	user, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	return user, nil
}

// ChangePassword validates the old password, stores a new hash, clears the
// forced-change flag, records password_changed_at, and revokes all refresh
// families so the client must log in again.
func (s *AuthService) ChangePassword(ctx context.Context, userID, oldPassword, newPassword, ip string) error {
	if len(newPassword) < 8 || len(newPassword) > 72 {
		return ErrPasswordPolicy
	}
	user, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	if !auth.VerifyPassword(user.PasswordHash, oldPassword) {
		return ErrWrongPassword
	}

	newHash, err := auth.HashPassword(newPassword)
	if err != nil {
		return err
	}
	changedAt := s.now()
	if err := s.users.UpdatePassword(ctx, userID, newHash, changedAt); err != nil {
		return err
	}
	// Rotate out every refresh family so the client must log in again.
	if err := s.sessions.RevokeAllByUser(ctx, userID); err != nil {
		return err
	}
	return s.writeAudit(ctx, &domain.AuditLog{
		Actor: userID, Action: domain.ActionPasswordChange,
		ResourceType: "user", ResourceID: &userID, IP: ip,
	})
}

func (s *AuthService) issueSession(ctx context.Context, user *domain.User, ip string) (*Session, error) {
	jti, err := auth.NewRefreshJTI()
	if err != nil {
		return nil, err
	}
	csrf, err := auth.NewRefreshJTI()
	if err != nil {
		return nil, err
	}
	familyID, err := auth.NewRefreshJTI()
	if err != nil {
		return nil, err
	}
	sess := &domain.RefreshSession{
		ID:        newUUID(),
		UserID:    user.ID,
		JTI:       jti,
		FamilyID:  familyID,
		ExpiresAt: s.now().Add(auth.RefreshTTL),
		CreatedAt: s.now(),
	}
	if err := s.sessions.Insert(ctx, sess); err != nil {
		return nil, err
	}

	token, err := auth.SignAccessToken(s.secret, user.ID, user.Role, s.now())
	if err != nil {
		return nil, err
	}
	if err := s.writeAudit(ctx, &domain.AuditLog{
		Actor: user.ID, Action: domain.ActionLogin,
		ResourceType: "user", ResourceID: &user.ID, IP: ip,
	}); err != nil {
		return nil, err
	}
	return &Session{
		AccessToken: token,
		ExpiresIn:   int64(auth.AccessTokenTTL.Seconds()),
		RefreshJTI:  jti,
		FamilyID:    familyID,
		CSRF:        csrf,
		User:        user,
	}, nil
}

func (s *AuthService) writeAudit(ctx context.Context, entry *domain.AuditLog) error {
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = s.now()
	}
	if err := s.audit.Insert(ctx, entry); err != nil {
		return fmt.Errorf("write audit: %w", err)
	}
	return nil
}
