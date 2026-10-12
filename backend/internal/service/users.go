// Package service implements the admin user-management use-cases: creation
// with a temporary password (forced change), partial updates with
// self-demotion protection, enable/disable with session revocation, and
// password resets that return the temporary password exactly once.
package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"news-admin/backend/internal/auth"
	"news-admin/backend/internal/domain"
)

// Admin user-management validation helpers.
var (
	// ErrUsernameInvalid is returned when a username fails the format or
	// length rules.
	ErrUsernameInvalid = errors.New("username is required and limited to 50 characters")
	// ErrPasswordPolicy is reused for temporary passwords (8-72 chars).
	ErrDisplayNameInvalid = errors.New("display name is required and limited to 50 characters")
	// ErrInvalidRole is returned when a role is not assignable in M0.
	ErrInvalidRole = errors.New("invalid role")
	// ErrInvalidStatus is returned when a status is not active/disabled.
	ErrInvalidStatus = errors.New("invalid status")
)

// validRoles enumerates the assignable roles in M0 (operator is reserved).
var validRoles = []string{domain.RoleAdmin, domain.RoleEditor, domain.RoleReviewer}

func validRole(role string) bool {
	for _, r := range validRoles {
		if r == role {
			return true
		}
	}
	return false
}

// UserService orchestrates admin user management.
type UserService struct {
	users    domain.UserRepository
	sessions domain.SessionRepository
	audit    domain.AuditRepository
	now      func() time.Time
}

// NewUserService builds a UserService.
func NewUserService(users domain.UserRepository, sessions domain.SessionRepository, audit domain.AuditRepository) *UserService {
	return &UserService{users: users, sessions: sessions, audit: audit, now: time.Now}
}

// Create validates the input, hashes the temporary password, and opens the
// account in forced-change state (M0).
func (s *UserService) Create(ctx context.Context, in *domain.UserInput, actorID, ip string) (*domain.User, error) {
	if err := s.validateCreate(in); err != nil {
		return nil, err
	}
	hash, err := auth.HashPassword(in.PasswordHash)
	if err != nil {
		return nil, err
	}
	in.PasswordHash = hash
	user, err := s.users.Create(ctx, in, s.now())
	if err != nil {
		return nil, err
	}
	_ = s.writeAudit(ctx, &domain.AuditLog{
		Actor: actorID, Action: domain.ActionUserCreate,
		ResourceType: "user", ResourceID: &user.ID, IP: ip,
	})
	return user, nil
}

// Update applies optional display-name/role changes. An admin may not change
// its own role (self-demotion protection).
func (s *UserService) Update(ctx context.Context, id string, in *domain.UserUpdateInput, actorID, ip string) (*domain.User, error) {
	if in.DisplayName != nil {
		if len(*in.DisplayName) == 0 || len(*in.DisplayName) > 50 {
			return nil, ErrDisplayNameInvalid
		}
	}
	if in.Role != nil {
		if !validRole(*in.Role) {
			return nil, ErrInvalidRole
		}
		if id == actorID && *in.Role != domain.RoleAdmin {
			// Demoting yourself would lock the account out of admin.
			return nil, domain.ErrSelfRoleChange
		}
	}
	user, err := s.users.Update(ctx, id, in, s.now())
	if err != nil {
		return nil, err
	}
	_ = s.writeAudit(ctx, &domain.AuditLog{
		Actor: actorID, Action: domain.ActionUserUpdate,
		ResourceType: "user", ResourceID: &id, IP: ip,
	})
	return user, nil
}

// SetStatus enables or disables an account. Disabling yourself is rejected;
// disabling revokes every refresh family so sessions die immediately.
func (s *UserService) SetStatus(ctx context.Context, id, status, actorID, ip string) (*domain.User, error) {
	if status != domain.UserStatusActive && status != domain.UserStatusDisabled {
		return nil, ErrInvalidStatus
	}
	user, err := s.users.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if status == domain.UserStatusDisabled && id == actorID {
		return nil, domain.ErrSelfStatusChange
	}
	updated, err := s.users.SetStatus(ctx, id, status, s.now())
	if err != nil {
		return nil, err
	}
	if status == domain.UserStatusDisabled {
		if err := s.sessions.RevokeAllByUser(ctx, id); err != nil {
			return nil, err
		}
	}
	entry := &domain.AuditLog{
		Actor: actorID, Action: domain.ActionUserUpdate,
		ResourceType: "user", ResourceID: &id,
		Before: map[string]any{"status": user.Status},
		After:  map[string]any{"status": status},
		IP:     ip,
	}
	if status == domain.UserStatusDisabled {
		entry.Action = domain.ActionUserDisable
	}
	_ = s.writeAudit(ctx, entry)
	return updated, nil
}

// ResetPassword generates a temporary password, forces the next-login change
// (M0), revokes existing sessions, and returns the password exactly once.
func (s *UserService) ResetPassword(ctx context.Context, id, actorID, ip string) (string, *domain.User, error) {
	user, err := s.users.FindByID(ctx, id)
	if err != nil {
		return "", nil, err
	}
	temp := newTemporaryPassword()
	hash, err := auth.HashPassword(temp)
	if err != nil {
		return "", nil, err
	}
	if err := s.users.SetPasswordHash(ctx, id, hash, s.now()); err != nil {
		return "", nil, err
	}
	if err := s.sessions.RevokeAllByUser(ctx, id); err != nil {
		return "", nil, err
	}
	_ = s.writeAudit(ctx, &domain.AuditLog{
		Actor: actorID, Action: domain.ActionPasswordReset,
		ResourceType: "user", ResourceID: &id, IP: ip,
	})
	return temp, user, nil
}

// List returns a paged user directory with role/status/keyword filters.
func (s *UserService) List(ctx context.Context, q *domain.UserQuery) (*domain.UserPage, error) {
	return s.users.List(ctx, q)
}

func (s *UserService) validateCreate(in *domain.UserInput) error {
	if len(in.Username) == 0 || len(in.Username) > 50 {
		return ErrUsernameInvalid
	}
	if len(in.PasswordHash) < 8 || len(in.PasswordHash) > 72 {
		return ErrPasswordPolicy
	}
	if len(in.DisplayName) == 0 || len(in.DisplayName) > 50 {
		return ErrDisplayNameInvalid
	}
	if !validRole(in.Role) {
		return ErrInvalidRole
	}
	return nil
}

func (s *UserService) writeAudit(ctx context.Context, entry *domain.AuditLog) error {
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = s.now()
	}
	if err := s.audit.Insert(ctx, entry); err != nil {
		return fmt.Errorf("write audit: %w", err)
	}
	return nil
}

// newTemporaryPassword returns a 20-hex-character (10-byte) temporary
// password that satisfies the 8-72 character policy.
func newTemporaryPassword() string {
	buf := make([]byte, 10)
	if _, err := rand.Read(buf); err != nil {
		return "TmpPass!" + fmt.Sprintf("%d", time.Now().UnixNano())[:12]
	}
	return hex.EncodeToString(buf)
}
