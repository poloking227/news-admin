// Command seed sets the initial admin password from the ADMIN_INITIAL_PASSWORD
// environment variable, overwriting the placeholder hash created by the seed
// migration. The account keeps must_change_password=true so the admin is
// forced to change it on first login (M0).
package main

import (
	"errors"
	"log/slog"
	"os"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"news-admin/backend/internal/auth"
	"news-admin/backend/internal/config"
)

const adminUsername = "admin"

func main() {
	if err := run(); err != nil {
		slog.Error("seed failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	password := os.Getenv("ADMIN_INITIAL_PASSWORD")
	if password == "" {
		return errors.New("ADMIN_INITIAL_PASSWORD must be set")
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}

	db, err := gorm.Open(postgres.Open(cfg.DatabaseURL), &gorm.Config{})
	if err != nil {
		return err
	}
	res := db.Table("users").
		Where("username = ?", adminUsername).
		Updates(map[string]any{
			"password_hash":        hash,
			"must_change_password": true, // M0: first login must change the initial password
			"updated_at":           time.Now(),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.New("admin account not found (run migrations first)")
	}
	slog.Info("admin password seeded", "username", adminUsername, "must_change_password", true)
	return nil
}
