package config

import "testing"

func TestLoadDefaults(t *testing.T) {
	t.Setenv("APP_ENV", "")
	t.Setenv("APP_PORT", "")
	t.Setenv("LOG_LEVEL", "")
	t.Setenv("CORS_ORIGIN", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.AppEnv != "development" {
		t.Errorf("AppEnv = %q, want development", cfg.AppEnv)
	}
	if cfg.Port != "8080" {
		t.Errorf("Port = %q, want 8080", cfg.Port)
	}
	if len(cfg.CORSOrigins) != 1 || cfg.CORSOrigins[0] != "http://localhost:5173" {
		t.Errorf("CORSOrigins = %v, want [http://localhost:5173]", cfg.CORSOrigins)
	}
}

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("APP_PORT", "9090")
	t.Setenv("LOG_LEVEL", "warn")
	t.Setenv("CORS_ORIGIN", "https://admin.example.com, https://www.example.com")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.AppEnv != "production" || cfg.Port != "9090" || cfg.LogLevel != "warn" {
		t.Errorf("env values not applied: %+v", cfg)
	}
	if len(cfg.CORSOrigins) != 2 {
		t.Fatalf("CORSOrigins = %v, want 2 origins", cfg.CORSOrigins)
	}
	if cfg.CORSOrigins[1] != "https://www.example.com" {
		t.Errorf("CORSOrigins[1] = %q, want trimmed second origin", cfg.CORSOrigins[1])
	}
}

func TestLoadRejectsInvalidEnv(t *testing.T) {
	t.Setenv("APP_ENV", "staging")
	if _, err := Load(); err == nil {
		t.Fatal("Load() with APP_ENV=staging expected an error, got nil")
	}
}

func TestSplitCSV(t *testing.T) {
	got := splitCSV(" a ,,b ,")
	want := []string{"a", "b"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("splitCSV() = %v, want %v", got, want)
	}
}
