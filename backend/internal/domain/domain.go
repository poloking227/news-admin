// Package domain holds the core entities, permissions, and repository
// interfaces shared by the service and repository layers.
package domain

import (
	"context"
	"errors"
	"time"
)

// Roles and statuses, matching the contract enums.
const (
	RoleAdmin    = "admin"
	RoleEditor   = "editor"
	RoleReviewer = "reviewer"
	RoleOperator = "operator"

	UserStatusActive   = "active"
	UserStatusDisabled = "disabled"
)

// Permission points (RBAC), matching docs/openapi.yaml and the shared contract.
const (
	PermArticleCreate    = "articles:create"
	PermArticleUpdate    = "articles:update"
	PermArticleDelete    = "articles:delete"
	PermArticleSubmit    = "articles:submit"
	PermArticleApprove   = "articles:approve"
	PermArticleReject    = "articles:reject"
	PermArticleUnpublish = "articles:unpublish"
	PermArticlePin       = "articles:pin"
	PermCategoriesManage = "categories:manage"
	PermUsersManage      = "users:manage"
	PermAuditRead        = "audit:read"
)

// RolePermissions maps each role to its permission points.
var RolePermissions = map[string][]string{
	RoleAdmin: {
		PermArticleCreate, PermArticleUpdate, PermArticleDelete, PermArticleSubmit,
		PermArticleApprove, PermArticleReject, PermArticleUnpublish, PermArticlePin,
		PermCategoriesManage, PermUsersManage, PermAuditRead,
	},
	RoleEditor: {
		PermArticleCreate, PermArticleUpdate, PermArticleDelete, PermArticleSubmit,
	},
	RoleReviewer: {
		PermArticleApprove, PermArticleReject, PermArticleUnpublish, PermArticlePin,
	},
	RoleOperator: {},
}

// PermissionsFor returns the permission points for a role.
func PermissionsFor(role string) []string {
	perms, ok := RolePermissions[role]
	if !ok {
		return nil
	}
	return perms
}

// User is the domain entity backed by the users table.
type User struct {
	ID                 string
	Username           string
	PasswordHash       string
	DisplayName        string
	Role               string
	Status             string
	MustChangePassword bool
	PasswordChangedAt  *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// RefreshSession is one issued refresh token (jti), bound to a reuse-detection
// family. Reusing a revoked jti revokes the whole family.
type RefreshSession struct {
	ID        string
	UserID    string
	JTI       string
	FamilyID  string
	ExpiresAt time.Time
	RevokedAt *time.Time
	CreatedAt time.Time
}

// AuditLog is one audit entry written in the same transaction as the change.
type AuditLog struct {
	Actor        string
	Action       string
	ResourceType string
	ResourceID   *string
	Before       map[string]any
	After        map[string]any
	IP           string
	CreatedAt    time.Time
}

// Audit actions, matching the contract enum.
const (
	ActionLogin           = "login"
	ActionFailedLogin     = "failed_login"
	ActionLogout          = "logout"
	ActionPasswordChange  = "user_password_change"
	ActionPasswordReset   = "user_reset_password"
	ActionUserCreate      = "user_create"
	ActionUserUpdate      = "user_update"
	ActionUserDisable     = "user_disable"
	ActionArticleCreate   = "article_create"
	ActionArticleUpdate   = "article_update"
	ActionArticleSoftDel  = "article_soft_delete"
	ActionArticleSubmit   = "article_submit"
	ActionArticleApprove  = "article_approve"
	ActionArticleReject   = "article_reject"
	ActionArticleUnpub    = "article_unpublish"
	ActionArticlePin      = "article_pin"
	ActionCategoryCreate  = "category_create"
	ActionCategoryUpdate  = "category_update"
	ActionCategorySoftDel = "category_soft_delete"
)

// Repository errors.
var (
	ErrNotFound = errors.New("not found")
)

// UserRepository persists users.
type UserRepository interface {
	// FindByUsername returns the user by exact username, or ErrNotFound.
	FindByUsername(ctx context.Context, username string) (*User, error)
	// FindByID returns the user by id, or ErrNotFound.
	FindByID(ctx context.Context, id string) (*User, error)
	// UpdatePassword writes a new password hash, clears must_change_password,
	// and records password_changed_at for the user.
	UpdatePassword(ctx context.Context, id, passwordHash string, changedAt time.Time) error
}

// SessionRepository persists refresh sessions.
type SessionRepository interface {
	// Insert stores a new refresh session.
	Insert(ctx context.Context, s *RefreshSession) error
	// FindByJTI returns the session by jti, or ErrNotFound.
	FindByJTI(ctx context.Context, jti string) (*RefreshSession, error)
	// Revoke marks the session with the given jti as revoked.
	Revoke(ctx context.Context, jti string) error
	// RevokeFamily revokes every non-revoked session in the family.
	RevokeFamily(ctx context.Context, familyID string) error
	// RevokeAllByUser revokes every non-revoked session of the user.
	RevokeAllByUser(ctx context.Context, userID string) error
}

// AuditRepository writes audit entries.
type AuditRepository interface {
	// Insert stores an audit entry.
	Insert(ctx context.Context, entry *AuditLog) error
}
