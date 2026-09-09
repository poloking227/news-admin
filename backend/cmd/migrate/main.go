// Command migrate runs the database migrations (goose, PostgreSQL) embedded
// in this package. It is used by the project Makefile targets:
//
//	go run ./cmd/migrate up     # apply pending migrations
//	go run ./cmd/migrate down   # roll back the latest migration
package main

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"news-admin/backend/internal/config"
)

//go:embed migrations/*.sql
var embedMigrations embed.FS

func main() {
	if err := run(os.Args[1:]); err != nil {
		slog.Error("migrate failed", "error", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) < 1 {
		return errors.New("usage: migrate <up|down>")
	}
	command := args[0]
	if command != "up" && command != "down" {
		return fmt.Errorf("unknown command %q, want up or down", command)
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	db, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	goose.SetBaseFS(embedMigrations)
	goose.SetLogger(goose.NopLogger())
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	if err := db.Ping(); err != nil {
		return fmt.Errorf("ping database (DATABASE_URL=%s): %w", cfg.DatabaseURL, err)
	}

	switch command {
	case "up":
		if err := goose.Up(db, "migrations"); err != nil {
			return fmt.Errorf("migrate up: %w", err)
		}
	case "down":
		if err := goose.Down(db, "migrations"); err != nil {
			return fmt.Errorf("migrate down: %w", err)
		}
	}
	slog.Info("migration applied", "command", command)
	return nil
}
