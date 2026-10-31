// Package store owns the PostgreSQL connection pool and all SQL.
package repositories

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"nepse/terminal/api/migrations"
)

type Store struct {
	pool *pgxpool.Pool
}

// Open connects, verifies the connection and returns a store.
func Open(ctx context.Context, dsn string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	cfg.MaxConns = 10
	cfg.MaxConnLifetime = time.Hour
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() { s.pool.Close() }

func (s *Store) Pool() *pgxpool.Pool { return s.pool }

// Ping backs the readiness probe.
func (s *Store) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return s.pool.Ping(ctx)
}

// MigrationStatus is what /readyz reports about schema state.
type MigrationStatus struct {
	Applied int
	Latest  string
	Pending []string
}

// Migrate applies every pending up-migration in filename order.
//
// It is idempotent: an applied version is skipped, and a checksum change on an
// already-applied file is a hard error, so an edited migration can never be
// silently skipped.
func (s *Store) Migrate(ctx context.Context) (MigrationStatus, error) {
	var status MigrationStatus

	_, err := s.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    text PRIMARY KEY,
			checksum   text NOT NULL,
			applied_at timestamptz NOT NULL DEFAULT now()
		)`)
	if err != nil {
		return status, fmt.Errorf("create schema_migrations: %w", err)
	}

	entries, err := fs.Glob(migrations.FS, "*.up.sql")
	if err != nil {
		return status, err
	}
	sort.Strings(entries)

	applied := map[string]string{}
	rows, err := s.pool.Query(ctx, `SELECT version, checksum FROM schema_migrations`)
	if err != nil {
		return status, fmt.Errorf("read schema_migrations: %w", err)
	}
	for rows.Next() {
		var v, c string
		if err := rows.Scan(&v, &c); err != nil {
			rows.Close()
			return status, err
		}
		applied[v] = c
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return status, err
	}

	for _, file := range entries {
		body, err := migrations.FS.ReadFile(file)
		if err != nil {
			return status, err
		}
		sum := sha256.Sum256(body)
		checksum := hex.EncodeToString(sum[:])
		version := strings.TrimSuffix(file, ".up.sql")

		if prev, ok := applied[version]; ok {
			if prev != checksum {
				return status, fmt.Errorf("migration %s changed after being applied (checksum mismatch)", version)
			}
			status.Applied++
			status.Latest = version
			continue
		}

		tx, err := s.pool.Begin(ctx)
		if err != nil {
			return status, err
		}
		// The whole file runs in one transaction: a failure leaves no partial schema.
		if _, err := tx.Exec(ctx, string(body)); err != nil {
			_ = tx.Rollback(ctx)
			return status, fmt.Errorf("apply %s: %w", version, err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO schema_migrations (version, checksum) VALUES ($1, $2)`, version, checksum); err != nil {
			_ = tx.Rollback(ctx)
			return status, err
		}
		if err := tx.Commit(ctx); err != nil {
			return status, err
		}
		status.Applied++
		status.Latest = version
	}

	return status, nil
}
