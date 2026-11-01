// Package repositories - financial report data access (Batch 4.6).
package repositories

import (
	"context"
)

type FinancialReportRow struct {
	ID             string `json:"id"`
	Symbol         string `json:"symbol"`
	ReportType     string `json:"report_type"`
	FiscalYear     string `json:"fiscal_year"`
	PeriodType     string `json:"period_type"`
	PeriodStart    string `json:"period_start"`
	PeriodEnd      string `json:"period_end"`
	FilingDate     string `json:"filing_date"`
	AuditStatus    string `json:"audit_status"`
	Scope          string `json:"scope"`
	Status         string `json:"status"`
	RevisionNumber int    `json:"revision_number"`
}

func (s *Store) ListFinancialReports(ctx context.Context, symbol string) ([]FinancialReportRow, error) {
	q := `SELECT id::text, symbol, report_type, fiscal_year, period_type, period_start::text, period_end::text, filing_date::text, audit_status, scope, status, revision_number FROM financial_reports`
	args := []any{}
	if symbol != "" {
		q += ` WHERE symbol = $1`
		args = append(args, symbol)
	}
	q += ` ORDER BY period_end DESC, revision_number DESC`
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]FinancialReportRow, 0)
	for rows.Next() {
		var r FinancialReportRow
		if err := rows.Scan(&r.ID, &r.Symbol, &r.ReportType, &r.FiscalYear, &r.PeriodType, &r.PeriodStart, &r.PeriodEnd, &r.FilingDate, &r.AuditStatus, &r.Scope, &r.Status, &r.RevisionNumber); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) FinancialReportCoverage(ctx context.Context) (map[string]int, error) {
	tables := []string{"financial_reports", "financial_statement_facts", "financial_ratio_observations", "financial_report_documents"}
	out := map[string]int{}
	for _, t := range tables {
		var n int
		if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM "+t).Scan(&n); err != nil {
			return nil, err
		}
		out[t] = n
	}
	return out, nil
}
