// Command seed loads captured exchange data into PostgreSQL and reports what it
// wrote. It is safe to run repeatedly: reference rows are upserted, bars are
// keyed by (security, date) and trades by contract id.
//
// Usage:
//
//	go run ./cmd/seed -data ../data
//	go run ./cmd/seed -data ../data -export ../exports/legacy/20261029T084007Z -trades 5000
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"time"

	"nepse/terminal/api/internal/config"
	"nepse/terminal/api/internal/jobs"
	"nepse/terminal/api/internal/repositories"
)

func main() {
	var (
		dataDir   = flag.String("data", envOr("SEED_DATA_DIR", "../data"), "directory with the captured JSON snapshots")
		exportDir = flag.String("export", os.Getenv("SEED_EXPORT_DIR"), "exports/legacy/<run> directory (default: newest found)")
		trades    = flag.Int("trades", -1, "maximum floorsheet rows to load; -1 = all, 0 = none")
	)
	flag.Parse()

	if err := run(*dataDir, *exportDir, *trades); err != nil {
		slog.Error("seed failed", "error", err)
		os.Exit(1)
	}
}

func run(dataDir, exportDir string, trades int) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	st, err := repositories.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer st.Close()

	if _, err := st.Migrate(ctx); err != nil {
		return err
	}

	started := time.Now()
	stats, err := jobs.Run(ctx, st, jobs.Options{
		DataDir:         dataDir,
		ExportDir:       exportDir,
		FloorsheetLimit: trades,
		Logger:          func(msg string, args ...any) { logger.Info(msg, args...) },
	})
	if err != nil {
		return err
	}

	logger.Info("seed complete",
		"duration_ms", time.Since(started).Milliseconds(),
		"companies", stats.Companies,
		"securities", stats.Securities,
		"sectors", stats.Sectors,
		"sessions", stats.Sessions,
		"bars", stats.Bars,
		"trades", stats.Trades,
		"indices", stats.Indices,
		"observations", stats.Observations,
		"summary_date", stats.SummaryDate,
		"market_cap", stats.MarketCap,
		"warnings", stats.Warnings,
	)
	return nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
