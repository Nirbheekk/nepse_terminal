package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// SnapshotAdapter reads the JSON files the collector writes into data/.
//
// These are point-in-time captures, not a log: each run overwrites the previous
// one. That is precisely why the archive step exists — the platform keeps the
// bytes so a later session cannot erase what an earlier one observed.
type SnapshotAdapter struct {
	Dir string
}

func (a *SnapshotAdapter) Slug() string { return "legacy-json" }

func (a *SnapshotAdapter) Authorization() Authorization {
	return Authorization{
		Mode:           ModeOperatorCaptured,
		LicenceClass:   LicenceUnverified,
		Redistribution: RedistributeNo,
		Note:           "Snapshot files written by the collector; overwritten each session, archived on read.",
	}
}

func (a *SnapshotAdapter) Streams() []string {
	return []string{StreamCompanies, StreamSecurities, StreamQuotes, StreamSummary, StreamIndices, StreamMarketStatus}
}

func (a *SnapshotAdapter) Each(ctx context.Context, stream string, fn func(map[string]any) error) (Batch, error) {
	batch := Batch{
		Stream:     stream,
		CapturedAt: time.Now().UTC(),
		Meta:       map[string]any{"adapter": a.Slug(), "data_dir": a.Dir},
	}

	switch stream {
	case StreamCompanies:
		rows, captured, err := a.readArray("company_list.json")
		if err != nil {
			return batch, err
		}
		batch.CapturedAt = captured
		for _, r := range rows {
			if err := fn(r); err != nil {
				return batch, err
			}
			batch.Rows++
			if s, ok := r["symbol"].(string); ok {
				batch.Cursor = s
			}
		}

	case StreamSecurities:
		rows, captured, err := a.readArray("security_list.json")
		if err != nil {
			return batch, err
		}
		batch.CapturedAt = captured
		for _, r := range rows {
			if err := fn(r); err != nil {
				return batch, err
			}
			batch.Rows++
			if s, ok := r["symbol"].(string); ok {
				batch.Cursor = s
			}
		}

	case StreamQuotes:
		files, err := filepath.Glob(filepath.Join(a.Dir, "company_details", "*.json"))
		if err != nil {
			return batch, err
		}
		for _, f := range files {
			base := strings.TrimSuffix(filepath.Base(f), ".json")
			if strings.HasSuffix(base, "_extras") {
				continue
			}
			row, err := readQuoteFile(f, base)
			if err != nil {
				// One unreadable snapshot is a gap in that symbol's history, not a
				// reason to abandon the other 424.
				continue
			}
			if err := fn(row); err != nil {
				return batch, err
			}
			batch.Rows++
			batch.Cursor = strings.ToUpper(base)
			if ts, ok := row["captured_at"].(string); ok && ts > "" {
				if t, err := time.Parse(time.RFC3339, ts); err == nil && t.After(batch.CapturedAt) {
					batch.CapturedAt = t
				}
			}
		}

	case StreamSummary:
		items, captured, err := a.readArray("summary.json")
		if err != nil {
			return batch, err
		}
		batch.CapturedAt = captured
		for _, it := range items {
			row := map[string]any{"kind": "summary_item"}
			for k, v := range it {
				row[k] = v
			}
			if err := fn(row); err != nil {
				return batch, err
			}
			batch.Rows++
		}
		quotes, _, err := a.readArray("nepse_index.json")
		if err != nil {
			return batch, err
		}
		for _, q := range quotes {
			row := map[string]any{"kind": "index_quote"}
			for k, v := range q {
				row[k] = v
			}
			if err := fn(row); err != nil {
				return batch, err
			}
			batch.Rows++
		}

	case StreamIndices:
		headline, captured, err := a.readArray("nepse_index.json")
		if err != nil {
			return batch, err
		}
		batch.CapturedAt = captured
		// sub_indices.json carries values only, no timestamp. The sector rows were
		// captured by the same request as the headline indices, so the headline's
		// generated time is the correct session for them; without it every sector
		// row would have to be discarded for want of a date.
		var headlineTime any
		for _, q := range headline {
			row := map[string]any{"kind": "headline"}
			for k, v := range q {
				row[k] = v
			}
			if headlineTime == nil {
				headlineTime = q["generatedTime"]
			}
			if err := fn(row); err != nil {
				return batch, err
			}
			batch.Rows++
			if s, ok := q["index"].(string); ok {
				batch.Cursor = s
			}
		}
		subs, _, err := a.readArray("sub_indices.json")
		if err != nil {
			return batch, err
		}
		for _, q := range subs {
			row := map[string]any{"kind": "sector"}
			for k, v := range q {
				row[k] = v
			}
			if _, ok := row["generatedTime"]; !ok && headlineTime != nil {
				row["generatedTime"] = headlineTime
				row["date_inherited_from"] = "nepse_index.json"
			}
			if err := fn(row); err != nil {
				return batch, err
			}
			batch.Rows++
		}

	case StreamMarketStatus:
		// A single object rather than an array: the collector overwrites it as the
		// state changes, so each read is one observation. The row carries the
		// file's mtime so two reads of the same state still get distinct
		// observation times.
		row, captured, err := a.readObject("market_status.json")
		if err != nil {
			return batch, err
		}
		row["captured_at"] = captured.UTC().Format(time.RFC3339)
		batch.CapturedAt = captured
		if err := fn(row); err != nil {
			return batch, err
		}
		batch.Rows = 1
		batch.Cursor = fmt.Sprint(row["asOf"])

	default:
		return batch, fmt.Errorf("snapshot adapter does not provide stream %q", stream)
	}

	return batch, nil
}

