package repositories

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// MaxWatchlistItems caps a single list. The limit exists so one client cannot
// grow a list without bound; it is enforced inside the insert rather than by a
// count-then-insert in the handler, which two concurrent requests could both
// pass.
const MaxWatchlistItems = 500

// Watchlist is a named set of symbols owned by one user.
type Watchlist struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	ItemCount int             `json:"item_count"`
	Items     []WatchlistItem `json:"items"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// WatchlistItem is one member, joined to the latest quote so the list is useful
// without a second request. Quote fields stay pointers: a symbol with no stored
// session has no price, which is different from a price of zero.
type WatchlistItem struct {
	Symbol      string    `json:"symbol"`
	Name        string    `json:"name"`
	Note        string    `json:"note"`
	AddedAt     time.Time `json:"added_at"`
	TradingDate *string   `json:"trading_date,omitempty"`
	Close       *string   `json:"close,omitempty"`
	ChangePct   *string   `json:"change_pct,omitempty"`
	Volume      *string   `json:"volume,omitempty"`
}

// CreateWatchlist creates an empty list for one owner.
func (s *Store) CreateWatchlist(ctx context.Context, userID, name string) (*Watchlist, error) {
	w := &Watchlist{Name: name, Items: []WatchlistItem{}}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO watchlists (user_id, name) VALUES ($1::uuid, $2)
		RETURNING id::text, created_at, updated_at`, userID, name).
		Scan(&w.ID, &w.CreatedAt, &w.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrNameTaken
		}
		return nil, err
	}
	return w, nil
}

// ListWatchlists returns the owner's lists with their members and quotes.
func (s *Store) ListWatchlists(ctx context.Context, userID string) ([]Watchlist, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id::text, name, created_at, updated_at
		FROM watchlists WHERE user_id = $1::uuid
		ORDER BY updated_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Watchlist{}
	ids := []string{}
	for rows.Next() {
		var w Watchlist
		if err := rows.Scan(&w.ID, &w.Name, &w.CreatedAt, &w.UpdatedAt); err != nil {
			return nil, err
		}
		w.Items = []WatchlistItem{}
		out = append(out, w)
		ids = append(ids, w.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return out, nil
	}

	items, err := s.watchlistItems(ctx, userID, ids)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Items = items[out[i].ID]
		if out[i].Items == nil {
			out[i].Items = []WatchlistItem{}
		}
		out[i].ItemCount = len(out[i].Items)
	}
	return out, nil
}

// watchlistItems loads members for several lists in one query, scoped to the
// owner so an id from another tenant yields nothing.
func (s *Store) watchlistItems(ctx context.Context, userID string, listIDs []string) (map[string][]WatchlistItem, error) {
	rows, err := s.pool.Query(ctx, `
		WITH latest AS (SELECT max(trading_date) AS dt FROM daily_ohlcv)
		SELECT i.watchlist_id::text, i.symbol, COALESCE(c.name, i.symbol), i.note, i.added_at,
		       o.trading_date::text, o.close::text,
		       CASE WHEN o.previous_close > 0
		            THEN (100.0 * (o.close - o.previous_close) / o.previous_close)::text END,
		       o.volume::text
		FROM watchlist_items i
		JOIN watchlists w ON w.id = i.watchlist_id AND w.user_id = $1::uuid
		LEFT JOIN securities sec ON sec.symbol = i.symbol
		LEFT JOIN companies c ON c.id = sec.company_id
		LEFT JOIN latest l ON true
		LEFT JOIN daily_ohlcv o ON o.security_id = sec.id AND o.trading_date = l.dt
		WHERE i.watchlist_id = ANY($2::uuid[])
		ORDER BY i.symbol`, userID, listIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string][]WatchlistItem{}
	for rows.Next() {
		var (
			listID, symbol, name, note string
			addedAt                    time.Time
			tradingDate, close         sql.NullString
			change                     sql.NullString
			volume                     sql.NullString
		)
		if err := rows.Scan(&listID, &symbol, &name, &note, &addedAt,
			&tradingDate, &close, &change, &volume); err != nil {
			return nil, err
		}
		out[listID] = append(out[listID], WatchlistItem{
			Symbol: symbol, Name: name, Note: note, AddedAt: addedAt,
			TradingDate: strPtr(tradingDate), Close: strPtr(close),
			ChangePct: strPtr(change), Volume: strPtr(volume),
		})
	}
	return out, rows.Err()
}

