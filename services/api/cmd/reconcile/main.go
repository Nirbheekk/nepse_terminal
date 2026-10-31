// Command reconcile compares the legacy collector's records with the platform's
// and writes the coverage and discrepancy reports.
//
// It exits non-zero when a real discrepancy remains, so it can be used as a
// post-migration gate rather than a report nobody reads.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"nepse/terminal/api/internal/config"
	"nepse/terminal/api/internal/jobs"
	"nepse/terminal/api/internal/reconciliation"
	"nepse/terminal/api/internal/repositories"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "configuration:", err)
		os.Exit(2)
	}

	sqlitePath := flag.String("sqlite", cfg.LegacyDBPath, "legacy collector SQLite database")
	dataDir := flag.String("data", cfg.DataDir, "captured JSON snapshot directory")
	reportDir := flag.String("out", cfg.ReportDir, "directory to write the markdown reports into")
	sample := flag.Int("sample", 5000, "number of contracts to compare field by field")
	soft := flag.Bool("soft", false, "exit 0 even when discrepancies remain")
	jsonOut := flag.Bool("json", false, "print the raw report as JSON instead of writing markdown")
	flag.Parse()

	if err := run(*sqlitePath, *dataDir, *reportDir, *sample, *soft, *jsonOut); err != nil {
		slog.Error("reconcile failed", "error", err)
		os.Exit(1)
	}
}

func run(sqlitePath, dataDir, reportDir string, sample int, soft, jsonOut bool) error {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	st, err := repositories.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer st.Close()

	if _, err := st.Migrate(ctx); err != nil {
		return err
	}

	rep, err := reconciliation.Run(ctx, reconciliation.Options{
		LegacyPath: sqlitePath,
		Pool:       st.Pool(),
		DataDir:    dataDir,
		SampleSize: sample,
	})
	if err != nil {
		return err
	}

	coverage, err := reconciliation.CollectCoverage(ctx, st.Pool())
	if err != nil {
		return err
	}
	fresh, freshSummary, err := jobs.Evaluate(ctx, st.Pool())
	if err != nil {
		return err
	}

	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]any{
			"report":    rep,
			"coverage":  coverage,
			"freshness": map[string]any{"datasets": fresh, "summary": freshSummary},
		})
	}

	if err := os.MkdirAll(reportDir, 0o755); err != nil {
		return err
	}
	coverageDoc := reconciliation.CoverageMarkdown(rep, coverage, fresh, freshSummary)
	discrepancyDoc := reconciliation.DiscrepancyMarkdown(rep)

	if err := os.WriteFile(filepath.Join(reportDir, "COVERAGE_REPORT.md"), []byte(coverageDoc), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(reportDir, "DISCREPANCY_REPORT.md"), []byte(discrepancyDoc), 0o644); err != nil {
		return err
	}

	// Keep a machine-readable copy of every run so drift can be diffed later.
	archiveDir := filepath.Join("var", "reports")
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(map[string]any{
		"generated_at": time.Now().UTC(),
		"report":       rep,
		"coverage":     coverage,
		"freshness":    map[string]any{"datasets": fresh, "summary": freshSummary},
	}, "", "  ")
	if err != nil {
		return err
	}
	stamped := filepath.Join(archiveDir, "reconciliation-"+time.Now().UTC().Format("20060102T150405Z")+".json")
	if err := os.WriteFile(stamped, payload, 0o644); err != nil {
		return err
	}

	// Persist the verdict alongside the data it describes.
	stored, _ := json.Marshal(rep.Discrepancies())
	if _, err := st.Pool().Exec(ctx, `
		INSERT INTO reconciliation_runs (status, legacy_summary, platform_summary, discrepancies)
		VALUES ($1, $2::jsonb, $3::jsonb, $4::jsonb)`,
		rep.Status(),
		mustJSON(map[string]any{"datasets": rep.Datasets, "ledger": rep.Ledger()}),
		mustJSON(map[string]any{
			"rows_compared":    rep.RowsCompared,
			"rows_missing":     rep.RowsMissing,
			"field_mismatches": rep.FieldMismatches,
		}),
		string(stored)); err != nil {
		return err
	}

	matches, mismatches, expected := rep.Totals()
	fmt.Printf("reconciliation: %s\n", rep.Status())
	fmt.Printf("  checks:      %d matched, %d mismatched, %d expected difference(s)\n", matches, mismatches, expected)
	fmt.Printf("  contracts:   %d compared, %d missing, %d field mismatches\n",
		rep.RowsCompared, rep.RowsMissing, rep.FieldMismatches)
	fmt.Printf("  reports:     %s, %s\n",
		filepath.Join(reportDir, "COVERAGE_REPORT.md"),
		filepath.Join(reportDir, "DISCREPANCY_REPORT.md"))
	fmt.Printf("  snapshot:    %s\n", stamped)

	for _, c := range rep.Checks {
		if !c.Match && !c.Expected {
			fmt.Printf("  MISMATCH %s/%s: legacy=%s platform=%s\n", c.Dataset, c.Field, c.Legacy, c.Platform)
		}
	}
	if len(rep.MismatchSamples) > 0 {
		fmt.Println("  sample differences:")
		for _, m := range rep.MismatchSamples[:min(5, len(rep.MismatchSamples))] {
			fmt.Printf("    %s %s: legacy=%s platform=%s\n", m.ContractID, m.Field, m.Legacy, m.Platform)
		}
	}

	if rep.Status() != "match" && !soft {
		return fmt.Errorf("discrepancies remain; see %s", filepath.Join(reportDir, "DISCREPANCY_REPORT.md"))
	}
	return nil
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
