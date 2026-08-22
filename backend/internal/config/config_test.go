package config

import (
	"os"
	"testing"
)

func TestLoad_Defaults(t *testing.T) {
	// Clean env vars for test
	os.Unsetenv("PORT")
	os.Unsetenv("TURSO_DATABASE_URL")
	os.Unsetenv("TURSO_AUTH_TOKEN")
	os.Unsetenv("SESSION_SECRET")
	os.Unsetenv("COOKIE_DOMAIN")
	os.Unsetenv("RATE_LIMIT_RPH")
	os.Unsetenv("ENV")

	cfg := Load()

	if cfg.Port != "8080" {
		t.Errorf("expected default Port 8080, got %s", cfg.Port)
	}
	if cfg.SessionSecret != "dev-secret-change-in-production" {
		t.Errorf("expected default SessionSecret, got %s", cfg.SessionSecret)
	}
	if cfg.RateLimitRPH != 60 {
		t.Errorf("expected default RateLimitRPH 60, got %d", cfg.RateLimitRPH)
	}
	if cfg.Env != "development" {
		t.Errorf("expected default Env development, got %s", cfg.Env)
	}
	if cfg.IsProduction() {
		t.Errorf("expected IsProduction() false for development")
	}
}

func TestLoad_CustomEnv(t *testing.T) {
	t.Setenv("PORT", "3000")
	t.Setenv("TURSO_DATABASE_URL", "libsql://my-db.turso.io")
	t.Setenv("TURSO_AUTH_TOKEN", "secret-token")
	t.Setenv("SESSION_SECRET", "custom-secret-key")
	t.Setenv("COOKIE_DOMAIN", "escuela.edu.ar")
	t.Setenv("RATE_LIMIT_RPH", "120")
	t.Setenv("ENV", "production")

	cfg := Load()

	if cfg.Port != "3000" {
		t.Errorf("expected Port 3000, got %s", cfg.Port)
	}
	if cfg.TursoDatabaseURL != "libsql://my-db.turso.io" {
		t.Errorf("expected TursoDatabaseURL, got %s", cfg.TursoDatabaseURL)
	}
	if cfg.TursoAuthToken != "secret-token" {
		t.Errorf("expected TursoAuthToken, got %s", cfg.TursoAuthToken)
	}
	if cfg.SessionSecret != "custom-secret-key" {
		t.Errorf("expected SessionSecret custom-secret-key, got %s", cfg.SessionSecret)
	}
	if cfg.CookieDomain != "escuela.edu.ar" {
		t.Errorf("expected CookieDomain escuela.edu.ar, got %s", cfg.CookieDomain)
	}
	if cfg.RateLimitRPH != 120 {
		t.Errorf("expected RateLimitRPH 120, got %d", cfg.RateLimitRPH)
	}
	if cfg.Env != "production" {
		t.Errorf("expected Env production, got %s", cfg.Env)
	}
	if !cfg.IsProduction() {
		t.Errorf("expected IsProduction() true for production")
	}
}
