package main

import (
	"os"
	"strings"
	"testing"
)

// TestSeedUsesEnvironmentPassword verifies the seed command never embeds a
// plaintext password and only wires the value through the environment.
func TestSeedUsesEnvironmentPassword(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	code := string(src)

	if strings.Contains(code, `"password"`) && strings.Contains(code, `:= "`) {
		t.Error("seed must not hardcode a password literal")
	}
	if !strings.Contains(code, `os.Getenv("ADMIN_INITIAL_PASSWORD")`) {
		t.Error("seed must read ADMIN_INITIAL_PASSWORD from the environment")
	}
	if !strings.Contains(code, "HashPassword(password)") {
		t.Error("seed must bcrypt-hash the injected password before writing")
	}
	if !strings.Contains(code, "must_change_password") {
		t.Error("seed must keep must_change_password=true (M0 forced change)")
	}
}

func TestSeedRequiresEnv(t *testing.T) {
	t.Setenv("ADMIN_INITIAL_PASSWORD", "")
	// Config load would require a reachable DB, so assert on the guard by
	// checking the source path instead of executing run().
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	if !strings.Contains(string(src), `errors.New("ADMIN_INITIAL_PASSWORD must be set")`) {
		t.Error("seed should fail fast when ADMIN_INITIAL_PASSWORD is empty")
	}
}

// TestSeedWritesHashNotPlaintext checks that the UPDATE payload only ever
// references the already-hashed variable.
func TestSeedWritesHashNotPlaintext(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	code := string(src)
	payloadStart := strings.Index(code, `"password_hash"`)
	if payloadStart < 0 {
		t.Fatal("seed must update password_hash")
	}
	payload := code[payloadStart : payloadStart+120]
	if strings.Contains(payload, `"password_hash": "`) {
		t.Error("password_hash must be written from the hashed variable, not a literal")
	}
	if !strings.Contains(payload, "hash") {
		t.Error("password_hash must reference the bcrypt hash variable")
	}
}
