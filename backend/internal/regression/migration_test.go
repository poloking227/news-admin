package regression

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"news-admin/backend/internal/api"
	"news-admin/backend/internal/repository"
)

// migrationHarness is a dedicated database for the migration sanity checks so
// the reset/re-apply cycles never interfere with the shared regression
// database used by the HTTP suites.
type migrationHarness struct {
	dsn  string
	sql  *sql.DB
	main http.Handler
}

// newMigrationHarness creates a fresh dedicated database, applies the
// migrations once, and wires a router against it for the seed assertions.
func newMigrationHarness(t *testing.T) *migrationHarness {
	t.Helper()
	baseDSN, err := baseDatabaseURL()
	if err != nil {
		t.Fatalf("load base DSN: %v", err)
	}
	dsn, err := derivedDSN(baseDSN, "_regression_mig")
	if err != nil {
		t.Fatalf("derive migration DSN: %v", err)
	}
	if err := ensureDatabase(baseDSN, dsn); err != nil {
		t.Fatalf("ensure migration database: %v", err)
	}
	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open migration database: %v", err)
	}
	if err := sqlDB.Ping(); err != nil {
		t.Fatalf("ping migration database: %v", err)
	}
	if err := resetSchema(sqlDB); err != nil {
		t.Fatalf("reset migration schema: %v", err)
	}
	if err := migrateUp(sqlDB); err != nil {
		t.Fatalf("migrate up once: %v", err)
	}

	gormDB := openGorm(t, dsn)
	gin.SetMode(gin.TestMode)
	cfg := testConfig()
	router := api.NewRouter(cfg, discardLogger(), api.Deps{
		Secret:     cfg.JWTSecret,
		Users:      repository.NewUserRepository(gormDB),
		Sessions:   repository.NewSessionRepository(gormDB),
		Audit:      repository.NewAuditRepository(gormDB),
		Categories: repository.NewCategoryRepository(gormDB),
		Articles:   repository.NewArticleRepository(gormDB),
	})
	return &migrationHarness{dsn: dsn, sql: sqlDB, main: router}
}

// tableExists checks whether a table survives a migration cycle.
func (m *migrationHarness) tableExists(t *testing.T, name string) bool {
	t.Helper()
	var exists bool
	err := m.sql.QueryRow(
		`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema='public' AND table_name=$1)`,
		name,
	).Scan(&exists)
	if err != nil {
		t.Fatalf("check table %s: %v", name, err)
	}
	return exists
}

// userRow is the minimal projection needed by the seed assertions.
type userRow struct {
	Username           string
	Role               string
	MustChangePassword bool
}

func (m *migrationHarness) seedUsers(t *testing.T) []userRow {
	t.Helper()
	rows, err := m.sql.Query(
		`SELECT username, role, must_change_password FROM users WHERE deleted_at IS NULL ORDER BY username`,
	)
	if err != nil {
		t.Fatalf("query seed users: %v", err)
	}
	defer rows.Close()
	var out []userRow
	for rows.Next() {
		var u userRow
		if err := rows.Scan(&u.Username, &u.Role, &u.MustChangePassword); err != nil {
			t.Fatalf("scan seed user: %v", err)
		}
		out = append(out, u)
	}
	return out
}

func TestMigrationUpIsIdempotent(t *testing.T) {
	requirePG(t)

	baseDSN, err := baseDatabaseURL()
	if err != nil {
		t.Fatalf("load base DSN: %v", err)
	}
	dsn, err := derivedDSN(baseDSN, "_regression_mig_idem")
	if err != nil {
		t.Fatalf("derive DSN: %v", err)
	}
	if err := ensureDatabase(baseDSN, dsn); err != nil {
		t.Fatalf("ensure database: %v", err)
	}
	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := resetSchema(sqlDB); err != nil {
		t.Fatalf("reset schema: %v", err)
	}
	if err := migrateUp(sqlDB); err != nil {
		t.Fatalf("first up: %v", err)
	}
	// A second apply must succeed and leave the schema unchanged.
	if err := migrateUp(sqlDB); err != nil {
		t.Fatalf("second up (idempotence): %v", err)
	}
	for _, table := range []string{"users", "categories", "articles", "audit_logs", "refresh_sessions"} {
		if !tableExistsVia(sqlDB, table) {
			t.Errorf("table %s missing after second up", table)
		}
	}
}

func tableExistsVia(db *sql.DB, name string) bool {
	var exists bool
	_ = db.QueryRow(
		`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema='public' AND table_name=$1)`,
		name,
	).Scan(&exists)
	return exists
}

