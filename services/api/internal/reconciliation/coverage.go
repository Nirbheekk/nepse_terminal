package reconciliation

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Coverage describes what the platform actually holds for one stream.
//
// Counts alone are not coverage: "48,273 trades" says nothing about which
// symbols, which sessions, or whether anything arrived after the load.
type Coverage struct {
	Stream     string     `json:"stream"`
	Rows       int64      `json:"rows"`
	Distinct   int64      `json:"distinct_subjects"`
	Subject    string     `json:"subject"`
	FirstDate  string     `json:"first_date,omitempty"`
	LastDate   string     `json:"last_date,omitempty"`
	Sessions   int64      `json:"sessions"`
	Archives   int64      `json:"archives"`
	LastIngest *time.Time `json:"last_ingest,omitempty"`
	Extra      string     `json:"extra,omitempty"`
}

// CollectCoverage gathers per-stream facts for the coverage report.
func CollectCoverage(ctx context.Context, pool *pgxpool.Pool) ([]Coverage, error) {
	specs := []struct {
		stream  string
		subject string
		query   string
	}{
		{
			stream: "floorsheet", subject: "symbols",
			query: `SELECT count(*), count(DISTINCT symbol), min(trading_date)::text,
			               max(trading_date)::text, count(DISTINCT trading_date)
			        FROM floorsheet_trades`,
		},
		{
			stream: "quotes", subject: "securities",
			query: `SELECT count(*), count(DISTINCT security_id), min(trading_date)::text,
			               max(trading_date)::text, count(DISTINCT trading_date)
			        FROM daily_ohlcv`,
		},
		{
			stream: "summary", subject: "sessions",
			query: `SELECT count(*), count(DISTINCT trading_date), min(trading_date)::text,
			               max(trading_date)::text, count(DISTINCT trading_date)
			        FROM market_summaries`,
		},
		{
			stream: "indices", subject: "indices",
			query: `SELECT count(*), count(DISTINCT index_id), min(trading_date)::text,
			               max(trading_date)::text, count(DISTINCT trading_date)
			        FROM index_observations WHERE granularity = 'daily'`,
		},
		{
			stream: "market_cap", subject: "sessions with capitalisation",
			query: `SELECT count(*), count(DISTINCT trading_date), min(trading_date)::text,
			               max(trading_date)::text, count(DISTINCT trading_date)
			        FROM market_summaries WHERE market_cap IS NOT NULL`,
		},
		{
			stream: "companies", subject: "sectors",
			// Registry entries only: a placeholder issuer carries no sector by
			// construction, so counting it would report a coverage gap that is
			// really a missing registry entry, not a missing sector.
			query: `SELECT count(*), count(DISTINCT sector_id), ''::text, ''::text, 0
			        FROM companies WHERE origin = 'registry'`,
		},
		{
			stream: "securities", subject: "ISINs",
			query: `SELECT count(*), count(isin), ''::text, ''::text, 0 FROM securities`,
		},
	}

	out := make([]Coverage, 0, len(specs))
	for _, s := range specs {
		var c Coverage
		c.Stream = s.stream
		c.Subject = s.subject
		if err := pool.QueryRow(ctx, s.query).Scan(
			&c.Rows, &c.Distinct, &c.FirstDate, &c.LastDate, &c.Sessions); err != nil {
			return nil, err
		}
		if s.stream == "securities" {
			var total int64
			_ = pool.QueryRow(ctx, `SELECT count(*) FROM securities`).Scan(&total)
			c.Extra = "ISINs stored"
		}
		_ = pool.QueryRow(ctx,
			`SELECT count(*) FROM raw_archives WHERE stream = $1`, s.stream).Scan(&c.Archives)
		var last *time.Time
		_ = pool.QueryRow(ctx,
			`SELECT max(finished_at) FROM ingest_runs WHERE stream = $1 AND status IN ('success','partial')`,
			s.stream).Scan(&last)
		c.LastIngest = last
		out = append(out, c)
	}

	// Archive and run totals are part of coverage too: they say whether the load
	// is reproducible from stored bytes.
	return out, nil
}
