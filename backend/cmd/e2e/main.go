// Command e2e prepares the dedicated browser-test database. It never touches
// the development database: prep drops and recreates the database named by
// DATABASE_URL (the Playwright harness points it at news_admin_e2e), and
// seed installs known bcrypt passwords on the three seeded accounts with the
// forced-change flag still set, mirroring what the seed command does for the
// admin. The Playwright web server runs:
//
//	go run ./cmd/e2e prep && go run ./cmd/migrate up && go run ./cmd/e2e seed
package main

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"

	"news-admin/backend/internal/auth"
	"news-admin/backend/internal/config"
)

// seedPassword is the known first-login password the browser suite uses for
// the seeded accounts; every account keeps must_change_password=true so the
// forced password change is exercised in the UI.
const seedPassword = "E2eSeedPass1!"

var seedUsernames = []string{"admin", "editor", "reviewer"}

func main() {
	if err := run(os.Args[1:]); err != nil {
		slog.Error("e2e failed", "error", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) < 1 {
		return errors.New("usage: e2e <prep|seed>")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	switch args[0] {
	case "prep":
		return prep(cfg.DatabaseURL)
	case "seed":
		return seed(cfg.DatabaseURL)
	default:
		return fmt.Errorf("unknown command %q, want prep or seed", args[0])
	}
}

// prep ensures the e2e database exists and returns it to a pristine state:
// the public schema is dropped and recreated, so every run starts from fresh
// migrations regardless of the previous run's mutations.
func prep(dsn string) error {
	maintenanceDSN, dbName, err := maintenanceTarget(dsn)
	if err != nil {
		return err
	}
	mdb, err := sql.Open("pgx", maintenanceDSN)
	if err != nil {
		return fmt.Errorf("open maintenance database: %w", err)
	}
	if err := mdb.Ping(); err != nil {
		mdb.Close()
		return fmt.Errorf("ping maintenance database: %w", err)
	}
	var exists bool
	if err := mdb.QueryRow(
		`SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)`, dbName,
	).Scan(&exists); err != nil {
		mdb.Close()
		return fmt.Errorf("check e2e database existence: %w", err)
	}
	if !exists {
		if _, err := mdb.Exec(`CREATE DATABASE "` + dbName + `"`); err != nil {
			mdb.Close()
			return fmt.Errorf("create e2e database %q (grant CREATEDB to the database role if needed): %w", dbName, err)
		}
	}
	mdb.Close()

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("open e2e database: %w", err)
	}
	defer db.Close()
	if _, err := db.Exec(`DROP SCHEMA IF EXISTS public CASCADE`); err != nil {
		return fmt.Errorf("drop e2e schema: %w", err)
	}
	if _, err := db.Exec(`CREATE SCHEMA public`); err != nil {
		return fmt.Errorf("create e2e schema: %w", err)
	}
	slog.Info("e2e database prepared", "database", dbName)
	return nil
}

// seed writes the known first-login password hash onto the seeded accounts,
// keeping must_change_password=true so the first-login flow can run.
func seed(dsn string) error {
	hash, err := auth.HashPassword(seedPassword)
	if err != nil {
		return err
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("open e2e database: %w", err)
	}
	defer db.Close()
	if _, err := db.Exec(
		`UPDATE users
         SET password_hash = $1, must_change_password = TRUE, password_changed_at = NULL
         WHERE username = ANY($2)`,
		hash, seedUsernames,
	); err != nil {
		return fmt.Errorf("seed e2e passwords: %w", err)
	}
	slog.Info("e2e seed passwords written", "accounts", strings.Join(seedUsernames, ","))
	return nil
}

// maintenanceTarget returns a DSN pointing at the maintenance database of the
// same server, plus the target database name, so prep can drop and recreate
// the database named in the original DSN.
func maintenanceTarget(dsn string) (string, string, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return "", "", fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	name := strings.TrimPrefix(u.Path, "/")
	if name == "" {
		return "", "", errors.New("DATABASE_URL must name a database")
	}
	u.Path = "/postgres"
	return u.String(), name, nil
}
