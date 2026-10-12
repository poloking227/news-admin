package service

import (
	"context"

	"news-admin/backend/internal/domain"
)

// AuditService exposes the read-only audit-log listing for the admin console.
// Reads are never audited themselves.
type AuditService struct {
	audit domain.AuditRepository
}

// NewAuditService builds an AuditService.
func NewAuditService(audit domain.AuditRepository) *AuditService {
	return &AuditService{audit: audit}
}

// List returns a paged audit trail filtered by actor/action/resource and an
// inclusive createdAt window, newest first.
func (s *AuditService) List(ctx context.Context, q *domain.AuditQuery) (*domain.AuditPage, error) {
	return s.audit.List(ctx, q)
}
