package repository

import (
	"context"
	"time"

	"gorm.io/gorm"

	"news-admin/backend/internal/domain"
)

// auditRow maps the audit_logs table. Before/After are JSONB columns that
// GORM marshals from maps automatically. ActorName is joined from users.
type auditRow struct {
	ID           int64          `gorm:"column:id;primaryKey"`
	Actor        string         `gorm:"column:actor;type:uuid"`
	ActorName    string         `gorm:"column:actor_name"`
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
	// actor_name is a read-side join projection, never a stored column.
	return r.db.WithContext(ctx).Omit("actor_name").Create(&row).Error
}

// List returns audit entries matching every supplied filter, sorted newest
// first. ActorName resolves to the actor's display name (deleted users fall
// back to the username at insert time via the left join).
func (r *AuditRepository) List(ctx context.Context, q *domain.AuditQuery) (*domain.AuditPage, error) {
	base := r.db.WithContext(ctx).
		Table("audit_logs a").
		Joins("LEFT JOIN users u ON u.id = a.actor").
		Select("a.*, COALESCE(u.display_name, u.username, '') AS actor_name")
	if q.ActorID != nil && *q.ActorID != "" {
		base = base.Where("a.actor = ?", *q.ActorID)
	}
	if q.Action != nil && *q.Action != "" {
		base = base.Where("a.action = ?", *q.Action)
	}
	if q.ResourceType != nil && *q.ResourceType != "" {
		base = base.Where("a.resource_type = ?", *q.ResourceType)
	}
	if q.ResourceID != nil && *q.ResourceID != "" {
		base = base.Where("a.resource_id = ?", *q.ResourceID)
	}
	if q.From != nil {
		base = base.Where("a.created_at >= ?", *q.From)
	}
	if q.To != nil {
		base = base.Where("a.created_at <= ?", *q.To)
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
	var rows []auditRow
	err := base.
		Order("a.created_at DESC, a.id DESC").
		Offset((q.Page - 1) * q.PageSize).
		Limit(q.PageSize).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	items := make([]*domain.AuditLog, 0, len(rows))
	for i := range rows {
		items = append(items, rows[i].toDomain())
	}
	return &domain.AuditPage{Items: items, Total: total, Page: q.Page, PageSize: q.PageSize}, nil
}

func (r auditRow) toDomain() *domain.AuditLog {
	return &domain.AuditLog{
		ID:           r.ID,
		Actor:        r.Actor,
		ActorName:    r.ActorName,
		Action:       r.Action,
		ResourceType: r.ResourceType,
		ResourceID:   r.ResourceID,
		Before:       r.Before,
		After:        r.After,
		IP:           r.IP,
		CreatedAt:    r.CreatedAt,
	}
}
