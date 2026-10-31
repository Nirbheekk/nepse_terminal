// Package ingest folds archived source batches into PostgreSQL.
//
// The pipeline is deliberately two-pass:
//
//	source → archive (bytes on disk + digest) → replay → validate → stage → merge
//
// Ingestion reads the archive rather than the adapter, so what is in the
// database is provably derived from the exact file whose SHA-256 is recorded.
// Rows that fail validation are counted and sampled, never silently dropped and
// never turned into a zero.
package ingestion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"nepse/terminal/api/internal/marketdata"
)

// Ingester owns the pool, the archive and the per-stream SQL.
type Ingester struct {
	Pool    *pgxpool.Pool
	Archive *Store
	Logger  *slog.Logger
}

// Options narrows a run. The zero value ingests everything available.
type Options struct {
	Streams []string // empty = every stream the adapters offer
	Sources []string // empty = every available adapter
	Force   bool     // ignore the "same archive already ingested" checkpoint
	Trigger string   // label recorded on the collection job
}

// Rejection is one row that could not be accepted, with the reason.
type Rejection struct {
	Row    int            `json:"row"`
	Field  string         `json:"field,omitempty"`
	Reason string         `json:"reason"`
	Sample map[string]any `json:"sample,omitempty"`
}

// Result is the outcome of one adapter/stream pair.
type Result struct {
	Source        string      `json:"source"`
	Stream        string      `json:"stream"`
	Status        string      `json:"status"`
	RowsRead      int         `json:"rows_read"`
	RowsWritten   int         `json:"rows_written"`
	RowsUnchanged int         `json:"rows_unchanged"`
	RowsRejected  int         `json:"rows_rejected"`
	RowsDeduped   int         `json:"rows_deduped"`
	RowsDerived   int         `json:"rows_derived"`
	ArchiveSHA    string      `json:"archive_sha256,omitempty"`
	ArchivePath   string      `json:"archive_path,omitempty"`
	ArchiveReused bool        `json:"archive_reused"`
	Skipped       bool        `json:"skipped"`
	Rejections    []Rejection `json:"rejections,omitempty"`
	Err           error       `json:"-"`
	Error         string      `json:"error,omitempty"`
	DurationMS    int64       `json:"duration_ms"`
}

// Summary is the outcome of a whole run.
type Summary struct {
	JobID     string    `json:"job_id"`
	Status    string    `json:"status"`
	Trigger   string    `json:"trigger"`
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at"`
	Results   []Result  `json:"results"`
	Notes     []string  `json:"notes,omitempty"`

	RowsWritten  int `json:"rows_written"`
	RowsRejected int `json:"rows_rejected"`
	Streams      int `json:"streams"`
	Failed       int `json:"failed"`
	Skipped      int `json:"skipped"`
}

const chunkSize = 2000

// Run ingests every requested stream and records one collection job.
func (i *Ingester) Run(ctx context.Context, adapters []marketdata.Adapter, opts Options) (Summary, error) {
	if len(adapters) == 0 {
		return Summary{}, errors.New("no source adapters are available")
	}
	if i.Archive == nil || i.Archive.Pool == nil {
		return Summary{}, errors.New("archive store is not configured")
	}

	sum := Summary{Trigger: opts.Trigger, StartedAt: time.Now().UTC()}
	jobID, err := i.startJob(ctx, opts.Trigger)
	if err != nil {
		return sum, err
	}
	sum.JobID = jobID

	for _, a := range adapters {
		if !wants(opts.Sources, a.Slug()) {
			continue
		}
		for _, stream := range a.Streams() {
			if !wants(opts.Streams, stream) {
				continue
			}
			res := i.runStream(ctx, jobID, a, stream, opts)
			sum.Results = append(sum.Results, res)
			sum.Streams++
			sum.RowsWritten += res.RowsWritten
			sum.RowsRejected += res.RowsRejected
			if res.Err != nil {
				sum.Failed++
				i.log().Error("ingest stream failed", "source", res.Source, "stream", res.Stream, "error", res.Err)
			}
			if res.Skipped {
				sum.Skipped++
			}
		}
	}

	sum.EndedAt = time.Now().UTC()
	sum.Status = statusFor(sum)
	if err := i.finishJob(ctx, jobID, sum); err != nil {
		return sum, err
	}
	return sum, nil
}

func statusFor(s Summary) string {
	switch {
	case s.Failed > 0 && len(s.Results) == s.Failed:
		return "failed"
	case s.Failed > 0:
		return "partial"
	default:
		return "success"
	}
}

