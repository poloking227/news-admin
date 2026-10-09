// Package domain holds the core entities, permissions, and repository
// interfaces shared by the service and repository layers.
package domain

import (
	"context"
	"errors"
	"slices"
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

// Permission points (RBAC), matching the shared contract: articles carry
// create/update/soft-delete/submit/approve/reject/unpublish/pin plus a read
// point for detail/list access.
const (
	PermArticleRead       = "articles:read"
	PermArticleCreate     = "articles:create"
	PermArticleUpdate     = "articles:update"
	PermArticleSoftDelete = "articles:soft_delete"
	PermArticleSubmit     = "articles:submit"
	PermArticleApprove    = "articles:approve"
	PermArticleReject     = "articles:reject"
	PermArticleUnpublish  = "articles:unpublish"
	PermArticlePin        = "articles:pin"
	PermCategoriesManage  = "categories:manage"
	PermUsersManage       = "users:manage"
	PermAuditRead         = "audit:read"
)

// ArticlePermissions lists every article-scoped permission point.
var ArticlePermissions = []string{
	PermArticleRead, PermArticleCreate, PermArticleUpdate, PermArticleSoftDelete,
	PermArticleSubmit, PermArticleApprove, PermArticleReject, PermArticleUnpublish,
	PermArticlePin,
}

// RolePermissions maps each role to its permission points. admin has the full
// set; editor produces/submits and may soft-delete its own drafts; reviewer
// approves/rejects/unpublishes/pins without editing content; operator is
// reserved for P1 and inactive in M0.
var RolePermissions = map[string][]string{
	RoleAdmin: append(append([]string{}, ArticlePermissions...), PermCategoriesManage, PermUsersManage, PermAuditRead),
	RoleEditor: {
		PermArticleRead, PermArticleCreate, PermArticleUpdate, PermArticleSoftDelete, PermArticleSubmit,
	},
	RoleReviewer: {
		PermArticleRead, PermArticleApprove, PermArticleReject, PermArticleUnpublish, PermArticlePin,
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

// HasPermission reports whether the role carries the given permission point.
func HasPermission(role, permission string) bool {
	return slices.Contains(PermissionsFor(role), permission)
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

// UserInput carries the mutable fields for creating an account. The stored
// password is always a bcrypt hash; MustChangePassword is forced for new
// temporary-password accounts.
type UserInput struct {
	Username     string
	PasswordHash string
	DisplayName  string
	Role         string
}

// UserUpdateInput carries optional field changes; nil fields are untouched.
type UserUpdateInput struct {
	DisplayName *string
	Role        *string
}

// UserQuery filters the admin user listing.
type UserQuery struct {
	Role     *string
	Status   *string
	Keyword  *string
	Page     int
	PageSize int
}

// UserPage is a page of admin users.
type UserPage struct {
	Items    []*User
	Total    int64
	Page     int
	PageSize int
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
	// ErrSlugConflict is returned when a category name/slug already exists.
	ErrSlugConflict = errors.New("category slug already exists")
	// ErrCategoryInUse is returned when soft-deleting a category that still
	// has linked, non-deleted articles.
	ErrCategoryInUse = errors.New("category still has linked articles")
	// ErrVersionConflict is returned on optimistic-lock mismatch (If-Match).
	ErrVersionConflict = errors.New("version conflict")
	// ErrArticleNotEditable is returned when updating/deleting an article in
	// a state other than draft (including rejected drafts).
	ErrArticleNotEditable = errors.New("article not editable in its current state")
	// ErrArticlePublished is returned when deleting a published article.
	ErrArticlePublished = errors.New("published article cannot be deleted")
	// ErrIllegalTransition is returned when an article status change is not
	// one of the legal state-machine transitions.
	ErrIllegalTransition = errors.New("illegal article status transition")
	// ErrNotArticleOwner is returned when a caller tries to submit an article
	// it did not create.
	ErrNotArticleOwner = errors.New("only the article author can submit it")
	// ErrUsernameTaken is returned when a new username collides with an
	// existing (non-deleted) account.
	ErrUsernameTaken = errors.New("username already exists")
	// ErrSelfRoleChange is returned when an admin tries to demote itself.
	ErrSelfRoleChange = errors.New("cannot change your own role")
	// ErrSelfStatusChange is returned when an admin tries to disable itself.
	ErrSelfStatusChange = errors.New("cannot disable your own account")
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
	// Create inserts a new user; ErrUsernameTaken when the username exists.
	Create(ctx context.Context, in *UserInput, now time.Time) (*User, error)
	// Update applies optional field changes; nil fields are untouched.
	Update(ctx context.Context, id string, in *UserUpdateInput, now time.Time) (*User, error)
	// SetStatus toggles active/disabled.
	SetStatus(ctx context.Context, id, status string, now time.Time) (*User, error)
	// SetPasswordHash writes a new password hash, forces must_change_password,
	// and clears password_changed_at (temporary password flow).
	SetPasswordHash(ctx context.Context, id, passwordHash string, now time.Time) error
	// List returns users matching the query with counts.
	List(ctx context.Context, q *UserQuery) (*UserPage, error)
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

// Category is the domain entity backed by the categories table.
type Category struct {
	ID          string
	Name        string
	Slug        string
	Description *string
	SortOrder   int
	// ArticleCount is the number of non-deleted articles linked to the
	// category (admin listing) or published articles (public listing).
	ArticleCount int64
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// CategoryInput carries the mutable fields of a category.
type CategoryInput struct {
	Name        string
	Slug        string
	Description *string
	SortOrder   int
}

// CategoryRepository persists categories.
type CategoryRepository interface {
	// List returns all non-deleted categories, newest first, each with its
	// non-deleted article count.
	List(ctx context.Context) ([]*Category, error)
	// ListPublished returns categories that have at least one published,
	// non-deleted article, with the published article count.
	ListPublished(ctx context.Context) ([]*Category, error)
	// Create inserts a new category and returns it with counts zeroed.
	Create(ctx context.Context, in *CategoryInput) (*Category, error)
	// Update changes mutable fields; the slug uniqueness check excludes the
	// category itself.
	Update(ctx context.Context, id string, in *CategoryInput) (*Category, error)
	// SoftDelete marks the category as deleted; ErrCategoryInUse when linked
	// non-deleted articles exist.
	SoftDelete(ctx context.Context, id string) error
	// ExistsSlug reports whether any non-deleted category uses the slug,
	// ignoring the given id.
	ExistsSlug(ctx context.Context, slug, exceptID string) (bool, error)
	// HasLinkedArticles reports whether non-deleted articles reference the
	// category.
	HasLinkedArticles(ctx context.Context, categoryID string) (bool, error)
	// FindByID returns the category or ErrNotFound.
	FindByID(ctx context.Context, id string) (*Category, error)
}

// Article statuses, matching the contract enum. There is no rejected value:
// rejection is draft + rejectReason/rejectedAt set.
const (
	ArticleStatusDraft         = "draft"
	ArticleStatusPendingReview = "pending_review"
	ArticleStatusPublished     = "published"
	ArticleStatusUnpublished   = "unpublished"
)

// Article is the domain entity backed by the articles table.
type Article struct {
	ID            string
	Title         string
	Summary       string
	BodyHTML      string
	BodyText      string
	CategoryID    string
	CategoryName  string
	CoverURL      *string
	Status        string
	RejectReason  *string
	RejectedAt    *time.Time
	Pinned        bool
	PinnedAt      *time.Time
	SubmittedAt   *time.Time
	PublishedAt   *time.Time
	UnpublishedAt *time.Time
	CreatedBy     string
	CreatedByName string
	UpdatedBy     *string
	Version       int
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// ArticleInput carries the mutable fields of an article draft. BodyText is
// the plain-text derivative computed by the service from the sanitized HTML.
type ArticleInput struct {
	Title      string
	Summary    string
	BodyHTML   string
	BodyText   string
	CategoryID string
	CoverURL   *string
}

// ArticleQuery filters the admin article listing.
type ArticleQuery struct {
	Status     *string
	CategoryID *string
	Keyword    *string
	Pinned     *bool
	Page       int
	PageSize   int
	// UserID is the caller for visibility; admin/reviewer see all,
	// editor sees own non-deleted plus all published.
	UserID string
	Role   string
}

// PublicArticleQuery filters the anonymous public listing and search. Only
// published, non-deleted articles are ever matched; sorting is pinned-first
// then newest publishedAt.
type PublicArticleQuery struct {
	CategoryID *string
	Keyword    *string
	Page       int
	PageSize   int
}

// ArticlePage is a page of admin articles.
type ArticlePage struct {
	Items    []*Article
	Total    int64
	Page     int
	PageSize int
}

// ArticleRepository persists articles.
type ArticleRepository interface {
	// Create inserts a new draft article.
	Create(ctx context.Context, in *ArticleInput, actorID string, now time.Time) (*Article, error)
	// FindByID returns the article by id (non-deleted), or ErrNotFound.
	FindByID(ctx context.Context, id string) (*Article, error)
	// Update applies a partial change guarded by the stored version; the
	// version is bumped on success. ErrVersionConflict when the stored
	// version differs from the caller's expectation.
	Update(ctx context.Context, id string, in *ArticleInput, expectedVersion int, actorID string, now time.Time) (*Article, error)
	// SoftDelete marks the article deleted, guarded by status.
	SoftDelete(ctx context.Context, id string, now time.Time) error
	// List returns articles matching the query with counts.
	List(ctx context.Context, q *ArticleQuery) (*ArticlePage, error)
	// Transition applies one legal status transition guarded by the current
	// status. extra carries transition-specific column values (timestamps,
	// reject reason); the version is bumped and updated_by is refreshed.
	// Illegal or stale-status transitions return ErrIllegalTransition.
	Transition(ctx context.Context, id, from, to string, extra map[string]any, actorID string, now time.Time) (*Article, error)
	// SetPinned toggles the pin on a published article; the version is
	// bumped. Non-published articles return ErrIllegalTransition.
	SetPinned(ctx context.Context, id string, pinned bool, actorID string, now time.Time) (*Article, error)
	// ListPublic returns published, non-deleted articles matching the query,
	// pinned first then by newest publishedAt.
	ListPublic(ctx context.Context, q *PublicArticleQuery) (*ArticlePage, error)
	// SearchPublic returns published, non-deleted articles whose title,
	// summary, or body text matches the keyword (pg_trgm ILIKE).
	SearchPublic(ctx context.Context, q *PublicArticleQuery) (*ArticlePage, error)
	// FindPublic returns a published, non-deleted article by id, or
	// ErrNotFound so hidden articles stay indistinguishable.
	FindPublic(ctx context.Context, id string) (*Article, error)
}
