package repository

import (
	"context"
	"time"

	"gorm.io/gorm"

	"news-admin/backend/internal/domain"
)

// auditRow maps the audit_logs table. Before/After are JSONB columns that
// GORM marshals from maps automatically.
type auditRow struct {
	ID           int64          `gorm:"column:id;primaryKey"`
	Actor        string         `gorm:"column:actor;type:uuid"`
	Action       string         `gorm:"column:action"`
	ResourceType string         `gorm:"column:resource_type"`
	ResourceID   *string        `gorm:"column:resource_id"`
	Before       map[string]any `gorm:"column:before;type:jsonb"`
	After        map[string]any `gorm:"column:after;type:jsonb"`
	IP           string         `gorm:"column:ip"`
	CreatedAt    time.Time      `gorm:"column:created_at"`
}

func (auditRow) TableName() string { return "audit_logs" }

// AuditRepository is the GORM-backed audit repository.
type AuditRepository struct {
	db *gorm.DB
}

// NewAuditRepository builds an AuditRepository.
func NewAuditRepository(db *gorm.DB) *AuditRepository {
	return &AuditRepository{db: db}
}

func (r *AuditRepository) Insert(ctx context.Context, entry *domain.AuditLog) error {
	row := auditRow{
		Actor:        entry.Actor,
		Action:       entry.Action,
		ResourceType: entry.ResourceType,
		ResourceID:   entry.ResourceID,
		Before:       entry.Before,
		After:        entry.After,
		IP:           entry.IP,
		CreatedAt:    entry.CreatedAt,
	}
	return r.db.WithContext(ctx).Create(&row).Error
}
