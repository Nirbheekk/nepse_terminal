package marketdata

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	_ "modernc.org/sqlite" // pure-Go driver: no CGO, so the binary stays static
)

// SQLiteAdapter reads the legacy collector's database directly.
//
// It opens the file read-only and with query_only enabled, so a mistake here can
// never corrupt the collector's primary store. That matters because this file is
// still being written by a running process.
type SQLiteAdapter struct {
	Path string
}

func (a *SQLiteAdapter) Slug() string { return "legacy-sqlite" }

func (a *SQLiteAdapter) Authorization() Authorization {
	return Authorization{
		Mode:           ModeOperatorCaptured,
		LicenceClass:   LicenceUnverified,
		Redistribution: RedistributeNo,
		Note:           "Read-only copy of the collector's SQLite store; personal research only, no redistribution.",
	}
}

// sqliteStream maps a platform stream onto the legacy table backing it.
type sqliteStream struct {
	stream string
	table  string
	cursor string // monotonic column used as the high-water mark
}

// Only these tables may be read. The table name goes into the query string, so it
// comes from this fixed list and never from a caller.
var sqliteStreams = []sqliteStream{
	{StreamFloorsheet, "floorsheet", "contract_id"},
	{StreamIndices, "index_snapshots", "id"},
	{StreamMarketCap, "market_cap_snapshots", "id"},
	{StreamQuotes, "daily_prices", "id"},
}

func (a *SQLiteAdapter) Streams() []string {
	out := make([]string, 0, len(sqliteStreams))
	for _, s := range sqliteStreams {
		out = append(out, s.stream)
	}
	return out
}

func (a *SQLiteAdapter) db() (*sql.DB, error) {
	abs, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	dbfile := a.Path
	if !strings.HasPrefix(dbfile, "/") && !strings.Contains(dbfile, ":\\") {
		dbfile = abs + string(os.PathSeparator) + dbfile
	}
	q := url.Values{}
	q.Set("mode", "ro")
	q.Set("_pragma", "query_only(1)")
	q.Add("_pragma", "busy_timeout(5000)")
	return sql.Open("sqlite", "file:"+dbfile+"?"+q.Encode())
}

func (a *SQLiteAdapter) Each(ctx context.Context, stream string, fn func(map[string]any) error) (Batch, error) {
	var spec *sqliteStream
	for i := range sqliteStreams {
		if sqliteStreams[i].stream == stream {
			spec = &sqliteStreams[i]
			break
		}
	}
	if spec == nil {
		return Batch{}, fmt.Errorf("sqlite adapter does not provide stream %q", stream)
	}

	db, err := a.db()
	if err != nil {
		return Batch{}, err
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return Batch{}, fmt.Errorf("open %s: %w", a.Path, err)
	}

	exists, err := tableExists(ctx, db, spec.table)
	if err != nil {
		return Batch{}, err
	}
	if !exists {
		return Batch{}, fmt.Errorf("legacy table %q does not exist", spec.table)
	}

	// Ordered by the cursor column so rows arrive in the order they were
	// collected, which makes a partial run resumable in a predictable place.
	query := fmt.Sprintf("SELECT * FROM %s ORDER BY %s", spec.table, spec.cursor)
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return Batch{}, fmt.Errorf("query %s: %w", spec.table, err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return Batch{}, err
	}

	batch := Batch{
		Stream:     stream,
		CapturedAt: time.Now().UTC(),
		Meta: map[string]any{
			"adapter":       a.Slug(),
			"legacy_table":  spec.table,
			"source_path":   a.Path,
			"query_ordered": spec.cursor,
		},
	}

	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return batch, err
		}
		row := make(map[string]any, len(cols))
		for i, c := range cols {
			row[c] = scanValue(vals[i])
		}
		if err := fn(row); err != nil {
			return batch, err
		}
		batch.Rows++
		if c := cursorColumn(row, spec.cursor); c != "" {
			batch.Cursor = c
		}
	}
	return batch, rows.Err()
}

func tableExists(ctx context.Context, db *sql.DB, table string) (bool, error) {
	var n int
	err := db.QueryRowContext(ctx,
		`SELECT count(*) FROM sqlite_master WHERE type='table' AND name = ?`, table).Scan(&n)
	return n > 0, err
}

// scanValue converts driver types into the JSON-friendly types the archive and
// the normalisers expect.
func scanValue(v any) any {
	switch t := v.(type) {
	case []byte:
		return string(t)
	case time.Time:
		return t.UTC().Format(time.RFC3339Nano)
	default:
		return v
	}
}

// cursorColumn renders an int64 or string cursor without float formatting.
func cursorColumn(row map[string]any, col string) string {
	switch v := row[col].(type) {
	case nil:
		return ""
	case int64:
		return fmt.Sprintf("%d", v)
	case float64:
		return fmt.Sprintf("%.0f", v)
	case string:
		return v
	}
	return ""
}
