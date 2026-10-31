package handlers

import (
	"net/http"

	"nepse/terminal/api/internal/jobs"
)

// handleFreshness reports how old each dataset is against its SLA.
//
// It is separate from /healthz on purpose: liveness is a process fact, readiness
// is a database fact, and freshness is a data fact. Collapsing them would make a
// stale-but-serving system look healthy, which is the failure mode this endpoint
// exists to prevent.
func (s *Server) handleFreshness(w http.ResponseWriter, r *http.Request) {
	datasets, summary, err := jobs.Evaluate(r.Context(), s.store.Pool())
	if err != nil {
		s.internal(w, r, "freshness", err)
		return
	}
	meta := metaFrom(r)
	meta.Source = "postgres:dataset_slas"
	meta.LicenseClass = "public_unverified"
	if summary.Worst == "stale" || summary.Worst == "unknown" {
		// Still a 200: the endpoint answered the question. Callers alert on the
		// payload, not on the status code, so a monitoring rule can distinguish
		// "platform down" from "platform up, data late".
	}
	OK(w, r, map[string]any{"datasets": datasets, "summary": summary}, meta)
}

// handleIngestRuns exposes recent ingestion history for operators.
func (s *Server) handleIngestRuns(w http.ResponseWriter, r *http.Request) {
	limit, err := queryInt(r, "limit", 20, 1, 100)
	if err != nil {
		badRequest(w, r, err.Error())
		return
	}
	runs, err := s.store.RecentIngestRuns(r.Context(), limit)
	if err != nil {
		s.internal(w, r, "ingest runs", err)
		return
	}
	meta := metaFrom(r)
	meta.Source = "postgres:ingest_runs"
	OK(w, r, runs, meta)
}

// handleArchives exposes the immutable archive inventory.
func (s *Server) handleArchives(w http.ResponseWriter, r *http.Request) {
	stats, err := s.store.ArchivesByStream(r.Context())
	if err != nil {
		s.internal(w, r, "archives", err)
		return
	}
	meta := metaFrom(r)
	meta.Source = "postgres:raw_archives"
	OK(w, r, stats, meta)
}
