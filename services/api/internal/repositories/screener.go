package repositories

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"nepse/terminal/api/internal/screener"
)

// ErrNameTaken is returned when a tenant already owns a resource with that name.
// Names are unique per owner, never globally, so two tenants may both keep a
// list called "Banks".
var ErrNameTaken = errors.New("name already in use")

// ErrLimitReached is returned when a collection is at its cap.
var ErrLimitReached = errors.New("limit reached")

// ScreenerCoverage states how many securities can answer each class of filter.
// It travels with every screen response so an empty result is explainable.
type ScreenerCoverage struct {
	Securities    int    `json:"securities"`
	WithBars20    int    `json:"with_20_sessions"`
	WithBars50    int    `json:"with_50_sessions"`
	WithBars252   int    `json:"with_252_sessions"`
	WithFloatCap  int    `json:"with_float_cap"`
	WithFreeFloat int    `json:"with_free_float"`
	MaxBars       int    `json:"max_sessions"`
	LatestSession string `json:"latest_session"`
}

// RunScreen executes a normalised rule and returns its rows.
func (s *Store) RunScreen(ctx context.Context, rule screener.Rule) ([]screener.Row, error) {
	q, args, err := screener.Query(rule)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]screener.Row, 0, rule.Limit)
	for rows.Next() {
		var (
			r                               screener.Row
			close, prev, change             sql.NullString
			volume, turnover                sql.NullString
			transactions                    sql.NullInt64
			mcap, fcap, freeFloat           sql.NullString
			listed, public                  sql.NullString
			sma20, sma50, high52, low52     sql.NullString
			fromHigh, volRatio, ret5, ret20 sql.NullString
			bars                            int64
		)
		if err := rows.Scan(
			&r.SecurityID, &r.Symbol, &r.Name, &r.Sector, &r.SectorCode, &r.Instrument,
			&r.Status, &r.TradingDate, &close, &prev, &change, &volume, &turnover,
			&transactions, &mcap, &fcap, &freeFloat, &listed, &public, &sma20, &sma50,
			&high52, &low52, &fromHigh, &volRatio, &ret5, &ret20, &bars,
		); err != nil {
			return nil, err
		}
		r.Close, r.PreviousClose, r.ChangePct = strPtr(close), strPtr(prev), strPtr(change)
		r.Volume, r.Turnover = strPtr(volume), strPtr(turnover)
		if transactions.Valid {
			v := transactions.Int64
			r.Transactions = &v
		}
		r.MarketCap, r.FloatCap, r.FreeFloatPct = strPtr(mcap), strPtr(fcap), strPtr(freeFloat)
		r.ListedShares, r.PublicShares = strPtr(listed), strPtr(public)
		r.SMA20, r.SMA50 = strPtr(sma20), strPtr(sma50)
		r.High52W, r.Low52W = strPtr(high52), strPtr(low52)
		r.PctFromHigh, r.VolumeRatio = strPtr(fromHigh), strPtr(volRatio)
		r.Return5D, r.Return20D = strPtr(ret5), strPtr(ret20)
		r.Bars = int(bars)
		out = append(out, r)
	}
	return out, rows.Err()
}

// ScreenerCoverage reports how much history and share-count data exists.
func (s *Store) ScreenerCoverage(ctx context.Context) (*ScreenerCoverage, error) {
	var c ScreenerCoverage
	var maxBars sql.NullInt64
	err := s.pool.QueryRow(ctx, screener.CoverageQuery()).Scan(
		&c.Securities, &c.WithBars20, &c.WithBars50, &c.WithBars252,
		&c.WithFloatCap, &c.WithFreeFloat, &maxBars, &c.LatestSession)
	if err != nil {
		return nil, err
	}
	if maxBars.Valid {
		c.MaxBars = int(maxBars.Int64)
	}
	return &c, nil
}

func strPtr(v sql.NullString) *string {
	if !v.Valid {
		return nil
	}
	s := v.String
	return &s
}

// ---------------------------------------------------------------- saved screens

// SavedScreen is a tenant's stored rule.
type SavedScreen struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Definition  json.RawMessage `json:"definition"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// CreateScreen stores a rule for one owner. The definition has already been
// validated by the handler; it is stored verbatim so a later run reproduces it.
func (s *Store) CreateScreen(ctx context.Context, userID, name, description string, definition []byte) (*SavedScreen, error) {
	out := &SavedScreen{Name: name, Description: description, Definition: definition}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO screening_rules (user_id, name, description, definition)
		VALUES ($1::uuid, $2, $3, $4)
		RETURNING id::text, created_at, updated_at`,
		userID, name, description, definition).
		Scan(&out.ID, &out.CreatedAt, &out.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrNameTaken
		}
		return nil, err
	}
	return out, nil
}

// ListScreens returns every screen the owner has saved, newest first.
func (s *Store) ListScreens(ctx context.Context, userID string) ([]SavedScreen, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id::text, name, description, definition, created_at, updated_at
		FROM screening_rules WHERE user_id = $1::uuid
		ORDER BY updated_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []SavedScreen{}
	for rows.Next() {
		var sc SavedScreen
		if err := rows.Scan(&sc.ID, &sc.Name, &sc.Description, &sc.Definition,
			&sc.CreatedAt, &sc.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, sc)
	}
	return out, rows.Err()
}

// GetScreen returns one screen only if the caller owns it.
//
// A screen owned by someone else is reported as absent rather than forbidden:
// telling a caller that an id exists but belongs to another tenant is itself a
// disclosure.
func (s *Store) GetScreen(ctx context.Context, userID, id string) (*SavedScreen, error) {
	var sc SavedScreen
	err := s.pool.QueryRow(ctx, `
		SELECT id::text, name, description, definition, created_at, updated_at
		FROM screening_rules WHERE id = $1::uuid AND user_id = $2::uuid`,
		id, userID).
		Scan(&sc.ID, &sc.Name, &sc.Description, &sc.Definition, &sc.CreatedAt, &sc.UpdatedAt)
	if err != nil {
		return nil, wrapNoRows(err)
	}
	return &sc, nil
}

// UpdateScreen replaces a screen's definition and metadata for its owner.
func (s *Store) UpdateScreen(ctx context.Context, userID, id, name, description string, definition []byte) (*SavedScreen, error) {
	var sc SavedScreen
	err := s.pool.QueryRow(ctx, `
		UPDATE screening_rules
		SET name = $3, description = $4, definition = $5, updated_at = now()
		WHERE id = $1::uuid AND user_id = $2::uuid
		RETURNING id::text, name, description, definition, created_at, updated_at`,
		id, userID, name, description, definition).
		Scan(&sc.ID, &sc.Name, &sc.Description, &sc.Definition, &sc.CreatedAt, &sc.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrNameTaken
		}
		return nil, wrapNoRows(err)
	}
	return &sc, nil
}

// DeleteScreen removes a screen the caller owns and reports whether it existed.
func (s *Store) DeleteScreen(ctx context.Context, userID, id string) (bool, error) {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM screening_rules WHERE id = $1::uuid AND user_id = $2::uuid`, id, userID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
