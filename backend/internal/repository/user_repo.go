// Package repository implements the domain repository interfaces with GORM
// against PostgreSQL.
package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"news-admin/backend/internal/domain"
)

// userRow maps the users table (snake_case) to the domain User.
type userRow struct {
	ID                 string     `gorm:"column:id;type:uuid;primaryKey"`
	Username           string     `gorm:"column:username"`
	PasswordHash       string     `gorm:"column:password_hash"`
	DisplayName        string     `gorm:"column:display_name"`
	Role               string     `gorm:"column:role"`
	Status             string     `gorm:"column:status"`
	MustChangePassword bool       `gorm:"column:must_change_password"`
	PasswordChangedAt  *time.Time `gorm:"column:password_changed_at"`
	CreatedAt          time.Time  `gorm:"column:created_at"`
	UpdatedAt          time.Time  `gorm:"column:updated_at"`
	DeletedAt          *time.Time `gorm:"column:deleted_at"`
}

func (userRow) TableName() string { return "users" }

func (r userRow) toDomain() *domain.User {
	return &domain.User{
		ID:                 r.ID,
		Username:           r.Username,
		PasswordHash:       r.PasswordHash,
		DisplayName:        r.DisplayName,
		Role:               r.Role,
		Status:             r.Status,
		MustChangePassword: r.MustChangePassword,
		PasswordChangedAt:  r.PasswordChangedAt,
		CreatedAt:          r.CreatedAt,
		UpdatedAt:          r.UpdatedAt,
	}
}

// UserRepository is the GORM-backed user repository.
type UserRepository struct {
	db *gorm.DB
}

// NewUserRepository builds a UserRepository.
func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) FindByUsername(ctx context.Context, username string) (*domain.User, error) {
	var row userRow
	err := r.db.WithContext(ctx).Where("username = ? AND deleted_at IS NULL", username).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return row.toDomain(), nil
}

func (r *UserRepository) FindByID(ctx context.Context, id string) (*domain.User, error) {
	var row userRow
	err := r.db.WithContext(ctx).Where("id = ? AND deleted_at IS NULL", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return row.toDomain(), nil
}

func (r *UserRepository) UpdatePassword(ctx context.Context, id, passwordHash string, changedAt time.Time) error {
	return r.db.WithContext(ctx).Model(&userRow{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"password_hash":        passwordHash,
			"must_change_password": false,
			"password_changed_at":  changedAt,
			"updated_at":           changedAt,
		}).Error
}

// Create inserts a new user with the given bcrypt hash and forces the M0
// temporary-password flag. A citext uniqueness violation becomes
// ErrUsernameTaken.
func (r *UserRepository) Create(ctx context.Context, in *domain.UserInput, now time.Time) (*domain.User, error) {
	row := userRow{
		ID:                 newCategoryID(),
		Username:           in.Username,
		PasswordHash:       in.PasswordHash,
		DisplayName:        in.DisplayName,
		Role:               in.Role,
		Status:             domain.UserStatusActive,
		MustChangePassword: true,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, domain.ErrUsernameTaken
		}
		return nil, err
	}
	return row.toDomain(), nil
}

// Update applies optional field changes; nil means "leave unchanged".
func (r *UserRepository) Update(ctx context.Context, id string, in *domain.UserUpdateInput, now time.Time) (*domain.User, error) {
	updates := map[string]any{"updated_at": now}
	if in.DisplayName != nil {
		updates["display_name"] = *in.DisplayName
	}
	if in.Role != nil {
		updates["role"] = *in.Role
	}
	res := r.db.WithContext(ctx).Model(&userRow{}).
		Where("id = ? AND deleted_at IS NULL", id).
		Updates(updates)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, domain.ErrNotFound
	}
	return r.FindByID(ctx, id)
}

// SetStatus toggles the account between active and disabled.
func (r *UserRepository) SetStatus(ctx context.Context, id, status string, now time.Time) (*domain.User, error) {
	res := r.db.WithContext(ctx).Model(&userRow{}).
		Where("id = ? AND deleted_at IS NULL", id).
		Updates(map[string]any{"status": status, "updated_at": now})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, domain.ErrNotFound
	}
	return r.FindByID(ctx, id)
}

// SetPasswordHash installs a temporary password and clears the change
// timestamp: the account must change it on the next login (M0).
func (r *UserRepository) SetPasswordHash(ctx context.Context, id, passwordHash string, now time.Time) error {
	return r.db.WithContext(ctx).Model(&userRow{}).
		Where("id = ? AND deleted_at IS NULL", id).
		Updates(map[string]any{
			"password_hash":        passwordHash,
			"must_change_password": true,
			"password_changed_at":  nil,
			"updated_at":           now,
		}).Error
}

// List returns users matching role/status/keyword filters with pagination
// (defaults page=1, pageSize=10, max 100), newest first.
func (r *UserRepository) List(ctx context.Context, q *domain.UserQuery) (*domain.UserPage, error) {
	base := r.db.WithContext(ctx).Model(&userRow{}).Where("deleted_at IS NULL")
	if q.Role != nil && *q.Role != "" {
		base = base.Where("role = ?", *q.Role)
	}
	if q.Status != nil && *q.Status != "" {
		base = base.Where("status = ?", *q.Status)
	}
	if q.Keyword != nil && *q.Keyword != "" {
		kw := "%" + strings.ToLower(*q.Keyword) + "%"
		base = base.Where("(LOWER(username) LIKE ? OR LOWER(display_name) LIKE ?)", kw, kw)
	}
	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, err
	}
	if q.Page < 1 {
		q.Page = 1
	}
	if q.PageSize < 1 || q.PageSize > 100 {
		q.PageSize = 10
	}
	var rows []userRow
	err := base.
		Order("created_at DESC").
		Offset((q.Page - 1) * q.PageSize).
		Limit(q.PageSize).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	items := make([]*domain.User, 0, len(rows))
	for i := range rows {
		items = append(items, rows[i].toDomain())
	}
	return &domain.UserPage{Items: items, Total: total, Page: q.Page, PageSize: q.PageSize}, nil
}
