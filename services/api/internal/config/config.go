// Package config loads and validates runtime configuration.
//
// Every value comes from the environment. Nothing is defaulted in a way that
// would be unsafe in production: validation refuses to start instead.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Env            string
	HTTPAddr       string
	DatabaseURL    string
	JWTSecret      []byte
	JWTIssuer      string
	AccessTTL      time.Duration
	RefreshTTL     time.Duration
	CookieSecure   bool
	CookieDomain   string
	AllowedOrigins []string
	RateLimitRPM   int
	SeedOnStart    bool
	LogLevel       string

	// Data-plane paths. Defaults are relative to the platform directory, which is
	// where every documented command runs from.
	LegacyDBPath string // legacy collector SQLite file
	DataDir      string // captured JSON snapshots
	ExportDir    string // exports/legacy/<run> (empty = newest found)
	ArchiveDir   string // immutable raw archive root
	ReportDir    string // where generated reports are written
	// FloorsheetPagesDir holds the per-page trade tape archive (floorsheet/pages).
	FloorsheetPagesDir string
}

const minSecretBytes = 32

// Load reads the environment and validates it.
//
// A .env file in the working directory is read first, without overriding
// variables that are already set, so `go run ./cmd/api` works from the platform
// directory while a container or CI job can still inject real values.
func Load() (*Config, error) {
	loadDotEnv(".env")

	c := &Config{
		Env:          getenv("APP_ENV", "development"),
		HTTPAddr:     getenv("HTTP_ADDR", ":8081"),
		DatabaseURL:  getenv("DATABASE_URL", ""),
		JWTIssuer:    getenv("JWT_ISSUER", "nepse-platform"),
		CookieDomain: os.Getenv("COOKIE_DOMAIN"),
		LogLevel:     getenv("LOG_LEVEL", "info"),

		LegacyDBPath:       getenv("LEGACY_DB_PATH", "../nepse.db"),
		DataDir:            getenv("SEED_DATA_DIR", "../data"),
		ExportDir:          os.Getenv("SEED_EXPORT_DIR"),
		ArchiveDir:         getenv("ARCHIVE_DIR", "../var/archive"),
		ReportDir:          getenv("REPORT_DIR", "../docs"),
		FloorsheetPagesDir: getenv("FLOORSHEET_PAGES_DIR", "../floorsheet/pages"),
	}
	secret := os.Getenv("JWT_SECRET")
	c.JWTSecret = []byte(secret)

	var err error
	if c.AccessTTL, err = parseDuration("ACCESS_TTL", "15m"); err != nil {
		return nil, err
	}
	if c.RefreshTTL, err = parseDuration("REFRESH_TTL", "720h"); err != nil {
		return nil, err
	}
	if c.CookieSecure, err = parseBool("COOKIE_SECURE", c.Env == "production"); err != nil {
		return nil, err
	}
	if c.SeedOnStart, err = parseBool("SEED_ON_START", c.Env != "production"); err != nil {
		return nil, err
	}
	if c.RateLimitRPM, err = parseInt("RATE_LIMIT_RPM", 240); err != nil {
		return nil, err
	}
	if origins := os.Getenv("ALLOWED_ORIGINS"); origins != "" {
		for _, o := range strings.Split(origins, ",") {
			if o = strings.TrimSpace(o); o != "" {
				c.AllowedOrigins = append(c.AllowedOrigins, o)
			}
		}
	}

	if err := c.Validate(); err != nil {
		return nil, err
	}
	return c, nil
}

// Validate checks cross-field invariants. It is exported so tests can assert
// that unsafe combinations are rejected.
func (c *Config) Validate() error {
	var problems []string

	if c.DatabaseURL == "" {
		problems = append(problems, "DATABASE_URL is required")
	}
	if len(c.JWTSecret) < minSecretBytes {
		problems = append(problems, fmt.Sprintf("JWT_SECRET must be at least %d bytes", minSecretBytes))
	}
	if c.Env == "production" {
		if !c.CookieSecure {
			problems = append(problems, "COOKIE_SECURE must be true when APP_ENV=production")
		}
		if strings.Contains(string(c.JWTSecret), "change-me") || strings.Contains(string(c.JWTSecret), "dev-secret") {
			problems = append(problems, "JWT_SECRET must not be a development placeholder in production")
		}
		if len(c.AllowedOrigins) == 0 {
			problems = append(problems, "ALLOWED_ORIGINS must list the public origin when APP_ENV=production")
		}
	}
	if c.AccessTTL <= 0 || c.AccessTTL > time.Hour {
		problems = append(problems, "ACCESS_TTL must be > 0 and <= 1h")
	}
	if c.RefreshTTL <= c.AccessTTL {
		problems = append(problems, "REFRESH_TTL must exceed ACCESS_TTL")
	}
	if c.RateLimitRPM <= 0 {
		problems = append(problems, "RATE_LIMIT_RPM must be positive")
	}

	if len(problems) > 0 {
		return errors.New("invalid configuration: " + strings.Join(problems, "; "))
	}
	return nil
}

// IsProduction reports whether production-only hardening applies.
func (c *Config) IsProduction() bool { return c.Env == "production" }

// loadDotEnv applies KEY=VALUE lines from path. Blank lines and # comments are
// ignored; existing environment variables always win.
func loadDotEnv(path string) {
	body, err := os.ReadFile(path)
	if err != nil {
		return // no file is the normal case in production
	}
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		_ = os.Setenv(key, value)
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func parseDuration(key, fallback string) (time.Duration, error) {
	v := getenv(key, fallback)
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return d, nil
}

func parseBool(key string, fallback bool) (bool, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("%s: %w", key, err)
	}
	return b, nil
}

func parseInt(key string, fallback int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return n, nil
}
