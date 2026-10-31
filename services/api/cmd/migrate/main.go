// Command migrate applies the embedded SQL migration set and exits.
//
// It exists so schema changes can be applied by a deliberate CI or deploy step
// without also starting an HTTP listener. The server keeps its own
// -migrate-only flag for local convenience; this binary is the one a pipeline
// should call.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"nepse/terminal/api/internal/config"
	"nepse/terminal/api/internal/repositories"
)

func main() {
	timeout := flag.Duration("timeout", 2*time.Minute, "overall migration timeout")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	if err := run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	st, err := repositories.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer st.Close()

	status, err := st.Migrate(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("applied=%d latest=%s\n", status.Applied, status.Latest)
	return nil
}
