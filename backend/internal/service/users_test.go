package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"news-admin/backend/internal/domain"
	"news-admin/backend/internal/service"
)

func newUserSvc() (*service.UserService, *fakeUserRepo, *fakeSessionRepo, *fakeAuditRepo) {
	users := newFakeUserRepo()
	users.upsert(&domain.User{ID: "admin-1", Username: "boss", DisplayName: "老大", Role: domain.RoleAdmin, Status: domain.UserStatusActive})
	users.upsert(&domain.User{ID: "editor-1", Username: "writer", DisplayName: "作者", Role: domain.RoleEditor, Status: domain.UserStatusActive})
	sessions := newFakeSessionRepo()
	audit := newFakeAuditRepo()
	return service.NewUserService(users, sessions, audit), users, sessions, audit
}

func TestUserCreateOpensInForcedChangeState(t *testing.T) {
	svc, _, _, _ := newUserSvc()
	user, err := svc.Create(context.Background(), &domain.UserInput{
		Username: "newbie", PasswordHash: "S3cret!pass", DisplayName: "新人", Role: domain.RoleEditor,
	}, "admin-1", "ip")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if user.Status != domain.UserStatusActive {
		t.Errorf("status = %q, want active", user.Status)
	}
	if !user.MustChangePassword {
		t.Error("new user must have mustChangePassword=true (M0)")
	}
}

func TestUserCreateRejectsDuplicateUsername(t *testing.T) {
	svc, _, _, _ := newUserSvc()
	_, err := svc.Create(context.Background(), &domain.UserInput{
		Username: "boss", PasswordHash: "S3cret!pass", DisplayName: "老大2", Role: domain.RoleEditor,
	}, "admin-1", "ip")
	if !errors.Is(err, domain.ErrUsernameTaken) {
		t.Fatalf("Create(dup) error = %v, want ErrUsernameTaken", err)
	}
}

func TestUserCreateValidatesPasswordAndRole(t *testing.T) {
	svc, _, _, _ := newUserSvc()
	if _, err := svc.Create(context.Background(), &domain.UserInput{
		Username: "x1", PasswordHash: "short", DisplayName: "x", Role: domain.RoleEditor,
	}, "admin-1", "ip"); !errors.Is(err, service.ErrPasswordPolicy) {
		t.Errorf("weak password error = %v, want ErrPasswordPolicy", err)
	}
	if _, err := svc.Create(context.Background(), &domain.UserInput{
		Username: "x2", PasswordHash: "long-enough-pass", DisplayName: "x", Role: "operator",
	}, "admin-1", "ip"); !errors.Is(err, service.ErrInvalidRole) {
		t.Errorf("operator role error = %v, want ErrInvalidRole", err)
	}
}

func TestUserUpdateRejectsSelfDemotion(t *testing.T) {
	svc, _, _, _ := newUserSvc()
	role := domain.RoleEditor
	_, err := svc.Update(context.Background(), "admin-1", &domain.UserUpdateInput{Role: &role}, "admin-1", "ip")
	if !errors.Is(err, domain.ErrSelfRoleChange) {
		t.Fatalf("Update(self demote) error = %v, want ErrSelfRoleChange", err)
	}
	// A non-self demotion is fine.
	role = domain.RoleAdmin
	updated, err := svc.Update(context.Background(), "editor-1", &domain.UserUpdateInput{Role: &role}, "admin-1", "ip")
	if err != nil {
		t.Fatalf("Update(other) error = %v", err)
	}
	if updated.Role != domain.RoleAdmin {
		t.Errorf("role = %q, want admin", updated.Role)
	}
}

func TestUserSetStatusSelfDisableRejected(t *testing.T) {
	svc, _, _, _ := newUserSvc()
	if _, err := svc.SetStatus(context.Background(), "admin-1", domain.UserStatusDisabled, "admin-1", "ip"); !errors.Is(err, domain.ErrSelfStatusChange) {
		t.Fatalf("SetStatus(self) error = %v, want ErrSelfStatusChange", err)
	}
}

func TestUserDisableRevokesSessions(t *testing.T) {
	users := newFakeUserRepo()
	users.upsert(&domain.User{ID: "editor-1", Username: "writer", DisplayName: "作者", Role: domain.RoleEditor, Status: domain.UserStatusActive})
	sessions := newFakeSessionRepo()
	_ = sessions.Insert(context.Background(), &domain.RefreshSession{ID: "s1", UserID: "editor-1", JTI: "j1", FamilyID: "f1", ExpiresAt: time.Now().Add(time.Hour)})
	svc := service.NewUserService(users, sessions, newFakeAuditRepo())

	if _, err := svc.SetStatus(context.Background(), "editor-1", domain.UserStatusDisabled, "admin-1", "ip"); err != nil {
		t.Fatalf("SetStatus() error = %v", err)
	}
	for _, s := range sessions.sessions {
		if s.RevokedAt == nil {
			t.Error("session not revoked after disable")
		}
	}
}

func TestUserResetPasswordReturnsTempAndRevokesSessions(t *testing.T) {
	users := newFakeUserRepo()
	users.upsert(&domain.User{ID: "editor-1", Username: "writer", DisplayName: "作者", Role: domain.RoleEditor, Status: domain.UserStatusActive})
	sessions := newFakeSessionRepo()
	_ = sessions.Insert(context.Background(), &domain.RefreshSession{ID: "s1", UserID: "editor-1", JTI: "j1", FamilyID: "f1", ExpiresAt: time.Now().Add(time.Hour)})
	svc := service.NewUserService(users, sessions, newFakeAuditRepo())

	temp, _, err := svc.ResetPassword(context.Background(), "editor-1", "admin-1", "ip")
	if err != nil {
		t.Fatalf("ResetPassword() error = %v", err)
	}
	if len(temp) < 8 || len(temp) > 72 {
		t.Errorf("temporary password length %d outside 8-72", len(temp))
	}
	user, err := users.FindByID(context.Background(), "editor-1")
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	if !user.MustChangePassword {
		t.Error("after reset mustChangePassword should be true")
	}
	for _, s := range sessions.sessions {
		if s.RevokedAt == nil {
			t.Error("session not revoked after reset")
		}
	}
}

func TestUserListFilters(t *testing.T) {
	users := newFakeUserRepo()
	users.upsert(&domain.User{ID: "a", Username: "admin1", DisplayName: "管理员甲", Role: domain.RoleAdmin, Status: domain.UserStatusActive})
	users.upsert(&domain.User{ID: "e", Username: "editor1", DisplayName: "编辑乙", Role: domain.RoleEditor, Status: domain.UserStatusDisabled})
	svc := service.NewUserService(users, newFakeSessionRepo(), newFakeAuditRepo())

	role := domain.RoleEditor
	page, err := svc.List(context.Background(), &domain.UserQuery{Role: &role, Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if page.Total != 1 || page.Items[0].Username != "editor1" {
		t.Errorf("role filter got %d items: %+v", page.Total, page.Items)
	}

	kw := "甲"
	page, err = svc.List(context.Background(), &domain.UserQuery{Keyword: &kw, Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("List(keyword) error = %v", err)
	}
	if page.Total != 1 || page.Items[0].Username != "admin1" {
		t.Errorf("keyword filter got %d items", page.Total)
	}
}
