// Package repositories - fundamental data access (Batch 4.3).
package repositories

import (
	"context"
	"database/sql"
)

// CompanyProfile is the extended issuer profile.
type CompanyProfile struct {
	Symbol             string  `json:"symbol"`
	LegalName          string  `json:"legal_name"`
	RegisteredName     string  `json:"registered_name"`
	RegistrationNumber string  `json:"registration_number"`
	PANNumber          string  `json:"pan_number"`
	Sector             string  `json:"sector"`
	SubSector          string  `json:"sub_sector"`
	Address            string  `json:"address"`
	Website            string  `json:"website"`
	Email              string  `json:"email"`
	Phone              string  `json:"phone"`
	FiscalYearEnd      string  `json:"fiscal_year_end"`
	IncorporationDate  *string `json:"incorporation_date"`
	ListedDate         *string `json:"listed_date"`
	PaidUpCapital      *string `json:"paid_up_capital"`
	AuthorizedCapital  *string `json:"authorized_capital"`
	TotalShares        *string `json:"total_shares"`
	PromoterShares     *string `json:"promoter_shares"`
	PublicShares       *string `json:"public_shares"`
	Currency           string  `json:"currency"`
	EffectiveDate      *string `json:"effective_date"`
	Status             string  `json:"status"`
}

func (s *Store) GetCompanyProfile(ctx context.Context, symbol string) (*CompanyProfile, error) {
	const q = `
		SELECT symbol, legal_name, registered_name, registration_number, pan_number,
		       sector, sub_sector, address, website, email, phone, fiscal_year_end,
		       incorporation_date::text, listed_date::text, paid_up_capital::text,
		       authorized_capital::text, total_shares::text, promoter_shares::text,
		       public_shares::text, currency, effective_date::text, status
		FROM company_profiles WHERE symbol = $1
		ORDER BY effective_date DESC NULLS LAST LIMIT 1`
	var p CompanyProfile
	var inc, listed, eff sql.NullString
	var paid, auth, tot, prom, pub sql.NullString
	err := s.pool.QueryRow(ctx, q, symbol).Scan(
		&p.Symbol, &p.LegalName, &p.RegisteredName, &p.RegistrationNumber, &p.PANNumber,
		&p.Sector, &p.SubSector, &p.Address, &p.Website, &p.Email, &p.Phone, &p.FiscalYearEnd,
		&inc, &listed, &paid, &auth, &tot, &prom, &pub, &p.Currency, &eff, &p.Status)
	if err != nil {
		return nil, wrapNoRows(err)
	}
	if inc.Valid {
		v := inc.String
		p.IncorporationDate = &v
	}
	if listed.Valid {
		v := listed.String
		p.ListedDate = &v
	}
	if paid.Valid {
		v := paid.String
		p.PaidUpCapital = &v
	}
	if auth.Valid {
		v := auth.String
		p.AuthorizedCapital = &v
	}
	if tot.Valid {
		v := tot.String
		p.TotalShares = &v
	}
	if prom.Valid {
		v := prom.String
		p.PromoterShares = &v
	}
	if pub.Valid {
		v := pub.String
		p.PublicShares = &v
	}
	if eff.Valid {
		v := eff.String
		p.EffectiveDate = &v
	}
	return &p, nil
}
// BoardMember is a director row.
type BoardMember struct {
	FullName        string  `json:"full_name"`
	Role            string  `json:"role"`
	Designation     string  `json:"designation"`
	AppointmentDate *string `json:"appointment_date"`
	CessationDate   *string `json:"cessation_date"`
	IsIndependent   *bool   `json:"is_independent"`
}