func TestMigrationDownThenUpIsConsistent(t *testing.T) {
	requirePG(t)

	baseDSN, err := baseDatabaseURL()
	if err != nil {
		t.Fatalf("load base DSN: %v", err)
	}
	dsn, err := derivedDSN(baseDSN, "_regression_mig_cycle")
	if err != nil {
		t.Fatalf("derive DSN: %v", err)
	}
	if err := ensureDatabase(baseDSN, dsn); err != nil {
		t.Fatalf("ensure database: %v", err)
	}
	// The schema is dropped and recreated mid-test, which invalidates the
	// prepared statements pgx caches per connection. Each phase therefore
	// uses a freshly opened connection so cached plans never go stale.
	migDB, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open migration database: %v", err)
	}
	if err := resetSchema(migDB); err != nil {
		t.Fatalf("reset schema: %v", err)
	}
	if err := migrateUp(migDB); err != nil {
		t.Fatalf("first up: %v", err)
	}
	migDB.Close()
	before := seedUsersVia(dsn)

	if err := migrateDownAllDB(dsn); err != nil {
		t.Fatalf("down all: %v", err)
	}
	if err := migrateUpDB(dsn); err != nil {
		t.Fatalf("up after down: %v", err)
	}
	after := seedUsersVia(dsn)
	if len(before) != len(after) {
		t.Fatalf("seed users differ after cycle: before=%d after=%d", len(before), len(after))
	}
	for i := range before {
		if before[i] != after[i] {
			t.Errorf("seed user %d differs: before=%+v after=%+v", i, before[i], after[i])
		}
	}
	for _, table := range []string{"users", "categories", "articles", "audit_logs", "refresh_sessions"} {
		if !tableExistsViaDB(dsn, table) {
			t.Errorf("table %s missing after down+up cycle", table)
		}
	}
}

// migrateDownAllDB opens its own connection and rolls every migration back.
func migrateDownAllDB(dsn string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	return migrateDownAll(db)
}

// migrateUpDB opens its own connection and applies every migration.
func migrateUpDB(dsn string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	return migrateUp(db)
}

// seedUsersVia queries the seeded accounts on a fresh connection.
func seedUsersVia(dsn string) []userRow {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		panic(err)
	}
	defer db.Close()
	rows, err := db.Query(
		`SELECT username, role, must_change_password FROM users WHERE deleted_at IS NULL ORDER BY username`,
	)
	if err != nil {
		panic(err)
	}
	defer rows.Close()
	var out []userRow
	for rows.Next() {
		var u userRow
		if err := rows.Scan(&u.Username, &u.Role, &u.MustChangePassword); err != nil {
			panic(err)
		}
		out = append(out, u)
	}
	return out
}

// tableExistsViaDB checks table existence on a fresh connection.
func tableExistsViaDB(dsn, name string) bool {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return false
	}
	defer db.Close()
	return tableExistsVia(db, name)
}

func TestSeedAdminForcesPasswordChange(t *testing.T) {
	requirePG(t)

	m := newMigrationHarness(t)
	users := m.seedUsers(t)
	var admin *userRow
	for i := range users {
		if users[i].Username == "admin" {
			admin = &users[i]
			break
		}
	}
	if admin == nil {
		t.Fatalf("seed admin account missing, got %v", users)
	}
	if admin.Role != "admin" {
		t.Errorf("seed admin role = %q, want admin", admin.Role)
	}
	if !admin.MustChangePassword {
		t.Error("seed admin must_change_password = false, want true (M0 first-login change)")
	}
}

// TestSeedEditorAndReviewerForcedChange guards the remaining seed accounts.
func TestSeedEditorAndReviewerForcedChange(t *testing.T) {
	requirePG(t)

	m := newMigrationHarness(t)
	users := m.seedUsers(t)
	got := map[string]userRow{}
	for _, u := range users {
		got[u.Username] = u
	}
	for _, name := range []string{"editor", "reviewer"} {
		u, ok := got[name]
		if !ok {
			t.Errorf("seed account %q missing", name)
			continue
		}
		if !u.MustChangePassword {
			t.Errorf("seed %s must_change_password = false, want true", name)
		}
		if u.Role != name {
			t.Errorf("seed %s role = %q", name, u.Role)
		}
	}
}

// TestSeedLoginReportsMustChange asserts the login response carries the M0 flag.
func TestSeedLoginReportsMustChange(t *testing.T) {
	requirePG(t)

	m := newMigrationHarness(t)
	// The seed password is still the placeholder, so login must fail; the
	// account is only usable after the seed command injects a real hash.
	r := doJSONWith(http.MethodPost, "/api/v1/auth/login", m.main, "",
		map[string]string{"username": "admin", "password": "anything"})
	if r.status != http.StatusUnauthorized {
		t.Fatalf("seed admin login status = %d, want 401", r.status)
	}
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(r.body, &env)
	if !strings.Contains(env.Error.Code, "UNAUTHENTICATED") {
		t.Errorf("error.code = %q, want UNAUTHENTICATED", env.Error.Code)
	}
}
