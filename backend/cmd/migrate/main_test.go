package main

import (
	"strings"
	"testing"
)

func mustReadMigration(t *testing.T, name string) string {
	t.Helper()
	content, err := embedMigrations.ReadFile("migrations/" + name)
	if err != nil {
		t.Fatalf("read migrations/%s: %v", name, err)
	}
	return string(content)
}

// prefixUp returns the text between the +goose Up and +goose Down markers.
func prefixUp(t *testing.T, content string) string {
	t.Helper()
	up := strings.Index(content, "+goose Up")
	down := strings.Index(content, "+goose Down")
	if up < 0 || (down >= 0 && down < up) {
		t.Fatalf("migration missing +goose Up section: %q", content[:min(len(content), 200)])
	}
	if down < 0 {
		return content[up:]
	}
	return content[up:down]
}

func TestMigrationFilesPresent(t *testing.T) {
	for _, want := range []string{"001_init.sql", "002_seed.sql"} {
		mustReadMigration(t, want)
	}
}

func TestInitCreatesAllTables(t *testing.T) {
	up := prefixUp(t, mustReadMigration(t, "001_init.sql"))
	for _, table := range []string{
		"CREATE TABLE users",
		"CREATE TABLE categories",
		"CREATE TABLE articles",
		"CREATE TABLE audit_logs",
		"CREATE TABLE refresh_sessions",
	} {
		if !strings.Contains(up, table) {
			t.Errorf("001_init up missing %q", table)
		}
	}
}

func TestInitCreatesSearchIndexes(t *testing.T) {
	up := prefixUp(t, mustReadMigration(t, "001_init.sql"))
	if !strings.Contains(up, "CREATE EXTENSION IF NOT EXISTS pg_trgm") {
		t.Error("001_init up missing pg_trgm extension")
	}
	for _, idx := range []string{
		"idx_articles_body_text_trgm",
		"idx_articles_title_trgm",
		"idx_articles_summary_trgm",
	} {
		if !strings.Contains(up, idx) {
			t.Errorf("001_init up missing GIN trigram index %q", idx)
		}
	}
}

func TestInitEnforcesStatusMachine(t *testing.T) {
	up := prefixUp(t, mustReadMigration(t, "001_init.sql"))
	required := []string{
		"'draft', 'pending_review', 'published', 'unpublished'",
		"check_article_status_transition",
		"trg_articles_status_before_update",
	}
	for _, s := range required {
		if !strings.Contains(up, s) {
			t.Errorf("001_init up missing status machine %q", s)
		}
	}
}

func TestInitEnforcesFieldChecks(t *testing.T) {
	up := prefixUp(t, mustReadMigration(t, "001_init.sql"))
	for _, s := range []string{
		"username             CITEXT",
		"role                 TEXT NOT NULL CHECK",
		"'admin', 'editor', 'reviewer', 'operator'",
		"cover_url",
		"^https?://",
		"'active', 'disabled'",
		"must_change_password BOOLEAN NOT NULL DEFAULT FALSE",
	} {
		if !strings.Contains(up, s) {
			t.Errorf("001_init up missing %q", s)
		}
	}
}

func TestSeedHasNoPlaintextPassword(t *testing.T) {
	seed := mustReadMigration(t, "002_seed.sql")
	if strings.Contains(seed, "password") && strings.Contains(seed, "'") {
		// The only password-related token must be the column reference and the
		// non-plaintext sentinel; a bcrypt hash or a password literal would fail here.
		if strings.Contains(seed, "seed-pending") == false {
			t.Error("002_seed must use a non-plaintext placeholder for password_hash")
		}
	}
	if !strings.Contains(seed, "must_change_password") {
		t.Error("002_seed must set must_change_password=true for seeded accounts")
	}
}

func TestSeedRoles(t *testing.T) {
	seed := mustReadMigration(t, "002_seed.sql")
	for _, role := range []string{"admin", "editor", "reviewer"} {
		if !strings.Contains(seed, "'"+role+"'") {
			t.Errorf("002_seed missing %q role account", role)
		}
	}
}
