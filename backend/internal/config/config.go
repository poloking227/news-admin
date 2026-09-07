// Package config loads runtime configuration from environment variables,
// with optional .env file support for local development.
package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Config holds all runtime settings for the API server.
type Config struct {
	// AppEnv is the deployment environment: development or production.
	AppEnv string
	// Port is the TCP port the HTTP server listens on.
	Port string
	// LogLevel controls the verbosity of slog output (debug/info/warn/error).
	LogLevel string
	// CORSOrigins lists the origins allowed by the CORS middleware.
	CORSOrigins []string
	// ReadTimeout / WriteTimeout bound a single HTTP request lifecycle.
	ReadTimeout time.Duration
	// ShutdownTimeout bounds the graceful shutdown wait for in-flight requests.
	ShutdownTimeout time.Duration
}

// Load reads configuration from the environment. When run locally it also
// loads a .env file next to the working directory if present; production
// deployments inject every value through real environment variables.
func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		AppEnv:          getenv("APP_ENV", "development"),
		Port:            getenv("APP_PORT", "8080"),
		LogLevel:        getenv("LOG_LEVEL", "info"),
		CORSOrigins:     splitCSV(getenv("CORS_ORIGIN", "http://localhost:5173")),
		ReadTimeout:     10 * time.Second,
		ShutdownTimeout: 10 * time.Second,
	}

	if cfg.AppEnv != "development" && cfg.AppEnv != "production" {
		return nil, fmt.Errorf("APP_ENV must be development or production, got %q", cfg.AppEnv)
	}
	return cfg, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
