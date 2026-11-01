# Batch 4.5 — Meetings

## Adapter
CompanyMeetingAdapter: AGM/SGM notices, agenda, venue, dates.

## Pipeline
run_meeting_pipeline: upsert by (symbol, meeting_type, meeting_date).

## Go API
GET /api/v1/company-meetings (optional ?symbol).
