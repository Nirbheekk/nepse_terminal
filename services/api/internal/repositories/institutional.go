package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"nepse/terminal/api/internal/marketdata"
	"nepse/terminal/api/internal/portfolios"
	"nepse/terminal/api/internal/services"
)

var (
	ErrNotOrgMember           = errors.New("user is not a member of this organization")
	ErrSelfApprovalNotAllowed = errors.New("self-approval is prohibited (requester cannot approve own request)")
)

// CreateOrganization creates a new institutional organization tenant.
func (s *Store) CreateOrganization(ctx context.Context, name, slug, plan string) (*services.Organization, error) {
	if plan == "" {
		plan = string(services.PlanInstitutional)
	}
	row := s.pool.QueryRow(ctx, `
		INSERT INTO organizations (name, slug, plan)
		VALUES ($1, $2, $3)
		RETURNING id, name, slug, plan, status, created_at, updated_at`,
		name, slug, plan,
	)
	var org services.Organization
	var p string
	if err := row.Scan(&org.ID, &org.Name, &org.Slug, &p, &org.Status, &org.CreatedAt, &org.UpdatedAt); err != nil {
		return nil, fmt.Errorf("create organization: %w", err)
	}
	org.Plan = services.OrganizationPlan(p)
	return &org, nil
}

