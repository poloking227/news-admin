package repository

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"news-admin/backend/internal/domain"
)

// sessionRow maps the refresh_sessions table.
type sessionRow struct {
	ID        string     `gorm:"column:id;type:uuid;primaryKey"`
	UserID    string     `gorm:"column:user_id;type:uuid"`
	JTI       string     `gorm:"column:jti"`
	FamilyID  string     `gorm:"column:family_id"`
	ExpiresAt time.Time  `gorm:"column:expires_at"`
	RevokedAt *time.Time `gorm:"column:revoked_at"`
	CreatedAt time.Time  `gorm:"column:created_at"`
}

func (sessionRow) TableName() string { return "refresh_sessions" }

func (r sessionRow) toDomain() *domain.RefreshSession {
	return &domain.RefreshSession{
		ID:        r.ID,
		UserID:    r.UserID,
		JTI:       r.JTI,
		FamilyID:  r.FamilyID,
		ExpiresAt: r.ExpiresAt,
		RevokedAt: r.RevokedAt,
		CreatedAt: r.CreatedAt,
	}
}

// SessionRepository is the GORM-backed refresh session repository.
type SessionRepository struct {
	db *gorm.DB
}

// NewSessionRepository builds a SessionRepository.
func NewSessionRepository(db *gorm.DB) *SessionRepository {
	return &SessionRepository{db: db}
}

func (r *SessionRepository) Insert(ctx context.Context, s *domain.RefreshSession) error {
	row := sessionRow{
		ID:        s.ID,
		UserID:    s.UserID,
		JTI:       s.JTI,
		FamilyID:  s.FamilyID,
		ExpiresAt: s.ExpiresAt,
		RevokedAt: s.RevokedAt,
		CreatedAt: s.CreatedAt,
	}
	return r.db.WithContext(ctx).Create(&row).Error
}

func (r *SessionRepository) FindByJTI(ctx context.Context, jti string) (*domain.RefreshSession, error) {
	var row sessionRow
	err := r.db.WithContext(ctx).Where("jti = ?", jti).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return row.toDomain(), nil
}

func (r *SessionRepository) Revoke(ctx context.Context, jti string) error {
	now := time.Now()
	return r.db.WithContext(ctx).Model(&sessionRow{}).
		Where("jti = ? AND revoked_at IS NULL", jti).
		Update("revoked_at", now).Error
}

func (r *SessionRepository) RevokeFamily(ctx context.Context, familyID string) error {
	now := time.Now()
	return r.db.WithContext(ctx).Model(&sessionRow{}).
		Where("family_id = ? AND revoked_at IS NULL", familyID).
		Update("revoked_at", now).Error
}

func (r *SessionRepository) RevokeAllByUser(ctx context.Context, userID string) error {
	now := time.Now()
	return r.db.WithContext(ctx).Model(&sessionRow{}).
		Where("user_id = ? AND revoked_at IS NULL", userID).
		Update("revoked_at", now).Error
}
