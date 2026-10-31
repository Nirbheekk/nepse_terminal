// Command worker runs background jobs against the same database as the API.
//
// Roles:
//
//	promote    fold captured snapshots into PostgreSQL via the seeder
//	ingest     archive and ingest every readable source stream
//	reconcile  compare the legacy records with the platform and record the verdict
//	freshness  measure every dataset against its SLA and log the result
//	alerts     evaluate every enabled alert rule and drain the retry queue
//	all        ingest, then freshness
//
// Jobs run as a separate process with their own lifecycle, so a slow promotion
// cannot add latency to a request and the API can be scaled without duplicating
// work. `-once` runs a single cycle (used by CI and smoke tests).
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"nepse/terminal/api/internal/alerts"
	"nepse/terminal/api/internal/config"
	"nepse/terminal/api/internal/ingestion"
	"nepse/terminal/api/internal/jobs"
	"nepse/terminal/api/internal/marketdata"
	"nepse/terminal/api/internal/reconciliation"
	"nepse/terminal/api/internal/repositories"
)

func main() {
	cfg, _ := config.Load() // flags carry the same defaults; a bad config surfaces in run()

	var (
		role     = flag.String("role", "promote", "job role: promote, ingest, reconcile, freshness, alerts, all")
		interval = flag.Duration("interval", 5*time.Minute, "how often to run the job")
		once     = flag.Bool("once", false, "run one cycle and exit")
		trades   = flag.Int("trades", 0, "max floorsheet rows per promote cycle; 0 = reference only, -1 = all")
		dataDir  = flag.String("data", cfg.DataDir, "captured snapshot directory")
		export   = flag.String("export", cfg.ExportDir, "exports/legacy/<run> directory (default: newest)")
		sqlite   = flag.String("sqlite", cfg.LegacyDBPath, "legacy collector SQLite database")
		archDir  = flag.String("archive", cfg.ArchiveDir, "raw archive root")
	)
	flag.Parse()

	err := run(jobConfig{
		role: *role, interval: *interval, once: *once, trades: *trades,
		dataDir: *dataDir, exportDir: *export, sqlitePath: *sqlite, archiveDir: *archDir,
	})
	if err != nil {
		slog.Error("worker failed", "error", err)
		os.Exit(1)
	}
}

type jobConfig struct {
	role       string
	interval   time.Duration
	once       bool
	trades     int
	dataDir    string
	exportDir  string
	sqlitePath string
	archiveDir string
}

