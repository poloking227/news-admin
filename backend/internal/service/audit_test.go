package service_test

import (
	"context"
	"testing"
	"time"

	"news-admin/backend/internal/domain"
	"news-admin/backend/internal/service"
)

func TestAuditListFilters(t *testing.T) {
	audit := newFakeAuditRepo()
	svc := service.NewAuditService(audit)

	base := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	entries := []*domain.AuditLog{
		{ID: 1, Actor: "u1", ActorName: "甲", Action: "login", ResourceType: "user", IP: "1.1.1.1", CreatedAt: base},
		{ID: 2, Actor: "u1", ActorName: "甲", Action: "article_create", ResourceType: "article", IP: "2.2.2.2", CreatedAt: base.Add(10 * time.Minute)},
		{ID: 3, Actor: "u2", ActorName: "乙", Action: "article_approve", ResourceType: "article", IP: "3.3.3.3", CreatedAt: base.Add(30 * time.Minute)},
		{ID: 4, Actor: "u1", ActorName: "甲", Action: "article_create", ResourceType: "article", IP: "4.4.4.4", CreatedAt: base.Add(60 * time.Minute)},
	}
	for _, e := range entries {
		if err := audit.Insert(context.Background(), e); err != nil {
			t.Fatalf("Insert() error = %v", err)
		}
	}

	// Single filter: action.
	action := "article_create"
	page, err := svc.List(context.Background(), &domain.AuditQuery{Action: &action, Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("List(action) error = %v", err)
	}
	if page.Total != 2 {
		t.Errorf("action filter total = %d, want 2", page.Total)
	}

	// Single filter: actorId.
	actor := "u2"
	page, err = svc.List(context.Background(), &domain.AuditQuery{ActorID: &actor, Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("List(actor) error = %v", err)
	}
	if page.Total != 1 || page.Items[0].ActorName != "乙" {
		t.Errorf("actor filter got total=%d items=%+v", page.Total, page.Items)
	}

	// Time window: inclusive boundaries (from <= t <= to).
	from := base.Add(10 * time.Minute)
	to := base.Add(30 * time.Minute)
	page, err = svc.List(context.Background(), &domain.AuditQuery{From: &from, To: &to, Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("List(from/to) error = %v", err)
	}
	if page.Total != 2 { // entries 2 and 3 sit exactly on the boundaries.
		t.Errorf("time window total = %d, want 2", page.Total)
	}

	// Combined filters: actor + resourceType.
	resource := "article"
	page, err = svc.List(context.Background(), &domain.AuditQuery{ActorID: &actor, ResourceType: &resource, Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("List(combined) error = %v", err)
	}
	if page.Total != 1 || page.Items[0].Action != "article_approve" {
		t.Errorf("combined filter got total=%d", page.Total)
	}

	// Empty result.
	miss := "no_such_action"
	page, err = svc.List(context.Background(), &domain.AuditQuery{Action: &miss, Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("List(miss) error = %v", err)
	}
	if page.Total != 0 || len(page.Items) != 0 {
		t.Errorf("miss filter got total=%d items=%d", page.Total, len(page.Items))
	}
}

func TestAuditListEmptyStore(t *testing.T) {
	svc := service.NewAuditService(newFakeAuditRepo())
	page, err := svc.List(context.Background(), &domain.AuditQuery{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if page.Total != 0 || len(page.Items) != 0 {
		t.Errorf("empty store got total=%d items=%d", page.Total, len(page.Items))
	}
}
