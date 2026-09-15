package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"news-admin/backend/internal/auth"
	"news-admin/backend/internal/domain"
	"news-admin/backend/internal/service"
)

const testSecret = "unit-test-secret"

func mustHash(t *testing.T, password string) string {
	t.Helper()
	h, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	return h
}

func mustUser(t *testing.T, id, username, role string, hash string, mustChange bool) *domain.User {
	t.Helper()
	return &domain.User{
		ID:                 id,
		Username:           username,
		PasswordHash:       hash,
		DisplayName:        username,
		Role:               role,
		Status:             domain.UserStatusActive,
		MustChangePassword: mustChange,
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}
}

func newTestService(users *fakeUserRepo, sessions *fakeSessionRepo, audits *fakeAuditRepo) *service.AuthService {
	return service.NewAuthService(users, sessions, audits, testSecret)
}

func TestLoginSuccessIssuesSessionAndAudits(t *testing.T) {
	users := newFakeUserRepo()
	users.upsert(mustUser(t, "u1", "admin", domain.RoleAdmin, mustHash(t, "secret-pw"), true))
	sessions := newFakeSessionRepo()
	audits := newFakeAuditRepo()
	svc := newTestService(users, sessions, audits)

	sess, err := svc.Login(context.Background(), "admin", "secret-pw", "10.0.0.1")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if sess.AccessToken == "" || sess.RefreshJTI == "" || sess.FamilyID == "" {
		t.Fatalf("Login() session incomplete: %+v", sess)
	}
	if !sess.User.MustChangePassword {
		t.Error("seeded admin should still require password change")
	}
	actions := audits.actions()
	if len(actions) != 1 || actions[0] != domain.ActionLogin {
		t.Errorf("audit actions = %v, want [login]", actions)
	}
}

func TestLoginRejectsWrongPasswordAndAuditsFailure(t *testing.T) {
	users := newFakeUserRepo()
	users.upsert(mustUser(t, "u1", "admin", domain.RoleAdmin, mustHash(t, "secret-pw"), false))
	audits := newFakeAuditRepo()
	svc := newTestService(users, newFakeSessionRepo(), audits)

	_, err := svc.Login(context.Background(), "admin", "wrong-pw", "10.0.0.2")
	if !errors.Is(err, service.ErrInvalidCredentials) {
		t.Fatalf("Login() error = %v, want ErrInvalidCredentials", err)
	}
	actions := audits.actions()
	if len(actions) != 1 || actions[0] != domain.ActionFailedLogin {
		t.Errorf("audit actions = %v, want [failed_login]", actions)
	}
}

func TestLoginRejectsDisabledAccount(t *testing.T) {
	user := mustUser(t, "u1", "admin", domain.RoleAdmin, mustHash(t, "secret-pw"), false)
	user.Status = domain.UserStatusDisabled
	users := newFakeUserRepo()
	users.upsert(user)

	svc := newTestService(users, newFakeSessionRepo(), newFakeAuditRepo())
	_, err := svc.Login(context.Background(), "admin", "secret-pw", "10.0.0.3")
	if !errors.Is(err, service.ErrAccountDisabled) {
		t.Fatalf("Login() error = %v, want ErrAccountDisabled", err)
	}
}

func TestLoginRateLimitBlocksAfterFiveFailures(t *testing.T) {
	users := newFakeUserRepo()
	users.upsert(mustUser(t, "u1", "admin", domain.RoleAdmin, mustHash(t, "secret-pw"), false))
	svc := newTestService(users, newFakeSessionRepo(), newFakeAuditRepo())

	for range 5 {
		_, _ = svc.Login(context.Background(), "nobody", "x", "203.0.113.7")
	}
	_, err := svc.Login(context.Background(), "nobody", "x", "203.0.113.7")
	if !errors.Is(err, service.ErrRateLimited) {
		t.Fatalf("6th attempt error = %v, want ErrRateLimited", err)
	}

	// A different IP is not blocked.
	_, err = svc.Login(context.Background(), "nobody", "x", "203.0.113.8")
	if !errors.Is(err, service.ErrInvalidCredentials) {
		t.Fatalf("different IP error = %v, want ErrInvalidCredentials", err)
	}
}

func TestRefreshRotatesAndReuseDetected(t *testing.T) {
	users := newFakeUserRepo()
	users.upsert(mustUser(t, "u1", "admin", domain.RoleAdmin, mustHash(t, "secret-pw"), false))
	sessions := newFakeSessionRepo()
	svc := newTestService(users, sessions, newFakeAuditRepo())

	sess, err := svc.Login(context.Background(), "admin", "secret-pw", "10.0.0.4")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	oldJTI, family := sess.RefreshJTI, sess.FamilyID

	next, err := svc.Refresh(context.Background(), oldJTI, sess.CSRF, "10.0.0.4")
	if err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	if next.RefreshJTI == oldJTI {
		t.Error("refresh should rotate the jti")
	}
	if next.FamilyID != family {
		t.Errorf("family changed: got %q want %q", next.FamilyID, family)
	}

	// Reusing the old (now revoked) jti must revoke the family.
	_, err = svc.Refresh(context.Background(), oldJTI, next.CSRF, "10.0.0.4")
	if !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("reused refresh error = %v, want ErrInvalidToken", err)
	}
	// The freshly rotated token is now in a revoked family too.
	_, err = svc.Refresh(context.Background(), next.RefreshJTI, next.CSRF, "10.0.0.4")
	if !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("family-revoked refresh error = %v, want ErrInvalidToken", err)
	}
}