// AddOrganizationMember adds a user to an organization with an assigned role.
func (s *Store) AddOrganizationMember(ctx context.Context, orgID, userID string, role services.Role) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO organization_members (organization_id, user_id, role)
		VALUES ($1, $2, $3)
		ON CONFLICT (organization_id, user_id) DO UPDATE SET role = EXCLUDED.role`,
		orgID, userID, string(role),
	)
	return err
}

// GetMemberRole retrieves the membership role of a user in an organization.
func (s *Store) GetMemberRole(ctx context.Context, orgID, userID string) (services.Role, error) {
	var r string
	err := s.pool.QueryRow(ctx, `
		SELECT role FROM organization_members
		WHERE organization_id = $1 AND user_id = $2`, orgID, userID).Scan(&r)
	if err != nil {
		return "", ErrNotOrgMember
	}
	return services.Role(r), nil
}

// ListOrganizationMembers returns all members of an organization.
func (s *Store) ListOrganizationMembers(ctx context.Context, orgID string) ([]services.OrganizationMember, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT om.organization_id, om.user_id, u.email, COALESCE(u.display_name, ''), om.role, om.joined_at
		FROM organization_members om
		JOIN users u ON om.user_id = u.id
		WHERE om.organization_id = $1
		ORDER BY om.joined_at ASC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []services.OrganizationMember
	for rows.Next() {
		var m services.OrganizationMember
		var r string
		if err := rows.Scan(&m.OrganizationID, &m.UserID, &m.Email, &m.DisplayName, &r, &m.JoinedAt); err != nil {
			return nil, err
		}
		m.Role = services.Role(r)
		members = append(members, m)
	}
	return members, rows.Err()
}

// ListUserOrganizations lists all organizations a user belongs to.
func (s *Store) ListUserOrganizations(ctx context.Context, userID string) ([]services.Organization, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT o.id, o.name, o.slug, o.plan, o.status, o.created_at, o.updated_at
		FROM organizations o
		JOIN organization_members om ON o.id = om.organization_id
		WHERE om.user_id = $1
		ORDER BY o.name ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orgs []services.Organization
	for rows.Next() {
		var o services.Organization
		var p string
		if err := rows.Scan(&o.ID, &o.Name, &o.Slug, &p, &o.Status, &o.CreatedAt, &o.UpdatedAt); err != nil {
			return nil, err
		}
		o.Plan = services.OrganizationPlan(p)
		orgs = append(orgs, o)
	}
	return orgs, rows.Err()
}

// CreateOrgPortfolio creates a portfolio scoped to an organization.
func (s *Store) CreateOrgPortfolio(ctx context.Context, orgID, userID, name, desc, method, benchmark string) (*portfolios.PortfolioState, error) {
	if method == "" {
		method = string(portfolios.MethodWAC)
	}
	if benchmark == "" {
		benchmark = "NEPSE"
	}
	row := s.pool.QueryRow(ctx, `
		INSERT INTO portfolios (org_id, user_id, name, description, cost_basis_method, benchmark_symbol)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, user_id, name, currency, cost_basis_method, cash_balance`,
		orgID, userID, name, desc, method, benchmark,
	)

	var pf portfolios.PortfolioState
	var m string
	var cashStr string
	if err := row.Scan(&pf.ID, &pf.UserID, &pf.Name, &pf.Currency, &m, &cashStr); err != nil {
		return nil, fmt.Errorf("create org portfolio: %w", err)
	}
	pf.CostBasisMethod = portfolios.CostBasisMethod(m)
	pf.CashBalance, _ = marketdata.Parse(cashStr)
	pf.Positions = make(map[string]*portfolios.Position)
	pf.Lots = make(map[string][]*portfolios.Lot)
	return &pf, nil
}

// ListOrgPortfolios lists all portfolios belonging to an organization.
func (s *Store) ListOrgPortfolios(ctx context.Context, orgID string) ([]portfolios.PortfolioState, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, name, currency, cost_basis_method, cash_balance
		FROM portfolios
		WHERE org_id = $1
		ORDER BY name ASC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var pfs []portfolios.PortfolioState
	for rows.Next() {
		var pf portfolios.PortfolioState
		var m, cashStr string
		if err := rows.Scan(&pf.ID, &pf.UserID, &pf.Name, &pf.Currency, &m, &cashStr); err != nil {
			return nil, err
		}
		pf.CostBasisMethod = portfolios.CostBasisMethod(m)
		pf.CashBalance, _ = marketdata.Parse(cashStr)
		pfs = append(pfs, pf)
	}
	return pfs, rows.Err()
}

// CreateResearchNote stores a research note.
func (s *Store) CreateResearchNote(ctx context.Context, n services.ResearchNote) (string, error) {
	row := s.pool.QueryRow(ctx, `
		INSERT INTO research_notes (
			org_id, author_id, symbol, portfolio_id, title, summary, body, tags, visibility, status
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id`,
		n.OrgID, n.AuthorID, n.Symbol, nullableDate(n.PortfolioID),
		n.Title, n.Summary, n.Body, n.Tags, n.Visibility, n.Status,
	)
	var id string
	if err := row.Scan(&id); err != nil {
		return "", err
	}
	return id, nil
}

// ListResearchNotes retrieves notes for an organization.
func (s *Store) ListResearchNotes(ctx context.Context, orgID, symbol, status string) ([]services.ResearchNote, error) {
	query := `
		SELECT id, org_id, author_id, symbol, COALESCE(portfolio_id::text, ''),
		       title, summary, body, tags, visibility, status, created_at, updated_at
		FROM research_notes
		WHERE org_id = $1`
	args := []any{orgID}
	argIdx := 2

	if symbol != "" {
		query += fmt.Sprintf(" AND symbol = $%d", argIdx)
		args = append(args, symbol)
		argIdx++
	}
	if status != "" {
		query += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, status)
		argIdx++
	}
	query += " ORDER BY updated_at DESC"

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var notes []services.ResearchNote
	for rows.Next() {
		var n services.ResearchNote
		if err := rows.Scan(
			&n.ID, &n.OrgID, &n.AuthorID, &n.Symbol, &n.PortfolioID,
			&n.Title, &n.Summary, &n.Body, &n.Tags, &n.Visibility, &n.Status,
			&n.CreatedAt, &n.UpdatedAt,
		); err != nil {
			return nil, err
		}
		notes = append(notes, n)
	}
	return notes, rows.Err()
}

// CreateApprovalRequest creates a pending approval request.
func (s *Store) CreateApprovalRequest(ctx context.Context, req services.ApprovalRequest) (string, error) {
	payloadBytes, _ := json.Marshal(req.Payload)
	row := s.pool.QueryRow(ctx, `
		INSERT INTO approval_requests (
			org_id, portfolio_id, requester_id, action_type, payload, status
		) VALUES ($1, $2, $3, $4, $5, 'pending')
		RETURNING id`,
		req.OrgID, nullableDate(req.PortfolioID), req.RequesterID, string(req.ActionType), payloadBytes,
	)
	var id string
	if err := row.Scan(&id); err != nil {
		return "", err
	}
	return id, nil
}

// ReviewApprovalRequest approves or rejects an approval request with anti-self-approval enforcement.
func (s *Store) ReviewApprovalRequest(ctx context.Context, orgID, requestID, reviewerID string, status services.ApprovalStatus, notes string) error {
	// Anti-self-approval rule
	var requesterID string
	var currStatus string
	err := s.pool.QueryRow(ctx, `
		SELECT requester_id, status FROM approval_requests
		WHERE id = $1 AND org_id = $2`, requestID, orgID).Scan(&requesterID, &currStatus)
	if err != nil {
		return fmt.Errorf("approval request not found: %w", err)
	}
	if currStatus != string(services.ApprovalPending) {
		return fmt.Errorf("request already %s", currStatus)
	}
	if requesterID == reviewerID {
		return ErrSelfApprovalNotAllowed
	}

	_, err = s.pool.Exec(ctx, `
		UPDATE approval_requests
		SET status = $1, reviewer_id = $2, reviewed_at = now(), review_notes = $3
		WHERE id = $4 AND org_id = $5`,
		string(status), reviewerID, notes, requestID, orgID,
	)
	return err
}

// ListApprovalRequests lists approval requests for an organization.
func (s *Store) ListApprovalRequests(ctx context.Context, orgID, status string) ([]services.ApprovalRequest, error) {
	query := `
		SELECT id, org_id, COALESCE(portfolio_id::text, ''), requester_id,
		       action_type, payload, status, COALESCE(reviewer_id::text, ''),
		       reviewed_at, review_notes, created_at
		FROM approval_requests
		WHERE org_id = $1`
	args := []any{orgID}
	argIdx := 2

	if status != "" {
		query += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, status)
		argIdx++
	}
	query += " ORDER BY created_at DESC"

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var requests []services.ApprovalRequest
	for rows.Next() {
		var req services.ApprovalRequest
		var actType, stat string
		var pBytes []byte
		var revAt *time.Time
		if err := rows.Scan(
			&req.ID, &req.OrgID, &req.PortfolioID, &req.RequesterID,
			&actType, &pBytes, &stat, &req.ReviewerID,
			&revAt, &req.ReviewNotes, &req.CreatedAt,
		); err != nil {
			return nil, err
		}
		req.ActionType = services.ApprovalActionType(actType)
		req.Status = services.ApprovalStatus(stat)
		req.ReviewedAt = revAt
		_ = json.Unmarshal(pBytes, &req.Payload)
		requests = append(requests, req)
	}
	return requests, rows.Err()
}

// CreateScheduledReport registers a scheduled report.
func (s *Store) CreateScheduledReport(ctx context.Context, rep services.ScheduledReport) (string, error) {
	pBytes, _ := json.Marshal(rep.Parameters)
	row := s.pool.QueryRow(ctx, `
		INSERT INTO scheduled_reports (
			org_id, portfolio_id, created_by, name, report_type, schedule, recipients, parameters, is_active
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id`,
		rep.OrgID, rep.PortfolioID, rep.CreatedBy, rep.Name,
		rep.ReportType, rep.Schedule, rep.Recipients, pBytes, rep.IsActive,
	)
	var id string
	if err := row.Scan(&id); err != nil {
		return "", err
	}
	return id, nil
}

// ListScheduledReports lists scheduled reports for an organization.
func (s *Store) ListScheduledReports(ctx context.Context, orgID string) ([]services.ScheduledReport, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, org_id, portfolio_id, created_by, name, report_type, schedule,
		       recipients, parameters, is_active, last_run_at, next_run_at, created_at
		FROM scheduled_reports
		WHERE org_id = $1
		ORDER BY created_at DESC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reps []services.ScheduledReport
	for rows.Next() {
		var r services.ScheduledReport
		var pBytes []byte
		if err := rows.Scan(
			&r.ID, &r.OrgID, &r.PortfolioID, &r.CreatedBy, &r.Name, &r.ReportType,
			&r.Schedule, &r.Recipients, &pBytes, &r.IsActive, &r.LastRunAt, &r.NextRunAt, &r.CreatedAt,
		); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(pBytes, &r.Parameters)
		reps = append(reps, r)
	}
	return reps, rows.Err()
}

// RecordInstitutionalAudit records an audit event.
func (s *Store) RecordInstitutionalAudit(ctx context.Context, entry services.AuditLogEntry) error {
	detailBytes, _ := json.Marshal(entry.Detail)
	_, err := s.pool.Exec(ctx, `
		INSERT INTO institutional_audit_logs (
			org_id, actor_id, action, resource_type, resource_id, detail, ip_address
		) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		entry.OrgID, entry.ActorID, entry.Action, entry.ResourceType, entry.ResourceID, detailBytes, entry.IPAddress,
	)
	return err
}

// ListInstitutionalAudit retrieves audit logs for an organization.
func (s *Store) ListInstitutionalAudit(ctx context.Context, orgID string, limit, offset int) ([]services.AuditLogEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, org_id, COALESCE(actor_id::text, ''), action, resource_type,
		       resource_id, detail, ip_address, created_at
		FROM institutional_audit_logs
		WHERE org_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`, orgID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []services.AuditLogEntry
	for rows.Next() {
		var e services.AuditLogEntry
		var dBytes []byte
		if err := rows.Scan(
			&e.ID, &e.OrgID, &e.ActorID, &e.Action, &e.ResourceType,
			&e.ResourceID, &dBytes, &e.IPAddress, &e.CreatedAt,
		); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(dBytes, &e.Detail)
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