func (i *Ingester) runStream(ctx context.Context, jobID string, a marketdata.Adapter, stream string, opts Options) Result {
	started := time.Now()
	res := Result{Source: a.Slug(), Stream: stream, Status: "success"}

	ref, err := i.Archive.Write(ctx, a, stream)
	if err != nil {
		res.Err = err
		res.Error = err.Error()
		res.Status = "failed"
		res.DurationMS = time.Since(started).Milliseconds()
		i.recordRun(ctx, jobID, res, nil)
		return res
	}
	res.ArchiveSHA = ref.SHA256
	res.ArchivePath = ref.Path
	res.ArchiveReused = ref.Reused

	// A checkpoint match means these exact bytes were already folded in. The
	// check is by digest, not by timestamp: a collector rewrite that changes
	// nothing produces the same digest and is correctly skipped, while any real
	// change produces a new one.
	if !opts.Force {
		var lastSHA *string
		err := i.Pool.QueryRow(ctx,
			`SELECT archive_sha256 FROM ingest_checkpoints WHERE source_slug = $1 AND stream = $2`,
			a.Slug(), stream).Scan(&lastSHA)
		if err == nil && lastSHA != nil && *lastSHA == ref.SHA256 {
			res.Skipped = true
			res.Status = "skipped"
			res.RowsRead = ref.Rows
			res.DurationMS = time.Since(started).Milliseconds()
			i.touchCheckpoint(ctx, a.Slug(), stream, ref.SHA256, "", ref.Rows, false)
			i.recordRun(ctx, jobID, res, nil)
			return res
		}
	}

	srcID, err := i.ensureSource(ctx, a)
	if err != nil {
		res.Err = err
		res.Error = err.Error()
		res.Status = "failed"
		res.DurationMS = time.Since(started).Milliseconds()
		i.recordRun(ctx, jobID, res, nil)
		return res
	}

	written, unchanged, deduped, derived, rejections, err := i.ingestArchive(ctx, srcID, stream, ref.Path)
	res.RowsRead = ref.Rows
	res.RowsWritten = written
	res.RowsUnchanged = unchanged
	res.RowsDeduped = deduped
	res.RowsDerived = derived
	res.RowsRejected = len(rejections)
	res.Rejections = limitRejections(rejections, 20)
	if err != nil {
		res.Err = err
		res.Error = err.Error()
		res.Status = "failed"
	} else if len(rejections) > 0 {
		// Accepted the batch but not every row: this is a data problem the
		// operator should see, not a successful clean load.
		res.Status = "partial"
	}
	res.DurationMS = time.Since(started).Milliseconds()

	if err == nil {
		i.touchCheckpoint(ctx, a.Slug(), stream, ref.SHA256, "", ref.Rows, true)
	}
	i.recordRun(ctx, jobID, res, rejections)
	return res
}

func limitRejections(all []Rejection, n int) []Rejection {
	if len(all) <= n {
		return all
	}
	return all[:n]
}

