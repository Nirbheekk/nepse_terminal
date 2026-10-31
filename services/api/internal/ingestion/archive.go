// Package archive persists the exact bytes read from a source before they are
// normalised, and records their digest.
//
// This is the difference between "we think we imported 48,000 trades" and "we
// imported the file with this SHA-256, and here it is". Without it, a disputed
// figure can never be traced back to what the exchange actually said.
package ingestion

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"nepse/terminal/api/internal/marketdata"
)

// Store writes archives under Dir and indexes them in raw_archives.
type Store struct {
	Dir  string
	Pool *pgxpool.Pool
}

// Ref identifies one archived batch.
type Ref struct {
	ID     string
	Path   string
	SHA256 string
	Rows   int
	Bytes  int64
	Reused bool
}

// Write walks the adapter's stream, writes a gzip JSONL file and records it.
//
// The file is written to a temporary name and renamed only after it is complete,
// so an interrupted run cannot leave a truncated archive that looks valid.
func (s *Store) Write(ctx context.Context, adapter marketdata.Adapter, stream string) (Ref, error) {
	if s.Dir == "" {
		return Ref{}, fmt.Errorf("archive directory is not configured")
	}

	captured := time.Now().UTC()
	tmpDir := filepath.Join(s.Dir, "incoming")
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return Ref{}, err
	}
	tmp, err := os.CreateTemp(tmpDir, "batch-*.jsonl.gz")
	if err != nil {
		return Ref{}, err
	}
	tmpName := tmp.Name()
	cleanup := func() {
		tmp.Close()
		os.Remove(tmpName)
	}

	hasher := sha256.New()
	counter := &countingWriter{w: io.MultiWriter(tmp, hasher)}
	gz := gzip.NewWriter(counter)
	enc := json.NewEncoder(gz)

	rows := 0
	batch, err := adapter.Each(ctx, stream, func(row map[string]any) error {
		if err := enc.Encode(row); err != nil {
			return err
		}
		rows++
		return nil
	})
	if err != nil {
		gz.Close()
		cleanup()
		return Ref{}, fmt.Errorf("read %s/%s: %w", adapter.Slug(), stream, err)
	}
	if batch.CapturedAt.IsZero() {
		batch.CapturedAt = captured
	}

	if err := gz.Close(); err != nil {
		cleanup()
		return Ref{}, err
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return Ref{}, err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return Ref{}, err
	}
	if rows != batch.Rows {
		os.Remove(tmpName)
		return Ref{}, fmt.Errorf("%s/%s: adapter reported %d rows but emitted %d",
			adapter.Slug(), stream, batch.Rows, rows)
	}

	digest := hex.EncodeToString(hasher.Sum(nil))
	dir := filepath.Join(s.Dir, adapter.Slug(), stream)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		os.Remove(tmpName)
		return Ref{}, err
	}
	final := filepath.Join(dir, fmt.Sprintf("%s-%s.jsonl.gz",
		batch.CapturedAt.UTC().Format("20060102T150405Z"), digest[:12]))

	info, statErr := os.Stat(final)
	reused := false
	if statErr == nil {
		// Identical bytes were archived before: keep the existing file.
		os.Remove(tmpName)
		reused = true
	} else if err := os.Rename(tmpName, final); err != nil {
		os.Remove(tmpName)
		return Ref{}, err
	} else if info, err = os.Stat(final); err != nil {
		return Ref{}, err
	}

	size := info.Size()
	id, dbReused, err := s.record(ctx, adapter, stream, final, digest, size, int64(rows), batch, captured)
	if err != nil {
		return Ref{}, err
	}

	return Ref{
		ID:     id,
		Path:   final,
		SHA256: digest,
		Rows:   rows,
		Bytes:  size,
		Reused: reused || dbReused,
	}, nil
}

func (s *Store) record(ctx context.Context, adapter marketdata.Adapter, stream, path, digest string,
	size, rows int64, batch marketdata.Batch, captured time.Time) (string, bool, error) {

	auth := adapter.Authorization()
	meta, err := json.Marshal(map[string]any{
		"adapter":       adapter.Slug(),
		"authorization": auth,
		"batch":         batch.Meta,
		"cursor":        batch.Cursor,
	})
	if err != nil {
		return "", false, err
	}

	var id string
	err = s.Pool.QueryRow(ctx, `
		INSERT INTO raw_archives
			(source_slug, stream, path, sha256, bytes, rows, captured_at, licence_class, meta)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb)
		ON CONFLICT (stream, sha256) DO NOTHING
		RETURNING id::text`,
		adapter.Slug(), stream, path, digest, size, rows, captured, auth.LicenceClass, string(meta)).
		Scan(&id)
	if err == nil {
		return id, false, nil
	}
	// Conflict: the same bytes are already indexed. Return the existing row.
	if err := s.Pool.QueryRow(ctx,
		`SELECT id::text FROM raw_archives WHERE stream = $1 AND sha256 = $2`, stream, digest).
		Scan(&id); err != nil {
		return "", false, fmt.Errorf("record archive: %w", err)
	}
	return id, true, nil
}

// Open returns a reader over an archived batch, which is what ingestion replays.
func Open(path string) (io.ReadCloser, *gzip.Reader, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	gz, err := gzip.NewReader(f)
	if err != nil {
		f.Close()
		return nil, nil, err
	}
	return f, gz, nil
}

// EachRow replays an archive file, handing every decoded row to fn.
func EachRow(path string, fn func(map[string]any) error) (int, error) {
	f, gz, err := Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	defer gz.Close()

	dec := json.NewDecoder(gz)
	n := 0
	for {
		var row map[string]any
		if err := dec.Decode(&row); err == io.EOF {
			break
		} else if err != nil {
			return n, err
		}
		if err := fn(row); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

type countingWriter struct {
	w io.Writer
	n int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}
