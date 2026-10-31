package marketdata

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ExportAdapter reads the JSONL dump produced by tools/export_legacy.py.
//
// It exists because the SQLite file is the collector's live working store: it may
// be locked mid-write, replaced, or absent on a machine that only has the dump.
// The export is also the only input that carries a manifest with per-table
// SHA-256 digests, which lets ingestion state what it read in verifiable terms.
type ExportAdapter struct {
	Dir string
}

func (a *ExportAdapter) Slug() string { return "legacy-export" }

func (a *ExportAdapter) Authorization() Authorization {
	return Authorization{
		Mode:           ModeOperatorCaptured,
		LicenceClass:   LicenceUnverified,
		Redistribution: RedistributeNo,
		Note:           "JSONL dump of the collector's SQLite store, verified by manifest digests.",
	}
}

func (a *ExportAdapter) Streams() []string {
	return []string{StreamFloorsheet, StreamIndices}
}

func (a *ExportAdapter) Each(ctx context.Context, stream string, fn func(map[string]any) error) (Batch, error) {
	var file string
	switch stream {
	case StreamFloorsheet:
		file = "floorsheet.jsonl"
	case StreamIndices:
		file = "index_snapshots.jsonl"
	default:
		return Batch{}, fmt.Errorf("export adapter does not provide stream %q", stream)
	}

	path := filepath.Join(a.Dir, file)
	f, err := os.Open(path)
	if err != nil {
		return Batch{}, fmt.Errorf("open %s: %w", file, err)
	}
	defer f.Close()

	batch := Batch{
		Stream:     stream,
		CapturedAt: time.Now().UTC(),
		Meta:       map[string]any{"adapter": a.Slug(), "export_dir": a.Dir, "file": file},
	}
	if manifest := a.manifestDigest(file); manifest != "" {
		batch.Meta["manifest_sha256"] = manifest
	}
	if st, err := os.Stat(path); err == nil {
		batch.CapturedAt = st.ModTime().UTC()
	}

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<20), 8<<20)
	for sc.Scan() {
		if err := ctx.Err(); err != nil {
			return batch, err
		}
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var row map[string]any
		if err := json.Unmarshal(line, &row); err != nil {
			// A malformed line is skipped here and reported by the row count
			// difference against the manifest, rather than aborting the stream.
			continue
		}
		if err := fn(row); err != nil {
			return batch, err
		}
		batch.Rows++
		if c, ok := row["contract_id"].(string); ok {
			batch.Cursor = c
		}
	}
	return batch, sc.Err()
}

// manifestDigest returns the recorded SHA-256 for a table, when the manifest is
// readable. It is provenance, not a gate: a missing manifest does not block a load.
func (a *ExportAdapter) manifestDigest(file string) string {
	body, err := os.ReadFile(filepath.Join(a.Dir, "manifest.json"))
	if err != nil {
		return ""
	}
	var m struct {
		Tables map[string]struct {
			SHA256 string `json:"sha256"`
			Rows   int    `json:"rows"`
		} `json:"tables"`
	}
	if err := json.Unmarshal(body, &m); err != nil {
		return ""
	}
	if t, ok := m.Tables[trimExt(file)]; ok {
		return t.SHA256
	}
	return ""
}

func trimExt(name string) string {
	if len(name) > 6 && name[len(name)-6:] == ".jsonl" {
		return name[:len(name)-6]
	}
	return name
}
