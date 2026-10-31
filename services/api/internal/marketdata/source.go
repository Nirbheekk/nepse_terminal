// Package source defines the adapters that read market data into the platform.
//
// # Authorization
//
// Every adapter states how it is allowed to obtain its data. None of them talk
// to the exchange: the legacy collector is the single exchange-facing process,
// and duplicating that polling here would double the request rate against a
// source that already rate-limits, while breaking the one-writer rule the
// architecture depends on. The platform therefore ingests what the collector
// captured, which is an operator-captured copy under the same unlicensed terms
// as the original.
//
// A licensed feed adapter would be a different thing: it would be allowed to
// redistribute, and it would need an entitlement check before any row left the
// database. That adapter does not exist yet because the licence does not.
package marketdata

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Authorization describes the legal and operational basis for reading a source.
type Authorization struct {
	// Mode is how the bytes were obtained.
	Mode string `json:"mode"`
	// LicenceClass mirrors data_sources.licence_class.
	LicenceClass string `json:"licence_class"`
	// Redistribution is what may be done with derived output.
	Redistribution string `json:"redistribution"`
	// Note is shown to operators so the constraint is not folklore.
	Note string `json:"note"`
}

const (
	ModeOperatorCaptured = "operator_captured"
	LicenceUnverified    = "public_unverified"
	RedistributeNo       = "prohibited"
)

// Batch summarises the stream an adapter just walked.
type Batch struct {
	Stream     string
	Rows       int
	Cursor     string
	CapturedAt time.Time
	Meta       map[string]any
}

// Adapter reads one or more named streams.
//
// Each streams rows to fn so a table with hundreds of thousands of rows never
// has to be materialised in memory; the caller decides whether to archive,
// normalise, or both.
type Adapter interface {
	// Slug is the stable identifier recorded in data_sources.
	Slug() string
	// Authorization states the basis for reading.
	Authorization() Authorization
	// Streams lists the stream names this adapter can serve.
	Streams() []string
	// Each walks every row of a stream as a map keyed by source column name.
	Each(ctx context.Context, stream string, fn func(map[string]any) error) (Batch, error)
}

// Stream names are shared vocabulary between adapters, checkpoints, SLA rows and
// reports. Adding a stream means adding it here, to dataset_slas, and to the
// ingest registry.
const (
	StreamCompanies    = "companies"
	StreamSecurities   = "securities"
	StreamQuotes       = "quotes"
	StreamSummary      = "summary"
	StreamMarketCap    = "market_cap"
	StreamIndices      = "indices"
	StreamFloorsheet   = "floorsheet"
	StreamMarketStatus = "market_status"
)

// AllStreams is the canonical order used in reports.
func AllStreams() []string {
	return []string{
		StreamCompanies, StreamSecurities, StreamQuotes,
		StreamSummary, StreamMarketCap, StreamIndices, StreamFloorsheet,
		StreamMarketStatus,
	}
}

// Config locates the inputs. Missing inputs disable the adapter that needs them
// rather than failing the whole run.
type Config struct {
	SQLitePath string // legacy nepse.db
	DataDir    string // captured JSON snapshots
	ExportDir  string // exports/legacy/<run> with JSONL tables
}

// Available returns every adapter that can actually read its inputs, in a stable
// order, plus a note for each adapter that was skipped.
func Available(cfg Config) ([]Adapter, []string) {
	var out []Adapter
	var skipped []string

	if cfg.SQLitePath != "" {
		if _, err := os.Stat(cfg.SQLitePath); err == nil {
			out = append(out, &SQLiteAdapter{Path: cfg.SQLitePath})
		} else {
			skipped = append(skipped, fmt.Sprintf("sqlite adapter disabled: %s not found", cfg.SQLitePath))
		}
	}

	if cfg.DataDir != "" {
		if _, err := os.Stat(filepath.Join(cfg.DataDir, "company_list.json")); err == nil {
			out = append(out, &SnapshotAdapter{Dir: cfg.DataDir})
		} else {
			skipped = append(skipped, fmt.Sprintf("snapshot adapter disabled: no company_list.json in %s", cfg.DataDir))
		}
	}

	if cfg.ExportDir != "" {
		if _, err := os.Stat(filepath.Join(cfg.ExportDir, "manifest.json")); err == nil {
			out = append(out, &ExportAdapter{Dir: cfg.ExportDir})
		} else {
			skipped = append(skipped, fmt.Sprintf("export adapter disabled: no manifest.json in %s", cfg.ExportDir))
		}
	}

	sort.SliceStable(out, func(i, j int) bool { return out[i].Slug() < out[j].Slug() })
	return out, skipped
}
