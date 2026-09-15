// Package repository implements the domain repository interfaces with GORM
// against PostgreSQL.
package repository

import (
	"context"
	"errors"
	"time"

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
