package repositories

import (
	"context"
	"fmt"

	"nepse/terminal/api/internal/research"
	"nepse/terminal/api/internal/services"
)

// Ensure Store implements research.EvidenceRetriever
var _ research.EvidenceRetriever = (*Store)(nil)

// GetDisclosures implements research.EvidenceRetriever.
func (s *Store) GetDisclosures(ctx context.Context, symbol, authority string, limit int) ([]services.Disclosure, error) {
	return s.ListDisclosures(ctx, authority, symbol, "", limit, 0)
}

// GetFinancialStatements implements research.EvidenceRetriever.
func (s *Store) GetFinancialStatements(ctx context.Context, symbol string) ([]*services.Statement, error) {
	periods, err := s.GetFinancialPeriods(ctx, symbol)
	if err != nil {
		return nil, err
	}
	var stmts []*services.Statement
	for _, p := range periods {
		stmt, err := s.GetStatement(ctx, p.ID)
		if err == nil && stmt != nil {
			stmts = append(stmts, stmt)
		}
	}
	return stmts, nil
}

// GetRatios implements research.EvidenceRetriever.
func (s *Store) GetRatios(ctx context.Context, symbol string, asOfDate string) (*services.PeriodRatios, error) {
	periods, err := s.GetFinancialPeriods(ctx, symbol)
	if err != nil || len(periods) == 0 {
		return nil, fmt.Errorf("no financial periods found")
	}

	var activePeriod *services.FinancialPeriod
	for i := range periods {
		if asOfDate == "" || periods[i].FilingDate <= asOfDate {
			activePeriod = &periods[i]
			break
		}
	}
	if activePeriod == nil {
		activePeriod = &periods[0]
	}

	stmt, err := s.GetStatement(ctx, activePeriod.ID)
	if err != nil {
		return nil, err
	}

	bars, _ := s.GetDailyOHLCV(ctx, symbol, "", "")
	price := stmt.GetLine(services.LineBookValuePerShare)
	if len(bars) > 0 {
		price = bars[len(bars)-1].Close
	}

	return services.ComputeRatios(stmt, price, asOfDate)
}

// GetLatestBar implements research.EvidenceRetriever.
func (s *Store) GetLatestBar(ctx context.Context, symbol string) (*services.OHLCVBar, error) {
	bars, err := s.GetDailyOHLCV(ctx, symbol, "", "")
	if err != nil || len(bars) == 0 {
		return nil, err
	}
	return &bars[len(bars)-1], nil
}

// RecordResearchQuery persists the AI research conversation and audit citations.
func (s *Store) RecordResearchQuery(ctx context.Context, q research.ResearchQuery, resp *research.ResearchResponse) (string, error) {
	var uid *string
	if q.UserID != "" {
		uid = &q.UserID
	}
	var pid *string
	if q.PortfolioID != "" {
		pid = &q.PortfolioID
	}

	row := s.pool.QueryRow(ctx, `
		INSERT INTO research_queries (
			user_id, query_text, symbol, scope, portfolio_id, answer, confidence_score
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id`,
		uid, q.QueryText, q.Symbol, q.Scope, pid, resp.SummaryAnswer, resp.ConfidenceScore.String(),
	)
	var queryID string
	if err := row.Scan(&queryID); err != nil {
		return "", err
	}

	// Persist citations
	allFindings := append(append(resp.Facts, resp.Calculations...), resp.Interpretations...)
	for _, f := range allFindings {
		for _, cite := range f.Citations {
			_, err := s.pool.Exec(ctx, `
				INSERT INTO research_citations (
					query_id, source_type, source_id, authority, citation_text,
					as_of_date, claim_type, attachment_url
				) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
				queryID, string(cite.SourceType), cite.SourceID, cite.Authority,
				cite.CitationText, nullableDate(cite.AsOfDate), string(cite.ClaimType), cite.AttachmentURL,
			)
			if err != nil {
				return queryID, err
			}
		}
	}

	return queryID, nil
}