func TestLogoutRevokesFamily(t *testing.T) {
	users := newFakeUserRepo()
	users.upsert(mustUser(t, "u1", "admin", domain.RoleAdmin, mustHash(t, "secret-pw"), false))
	sessions := newFakeSessionRepo()
	audits := newFakeAuditRepo()
	svc := newTestService(users, sessions, audits)

	sess, err := svc.Login(context.Background(), "admin", "secret-pw", "10.0.0.5")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if err := svc.Logout(context.Background(), sess.RefreshJTI, "10.0.0.5", sess.User.ID); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
	_, err = svc.Refresh(context.Background(), sess.RefreshJTI, sess.CSRF, "10.0.0.5")
	if !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("refresh after logout error = %v, want ErrInvalidToken", err)
	}
	if got := audits.actions(); !contains(got, domain.ActionLogout) {
		t.Errorf("audit actions = %v, want logout", got)
	}
}

func TestChangePasswordFlow(t *testing.T) {
	users := newFakeUserRepo()
	user := mustUser(t, "u1", "admin", domain.RoleAdmin, mustHash(t, "initial-pw"), true)
	users.upsert(user)
	sessions := newFakeSessionRepo()
	audits := newFakeAuditRepo()
	svc := newTestService(users, sessions, audits)

	sess, err := svc.Login(context.Background(), "admin", "initial-pw", "10.0.0.6")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}

	// Wrong old password is rejected.
	err = svc.ChangePassword(context.Background(), user.ID, "wrong-old", "new-password-123", "10.0.0.6")
	if !errors.Is(err, service.ErrWrongPassword) {
		t.Fatalf("ChangePassword(wrong old) error = %v, want ErrWrongPassword", err)
	}

	// Successful change clears the flag and records the timestamp.
	err = svc.ChangePassword(context.Background(), user.ID, "initial-pw", "new-password-123", "10.0.0.6")
	if err != nil {
		t.Fatalf("ChangePassword() error = %v", err)
	}
	u2, _ := users.FindByID(context.Background(), user.ID)
	if u2.MustChangePassword {
		t.Error("must_change_password should be cleared after change")
	}
	if u2.PasswordChangedAt == nil {
		t.Error("password_changed_at should be set")
	}
	if !auth.VerifyPassword(u2.PasswordHash, "new-password-123") {
		t.Error("new password hash should verify")
	}

	// After change, old refresh family must be revoked (re-login required).
	_, err = svc.Refresh(context.Background(), sess.RefreshJTI, sess.CSRF, "10.0.0.6")
	if !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("refresh after change error = %v, want ErrInvalidToken", err)
	}
	if got := audits.actions(); !contains(got, domain.ActionPasswordChange) {
		t.Errorf("audit actions = %v, want user_password_change", got)
	}
}

func TestChangePasswordPolicy(t *testing.T) {
	users := newFakeUserRepo()
	user := mustUser(t, "u1", "admin", domain.RoleAdmin, mustHash(t, "initial-pw"), true)
	users.upsert(user)
	svc := newTestService(users, newFakeSessionRepo(), newFakeAuditRepo())

	err := svc.ChangePassword(context.Background(), user.ID, "initial-pw", "short", "10.0.0.7")
	if !errors.Is(err, service.ErrPasswordPolicy) {
		t.Fatalf("ChangePassword(short) error = %v, want ErrPasswordPolicy", err)
	}
}

func TestAccessTokenRoundTrip(t *testing.T) {
	token, err := auth.SignAccessToken(testSecret, "u-1", domain.RoleAdmin, time.Now())
	if err != nil {
		t.Fatalf("SignAccessToken() error = %v", err)
	}
	claims, err := auth.ParseAccessToken(testSecret, token)
	if err != nil {
		t.Fatalf("ParseAccessToken() error = %v", err)
	}
	if claims.UserID != "u-1" || claims.Role != domain.RoleAdmin {
		t.Errorf("claims = %+v, want user u-1 role admin", claims)
	}

	// Wrong secret rejected.
	if _, err := auth.ParseAccessToken("other-secret", token); err == nil {
		t.Error("ParseAccessToken(wrong secret) should fail")
	}
	// Tampered token rejected.
	parts := strings.Split(token, ".")
	tampered := parts[0] + ".eyJhbGciOiJIUzI1NiJ9." + "c2lnbmF0dXJl"
	if _, err := auth.ParseAccessToken(testSecret, tampered); err == nil {
		t.Error("ParseAccessToken(tampered) should fail")
	}
}

func contains(s []string, want string) bool {
	for _, v := range s {
		if v == want {
			return true
		}
	}
	return false
}