func (a *SnapshotAdapter) readObject(name string) (map[string]any, time.Time, error) {
	path := filepath.Join(a.Dir, name)
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("read %s: %w", name, err)
	}
	var row map[string]any
	if err := json.Unmarshal(body, &row); err != nil {
		return nil, time.Time{}, fmt.Errorf("parse %s: %w", name, err)
	}
	captured := time.Now().UTC()
	if st, err := os.Stat(path); err == nil {
		captured = st.ModTime().UTC()
	}
	return row, captured, nil
}

func (a *SnapshotAdapter) readArray(name string) ([]map[string]any, time.Time, error) {
	path := filepath.Join(a.Dir, name)
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("read %s: %w", name, err)
	}
	var rows []map[string]any
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, time.Time{}, fmt.Errorf("parse %s: %w", name, err)
	}
	captured := time.Now().UTC()
	if st, err := os.Stat(path); err == nil {
		captured = st.ModTime().UTC()
	}
	return rows, captured, nil
}

// readQuoteFile flattens one company detail document into a single row. The
// nested shape is the exchange's, not ours; flattening here keeps the normaliser
// from having to know the nesting.
func readQuoteFile(path, symbol string) (map[string]any, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc struct {
		Symbol  string `json:"symbol"`
		Details struct {
			Quote struct {
				BusinessDate       any `json:"businessDate"`
				OpenPrice          any `json:"openPrice"`
				HighPrice          any `json:"highPrice"`
				LowPrice           any `json:"lowPrice"`
				LastTradedPrice    any `json:"lastTradedPrice"`
				PreviousClose      any `json:"previousClose"`
				TotalTradeQuantity any `json:"totalTradeQuantity"`
				TotalTrades        any `json:"totalTrades"`
			} `json:"securityDailyTradeDto"`
			Security struct {
				ISIN string `json:"isin"`
			} `json:"security"`
			ListedShares *float64 `json:"stockListedShares"`
			PublicShares *float64 `json:"publicShares"`
			MarketCap    *float64 `json:"marketCapitalization"`
		} `json:"details"`
		FetchedAt string `json:"fetched_at"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, err
	}

	q := doc.Details.Quote
	row := map[string]any{
		"symbol":          strings.ToUpper(strings.TrimSpace(firstNonEmpty(doc.Symbol, symbol))),
		"business_date":   q.BusinessDate,
		"open":            q.OpenPrice,
		"high":            q.HighPrice,
		"low":             q.LowPrice,
		"ltp":             q.LastTradedPrice,
		"previous_close":  q.PreviousClose,
		"volume":          q.TotalTradeQuantity,
		"trades":          q.TotalTrades,
		"isin":            strings.TrimSpace(doc.Details.Security.ISIN),
		"listed_shares":   doc.Details.ListedShares,
		"public_shares":   doc.Details.PublicShares,
		"market_cap":      doc.Details.MarketCap,
		"captured_at":     doc.FetchedAt,
		"snapshot_source": filepath.Base(path),
	}
	return row, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