func (s *Store) ListBoardMembers(ctx context.Context, symbol string) ([]BoardMember, error) {
	const q = `
		SELECT full_name, role, designation, appointment_date::text, cessation_date::text, is_independent
		FROM company_directors WHERE symbol = $1 ORDER BY appointment_date DESC NULLS LAST`
	rows, err := s.pool.Query(ctx, q, symbol)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]BoardMember, 0)
	for rows.Next() {
		var b BoardMember
		var appt, cess sql.NullString
		var ind sql.NullBool
		if err := rows.Scan(&b.FullName, &b.Role, &b.Designation, &appt, &cess, &ind); err != nil {
			return nil, err
		}
		if appt.Valid {
			v := appt.String
			b.AppointmentDate = &v
		}
		if cess.Valid {
			v := cess.String
			b.CessationDate = &v
		}
		if ind.Valid {
			v := ind.Bool
			b.IsIndependent = &v
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// OwnershipSnapshot row.
type OwnershipSnapshot struct {
	EffectiveDate        string  `json:"effective_date"`
	ObservationDate      *string `json:"observation_date"`
	PublicationDate      *string `json:"publication_date"`
	PromoterPercent      *string `json:"promoter_percent"`
	PublicPercent        *string `json:"public_percent"`
	GovernmentPercent    *string `json:"government_percent"`
	InstitutionalPercent *string `json:"institutional_percent"`
	ForeignPercent       *string `json:"foreign_percent"`
	OtherPercent         *string `json:"other_percent"`
	Status               string  `json:"status"`
}

func (s *Store) ListOwnershipSnapshots(ctx context.Context, symbol string) ([]OwnershipSnapshot, error) {
	const q = `
		SELECT effective_date::text, observation_date::text, publication_date::text,
		       promoter_percent::text, public_percent::text, government_percent::text,
		       institutional_percent::text, foreign_percent::text, other_percent::text, status
		FROM ownership_snapshots WHERE symbol = $1 ORDER BY effective_date DESC`
	rows, err := s.pool.Query(ctx, q, symbol)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]OwnershipSnapshot, 0)
	for rows.Next() {
		var o OwnershipSnapshot
		var obs, pub sql.NullString
		var prom, pubp, gov, inst, forp, other sql.NullString
		if err := rows.Scan(&o.EffectiveDate, &obs, &pub, &prom, &pubp, &gov, &inst, &forp, &other, &o.Status); err != nil {
			return nil, err
		}
		if obs.Valid {
			v := obs.String
			o.ObservationDate = &v
		}
		if pub.Valid {
			v := pub.String
			o.PublicationDate = &v
		}
		if prom.Valid {
			v := prom.String
			o.PromoterPercent = &v
		}
		if pubp.Valid {
			v := pubp.String
			o.PublicPercent = &v
		}
		if gov.Valid {
			v := gov.String
			o.GovernmentPercent = &v
		}
		if inst.Valid {
			v := inst.String
			o.InstitutionalPercent = &v
		}
		if forp.Valid {
			v := forp.String
			o.ForeignPercent = &v
		}
		if other.Valid {
			v := other.String
			o.OtherPercent = &v
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// MutualFund row.
type MutualFund struct {
	ID       string `json:"id"`
	FundCode string `json:"fund_code"`
	FundName string `json:"fund_name"`
	Manager  string `json:"manager"`
	FundType string `json:"fund_type"`
	Status   string `json:"status"`
}

func (s *Store) ListMutualFunds(ctx context.Context) ([]MutualFund, error) {
	const q = `SELECT id::text, fund_code, fund_name, manager, fund_type, status FROM mutual_funds ORDER BY fund_name`
	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]MutualFund, 0)
	for rows.Next() {
		var m MutualFund
		if err := rows.Scan(&m.ID, &m.FundCode, &m.FundName, &m.Manager, &m.FundType, &m.Status); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// MutualFundNAV row.
type MutualFundNAV struct {
	ValuationDate   string  `json:"valuation_date"`
	PublicationDate *string `json:"publication_date"`
	NAVPerUnit      string  `json:"nav_per_unit"`
	TotalNAV        *string `json:"total_nav"`
	Currency        string  `json:"currency"`
	Status          string  `json:"status"`
}

func (s *Store) ListMutualFundNAV(ctx context.Context, fundID string) ([]MutualFundNAV, error) {
	const q = `
		SELECT valuation_date::text, publication_date::text, nav_per_unit::text, total_nav::text, currency, status
		FROM mutual_fund_nav_history WHERE mutual_fund_id = $1::uuid ORDER BY valuation_date DESC`
	rows, err := s.pool.Query(ctx, q, fundID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]MutualFundNAV, 0)
	for rows.Next() {
		var n MutualFundNAV
		var pub, tot sql.NullString
		if err := rows.Scan(&n.ValuationDate, &pub, &n.NAVPerUnit, &tot, &n.Currency, &n.Status); err != nil {
			return nil, err
		}
		if pub.Valid {
			v := pub.String
			n.PublicationDate = &v
		}
		if tot.Valid {
			v := tot.String
			n.TotalNAV = &v
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *Store) DataCoverage(ctx context.Context) (map[string]int, error) {
	tables := []string{"company_profiles", "company_directors", "ownership_snapshots",
		"public_issues", "company_meetings", "merger_events", "financial_reports",
		"financial_statement_facts", "mutual_funds", "mutual_fund_nav_history",
		"source_documents", "ingestion_runs"}
	out := map[string]int{}
	for _, t := range tables {
		var n int
		q := "SELECT count(*) FROM " + t
		if err := s.pool.QueryRow(ctx, q).Scan(&n); err != nil {
			return nil, err
		}
		out[t] = n
	}
	return out, nil
}