// ingestArchive replays the archived file and merges it in chunks inside one
// transaction, so a failed stream leaves no partially applied rows.
func (i *Ingester) ingestArchive(ctx context.Context, srcID, stream, path string) (written, unchanged, deduped, derived int, rejections []Rejection, err error) {
	if summaryUsesScalarPath(stream) {
		w, u, d, rej, serr := i.ingestSummary(ctx, srcID, path)
		return w, u, 0, d, rej, serr
	}

	spec, ok := specs[stream]
	if !ok {
		return 0, 0, 0, 0, nil, fmt.Errorf("no ingestion spec for stream %q", stream)
	}

	tx, err := i.Pool.Begin(ctx)
	if err != nil {
		return 0, 0, 0, 0, nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, spec.stageDDL); err != nil {
		return 0, 0, 0, 0, nil, fmt.Errorf("create staging table: %w", err)
	}

	var chunk [][]any
	rowNum := 0
	flush := func() error {
		if len(chunk) == 0 {
			return nil
		}
		dedupedChunk, dupCount := dedupe(chunk, spec.keyIdx)
		deduped += dupCount

		if _, err := tx.Exec(ctx, spec.truncate); err != nil {
			return err
		}
		if len(dedupedChunk) > 0 {
			src := pgx.CopyFromRows(dedupedChunk)
			n, err := tx.CopyFrom(ctx, pgx.Identifier{spec.stageName}, spec.cols, src)
			if err != nil {
				return fmt.Errorf("stage rows: %w", err)
			}
			if int(n) != len(dedupedChunk) {
				return fmt.Errorf("staged %d rows, expected %d", n, len(dedupedChunk))
			}
			// prep runs after staging because its SQL reads the staged rows
			// (sessions, sectors and index definitions the merge depends on).
			if spec.prep != nil {
				if err := spec.prep(ctx, tx, dedupedChunk); err != nil {
					return err
				}
			}
			var tag pgconn.CommandTag
			if spec.usesSource {
				tag, err = tx.Exec(ctx, spec.merge, srcID)
			} else {
				tag, err = tx.Exec(ctx, spec.merge)
			}
			if err != nil {
				return fmt.Errorf("merge: %w", err)
			}
			written += int(tag.RowsAffected())
			unchanged += len(dedupedChunk) - int(tag.RowsAffected())

			if spec.post != nil {
				n, err := spec.post(ctx, tx, dedupedChunk, srcID)
				if err != nil {
					return fmt.Errorf("post-merge: %w", err)
				}
				derived += n
			}
		}
		chunk = chunk[:0]
		return nil
	}

	seen, err := EachRow(path, func(row map[string]any) error {
		rowNum++
		if err := ctx.Err(); err != nil {
			return err
		}
		vals, err := spec.normalize(row)
		if err != nil {
			rejections = append(rejections, Rejection{
				Row:    rowNum,
				Field:  fieldOf(err),
				Reason: reasonOf(err),
				Sample: sampleOf(row),
			})
			return nil
		}
		chunk = append(chunk, vals)
		if len(chunk) >= chunkSize {
			return flush()
		}
		return nil
	})
	if err != nil {
		return written, unchanged, deduped, derived, rejections, err
	}
	if err := flush(); err != nil {
		return written, unchanged, deduped, derived, rejections, err
	}
	if spec.finalize != nil {
		n, err := spec.finalize(ctx, tx)
		if err != nil {
			return written, unchanged, deduped, derived, rejections, fmt.Errorf("finalize: %w", err)
		}
		derived += n
	}

	// Rows skipped by a join are invisible in RowsAffected; count them and report
	// them rather than letting the totals quietly disagree with the source.
	if spec.orphanCheck != "" {
		var orphans int
		if err := tx.QueryRow(ctx, spec.orphanCheck).Scan(&orphans); err != nil {
			return written, unchanged, deduped, derived, rejections, err
		}
		if orphans > 0 {
			rejections = append(rejections, Rejection{
				Reason: fmt.Sprintf("%d rows skipped: referenced parent row does not exist", orphans),
			})
		}
	}

	if seen != rowNum {
		i.log().Warn("archive row count mismatch", "stream", stream, "decoded", seen, "visited", rowNum)
	}
	return written, unchanged, deduped, derived, rejections, tx.Commit(ctx)
}

// dedupe drops repeated natural keys inside one chunk, keeping the last
// occurrence. A batch can legitimately contain the same key twice (two captures
// of one contract); PostgreSQL refuses to update the same row twice in a single
// statement, so the collapse has to happen here.
func dedupe(rows [][]any, keyIdx []int) ([][]any, int) {
	if len(keyIdx) == 0 || len(rows) < 2 {
		return rows, 0
	}
	index := make(map[string]int, len(rows))
	out := make([][]any, 0, len(rows))
	dups := 0
	for _, r := range rows {
		key := ""
		for _, k := range keyIdx {
			key += fmt.Sprintf("%v|", r[k])
		}
		if pos, ok := index[key]; ok {
			out[pos] = r
			dups++
			continue
		}
		index[key] = len(out)
		out = append(out, r)
	}
	return out, dups
}

// ---------------------------------------------------------------- bookkeeping

func (i *Ingester) startJob(ctx context.Context, trigger string) (string, error) {
	detail, _ := json.Marshal(map[string]any{"trigger": trigger})
	var id string
	err := i.Pool.QueryRow(ctx,
		`INSERT INTO collection_jobs (job, status, detail) VALUES ('ingest', 'running', $1::jsonb)
		 RETURNING id::text`, string(detail)).Scan(&id)
	return id, err
}