func run(jc jobConfig) error {
	switch jc.role {
	case "promote", "ingest", "reconcile", "freshness", "alerts", "all":
	default:
		return errors.New("unknown role " + jc.role +
			" (supported: promote, ingest, reconcile, freshness, alerts, all)")
	}
	if jc.interval < time.Second {
		return errors.New("interval must be at least 1s")
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// The worker does not migrate: schema changes are the API's responsibility,
	// so two processes can never race on the same migration.
	openCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	st, err := repositories.Open(openCtx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer st.Close()

	logger.Info("worker started", "role", jc.role, "interval", jc.interval.String(), "once", jc.once)

	cycle := func() {
		runCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
		defer cancel()
		if err := runOnce(runCtx, logger, st, cfg, jc); err != nil {
			// A failed cycle is logged and retried next tick: a transient database
			// error is not a reason to crash-loop the process.
			logger.Error("job cycle failed", "role", jc.role, "error", err)
		}
	}

	cycle()
	if jc.once {
		return nil
	}

	ticker := time.NewTicker(jc.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			logger.Info("worker stopped")
			return nil
		case <-ticker.C:
			cycle()
		}
	}
}

func runOnce(ctx context.Context, logger *slog.Logger, st *repositories.Store, cfg *config.Config, jc jobConfig) error {
	switch jc.role {
	case "promote":
		stats, err := jobs.Run(ctx, st, jobs.Options{
			DataDir:         jc.dataDir,
			ExportDir:       jc.exportDir,
			FloorsheetLimit: jc.trades,
			Logger:          func(msg string, args ...any) { logger.Info(msg, args...) },
		})
		if err != nil {
			return err
		}
		logger.Info("promotion cycle complete",
			"companies", stats.Companies, "bars", stats.Bars, "trades", stats.Trades,
			"summary_date", stats.SummaryDate, "warnings", len(stats.Warnings))
		return nil

	case "ingest", "all":
		exportDir := jc.exportDir
		if exportDir == "" {
			exportDir = jobs.LatestExport(jc.dataDir)
		}
		adapters, skipped := marketdata.Available(marketdata.Config{
			SQLitePath: jc.sqlitePath, DataDir: jc.dataDir, ExportDir: exportDir,
		})
		for _, s := range skipped {
			logger.Warn("adapter unavailable", "reason", s)
		}
		ing := &ingestion.Ingester{
			Pool:    st.Pool(),
			Archive: &ingestion.Store{Dir: jc.archiveDir, Pool: st.Pool()},
			Logger:  logger,
		}
		sum, err := ing.Run(ctx, adapters, ingestion.Options{Trigger: "worker"})
		if err != nil {
			return err
		}
		logger.Info("ingest cycle complete",
			"status", sum.Status, "streams", sum.Streams,
			"rows_written", sum.RowsWritten, "rejected", sum.RowsRejected, "skipped", sum.Skipped)
		if jc.role == "ingest" {
			return nil
		}
		return logFreshness(ctx, logger, st)
	}

	switch jc.role {
	case "reconcile":
		rep, err := reconciliation.Run(ctx, reconciliation.Options{
			LegacyPath: jc.sqlitePath, Pool: st.Pool(), DataDir: jc.dataDir, SampleSize: 1000,
		})
		if err != nil {
			return err
		}
		matches, mismatches, expected := rep.Totals()
		logger.Info("reconciliation complete",
			"status", rep.Status(), "matched", matches, "mismatched", mismatches,
			"expected", expected, "rows_compared", rep.RowsCompared,
			"field_mismatches", rep.FieldMismatches)
		if _, err := st.Pool().Exec(ctx, `
			INSERT INTO reconciliation_runs (status, legacy_summary, platform_summary, discrepancies)
			VALUES ($1, '{}'::jsonb, $2::jsonb, $3::jsonb)`,
			rep.Status(),
			jsonOf(map[string]any{"rows_compared": rep.RowsCompared, "rows_missing": rep.RowsMissing, "field_mismatches": rep.FieldMismatches}),
			jsonOf(rep.Discrepancies())); err != nil {
			logger.Warn("record reconciliation", "error", err)
		}
		return nil

	case "freshness":
		return logFreshness(ctx, logger, st)

	case "alerts":
		// One engine per cycle: the worker is the only process that evaluates
		// every tenant's rules, and it never widens what a rule can see.
		engine := &alerts.Engine{Store: st, Logger: logger}
		res, err := engine.RunOnce(ctx)
		if err != nil {
			return err
		}
		logger.Info("alert cycle complete",
			"rules", res.RulesEvaluated, "observations", res.Observations,
			"new_events", res.NewEvents, "duplicates", res.Duplicates,
			"suppressed", res.Suppressed, "deliveries", res.Deliveries,
			"retried", res.Retried, "failed", res.Failed,
			"unavailable_metrics", res.Unavailable, "errors", len(res.Errors))
		for _, msg := range res.Errors {
			logger.Warn("alert cycle error", "detail", msg)
		}
		return nil
	}
	return nil
}

func logFreshness(ctx context.Context, logger *slog.Logger, st *repositories.Store) error {
	datasets, summary, err := jobs.Evaluate(ctx, st.Pool())
	if err != nil {
		return err
	}
	logger.Info("freshness evaluated",
		"worst", summary.Worst, "fresh", summary.Fresh, "stale", summary.Stale, "unknown", summary.Unknown)
	for _, d := range datasets {
		if d.Status == "stale" {
			logger.Warn("dataset stale", "stream", d.Stream, "severity", d.Severity,
				"age_seconds", d.AgeSeconds, "sla_seconds", d.MaxAgeSeconds)
		}
	}
	return nil
}

func jsonOf(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "[]"
	}
	return string(b)
}
