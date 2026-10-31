package repositories

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

// ErrEmailTaken is returned when registration collides with an existing address.
var ErrEmailTaken = errors.New("email already registered")

// ErrSessionReuse is returned when a refresh token that was already rotated is
// presented again. The caller must revoke the whole rotation family.
var ErrSessionReuse = errors.New("refresh token reuse detected")

// User is the authenticated principal.
type User struct {
	ID          string     `json:"id"`
	Email       string     `json:"email"`
	DisplayName string     `json:"display_name"`
	Status      string     `json:"status"`
	Roles       []string   `json:"roles"`
	CreatedAt   time.Time  `json:"created_at"`
	LastLoginAt *time.Time `json:"last_login_at"`
}

// CreateUser inserts a user and grants the default role in one round trip.
func (s *Store) CreateUser(ctx context.Context, email, passwordHash, displayName string) (*User, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	u := &User{Email: email, DisplayName: displayName, Roles: []string{"user"}}
	err = tx.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, display_name)
		VALUES ($1, $2, $3)
		RETURNING id::text, status, created_at`, email, passwordHash, displayName).
		Scan(&u.ID, &u.Status, &u.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrEmailTaken
		}
		return nil, err
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO user_roles (user_id, role_code) VALUES ($1::uuid, 'user')`, u.ID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return u, nil
}

// GetUserByEmail returns the user and their password hash for verification.
func (s *Store) GetUserByEmail(ctx context.Context, email string) (*User, string, error) {
	var u User
	var hash string
	err := s.pool.QueryRow(ctx, `
		SELECT id::text, email, COALESCE(display_name, ''), status, created_at, last_login_at, password_hash
		FROM users WHERE lower(email) = lower($1)`, email).
		Scan(&u.ID, &u.Email, &u.DisplayName, &u.Status, &u.CreatedAt, &u.LastLoginAt, &hash)
	if err != nil {
		return nil, "", wrapNoRows(err)
	}
	roles, err := s.RolesFor(ctx, u.ID)
	if err != nil {
		return nil, "", err
	}
	u.Roles = roles
	return &u, hash, nil
}

// GetUser returns a user by id, including current roles.
func (s *Store) GetUser(ctx context.Context, id string) (*User, error) {
	var u User
	err := s.pool.QueryRow(ctx, `
		SELECT id::text, email, COALESCE(display_name, ''), status, created_at, last_login_at
		FROM users WHERE id = $1::uuid`, id).
		Scan(&u.ID, &u.Email, &u.DisplayName, &u.Status, &u.CreatedAt, &u.LastLoginAt)
	if err != nil {
		return nil, wrapNoRows(err)
	}
	roles, err := s.RolesFor(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	u.Roles = roles
	return &u, nil
}

// RolesFor lists role codes for a user.
func (s *Store) RolesFor(ctx context.Context, userID string) ([]string, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT role_code FROM user_roles WHERE user_id = $1::uuid ORDER BY role_code`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	roles := []string{}
	for rows.Next() {
		var r string
		if err := rows.Scan(&r); err != nil {
			return nil, err
		}
		roles = append(roles, r)
	}
	return roles, rows.Err()
}

// TouchLogin records a successful sign-in.
func (s *Store) TouchLogin(ctx context.Context, userID string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE users SET last_login_at = now(), failed_logins = 0, locked_until = NULL WHERE id = $1::uuid`,
		userID)
	return err
}