func (i *Ingester) finishJob(ctx context.Context, jobID string, sum Summary) error {
	detail, err := json.Marshal(map[string]any{
		"trigger":       sum.Trigger,
		"streams":       sum.Streams,
		"rows_written":  sum.RowsWritten,
		"rows_rejected": sum.RowsRejected,
		"skipped":       sum.Skipped,
		"failed":        sum.Failed,
		"duration_ms":   sum.EndedAt.Sub(sum.StartedAt).Milliseconds(),
	})
	if err != nil {
		return err
	}
	_, err = i.Pool.Exec(ctx,
		`UPDATE collection_jobs SET status = $2, finished_at = now(), detail = $3::jsonb WHERE id = $1::uuid`,
		jobID, sum.Status, string(detail))
	return err
}

func (i *Ingester) recordRun(ctx context.Context, jobID string, res Result, rejections []Rejection) {
	sample := res.Rejections
	if sample == nil {
		sample = []Rejection{}
	}
	raw, _ := json.Marshal(sample)
	var archiveID *string
	if res.ArchiveSHA != "" {
		var id string
		if err := i.Pool.QueryRow(ctx,
			`SELECT id::text FROM raw_archives WHERE stream = $1 AND sha256 = $2`,
			res.Stream, res.ArchiveSHA).Scan(&id); err == nil {
			archiveID = &id
		}
	}
	var errText *string
	if res.Error != "" {
		errText = &res.Error
	}
	_, err := i.Pool.Exec(ctx, `
		INSERT INTO ingest_runs (job_id, source_slug, stream, status, archive_id,
			rows_read, rows_written, rows_unchanged, rows_rejected, rejected_sample, error,
			started_at, finished_at)
		VALUES ($1::uuid, $2, $3, $4, $5::uuid, $6, $7, $8, $9, $10::jsonb, $11,
			now() - make_interval(secs => $12), now())`,
		jobID, res.Source, res.Stream, res.Status, archiveID,
		res.RowsRead, res.RowsWritten, res.RowsUnchanged, res.RowsRejected,
		string(raw), errText, float64(res.DurationMS)/1000.0)
	if err != nil {
		i.log().Error("record ingest run", "error", err, "stream", res.Stream)
	}
}

func (i *Ingester) touchCheckpoint(ctx context.Context, slug, stream, sha, cursor string, rows int, success bool) {
	_, err := i.Pool.Exec(ctx, `
		INSERT INTO ingest_checkpoints
			(source_slug, stream, archive_sha256, cursor, rows_seen, last_run_at, last_success_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, now(), CASE WHEN $6 THEN now() ELSE NULL END, now())
		ON CONFLICT (source_slug, stream) DO UPDATE SET
			archive_sha256 = EXCLUDED.archive_sha256,
			cursor = COALESCE(NULLIF(EXCLUDED.cursor, ''), ingest_checkpoints.cursor),
			rows_seen = EXCLUDED.rows_seen,
			last_run_at = now(),
			last_success_at = CASE WHEN $6 THEN now() ELSE ingest_checkpoints.last_success_at END,
			updated_at = now()`,
		slug, stream, sha, cursor, rows, success)
	if err != nil {
		i.log().Error("update checkpoint", "error", err, "stream", stream)
	}
}

// ensureSource registers the adapter as a data source, carrying its declared
// authorization. Provenance is written at ingest time, not added later.
func (i *Ingester) ensureSource(ctx context.Context, a marketdata.Adapter) (string, error) {
	auth := a.Authorization()
	var id string
	err := i.Pool.QueryRow(ctx, `
		INSERT INTO data_sources (slug, display_name, endpoint, license_class, redistribution,
			access_mode, attribution, verified_at, notes)
		VALUES ($1, $2, NULL, $3, $4, $5, $6, now(), $7)
		ON CONFLICT (slug) DO UPDATE SET
			license_class = EXCLUDED.license_class,
			redistribution = EXCLUDED.redistribution,
			access_mode = EXCLUDED.access_mode,
			attribution = EXCLUDED.attribution,
			verified_at = now()
		RETURNING id::text`,
		a.Slug(), "Legacy collector ("+a.Slug()+")", auth.LicenceClass, auth.Redistribution,
		auth.Mode, auth.Note, auth.Note).Scan(&id)
	return id, err
}

func (i *Ingester) log() *slog.Logger {
	if i.Logger != nil {
		return i.Logger
	}
	return slog.Default()
}

func wants(list []string, name string) bool {
	if len(list) == 0 {
		return true
	}
	for _, v := range list {
		if v == name {
			return true
		}
	}
	return false
}
