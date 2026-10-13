// Package regression runs end-to-end checks against a real PostgreSQL 16
// instance through the full HTTP stack (router, middleware, handlers,
// services, repositories, and migrations). The suite is split into four
// groups: migration sanity, the article lifecycle, the RBAC denial matrix,
// and public-content visibility. Every check drives the same wiring the
// server binary uses, so a green run means the merged backend behaves
// consistently from the database up.
package regression

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"news-admin/backend/internal/api"
	"news-admin/backend/internal/auth"
	"news-admin/backend/internal/config"
	"news-admin/backend/internal/repository"
)

const (
	// testDBSuffix is appended to the configured database name so the
	// regression run never touches the development database.
	testDBSuffix = "_regression"
	// seedPassword replaces the placeholder hashes the seed migration
	// installed, mirroring what the seed command writes.
	seedPassword = "RegressionSeed1!"
	// createUserTempPassword is the temporary password assigned to accounts
	// created through the admin user-management endpoint.
	createUserTempPassword = "UserTempPass1!"
)

var dbUnreachable = errors.New("postgres is unreachable")

// harness wires the real repository stack against a fresh regression
// database and serves the standard router.
type harness struct {
	cfg    *config.Config
	db     *gorm.DB
	sqlDB  *sql.DB
	router http.Handler
}

// h is built once by TestMain and reused by every test.
var h *harness

// seedHash is the bcrypt hash written into the seeded accounts; resetForTest
// re-applies it so every test starts from the first-login state.
var seedHash string

func TestMain(m *testing.M) {
	if err := setupHarness(); err != nil {
		fmt.Fprintf(os.Stderr, "regression suite: %v\n", err)
		if errors.Is(err, dbUnreachable) {
			fmt.Fprintln(os.Stderr, "regression tests will skip: PostgreSQL 16 unreachable")
			os.Exit(m.Run())
		}
		os.Exit(1)
	}
	os.Exit(m.Run())
}

// requirePG skips the calling test when the harness could not connect to a
// PostgreSQL instance, keeping the rest of the backend suite green without
// one while still exercising the full chain when a database is present.
func requirePG(t *testing.T) {
	t.Helper()
	if h == nil {
		t.Skip("regression suite requires a reachable PostgreSQL 16 instance (DATABASE_URL)")
	}
}

// resetForTest returns the regression database to a pristine first-login
// state so tests run independently: tables are truncated and the seeded
// accounts get their known password back with the forced-change flag set.
func resetForTest(t *testing.T) {
	t.Helper()
	if _, err := h.sqlDB.Exec(
		`TRUNCATE articles, categories, audit_logs, refresh_sessions RESTART IDENTITY CASCADE`,
	); err != nil {
		t.Fatalf("truncate regression tables: %v", err)
	}
	if _, err := h.sqlDB.Exec(
		`DELETE FROM users WHERE username NOT IN ('admin', 'editor', 'reviewer')`,
	); err != nil {
		t.Fatalf("reset test users: %v", err)
	}
	if _, err := h.sqlDB.Exec(
		`UPDATE users SET password_hash = $1, must_change_password = TRUE WHERE username IN ('admin', 'editor', 'reviewer')`,
		seedHash,
	); err != nil {
		t.Fatalf("reset seed passwords: %v", err)
	}
}

func setupHarness() error {
	baseDSN, err := baseDatabaseURL()
	if err != nil {
		return err
	}
	testDSN, err := derivedDSN(baseDSN, testDBSuffix)
	if err != nil {
		return err
	}
	if err := ensureDatabase(baseDSN, testDSN); err != nil {
		return err
	}

	sqlDB, err := sql.Open("pgx", testDSN)
	if err != nil {
		return fmt.Errorf("open test database: %w", err)
	}
	if err := sqlDB.Ping(); err != nil {
		return errors.Join(err, dbUnreachable)
	}
	if err := resetSchema(sqlDB); err != nil {
		return err
	}
	if err := migrateUp(sqlDB); err != nil {
		return fmt.Errorf("migrate up on regression database: %w", err)
	}
	if err := writeSeedPasswords(sqlDB); err != nil {
		return err
	}

	gormDB, err := gorm.Open(postgres.Open(testDSN), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("open gorm: %w", err)
	}

	gin.SetMode(gin.TestMode)
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := api.NewRouter(cfg, logger, api.Deps{
		Secret:     cfg.JWTSecret,
		Secure:     false,
		Users:      repository.NewUserRepository(gormDB),
		Sessions:   repository.NewSessionRepository(gormDB),
		Audit:      repository.NewAuditRepository(gormDB),
		Categories: repository.NewCategoryRepository(gormDB),
		Articles:   repository.NewArticleRepository(gormDB),
	})
	h = &harness{cfg: cfg, db: gormDB, sqlDB: sqlDB, router: router}
	return nil
}

// baseDatabaseURL returns the configured DSN, falling back to the local
// docker-compose defaults when no DATABASE_URL is set.
func baseDatabaseURL() (string, error) {
	cfg, err := config.Load()
	if err != nil {
		return "", err
	}
	return cfg.DatabaseURL, nil
}