// Session is a server-side refresh session.
type Session struct {
	ID        string     `json:"id"`
	FamilyID  string     `json:"family_id"`
	UserAgent string     `json:"user_agent"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt time.Time  `json:"expires_at"`
	RevokedAt *time.Time `json:"revoked_at"`
}

// CreateSession stores a refresh-token hash and returns the session id.
func (s *Store) CreateSession(ctx context.Context, userID string, refreshHash []byte, expiresAt time.Time, userAgent, ip string) (string, string, error) {
	var id, family string
	err := s.pool.QueryRow(ctx, `
		INSERT INTO sessions (user_id, refresh_hash, family_id, expires_at, user_agent, ip)
		VALUES ($1::uuid, $2, gen_random_uuid(), $3, $4, NULLIF($5, '')::inet)
		RETURNING id::text, family_id::text`,
		userID, refreshHash, expiresAt, userAgent, ip).Scan(&id, &family)
	return id, family, err
}

// SessionByHash resolves a presented refresh token.
func (s *Store) SessionByHash(ctx context.Context, refreshHash []byte) (*Session, string, error) {
	var sess Session
	var userID string
	err := s.pool.QueryRow(ctx, `
		SELECT id::text, user_id::text, family_id::text, COALESCE(user_agent, ''), created_at, expires_at, revoked_at
		FROM sessions WHERE refresh_hash = $1`, refreshHash).
		Scan(&sess.ID, &userID, &sess.FamilyID, &sess.UserAgent, &sess.CreatedAt, &sess.ExpiresAt, &sess.RevokedAt)
	if err != nil {
		return nil, "", wrapNoRows(err)
	}
	return &sess, userID, nil
}

// RotateSession revokes the old row and inserts the successor in one transaction.
func (s *Store) RotateSession(ctx context.Context, oldID, userID, familyID string, newHash []byte, expiresAt time.Time, userAgent, ip string) (string, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `UPDATE sessions SET revoked_at = now() WHERE id = $1::uuid`, oldID); err != nil {
		return "", err
	}
	var newID string
	err = tx.QueryRow(ctx, `
		INSERT INTO sessions (user_id, refresh_hash, family_id, expires_at, user_agent, ip, rotated_from)
		VALUES ($1::uuid, $2, $3::uuid, $4, $5, NULLIF($6, '')::inet, $7::uuid)
		RETURNING id::text`,
		userID, newHash, familyID, expiresAt, userAgent, ip, oldID).Scan(&newID)
	if err != nil {
		return "", err
	}
	return newID, tx.Commit(ctx)
}

// RevokeSessionByHash revokes the session matching a refresh-token digest.
func (s *Store) RevokeSessionByHash(ctx context.Context, refreshHash []byte) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE sessions SET revoked_at = now() WHERE refresh_hash = $1 AND revoked_at IS NULL`, refreshHash)
	return err
}

// RevokeFamily revokes every session in a rotation family (reuse defence).
func (s *Store) RevokeFamily(ctx context.Context, familyID string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE sessions SET revoked_at = now() WHERE family_id = $1::uuid AND revoked_at IS NULL`, familyID)
	return err
}

// RevokeSession revokes one session owned by the user.
func (s *Store) RevokeSession(ctx context.Context, sessionID, userID string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE sessions SET revoked_at = now()
		 WHERE id = $1::uuid AND user_id = $2::uuid AND revoked_at IS NULL`, sessionID, userID)
	return err
}

// ListSessions returns active sessions for the account screen.
func (s *Store) ListSessions(ctx context.Context, userID string) ([]Session, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id::text, family_id::text, COALESCE(user_agent, ''), created_at, expires_at, revoked_at
		FROM sessions
		WHERE user_id = $1::uuid AND revoked_at IS NULL AND expires_at > now()
		ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Session, 0, 4)
	for rows.Next() {
		var sess Session
		if err := rows.Scan(&sess.ID, &sess.FamilyID, &sess.UserAgent,
			&sess.CreatedAt, &sess.ExpiresAt, &sess.RevokedAt); err != nil {
			return nil, err
		}
		out = append(out, sess)
	}
	return out, rows.Err()
}

// Audit appends an audit record. Failures are deliberately non-fatal to callers.
func (s *Store) Audit(ctx context.Context, actorID, ip, action, target, outcome, requestID string, detail map[string]any) error {
	if detail == nil {
		detail = map[string]any{}
	}
	var actor *string
	if actorID != "" {
		actor = &actorID
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO audit_logs (actor_id, actor_ip, action, target, outcome, request_id, detail)
		VALUES ($1::uuid, NULLIF($2, '')::inet, $3, $4, $5, $6, $7)`,
		actor, ip, action, target, outcome, requestID, detail)
	return err
}

// CountUsers is used by tests to assert persistence.
func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&n)
	return n, err
}