// GetWatchlist returns one list with its members, or ErrNotFound when the caller
// does not own it.
func (s *Store) GetWatchlist(ctx context.Context, userID, id string) (*Watchlist, error) {
	var w Watchlist
	err := s.pool.QueryRow(ctx, `
		SELECT id::text, name, created_at, updated_at
		FROM watchlists WHERE id = $1::uuid AND user_id = $2::uuid`, id, userID).
		Scan(&w.ID, &w.Name, &w.CreatedAt, &w.UpdatedAt)
	if err != nil {
		return nil, wrapNoRows(err)
	}
	items, err := s.watchlistItems(ctx, userID, []string{w.ID})
	if err != nil {
		return nil, err
	}
	w.Items = items[w.ID]
	if w.Items == nil {
		w.Items = []WatchlistItem{}
	}
	w.ItemCount = len(w.Items)
	return &w, nil
}

// RenameWatchlist changes a list's name for its owner.
func (s *Store) RenameWatchlist(ctx context.Context, userID, id, name string) (*Watchlist, error) {
	var w Watchlist
	err := s.pool.QueryRow(ctx, `
		UPDATE watchlists SET name = $3, updated_at = now()
		WHERE id = $1::uuid AND user_id = $2::uuid
		RETURNING id::text, name, created_at, updated_at`, id, userID, name).
		Scan(&w.ID, &w.Name, &w.CreatedAt, &w.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrNameTaken
		}
		return nil, wrapNoRows(err)
	}
	w.Items = []WatchlistItem{}
	return &w, nil
}

// DeleteWatchlist removes a list and its members.
func (s *Store) DeleteWatchlist(ctx context.Context, userID, id string) (bool, error) {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM watchlists WHERE id = $1::uuid AND user_id = $2::uuid`, id, userID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// AddWatchlistItem inserts one symbol.
//
// The insert selects from securities, so an unknown symbol inserts nothing and is
// reported as not found; the cap is part of the same statement so two concurrent
// adds cannot both slip past it.
func (s *Store) AddWatchlistItem(ctx context.Context, userID, listID, symbol, note string) (bool, error) {
	var inserted bool
	err := s.pool.QueryRow(ctx, `
		WITH owned AS (
			SELECT id FROM watchlists WHERE id = $1::uuid AND user_id = $2::uuid
		), known AS (
			SELECT symbol FROM securities WHERE symbol = $3
		), counted AS (
			SELECT (SELECT count(*) FROM watchlist_items i
			        JOIN owned o ON o.id = i.watchlist_id) AS n
		)
		INSERT INTO watchlist_items (watchlist_id, symbol, note)
		SELECT o.id, k.symbol, $4 FROM owned o, known k, counted c
		WHERE c.n < $5
		ON CONFLICT (watchlist_id, symbol) DO UPDATE SET note = EXCLUDED.note
		-- xmax is zero only for a genuinely new row, so a re-add that updates the
		-- note reports "updated", not "added".
		RETURNING (xmax = 0)`, listID, userID, symbol, note, MaxWatchlistItems).Scan(&inserted)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Distinguish "no such list for you" from "no such symbol" from "list
			// is full": the caller needs a different fix for each.
			owned, ownErr := s.watchlistOwned(ctx, userID, listID)
			if ownErr != nil {
				return false, ownErr
			}
			if !owned {
				return false, ErrNotFound
			}
			exists, symErr := s.symbolExists(ctx, symbol)
			if symErr != nil {
				return false, symErr
			}
			if !exists {
				return false, ErrNotFound
			}
			return false, ErrLimitReached
		}
		return false, err
	}
	if inserted {
		_, _ = s.pool.Exec(ctx, `UPDATE watchlists SET updated_at = now() WHERE id = $1::uuid`, listID)
	}
	return inserted, nil
}

// RemoveWatchlistItem deletes one symbol from a list the caller owns.
func (s *Store) RemoveWatchlistItem(ctx context.Context, userID, listID, symbol string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM watchlist_items i
		USING watchlists w
		WHERE i.watchlist_id = w.id AND w.id = $1::uuid AND w.user_id = $2::uuid
		  AND i.symbol = $3`, listID, userID, symbol)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() == 0 {
		owned, err := s.watchlistOwned(ctx, userID, listID)
		if err != nil {
			return false, err
		}
		if !owned {
			return false, ErrNotFound
		}
	}
	return tag.RowsAffected() > 0, nil
}

func (s *Store) watchlistOwned(ctx context.Context, userID, listID string) (bool, error) {
	var ok bool
	err := s.pool.QueryRow(ctx, `
		SELECT true FROM watchlists WHERE id = $1::uuid AND user_id = $2::uuid`,
		listID, userID).Scan(&ok)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return ok, nil
}

func (s *Store) symbolExists(ctx context.Context, symbol string) (bool, error) {
	var ok bool
	err := s.pool.QueryRow(ctx, `SELECT true FROM securities WHERE symbol = $1`, symbol).Scan(&ok)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return ok, nil
}
