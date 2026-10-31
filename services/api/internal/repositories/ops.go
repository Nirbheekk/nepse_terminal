package repositories

import (
	"context"
	"time"
)

// IngestRun is one row of the ingestion history.
type IngestRun struct {
	Source       string     `json:"source"`
	Stream       string     `json:"stream"`
	Status       string     `json:"status"`
	RowsRead     int        `json:"rows_read"`
	RowsWritten  int        `json:"rows_written"`
	RowsRejected int        `json:"rows_rejected"`
	ArchiveSHA   string     `json:"archive_sha256,omitempty"`
	Error        string     `json:"error,omitempty"`
	StartedAt    time.Time  `json:"started_at"`
	FinishedAt   *time.Time `json:"finished_at"`
}

// RecentIngestRuns returns the latest runs across all streams.
func (s *Store) RecentIngestRuns(ctx context.Context, limit int) ([]IngestRun, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT r.source_slug, r.stream, r.status, r.rows_read, r.rows_written,
		       r.rows_rejected, COALESCE(a.sha256, ''), COALESCE(r.error, ''),
		       r.started_at, r.finished_at
		FROM ingest_runs r
		LEFT JOIN raw_archives a ON a.id = r.archive_id
		ORDER BY r.started_at DESC
		LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]IngestRun, 0, limit)
	for rows.Next() {
		var r IngestRun
		if err := rows.Scan(&r.Source, &r.Stream, &r.Status, &r.RowsRead, &r.RowsWritten,
			&r.RowsRejected, &r.ArchiveSHA, &r.Error, &r.StartedAt, &r.FinishedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ArchiveStats reports how many immutable batches are stored per stream.
type ArchiveStats struct {
	Stream   string     `json:"stream"`
	Archives int64      `json:"archives"`
	Rows     int64      `json:"rows"`
	Bytes    int64      `json:"bytes"`
	LatestAt *time.Time `json:"latest_at"`
}

// ArchivesByStream returns archive totals per stream.
func (s *Store) ArchivesByStream(ctx context.Context) ([]ArchiveStats, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT stream, count(*), COALESCE(sum(rows), 0), COALESCE(sum(bytes), 0), max(archived_at)
		FROM raw_archives GROUP BY stream ORDER BY stream`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]ArchiveStats, 0, 8)
	for rows.Next() {
		var a ArchiveStats
		if err := rows.Scan(&a.Stream, &a.Archives, &a.Rows, &a.Bytes, &a.LatestAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
