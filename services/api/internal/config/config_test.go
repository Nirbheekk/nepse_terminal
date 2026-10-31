package config

import (
	"strings"
	"testing"
	"time"
)

// baseEnv sets a configuration that must be accepted, so each test can change
// exactly one thing.
func baseEnv(t *testing.T) {
	t.Helper()
	t.Setenv("APP_ENV", "development")
	t.Setenv("DATABASE_URL", "postgres://u:p@localhost:5433/db?sslmode=disable")
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")
	t.Setenv("ACCESS_TTL", "15m")
	t.Setenv("REFRESH_TTL", "720h")
	t.Setenv("COOKIE_SECURE", "false")
	t.Setenv("ALLOWED_ORIGINS", "http://localhost:3100")
	t.Setenv("RATE_LIMIT_RPM", "240")
}

func TestLoadAcceptsValidConfiguration(t *testing.T) {
	baseEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected valid config, got %v", err)
	}
	if cfg.HTTPAddr != ":8081" {
		t.Errorf("default HTTPAddr = %q, want :8081", cfg.HTTPAddr)
	}
	if cfg.AccessTTL != 15*time.Minute {
		t.Errorf("AccessTTL = %v, want 15m", cfg.AccessTTL)
	}
	if len(cfg.AllowedOrigins) != 1 || cfg.AllowedOrigins[0] != "http://localhost:3100" {
		t.Errorf("AllowedOrigins = %v", cfg.AllowedOrigins)
	}
}

func TestLoadRejectsMissingDatabaseURL(t *testing.T) {
	baseEnv(t)
	t.Setenv("DATABASE_URL", "")
	assertFails(t, "DATABASE_URL is required")
}

func TestLoadRejectsShortSecret(t *testing.T) {
	baseEnv(t)
	t.Setenv("JWT_SECRET", "too-short")
	assertFails(t, "JWT_SECRET must be at least 32 bytes")
}

func TestLoadRejectsAccessTTLLongerThanHour(t *testing.T) {
	baseEnv(t)
	t.Setenv("ACCESS_TTL", "2h")
	assertFails(t, "ACCESS_TTL must be > 0 and <= 1h")
}

func TestLoadRejectsRefreshShorterThanAccess(t *testing.T) {
	baseEnv(t)
	t.Setenv("ACCESS_TTL", "30m")
	t.Setenv("REFRESH_TTL", "10m")
	assertFails(t, "REFRESH_TTL must exceed ACCESS_TTL")
}

// Production hardening is enforced, not documented: an insecure deployment must
// fail to boot rather than silently serve without Secure cookies.
func TestProductionRequiresSecureCookies(t *testing.T) {
	baseEnv(t)
	t.Setenv("APP_ENV", "production")
	t.Setenv("COOKIE_SECURE", "false")
	assertFails(t, "COOKIE_SECURE must be true when APP_ENV=production")
}

func TestProductionRequiresAllowedOrigins(t *testing.T) {
	baseEnv(t)
	t.Setenv("APP_ENV", "production")
	t.Setenv("COOKIE_SECURE", "true")
	t.Setenv("ALLOWED_ORIGINS", "")
	assertFails(t, "ALLOWED_ORIGINS must list the public origin")
}

func TestProductionRejectsPlaceholderSecret(t *testing.T) {
	baseEnv(t)
	t.Setenv("APP_ENV", "production")
	t.Setenv("COOKIE_SECURE", "true")
	t.Setenv("JWT_SECRET", "change-me-please-0123456789abcdefgh")
	assertFails(t, "development placeholder")
}

func TestLoadRejectsMalformedDuration(t *testing.T) {
	baseEnv(t)
	t.Setenv("ACCESS_TTL", "fifteen-minutes")
	if _, err := Load(); err == nil {
		t.Fatal("expected an error for a malformed duration")
	}
}

func TestLoadRejectsNonNumericRateLimit(t *testing.T) {
	baseEnv(t)
	t.Setenv("RATE_LIMIT_RPM", "lots")
	if _, err := Load(); err == nil {
		t.Fatal("expected an error for a non-numeric RATE_LIMIT_RPM")
	}
}

func assertFails(t *testing.T, want string) {
	t.Helper()
	_, err := Load()
	if err == nil {
		t.Fatalf("expected an error containing %q, got nil", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %q, want it to contain %q", err.Error(), want)
	}
}
