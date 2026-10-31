// Command ingest archives every readable source stream and folds it into
// PostgreSQL.
//
// Usage:
//
//	go run ./cmd/ingest                          # everything, skip unchanged bytes
//	go run ./cmd/ingest -stream floorsheet       # one stream
//	go run ./cmd/ingest -source legacy-sqlite    # one adapter
//	go run ./cmd/ingest -force                   # re-apply even if unchanged
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"nepse/terminal/api/internal/config"
	"nepse/terminal/api/internal/ingestion"
	"nepse/terminal/api/internal/jobs"
	"nepse/terminal/api/internal/marketdata"
	"nepse/terminal/api/internal/repositories"
)

type stringList []string

func (s *stringList) String() string     { return strings.Join(*s, ",") }
func (s *stringList) Set(v string) error { *s = append(*s, v); return nil }

func main() {
	var (
		streams stringList
		sources stringList
	)
	cfg, err := config.Load()
	if err != nil {
		// Paths still need defaults even when the database is unreachable, so the
		// flags are parsed after a best-effort load.
		fmt.Fprintln(os.Stderr, "configuration:", err)
		os.Exit(2)
	}

	sqlitePath := flag.String("sqlite", cfg.LegacyDBPath, "legacy collector SQLite database")
	dataDir := flag.String("data", cfg.DataDir, "captured JSON snapshot directory")
	exportDir := flag.String("export", cfg.ExportDir, "exports/legacy/<run> directory (default: newest)")
	archiveDir := flag.String("archive", cfg.ArchiveDir, "raw archive root")
	force := flag.Bool("force", false, "re-ingest even when the archive digest is unchanged")
	asJSON := flag.Bool("json", false, "print the run summary as JSON")
	trigger := flag.String("trigger", "cli", "label recorded on the collection job")
	flag.Var(&streams, "stream", "limit to a stream (repeatable)")
	flag.Var(&sources, "source", "limit to an adapter slug (repeatable)")
	flag.Parse()

	if err := run(*sqlitePath, *dataDir, *exportDir, *archiveDir, *trigger, streams, sources, *force, *asJSON); err != nil {
		slog.Error("ingest failed", "error", err)
		os.Exit(1)
	}
}

func run(sqlitePath, dataDir, exportDir, archiveDir, trigger string, streams, sources []string, force, asJSON bool) error {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	st, err := repositories.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer st.Close()

	if _, err := st.Migrate(ctx); err != nil {
		return err
	}

	if exportDir == "" {
		exportDir = jobs.LatestExport(dataDir)
	}
	adapters, skipped := marketdata.Available(marketdata.Config{
		SQLitePath: sqlitePath,
		DataDir:    dataDir,
		ExportDir:  exportDir,
	})
	for _, s := range skipped {
		logger.Warn("adapter unavailable", "reason", s)
	}
	if len(adapters) == 0 {
		return fmt.Errorf("no source adapters available (looked for %s, %s, %s)", sqlitePath, dataDir, exportDir)
	}

	ing := &ingestion.Ingester{
		Pool:    st.Pool(),
		Archive: &ingestion.Store{Dir: archiveDir, Pool: st.Pool()},
		Logger:  logger,
	}

	sum, err := ing.Run(ctx, adapters, ingestion.Options{
		Streams: streams,
		Sources: sources,
		Force:   force,
		Trigger: trigger,
	})
	if err != nil {
		return err
	}

	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(sum)
	}

	fmt.Printf("job %s  status=%s  %d stream(s)  %d rows written  %d rejected\n\n",
		sum.JobID, sum.Status, sum.Streams, sum.RowsWritten, sum.RowsRejected)
	fmt.Printf("%-14s %-16s %8s %8s %8s %8s %8s  %s\n",
		"SOURCE", "STREAM", "READ", "WRITTEN", "SAME", "REJECT", "DERIVED", "STATUS")
	for _, r := range sum.Results {
		fmt.Printf("%-14s %-16s %8d %8d %8d %8d %8d  %s",
			r.Source, r.Stream, r.RowsRead, r.RowsWritten, r.RowsUnchanged,
			r.RowsRejected, r.RowsDerived, r.Status)
		if r.Skipped {
			fmt.Printf(" (archive unchanged)")
		}
		if r.Error != "" {
			fmt.Printf(" — %s", r.Error)
		}
		fmt.Println()
		for _, rej := range r.Rejections {
			fmt.Printf("    rejected row %d: %s\n", rej.Row, rej.Reason)
		}
	}
	fmt.Printf("\narchives: %s\n", archiveDir)
	return nil
}
