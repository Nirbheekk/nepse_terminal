// Command api runs the HTTP service.
//
// Boot order is deliberate: connect and migrate before listening, so a probe
// never sees a process that is up but unable to serve. If any step fails the
// process exits non-zero instead of running degraded and silent.
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"nepse/terminal/api/internal/config"
	"nepse/terminal/api/internal/contracts"
	"nepse/terminal/api/internal/handlers"
	"nepse/terminal/api/internal/jobs"
	"nepse/terminal/api/internal/repositories"
)

func main() {
	migrateOnly := flag.Bool("migrate-only", false, "apply migrations and exit")
	flag.Parse()

	if err := run(*migrateOnly); err != nil {
		slog.Error("startup failed", "error", err)
		os.Exit(1)
	}
}

func run(migrateOnly bool) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger := newLogger(cfg.LogLevel)
	slog.SetDefault(logger)

	logger.Info("starting api",
		"version", contracts.Version,
		"env", cfg.Env,
		"addr", cfg.HTTPAddr,
	)

	// A bounded context for startup work keeps a hung database from blocking
	// the process forever.
	bootCtx, cancelBoot := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancelBoot()

	st, err := repositories.Open(bootCtx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer st.Close()

	schema, err := st.Migrate(bootCtx)
	if err != nil {
		return err
	}
	logger.Info("migrations applied", "applied", schema.Applied, "latest", schema.Latest)

	if migrateOnly {
		logger.Info("migrate-only: done")
		return nil
	}

	if cfg.SeedOnStart {
		if err := maybeSeed(bootCtx, st, logger); err != nil {
			// Seeding is a convenience for development. A failure is loud but
			// not fatal: an operator may be loading data another way.
			logger.Error("seed failed", "error", err)
		}
	}

	srv, err := handlers.New(cfg, st, logger, schema)
	if err != nil {
		return err
	}

	httpServer := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       90 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		logger.Info("listening", "addr", cfg.HTTPAddr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		logger.Info("shutting down")
	}

	// Give in-flight requests a bounded window to finish.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		return err
	}
	logger.Info("stopped")
	return nil
}

// maybeSeed loads the captured exchange data only while the database has no
// companies, so restarts never overwrite live rows.
func maybeSeed(ctx context.Context, st *repositories.Store, logger *slog.Logger) error {
	health, err := st.DataHealth(ctx)
	if err != nil {
		return err
	}
	if health.Companies > 0 {
		logger.Info("seed skipped, database already populated", "companies", health.Companies)
		return nil
	}

	stats, err := jobs.Run(ctx, st, jobs.Options{
		DataDir:         seedDataDir(),
		ExportDir:       seedExportDir(),
		FloorsheetLimit: seedTradeLimit(),
		Logger: func(msg string, args ...any) {
			logger.Info("seed: "+msg, args...)
		},
	})
	if err != nil {
		return err
	}
	logger.Info("seed complete",
		"companies", stats.Companies,
		"securities", stats.Securities,
		"bars", stats.Bars,
		"trades", stats.Trades,
		"indices", stats.Indices,
		"summary_date", stats.SummaryDate,
		"warnings", len(stats.Warnings),
	)
	return nil
}

func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		lvl = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl}))
}

func seedDataDir() string   { return envOr("SEED_DATA_DIR", "../data") }
func seedExportDir() string { return os.Getenv("SEED_EXPORT_DIR") }

func seedTradeLimit() int {
	if os.Getenv("SEED_FLOORSHEET") == "none" {
		return 0
	}
	return -1 // every row available in the export
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