// derivedDSN swaps the database name in dsn for name+suffix.
func derivedDSN(dsn, suffix string) (string, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return "", err
	}
	base := strings.TrimPrefix(u.Path, "/")
	u.Path = "/" + base + suffix
	return u.String(), nil
}

// ensureDatabase creates the test database when it does not exist yet,
// connecting through the maintenance database of the same server.
func ensureDatabase(mainDSN, testDSN string) error {
	u, err := url.Parse(mainDSN)
	if err != nil {
		return err
	}
	u.Path = "/postgres"
	mdb, err := sql.Open("pgx", u.String())
	if err != nil {
		return fmt.Errorf("open maintenance database: %w", err)
	}
	defer mdb.Close()
	if err := mdb.Ping(); err != nil {
		return errors.Join(fmt.Errorf("ping maintenance database: %w", err), dbUnreachable)
	}
	tu, err := url.Parse(testDSN)
	if err != nil {
		return err
	}
	name := strings.TrimPrefix(tu.Path, "/")
	var exists bool
	if err := mdb.QueryRow(
		"SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)", name,
	).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}
	if _, err := mdb.Exec(`CREATE DATABASE "` + name + `"`); err != nil {
		return fmt.Errorf("create test database: %w", err)
	}
	return nil
}

// resetSchema drops and recreates the public schema so every run starts from
// a pristine state regardless of previous runs.
func resetSchema(db *sql.DB) error {
	if _, err := db.Exec(`DROP SCHEMA IF EXISTS public CASCADE`); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE SCHEMA public`); err != nil {
		return err
	}
	return nil
}

// migrateDir locates the goose migrations directory relative to this test
// file, mirroring the embed used by the migrate command.
func migrateDir() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return ""
	}
	// harness_test.go -> regression -> internal -> backend/cmd/migrate
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "cmd", "migrate"))
}

func migrateUp(db *sql.DB) error {
	goose.SetBaseFS(os.DirFS(migrateDir()))
	goose.SetLogger(goose.NopLogger())
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	return goose.Up(db, "migrations")
}

func migrateDownAll(db *sql.DB) error {
	goose.SetBaseFS(os.DirFS(migrateDir()))
	goose.SetLogger(goose.NopLogger())
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	return goose.DownTo(db, "migrations", 0)
}

// writeSeedPasswords gives the seeded admin/editor/reviewer accounts a known
// bcrypt hash while keeping must_change_password=true, exactly like the seed
// command does, so the first-login flow can be exercised end to end.
func writeSeedPasswords(db *sql.DB) error {
	hash, err := auth.HashPassword(seedPassword)
	if err != nil {
		return err
	}
	seedHash = hash
	_, err = db.Exec(
		`UPDATE users SET password_hash = $1 WHERE username IN ('admin', 'editor', 'reviewer')`,
		hash,
	)
	return err
}

// insertOperator provisions an operator account directly in the database.
// The role is reserved for a later phase and cannot be assigned through the
// API, but the RBAC matrix must prove an operator is denied everywhere.
func insertOperator(db *sql.DB) (string, error) {
	hash, err := auth.HashPassword("OperatorSeed1!")
	if err != nil {
		return "", err
	}
	_, err = db.Exec(
		`INSERT INTO users (username, password_hash, display_name, role, status, must_change_password)
                 VALUES ('operator', $1, '操作员', 'operator', 'active', FALSE)
                 ON CONFLICT DO NOTHING`,
		hash,
	)
	if err != nil {
		return "", err
	}
	var id string
	err = db.QueryRow(`SELECT id::text FROM users WHERE username = 'operator'`).Scan(&id)
	return id, err
}

// --- HTTP helpers ---

type resp struct {
	status int
	header http.Header
	body   []byte
}

// doJSON performs one request with an optional bearer token and JSON body
// against the shared regression router.
func doJSON(method, path, token string, body any) resp {
	return doJSONWith(method, path, h.router, token, body)
}

// doJSONWith performs one request against an arbitrary handler.
func doJSONWith(method, path string, target http.Handler, token string, body any) resp {
	return doJSONHeaders(method, path, target, token, nil, body)
}

// doJSONHeaders adds extra request headers (e.g. If-Match for the optimistic
// lock) on top of the bearer token.
func doJSONHeaders(method, path string, target http.Handler, token string, headers map[string]string, body any) resp {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, rd)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	target.ServeHTTP(rec, req)
	return resp{status: rec.Code, header: rec.Header(), body: rec.Body.Bytes()}
}

func testConfig() *config.Config {
	return &config.Config{
		AppEnv:      "development",
		Port:        "8080",
		LogLevel:    "info",
		CORSOrigins: []string{"http://localhost:5173"},
		JWTSecret:   "regression-test-secret",
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func openGorm(t *testing.T, dsn string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open gorm: %v", err)
	}
	return db
}

func decode[T any](t *testing.T, r resp) T {
	t.Helper()
	var out T
	if len(r.body) > 0 {
		if err := json.Unmarshal(r.body, &out); err != nil {
			t.Fatalf("decode response body %q: %v", r.body, err)
		}
	}
	return out
}

type loginPayload struct {
	AccessToken string `json:"accessToken"`
	User        struct {
		ID                 string   `json:"id"`
		Username           string   `json:"username"`
		Role               string   `json:"role"`
		MustChangePassword bool     `json:"mustChangePassword"`
		Permissions        []string `json:"permissions"`
	} `json:"user"`
}

// login returns the bearer token after a successful login, failing the test
// when the credentials are rejected.
func login(t *testing.T, username, password string) string {
	t.Helper()
	r := doJSON(http.MethodPost, "/api/v1/auth/login", "", map[string]string{
		"username": username, "password": password,
	})
	if r.status != http.StatusOK {
		t.Fatalf("login %s: status = %d, body = %s", username, r.status, r.body)
	}
	return decode[loginPayload](t, r).AccessToken
}

// changePassword performs the first-login forced password change (M0).
func changePassword(t *testing.T, token, oldPass, newPass string) {
	t.Helper()
	r := doJSON(http.MethodPost, "/api/v1/auth/change-password", token, map[string]string{
		"oldPassword": oldPass, "newPassword": newPass,
	})
	if r.status != http.StatusNoContent {
		t.Fatalf("change-password: status = %d, body = %s", r.status, r.body)
	}
}

// activatedToken logs in, completes the forced password change, and logs in
// again, returning a token for a fully usable account.
func activatedToken(t *testing.T, username string) string {
	t.Helper()
	first := login(t, username, seedPassword)
	changePassword(t, first, seedPassword, "ActivatedPass1!")
	newPass := "ActivatedPass1!"
	r := doJSON(http.MethodPost, "/api/v1/auth/login", "", map[string]string{
		"username": username, "password": newPass,
	})
	if r.status != http.StatusOK {
		t.Fatalf("relogin %s: status = %d, body = %s", username, r.status, r.body)
	}
	pay := decode[loginPayload](t, r)
	if pay.User.MustChangePassword {
		t.Fatalf("relogin %s: mustChangePassword still true", username)
	}
	return pay.AccessToken
}

// activateUser activates an admin-created account whose temporary password
// was set at creation time, mirroring activatedToken for seed accounts.
func activateUser(t *testing.T, username string) string {
	t.Helper()
	first := login(t, username, createUserTempPassword)
	changePassword(t, first, createUserTempPassword, "ActivatedPass1!")
	return loginNewPassword(t, username)
}

// loginNewPassword logs in with the post-change password shared by the
// activation helpers.
func loginNewPassword(t *testing.T, username string) string {
	t.Helper()
	r := doJSON(http.MethodPost, "/api/v1/auth/login", "", map[string]string{
		"username": username, "password": "ActivatedPass1!",
	})
	if r.status != http.StatusOK {
		t.Fatalf("relogin %s: status = %d, body = %s", username, r.status, r.body)
	}
	pay := decode[loginPayload](t, r)
	if pay.User.MustChangePassword {
		t.Fatalf("relogin %s: mustChangePassword still true", username)
	}
	return pay.AccessToken
}

// --- data helpers ---

type createCategoryPayload struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func createCategory(t *testing.T, token, name, slug string) string {
	t.Helper()
	r := doJSON(http.MethodPost, "/api/v1/admin/categories", token, map[string]any{
		"name": name, "slug": slug,
	})
	if r.status != http.StatusCreated {
		t.Fatalf("create category %s: status = %d, body = %s", name, r.status, r.body)
	}
	return decode[createCategoryPayload](t, r).ID
}

type articlePayload struct {
	ID           string  `json:"id"`
	Version      int     `json:"version"`
	Status       string  `json:"status"`
	RejectReason *string `json:"rejectReason"`
	PublishedAt  *string `json:"publishedAt"`
	Title        string  `json:"title"`
}

func createDraft(t *testing.T, token, categoryID, title string) string {
	t.Helper()
	r := doJSON(http.MethodPost, "/api/v1/admin/articles", token, map[string]any{
		"title": title, "summary": "回归用摘要", "bodyHtml": "<p>body</p>", "categoryId": categoryID,
	})
	if r.status != http.StatusCreated {
		t.Fatalf("create draft %s: status = %d, body = %s", title, r.status, r.body)
	}
	art := decode[articlePayload](t, r)
	if art.Version != 1 || art.Status != "draft" {
		t.Fatalf("create draft %s: version=%d status=%s, want 1/draft", title, art.Version, art.Status)
	}
	return art.ID
}

func createUser(t *testing.T, token, username, role string) {
	t.Helper()
	r := doJSON(http.MethodPost, "/api/v1/admin/users", token, map[string]any{
		"username": username, "password": createUserTempPassword, "displayName": username, "role": role,
	})
	if r.status != http.StatusCreated {
		t.Fatalf("create user %s: status = %d, body = %s", username, r.status, r.body)
	}
}
