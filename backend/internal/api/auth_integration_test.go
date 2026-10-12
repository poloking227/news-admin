// Package api contains HTTP-level integration tests for the auth endpoints
// using in-memory fake repositories.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"news-admin/backend/internal/auth"
	"news-admin/backend/internal/config"
	"news-admin/backend/internal/domain"
)

// --- in-memory fakes for handler-level tests ---

type tFakeUsers struct {
	mu    sync.Mutex
	users map[string]*domain.User
}

func newTFakeUsers() *tFakeUsers { return &tFakeUsers{users: map[string]*domain.User{}} }

func (f *tFakeUsers) FindByUsername(_ context.Context, username string) (*domain.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if u, ok := f.users[username]; ok {
		return u, nil
	}
	return nil, domain.ErrNotFound
}
func (f *tFakeUsers) FindByID(_ context.Context, id string) (*domain.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, u := range f.users {
		if u.ID == id {
			return u, nil
		}
	}
	return nil, domain.ErrNotFound
}
func (f *tFakeUsers) UpdatePassword(_ context.Context, id, hash string, at time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, u := range f.users {
		if u.ID == id {
			u.PasswordHash = hash
			u.MustChangePassword = false
			u.PasswordChangedAt = &at
			return nil
		}
	}
	return domain.ErrNotFound
}

func (f *tFakeUsers) Create(_ context.Context, in *domain.UserInput, now time.Time) (*domain.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.users[in.Username]; ok {
		return nil, domain.ErrUsernameTaken
	}
	u := &domain.User{
		ID:                 "tuser-" + in.Username,
		Username:           in.Username,
		PasswordHash:       in.PasswordHash,
		DisplayName:        in.DisplayName,
		Role:               in.Role,
		Status:             domain.UserStatusActive,
		MustChangePassword: true,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	f.users[u.Username] = u
	cp := *u
	return &cp, nil
}

func (f *tFakeUsers) Update(_ context.Context, id string, in *domain.UserUpdateInput, now time.Time) (*domain.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, u := range f.users {
		if u.ID == id {
			if in.DisplayName != nil {
				u.DisplayName = *in.DisplayName
			}
			if in.Role != nil {
				u.Role = *in.Role
			}
			u.UpdatedAt = now
			cp := *u
			return &cp, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (f *tFakeUsers) SetStatus(_ context.Context, id, status string, now time.Time) (*domain.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, u := range f.users {
		if u.ID == id {
			u.Status = status
			u.UpdatedAt = now
			cp := *u
			return &cp, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (f *tFakeUsers) SetPasswordHash(_ context.Context, id, passwordHash string, now time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, u := range f.users {
		if u.ID == id {
			u.PasswordHash = passwordHash
			u.MustChangePassword = true
			u.PasswordChangedAt = nil
			u.UpdatedAt = now
			return nil
		}
	}
	return domain.ErrNotFound
}

func (f *tFakeUsers) List(_ context.Context, q *domain.UserQuery) (*domain.UserPage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var items []*domain.User
	for _, u := range f.users {
		if q.Role != nil && *q.Role != "" && u.Role != *q.Role {
			continue
		}
		if q.Status != nil && *q.Status != "" && u.Status != *q.Status {
			continue
		}
		if q.Keyword != nil && *q.Keyword != "" &&
			!strings.Contains(strings.ToLower(u.Username), strings.ToLower(*q.Keyword)) &&
			!strings.Contains(strings.ToLower(u.DisplayName), strings.ToLower(*q.Keyword)) {
			continue
		}
		cp := *u
		items = append(items, &cp)
	}
	return &domain.UserPage{Items: items, Total: int64(len(items)), Page: q.Page, PageSize: q.PageSize}, nil
}

type tFakeSessions struct {
	mu       sync.Mutex
	sessions []*domain.RefreshSession
}

func newTFakeSessions() *tFakeSessions { return &tFakeSessions{} }

func (f *tFakeSessions) Insert(_ context.Context, s *domain.RefreshSession) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sessions = append(f.sessions, s)
	return nil
}
func (f *tFakeSessions) FindByJTI(_ context.Context, jti string) (*domain.RefreshSession, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, s := range f.sessions {
		if s.JTI == jti {
			return s, nil
		}
	}
	return nil, domain.ErrNotFound
}
func (f *tFakeSessions) Revoke(_ context.Context, jti string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := time.Now()
	for _, s := range f.sessions {
		if s.JTI == jti && s.RevokedAt == nil {
			s.RevokedAt = &now
		}
	}
	return nil
}
func (f *tFakeSessions) RevokeFamily(_ context.Context, family string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := time.Now()
	for _, s := range f.sessions {
		if s.FamilyID == family && s.RevokedAt == nil {
			s.RevokedAt = &now
		}
	}
	return nil
}
func (f *tFakeSessions) RevokeAllByUser(_ context.Context, userID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := time.Now()
	for _, s := range f.sessions {
		if s.UserID == userID && s.RevokedAt == nil {
			s.RevokedAt = &now
		}
	}
	return nil
}

type tFakeAudit struct {
	mu      sync.Mutex
	entries []*domain.AuditLog
}

func newTFakeAudit() *tFakeAudit { return &tFakeAudit{} }

func (f *tFakeAudit) Insert(_ context.Context, e *domain.AuditLog) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries = append(f.entries, e)
	return nil
}
func (f *tFakeAudit) actions() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, e := range f.entries {
		out = append(out, e.Action)
	}
	return out
}

// --- helpers ---

const testJWTSecret = "integration-test-secret"

func newAuthRouter(t *testing.T, users *tFakeUsers, sessions *tFakeSessions, audit *tFakeAudit) http.Handler {
	t.Helper()
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		AppEnv:      "development",
		Port:        "8080",
		LogLevel:    "info",
		CORSOrigins: []string{"http://localhost:5173"},
		JWTSecret:   testJWTSecret,
	}
	return NewRouter(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), Deps{
		Secret:   testJWTSecret,
		Users:    users,
		Sessions: sessions,
		Audit:    audit,
	})
}

func seedTestUser(users *tFakeUsers, mustChange bool) *domain.User {
	hash, _ := auth.HashPassword("initial-pass-1")
	u := &domain.User{
		ID:                 "11111111-1111-1111-1111-111111111111",
		Username:           "admin",
		PasswordHash:       hash,
		DisplayName:        "管理员",
		Role:               domain.RoleAdmin,
		Status:             domain.UserStatusActive,
		MustChangePassword: mustChange,
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}
	users.users[u.Username] = u
	return u
}

func loginRequest(t *testing.T, h http.Handler, username, password string) (*httptest.ResponseRecorder, string, string) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"username": username, "password": password})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	// capture cookies
	var refresh, csrf string
	for _, c := range rec.Result().Cookies() {
		if c.Name == "refresh_token" {
			refresh = c.Value
		}
		if c.Name == "csrf_token" {
			csrf = c.Value
		}
	}
	return rec, refresh, csrf
}

