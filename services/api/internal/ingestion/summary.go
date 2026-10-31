package ingestion

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"nepse/terminal/api/internal/marketdata"
)

// The summary stream is not row-per-row: the exchange sends one JSON array of
// label/value pairs plus an index quote, and the row is only meaningful once all
// of them are combined. It therefore gets a dedicated path instead of the generic
// stage-and-merge engine.
func (i *Ingester) ingestSummary(ctx context.Context, srcID, path string) (written, unchanged, derived int, rejections []Rejection, err error) {
	var turnover, shares, transactions, scrips *string
	var indexValue, pointChange, percentChange *string
	var tradingDate string
	rowNum := 0

	_, err = EachRow(path, func(row map[string]any) error {
		rowNum++
		kind := str(row, "kind")
		switch kind {
		case "summary_item":
			label := strings.ToLower(str(row, "detail"))
			val, verr := marketdata.OptionalDecimal(pick(row, "value"))
			if verr != nil {
				rejections = append(rejections, Rejection{
					Row: rowNum, Field: "value",
					Reason: fmt.Sprintf("%s: %v", label, verr), Sample: sampleOf(row),
				})
				return nil
			}
			if val == nil {
				return nil
			}
			switch {
			case strings.Contains(label, "turnover"):
				turnover = val
			case strings.Contains(label, "shares"):
				shares = val
			case strings.Contains(label, "transactions"):
				transactions = val
			case strings.Contains(label, "scrips"):
				scrips = val
			}
		case "index_quote":
			if str(row, "index") != "NEPSE Index" {
				return nil
			}
			if v, err := marketdata.OptionalDecimal(pick(row, "currentValue", "close")); err == nil {
				indexValue = v
			}
			if v, err := marketdata.OptionalDecimal(pick(row, "change")); err == nil {
				pointChange = v
			}
			if v, err := marketdata.OptionalDecimal(pick(row, "perChange")); err == nil {
				percentChange = v
			}
			// Snapshot JSON carries NPT wall-clock times with no offset.
			if t, err := marketdata.Timestamp(pick(row, "generatedTime"), marketdata.BasisNPT); err == nil && t != nil {
				tradingDate = t.UTC().Format("2006-01-02")
			}
		}
		return nil
	})
	if err != nil {
		return 0, 0, 0, rejections, err
	}

	if tradingDate == "" {
		return 0, 0, 0, append(rejections, Rejection{
			Row: 0, Field: "trading_date",
			Reason: "the summary carries no trading date; a session cannot be invented",
		}), nil
	}

	tx, err := i.Pool.Begin(ctx)
	if err != nil {
		return 0, 0, 0, rejections, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx,
		`INSERT INTO trading_sessions (trading_date) VALUES ($1::date) ON CONFLICT DO NOTHING`,
		tradingDate); err != nil {
		return 0, 0, 0, rejections, err
	}

	var trxCount any
	if transactions != nil {
		if n, err := marketdata.Integer(*transactions); err == nil {
			trxCount = n
		}
	}
	var scripCount any
	if scrips != nil {
		if n, err := marketdata.Integer(*scrips); err == nil {
			scripCount = n
		}
	}

	// Advance/decline is derived from the stored bars for the same session, which
	// is a count over observed prices rather than a reported figure.
	var advanced, declined, flat int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE close > previous_close),
		       count(*) FILTER (WHERE close < previous_close),
		       count(*) FILTER (WHERE previous_close IS NULL OR close = previous_close)
		FROM daily_ohlcv
		WHERE trading_date = $1::date AND previous_close IS NOT NULL AND previous_close > 0`,
		tradingDate).Scan(&advanced, &declined, &flat); err != nil {
		return 0, 0, 0, rejections, err
	}

	tag, err := tx.Exec(ctx, `
		INSERT INTO market_summaries (trading_date, nepse_index, point_change, percent_change,
			turnover, transactions, traded_shares, traded_securities, advanced, declined, unchanged,
			data_source_id)
		VALUES ($1::date, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12::uuid)
		ON CONFLICT (trading_date) DO UPDATE SET
			nepse_index = COALESCE(EXCLUDED.nepse_index, market_summaries.nepse_index),
			point_change = COALESCE(EXCLUDED.point_change, market_summaries.point_change),
			percent_change = COALESCE(EXCLUDED.percent_change, market_summaries.percent_change),
			turnover = COALESCE(EXCLUDED.turnover, market_summaries.turnover),
			transactions = COALESCE(EXCLUDED.transactions, market_summaries.transactions),
			traded_shares = COALESCE(EXCLUDED.traded_shares, market_summaries.traded_shares),
			traded_securities = COALESCE(EXCLUDED.traded_securities, market_summaries.traded_securities),
			advanced = EXCLUDED.advanced, declined = EXCLUDED.declined, unchanged = EXCLUDED.unchanged,
			data_source_id = EXCLUDED.data_source_id,
			ingested_at = now()
		WHERE (market_summaries.nepse_index, market_summaries.point_change, market_summaries.percent_change,
		       market_summaries.turnover, market_summaries.transactions, market_summaries.traded_shares,
		       market_summaries.traded_securities, market_summaries.advanced,
		       market_summaries.declined, market_summaries.unchanged)
		      IS DISTINCT FROM
		      (COALESCE(EXCLUDED.nepse_index, market_summaries.nepse_index),
		       COALESCE(EXCLUDED.point_change, market_summaries.point_change),
		       COALESCE(EXCLUDED.percent_change, market_summaries.percent_change),
		       COALESCE(EXCLUDED.turnover, market_summaries.turnover),
		       COALESCE(EXCLUDED.transactions, market_summaries.transactions),
		       COALESCE(EXCLUDED.traded_shares, market_summaries.traded_shares),
		       COALESCE(EXCLUDED.traded_securities, market_summaries.traded_securities),
		       EXCLUDED.advanced, EXCLUDED.declined, EXCLUDED.unchanged)`,
		tradingDate, indexValue, pointChange, percentChange, turnover, trxCount, shares,
		scripCount, advanced, declined, flat, srcID)
	if err != nil {
		return 0, 0, 0, rejections, fmt.Errorf("merge summary: %w", err)
	}

	affected := int(tag.RowsAffected())
	if affected > 0 {
		written = affected
	} else {
		unchanged = 1
	}
	// The advance/decline split is computed, so it always counts as derived work.
	derived = 1

	if err := tx.Commit(ctx); err != nil {
		return 0, 0, 0, rejections, err
	}
	return written, unchanged, derived, rejections, nil
}

// summaryUsesScalarPath reports whether a stream bypasses the generic engine.
func summaryUsesScalarPath(stream string) bool {
	return stream == marketdata.StreamSummary
}

var _ = pgx.ErrNoRows
