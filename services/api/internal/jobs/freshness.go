// Package freshness answers one question per dataset: is what we have still
// recent enough to act on?
//
// A dashboard that renders a three-day-old quote without saying so is worse than
// one that shows an error, because the number looks live. Each dataset therefore
// carries an explicit age, the SLA it is measured against, and a status.
package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DatasetStatus is one row of the freshness table.
type DatasetStatus struct {
	Stream        string     `json:"stream"`
	Description   string     `json:"description"`
	Owner         string     `json:"owner"`
	Severity      string     `json:"severity"`
	MaxAgeSeconds int        `json:"max_age_seconds"`
	Rows          int64      `json:"rows"`
	LatestAt      *time.Time `json:"latest_at"`
	AgeSeconds    *int64     `json:"age_seconds"`
	Status        string     `json:"status"` // fresh | stale | unknown
}

// Summary aggregates the per-dataset verdicts.
type Summary struct {
	CheckedAt time.Time `json:"checked_at"`
	Datasets  int       `json:"datasets"`
	Fresh     int       `json:"fresh"`
	Stale     int       `json:"stale"`
	Unknown   int       `json:"unknown"`
	// Worst is the most severe status present, so a caller can alert on one field.
	Worst string `json:"worst"`
}

// probe is the SQL behind one dataset. fallback uses the checkpoint table for
// datasets whose rows carry no timestamp of their own.
type probe struct {
	query          string
	checkpointOnly bool
}

var probes = map[string]probe{
	"floorsheet": {query: `SELECT count(*), max(ingested_at) FROM floorsheet_trades`},
	"quotes":     {query: `SELECT count(*), max(ingested_at) FROM daily_ohlcv`},
	"summary":    {query: `SELECT count(*), max(ingested_at) FROM market_summaries`},
	"indices": {query: `SELECT count(*), max(observed_at) FROM index_observations
		WHERE granularity = 'daily'`},
	"market_cap": {query: `SELECT count(*), max(ingested_at) FROM market_summaries
		WHERE market_cap IS NOT NULL`},
	"companies":     {query: `SELECT count(*), max(updated_at) FROM companies`},
	"securities":    {query: `SELECT count(*), NULL::timestamptz FROM securities`, checkpointOnly: true},
	"market_status": {query: `SELECT count(*), max(as_of) FROM market_status_snapshots`},
}

// Evaluate reads the SLAs and measures every dataset against them.
func Evaluate(ctx context.Context, pool *pgxpool.Pool) ([]DatasetStatus, Summary, error) {
	sum := Summary{CheckedAt: time.Now().UTC()}

	rows, err := pool.Query(ctx,
		`SELECT stream, description, owner, max_age_seconds, severity FROM dataset_slas ORDER BY stream`)
	if err != nil {
		return nil, sum, err
	}
	defer rows.Close()

	type sla struct {
		stream, description, owner, severity string
		maxAge                               int
	}
	var slas []sla
	for rows.Next() {
		var s sla
		if err := rows.Scan(&s.stream, &s.description, &s.owner, &s.maxAge, &s.severity); err != nil {
			return nil, sum, err
		}
		slas = append(slas, s)
	}
	if err := rows.Err(); err != nil {
		return nil, sum, err
	}

	capacity := make([]DatasetStatus, 0, len(slas))
	for _, s := range slas {
		st := DatasetStatus{
			Stream: s.stream, Description: s.description, Owner: s.owner,
			Severity: s.severity, MaxAgeSeconds: s.maxAge, Status: "unknown",
		}

		if p, ok := probes[s.stream]; ok {
			var latest *time.Time
			if err := pool.QueryRow(ctx, p.query).Scan(&st.Rows, &latest); err != nil {
				return nil, sum, fmt.Errorf("probe %s: %w", s.stream, err)
			}
			if p.checkpointOnly || latest == nil {
				var cp *time.Time
				_ = pool.QueryRow(ctx,
					`SELECT last_success_at FROM ingest_checkpoints WHERE stream = $1
					 ORDER BY last_success_at DESC NULLS LAST LIMIT 1`, s.stream).Scan(&cp)
				if cp != nil {
					latest = cp
				}
			}
			st.LatestAt = latest
			if latest != nil {
				age := int64(time.Since(*latest).Seconds())
				st.AgeSeconds = &age
				if age <= int64(s.maxAge) {
					st.Status = "fresh"
				} else {
					st.Status = "stale"
				}
			}
		}

		switch st.Status {
		case "fresh":
			sum.Fresh++
		case "stale":
			sum.Stale++
		default:
			sum.Unknown++
		}
		capacity = append(capacity, st)
	}

	sum.Datasets = len(capacity)
	switch {
	case sum.Stale > 0:
		sum.Worst = "stale"
	case sum.Unknown == sum.Datasets && sum.Datasets > 0:
		sum.Worst = "unknown"
	default:
		sum.Worst = "fresh"
	}
	return capacity, sum, nil
}
