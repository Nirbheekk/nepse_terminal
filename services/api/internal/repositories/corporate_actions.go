// Package repositories - corporate action data access (Batch 4.5).
package repositories

import (
	"context"
	"database/sql"
)

type PublicIssueRow struct {
	Symbol        string  `json:"symbol"`
	IssueType     string  `json:"issue_type"`
	IssueName     string  `json:"issue_name"`
	OpenDate      *string `json:"open_date"`
	CloseDate     *string `json:"close_date"`
	AllotmentDate *string `json:"allotment_date"`
	ListingDate   *string `json:"listing_date"`
	IssuePrice    *string `json:"issue_price"`
	UnitsOffered  *string `json:"units_offered"`
	Status        string  `json:"status"`
}

func (s *Store) ListPublicIssues(ctx context.Context, symbol string) ([]PublicIssueRow, error) {
	q := `SELECT symbol, issue_type, issue_name, open_date::text, close_date::text, allotment_date::text, listing_date::text, issue_price::text, units_offered::text, status FROM public_issues`
	args := []any{}
	if symbol != "" {
		q += ` WHERE symbol = $1`
		args = append(args, symbol)
	}
	q += ` ORDER BY open_date DESC NULLS LAST`
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]PublicIssueRow, 0)
	for rows.Next() {
		var r PublicIssueRow
		var od, cd, ad, ld, ip, uo sql.NullString
		if err := rows.Scan(&r.Symbol, &r.IssueType, &r.IssueName, &od, &cd, &ad, &ld, &ip, &uo, &r.Status); err != nil {
			return nil, err
		}
		if od.Valid { v := od.String; r.OpenDate = &v }
		if cd.Valid { v := cd.String; r.CloseDate = &v }
		if ad.Valid { v := ad.String; r.AllotmentDate = &v }
		if ld.Valid { v := ld.String; r.ListingDate = &v }
		if ip.Valid { v := ip.String; r.IssuePrice = &v }
		if uo.Valid { v := uo.String; r.UnitsOffered = &v }
		out = append(out, r)
	}
	return out, rows.Err()
}

type MergerRow struct {
	EventName       string  `json:"event_name"`
	EventType       string  `json:"event_type"`
	AnnouncementDate *string `json:"announcement_date"`
	EffectiveDate   *string `json:"effective_date"`
	Status          string  `json:"status"`
	SurvivingSymbol string  `json:"surviving_symbol"`
	SwapRatio       *string `json:"swap_ratio"`
}

func (s *Store) ListMergers(ctx context.Context) ([]MergerRow, error) {
	q := `SELECT event_name, event_type, announcement_date::text, effective_date::text, status, surviving_symbol, swap_ratio::text FROM merger_events ORDER BY announcement_date DESC NULLS LAST`
	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]MergerRow, 0)
	for rows.Next() {
		var r MergerRow
		var ad, ed, sr sql.NullString
		if err := rows.Scan(&r.EventName, &r.EventType, &ad, &ed, &r.Status, &r.SurvivingSymbol, &sr); err != nil {
			return nil, err
		}
		if ad.Valid { v := ad.String; r.AnnouncementDate = &v }
		if ed.Valid { v := ed.String; r.EffectiveDate = &v }
		if sr.Valid { v := sr.String; r.SwapRatio = &v }
		out = append(out, r)
	}
	return out, rows.Err()
}

type CompanyMeetingRow struct {
	Symbol          string  `json:"symbol"`
	MeetingType     string  `json:"meeting_type"`
	MeetingDate     string  `json:"meeting_date"`
	BookClosureDate *string `json:"book_closure_date"`
	Agenda          string  `json:"agenda"`
	Venue           string  `json:"venue"`
	Status          string  `json:"status"`
}

func (s *Store) ListCompanyMeetings(ctx context.Context, symbol string) ([]CompanyMeetingRow, error) {
	q := `SELECT symbol, meeting_type, meeting_date::text, book_closure_date::text, agenda, venue, status FROM company_meetings`
	args := []any{}
	if symbol != "" {
		q += ` WHERE symbol = $1`
		args = append(args, symbol)
	}
	q += ` ORDER BY meeting_date DESC`
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]CompanyMeetingRow, 0)
	for rows.Next() {
		var r CompanyMeetingRow
		var bc sql.NullString
		if err := rows.Scan(&r.Symbol, &r.MeetingType, &r.MeetingDate, &bc, &r.Agenda, &r.Venue, &r.Status); err != nil {
			return nil, err
		}
		if bc.Valid { v := bc.String; r.BookClosureDate = &v }
		out = append(out, r)
	}
	return out, rows.Err()
}