func TestLoginHandlerSetsCookiesAndReturnsCurrentUser(t *testing.T) {
	users, sessions, audit := newTFakeUsers(), newTFakeSessions(), newTFakeAudit()
	seedTestUser(users, true)
	router := newAuthRouter(t, users, sessions, audit)

	rec, refresh, csrf := loginRequest(t, router, "admin", "initial-pass-1")
	if rec.Code != http.StatusOK {
		t.Fatalf("login status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if refresh == "" || csrf == "" {
		t.Fatal("login should set refresh_token and csrf_token cookies")
	}
	var resp struct {
		AccessToken string `json:"accessToken"`
		ExpiresIn   int64  `json:"expiresIn"`
		User        struct {
			Username           string   `json:"username"`
			Role               string   `json:"role"`
			MustChangePassword bool     `json:"mustChangePassword"`
			Permissions        []string `json:"permissions"`
		} `json:"user"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	if resp.AccessToken == "" || resp.ExpiresIn != 3600 {
		t.Errorf("token fields wrong: %+v", resp)
	}
	if resp.User.Username != "admin" || resp.User.Role != "admin" || !resp.User.MustChangePassword {
		t.Errorf("user payload wrong: %+v", resp.User)
	}
	if len(resp.User.Permissions) == 0 {
		t.Error("permissions should be populated for admin")
	}
}

func TestLoginHandlerRejectsBadCredentials(t *testing.T) {
	users, sessions, audit := newTFakeUsers(), newTFakeSessions(), newTFakeAudit()
	seedTestUser(users, false)
	router := newAuthRouter(t, users, sessions, audit)

	rec, _, _ := loginRequest(t, router, "admin", "wrong-password")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("login status = %d, want 401", rec.Code)
	}
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	if env.Error.Code != "UNAUTHENTICATED" {
		t.Errorf("error.code = %q, want UNAUTHENTICATED", env.Error.Code)
	}
	if got := audit.actions(); !containsStr(got, "failed_login") {
		t.Errorf("audit actions = %v, want failed_login", got)
	}
}

func TestRefreshHandlersRotateAndEnforceCSRF(t *testing.T) {
	users, sessions, audit := newTFakeUsers(), newTFakeSessions(), newTFakeAudit()
	seedTestUser(users, false)
	router := newAuthRouter(t, users, sessions, audit)

	rec, refresh, csrf := loginRequest(t, router, "admin", "initial-pass-1")
	if rec.Code != http.StatusOK {
		t.Fatalf("login failed: %d", rec.Code)
	}

	// postRefresh sends refresh_token cookie, an optional csrf cookie, and an
	// optional X-CSRF-Token header.
	postRefresh := func(token, csrfCookie, csrfHeaderVal string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
		if token != "" {
			req.AddCookie(&http.Cookie{Name: "refresh_token", Value: token})
		}
		if csrfCookie != "" {
			req.AddCookie(&http.Cookie{Name: "csrf_token", Value: csrfCookie})
		}
		if csrfHeaderVal != "" {
			req.Header.Set("X-CSRF-Token", csrfHeaderVal)
		}
		r := httptest.NewRecorder()
		router.ServeHTTP(r, req)
		return r
	}

	// Missing CSRF -> 401 even with valid cookie.
	if r := postRefresh(refresh, csrf, ""); r.Code != http.StatusUnauthorized {
		t.Errorf("refresh without CSRF: status = %d, want 401", r.Code)
	}
	// Wrong CSRF (header != cookie) -> 401.
	if r := postRefresh(refresh, csrf, "bogus"); r.Code != http.StatusUnauthorized {
		t.Errorf("refresh with wrong CSRF: status = %d, want 401", r.Code)
	}
	// Valid refresh rotates the token.
	r := postRefresh(refresh, csrf, csrf)
	if r.Code != http.StatusOK {
		t.Fatalf("valid refresh: status = %d, body=%s", r.Code, r.Body.String())
	}
	var newRefresh string
	for _, c := range r.Result().Cookies() {
		if c.Name == "refresh_token" {
			newRefresh = c.Value
		}
	}
	if newRefresh == "" || newRefresh == refresh {
		t.Error("refresh should issue a rotated refresh cookie")
	}
	// Old token reuse after rotation -> revokes family -> 401.
	if r := postRefresh(refresh, csrf, csrf); r.Code != http.StatusUnauthorized {
		t.Errorf("reused refresh: status = %d, want 401", r.Code)
	}
}

func TestMeRequiresAccessToken(t *testing.T) {
	users, sessions, audit := newTFakeUsers(), newTFakeSessions(), newTFakeAudit()
	seedTestUser(users, false)
	router := newAuthRouter(t, users, sessions, audit)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("me without token: status = %d, want 401", rec.Code)
	}

	token, _ := auth.SignAccessToken(testJWTSecret, "11111111-1111-1111-1111-111111111111", "admin", time.Now())
	req = httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("me with token: status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var me struct {
		Username           string `json:"username"`
		MustChangePassword bool   `json:"mustChangePassword"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &me)
	if me.Username != "admin" {
		t.Errorf("me.username = %q, want admin", me.Username)
	}
}

func TestChangePasswordHandler(t *testing.T) {
	users, sessions, audit := newTFakeUsers(), newTFakeSessions(), newTFakeAudit()
	seedTestUser(users, true)
	router := newAuthRouter(t, users, sessions, audit)

	rec, _, _ := loginRequest(t, router, "admin", "initial-pass-1")
	token := ""
	var loginResp struct {
		AccessToken string `json:"accessToken"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &loginResp)
	token = loginResp.AccessToken

	change := func(oldPw, newPw string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]string{"oldPassword": oldPw, "newPassword": newPw})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/change-password", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		r := httptest.NewRecorder()
		router.ServeHTTP(r, req)
		return r
	}

	// Wrong old password -> 422.
	if r := change("wrong-old", "new-pass-123456"); r.Code != http.StatusUnprocessableEntity {
		t.Errorf("wrong old password: status = %d, want 422", r.Code)
	}
	// Too short new password -> 400.
	if r := change("initial-pass-1", "short"); r.Code != http.StatusBadRequest {
		t.Errorf("too short new password: status = %d, want 400", r.Code)
	}
	// Valid change -> 204 and clears flag.
	if r := change("initial-pass-1", "new-pass-123456"); r.Code != http.StatusNoContent {
		t.Fatalf("valid change: status = %d, body=%s", r.Code, r.Body.String())
	}
	u, _ := users.FindByUsername(context.Background(), "admin")
	if u.MustChangePassword {
		t.Error("must_change_password should be cleared after change")
	}
	if got := audit.actions(); !containsStr(got, "user_password_change") {
		t.Errorf("audit actions = %v, want user_password_change", got)
	}
}

func TestChangePasswordRevokesFamily(t *testing.T) {
	users, sessions, audit := newTFakeUsers(), newTFakeSessions(), newTFakeAudit()
	seedTestUser(users, false)
	router := newAuthRouter(t, users, sessions, audit)

	rec, refresh, csrf := loginRequest(t, router, "admin", "initial-pass-1")
	var loginResp struct {
		AccessToken string `json:"accessToken"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &loginResp)

	body, _ := json.Marshal(map[string]string{"oldPassword": "initial-pass-1", "newPassword": "new-pass-123456"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/change-password", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+loginResp.AccessToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("change-password: status = %d", rec.Code)
	}

	// Old refresh family must be revoked: refresh returns 401.
	rreq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	rreq.AddCookie(&http.Cookie{Name: "refresh_token", Value: refresh})
	rreq.AddCookie(&http.Cookie{Name: "csrf_token", Value: csrf})
	rreq.Header.Set("X-CSRF-Token", csrf)
	rrec := httptest.NewRecorder()
	router.ServeHTTP(rrec, rreq)
	if rrec.Code != http.StatusUnauthorized {
		t.Errorf("refresh after password change: status = %d, want 401", rrec.Code)
	}
}

func containsStr(s []string, want string) bool {
	for _, v := range s {
		if v == want {
			return true
		}
	}
	return false
}
