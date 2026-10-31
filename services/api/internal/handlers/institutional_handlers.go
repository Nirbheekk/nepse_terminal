package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"nepse/terminal/api/internal/marketdata"
	"nepse/terminal/api/internal/portfolios"
	"nepse/terminal/api/internal/services"
)

// checkOrgPermission verifies that the calling user is a member of orgID and has the required permission.
func (s *Server) checkOrgPermission(r *http.Request, orgID string, perm services.Permission) (services.Role, error) {
	uid, ok := userIDFrom(r)
	if !ok {
		return "", fmt.Errorf("unauthenticated")
	}

	role, err := s.store.GetMemberRole(r.Context(), orgID, uid)
	if err != nil {
		return "", fmt.Errorf("forbidden: not a member of organization %s", orgID)
	}

	if !services.HasPermission(role, perm) {
		return role, fmt.Errorf("forbidden: role %s lacks permission %s", role, perm)
	}

	return role, nil
}

// handleListWorkspaces returns all organizations the authenticated user belongs to.
func (s *Server) handleListWorkspaces(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r)
	if !ok {
		unauthenticated(w, r)
		return
	}

	orgs, err := s.store.ListUserOrganizations(r.Context(), uid)
	if err != nil {
		Fail(w, r, http.StatusInternalServerError, CodeInternal, "failed to list organizations")
		return
	}

	OK(w, r, orgs, metaFrom(r))
}

type createOrgRequest struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
	Plan string `json:"plan"`
}

// handleCreateWorkspace creates an institutional organization.
func (s *Server) handleCreateWorkspace(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r)
	if !ok {
		unauthenticated(w, r)
		return
	}

	var req createOrgRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, r, "invalid request body")
		return
	}

	org, err := s.store.CreateOrganization(r.Context(), req.Name, req.Slug, req.Plan)
	if err != nil {
		Fail(w, r, http.StatusConflict, CodeConflict, err.Error())
		return
	}

	// Make creator org_admin
	_ = s.store.AddOrganizationMember(r.Context(), org.ID, uid, services.RoleOrgAdmin)

	_ = s.store.RecordInstitutionalAudit(r.Context(), services.AuditLogEntry{
		OrgID:        org.ID,
		ActorID:      uid,
		Action:       "organization.create",
		ResourceType: "organization",
		ResourceID:   org.ID,
		Detail:       map[string]any{"slug": org.Slug, "plan": org.Plan},
	})

	Succeed(w, r, http.StatusCreated, org, metaFrom(r))
}

// handleListOrgMembers lists all members of an organization.
func (s *Server) handleListOrgMembers(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "orgId")
	if _, err := s.checkOrgPermission(r, orgID, services.PermPortfolioRead); err != nil {
		Fail(w, r, http.StatusForbidden, CodeForbidden, err.Error())
		return
	}

	members, err := s.store.ListOrganizationMembers(r.Context(), orgID)
	if err != nil {
		Fail(w, r, http.StatusInternalServerError, CodeInternal, err.Error())
		return
	}

	OK(w, r, members, metaFrom(r))
}

type addMemberRequest struct {
	UserID string        `json:"user_id"`
	Role   services.Role `json:"role"`
}

// handleAddOrgMember adds a member with a role. Requires PermMemberManage (OrgAdmin).
func (s *Server) handleAddOrgMember(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "orgId")
	if _, err := s.checkOrgPermission(r, orgID, services.PermMemberManage); err != nil {
		Fail(w, r, http.StatusForbidden, CodeForbidden, err.Error())
		return
	}

	var req addMemberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, r, "invalid member payload")
		return
	}

	if err := s.store.AddOrganizationMember(r.Context(), orgID, req.UserID, req.Role); err != nil {
		Fail(w, r, http.StatusInternalServerError, CodeInternal, err.Error())
		return
	}

	uid, _ := userIDFrom(r)
	_ = s.store.RecordInstitutionalAudit(r.Context(), services.AuditLogEntry{
		OrgID:        orgID,
		ActorID:      uid,
		Action:       "member.add",
		ResourceType: "member",
		ResourceID:   req.UserID,
		Detail:       map[string]any{"role": req.Role},
	})

	OK(w, r, map[string]string{"status": "member added"}, metaFrom(r))
}

// handleListOrgPortfolios lists all portfolios in an organization.
func (s *Server) handleListOrgPortfolios(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "orgId")
	if _, err := s.checkOrgPermission(r, orgID, services.PermPortfolioRead); err != nil {
		Fail(w, r, http.StatusForbidden, CodeForbidden, err.Error())
		return
	}

	pfs, err := s.store.ListOrgPortfolios(r.Context(), orgID)
	if err != nil {
		Fail(w, r, http.StatusInternalServerError, CodeInternal, err.Error())
		return
	}

	OK(w, r, pfs, metaFrom(r))
}

type createOrgPortfolioRequest struct {
	Name            string `json:"name"`
	Description     string `json:"description"`
	CostBasisMethod string `json:"cost_basis_method"`
	BenchmarkSymbol string `json:"benchmark_symbol"`
}

// handleCreateOrgPortfolio creates a portfolio scoped to the organization. Requires PermPortfolioWrite.
func (s *Server) handleCreateOrgPortfolio(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "orgId")
	if _, err := s.checkOrgPermission(r, orgID, services.PermPortfolioWrite); err != nil {
		Fail(w, r, http.StatusForbidden, CodeForbidden, err.Error())
		return
	}

	uid, _ := userIDFrom(r)
	var req createOrgPortfolioRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, r, "invalid portfolio payload")
		return
	}

	pf, err := s.store.CreateOrgPortfolio(r.Context(), orgID, uid, req.Name, req.Description, req.CostBasisMethod, req.BenchmarkSymbol)
	if err != nil {
		Fail(w, r, http.StatusConflict, CodeConflict, err.Error())
		return
	}

	_ = s.store.RecordInstitutionalAudit(r.Context(), services.AuditLogEntry{
		OrgID:        orgID,
		ActorID:      uid,
		Action:       "portfolio.create",
		ResourceType: "portfolio",
		ResourceID:   pf.ID,
		Detail:       map[string]any{"name": pf.Name, "method": pf.CostBasisMethod},
	})

	Succeed(w, r, http.StatusCreated, pf, metaFrom(r))
}

// handleListNotes lists research notes for the organization.
func (s *Server) handleListNotes(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "orgId")
	if _, err := s.checkOrgPermission(r, orgID, services.PermPortfolioRead); err != nil {
		Fail(w, r, http.StatusForbidden, CodeForbidden, err.Error())
		return
	}

	symbol := strings.ToUpper(r.URL.Query().Get("symbol"))
	status := r.URL.Query().Get("status")

	notes, err := s.store.ListResearchNotes(r.Context(), orgID, symbol, status)
	if err != nil {
		Fail(w, r, http.StatusInternalServerError, CodeInternal, err.Error())
		return
	}

	OK(w, r, notes, metaFrom(r))
}

// handleCreateNote creates a research note. Requires PermNoteCreate.
func (s *Server) handleCreateNote(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "orgId")
	if _, err := s.checkOrgPermission(r, orgID, services.PermNoteCreate); err != nil {
		Fail(w, r, http.StatusForbidden, CodeForbidden, err.Error())
		return
	}

	uid, _ := userIDFrom(r)
	var n services.ResearchNote
	if err := json.NewDecoder(r.Body).Decode(&n); err != nil {
		badRequest(w, r, "invalid note payload")
		return
	}

	n.OrgID = orgID
	n.AuthorID = uid
	n.Symbol = strings.ToUpper(strings.TrimSpace(n.Symbol))
	if n.Title == "" {
		badRequest(w, r, "note title is required")
		return
	}
	if n.Status == "" {
		n.Status = "draft"
	}
	if n.Visibility == "" {
		n.Visibility = "internal_team"
	}

	id, err := s.store.CreateResearchNote(r.Context(), n)
	if err != nil {
		Fail(w, r, http.StatusInternalServerError, CodeInternal, err.Error())
		return
	}
	n.ID = id

	_ = s.store.RecordInstitutionalAudit(r.Context(), services.AuditLogEntry{
		OrgID:        orgID,
		ActorID:      uid,
		Action:       "note.create",
		ResourceType: "research_note",
		ResourceID:   id,
		Detail:       map[string]any{"symbol": n.Symbol, "title": n.Title},
	})

	Succeed(w, r, http.StatusCreated, n, metaFrom(r))
}

// handleListApprovals lists approval requests.
func (s *Server) handleListApprovals(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "orgId")
	if _, err := s.checkOrgPermission(r, orgID, services.PermPortfolioRead); err != nil {
		Fail(w, r, http.StatusForbidden, CodeForbidden, err.Error())
		return
	}

	status := r.URL.Query().Get("status")
	reqs, err := s.store.ListApprovalRequests(r.Context(), orgID, status)
	if err != nil {
		Fail(w, r, http.StatusInternalServerError, CodeInternal, err.Error())
		return
	}

	OK(w, r, reqs, metaFrom(r))
}

type reviewRequestPayload struct {
	Status services.ApprovalStatus `json:"status"` // approved, rejected
	Notes  string                  `json:"notes"`
}

// handleReviewApproval reviews an approval request. Requires PermTradeApprove, enforces anti-self-approval.
func (s *Server) handleReviewApproval(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "orgId")
	reqID := chi.URLParam(r, "id")

	if _, err := s.checkOrgPermission(r, orgID, services.PermTradeApprove); err != nil {
		Fail(w, r, http.StatusForbidden, CodeForbidden, err.Error())
		return
	}

	uid, _ := userIDFrom(r)
	var payload reviewRequestPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		badRequest(w, r, "invalid review payload")
		return
	}

	if err := s.store.ReviewApprovalRequest(r.Context(), orgID, reqID, uid, payload.Status, payload.Notes); err != nil {
		if strings.Contains(err.Error(), "self-approval") {
			Fail(w, r, http.StatusForbidden, CodeForbidden, err.Error())
			return
		}
		badRequest(w, r, err.Error())
		return
	}

	_ = s.store.RecordInstitutionalAudit(r.Context(), services.AuditLogEntry{
		OrgID:        orgID,
		ActorID:      uid,
		Action:       "approval.review",
		ResourceType: "approval_request",
		ResourceID:   reqID,
		Detail:       map[string]any{"status": payload.Status, "notes": payload.Notes},
	})

	OK(w, r, map[string]string{"status": string(payload.Status)}, metaFrom(r))
}

// handleLiquidityRisk computes days-to-liquidate and slippage costs. Requires PermRiskView.
func (s *Server) handleLiquidityRisk(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "orgId")
	pfID := chi.URLParam(r, "portfolioId")

	if _, err := s.checkOrgPermission(r, orgID, services.PermRiskView); err != nil {
		Fail(w, r, http.StatusForbidden, CodeForbidden, err.Error())
		return
	}

	uid, _ := userIDFrom(r)
	pf, err := s.store.GetPortfolio(r.Context(), pfID, uid)
	if err != nil {
		// Try fetching without user_id if member of org
		pfs, _ := s.store.ListOrgPortfolios(r.Context(), orgID)
		for _, p := range pfs {
			if p.ID == pfID {
				pf, _ = s.store.GetPortfolio(r.Context(), pfID, p.UserID)
				break
			}
		}
	}
	if pf == nil {
		Fail(w, r, http.StatusNotFound, CodeNotFound, "portfolio not found")
		return
	}

	// Prepare position inputs
	var inputs []services.PositionInput
	for sym, pos := range pf.Positions {
		if pos.Quantity.IsZero() {
			continue
		}
		bars, _ := s.store.GetDailyOHLCV(r.Context(), sym, "", "")
		price := pos.MarketPrice
		adv := marketdata.MustParse("10000") // default baseline ADV if bars empty
		if len(bars) > 0 {
			price = bars[len(bars)-1].Close
			adv = bars[len(bars)-1].Volume
		}
		inputs = append(inputs, services.PositionInput{
			Symbol: sym,
			Shares: pos.Quantity,
			Price:  price,
			ADV:    adv,
		})
	}

	partRate := marketdata.MustParse("0.15") // 15% participation
	rep, err := services.ComputeLiquidityRisk(pfID, inputs, partRate)
	if err != nil {
		Fail(w, r, http.StatusInternalServerError, CodeInternal, err.Error())
		return
	}

	OK(w, r, rep, metaFrom(r))
}

// handleConcentrationRisk computes Top-N, HHI, and breach checks. Requires PermRiskView.
func (s *Server) handleConcentrationRisk(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "orgId")
	pfID := chi.URLParam(r, "portfolioId")

	if _, err := s.checkOrgPermission(r, orgID, services.PermRiskView); err != nil {
		Fail(w, r, http.StatusForbidden, CodeForbidden, err.Error())
		return
	}

	pfs, _ := s.store.ListOrgPortfolios(r.Context(), orgID)
	var pf *portfolios.PortfolioState
	for _, p := range pfs {
		if p.ID == pfID {
			pf, _ = s.store.GetPortfolio(r.Context(), pfID, p.UserID)
			break
		}
	}
	if pf == nil {
		Fail(w, r, http.StatusNotFound, CodeNotFound, "portfolio not found")
		return
	}

	var inputs []services.PositionInput
	for sym, pos := range pf.Positions {
		if pos.Quantity.IsZero() {
			continue
		}
		bars, _ := s.store.GetDailyOHLCV(r.Context(), sym, "", "")
		price := pos.MarketPrice
		if len(bars) > 0 {
			price = bars[len(bars)-1].Close
		}
		inputs = append(inputs, services.PositionInput{
			Symbol: sym,
			Sector: "Equity",
			Shares: pos.Quantity,
			Price:  price,
		})
	}

	maxStock := marketdata.MustParse("10.0")  // 10% SEBON limit
	maxSector := marketdata.MustParse("25.0") // 25% sector limit
	rep, err := services.ComputeConcentrationRisk(pfID, inputs, maxStock, maxSector)
	if err != nil {
		Fail(w, r, http.StatusInternalServerError, CodeInternal, err.Error())
		return
	}

	OK(w, r, rep, metaFrom(r))
}

// handleStressTesting executes stress scenarios. Requires PermScenarioRun.
func (s *Server) handleStressTesting(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "orgId")
	pfID := chi.URLParam(r, "portfolioId")

	if _, err := s.checkOrgPermission(r, orgID, services.PermScenarioRun); err != nil {
		Fail(w, r, http.StatusForbidden, CodeForbidden, err.Error())
		return
	}

	var scenario services.StressScenario
	if err := json.NewDecoder(r.Body).Decode(&scenario); err != nil {
		badRequest(w, r, "invalid stress scenario payload")
		return
	}
	if scenario.Name == "" {
		scenario.Name = "Market Crash -20%"
		scenario.MarketDropPct = marketdata.MustParse("-20.0")
	}

	pfs, _ := s.store.ListOrgPortfolios(r.Context(), orgID)
	var pf *portfolios.PortfolioState
	for _, p := range pfs {
		if p.ID == pfID {
			pf, _ = s.store.GetPortfolio(r.Context(), pfID, p.UserID)
			break
		}
	}
	if pf == nil {
		Fail(w, r, http.StatusNotFound, CodeNotFound, "portfolio not found")
		return
	}

	var inputs []services.PositionInput
	for sym, pos := range pf.Positions {
		if pos.Quantity.IsZero() {
			continue
		}
		bars, _ := s.store.GetDailyOHLCV(r.Context(), sym, "", "")
		price := pos.MarketPrice
		if len(bars) > 0 {
			price = bars[len(bars)-1].Close
		}
		inputs = append(inputs, services.PositionInput{
			Symbol: sym,
			Shares: pos.Quantity,
			Price:  price,
			Beta:   marketdata.MustParse("1.1"),
		})
	}

	res, err := services.RunStressScenario(inputs, scenario)
	if err != nil {
		Fail(w, r, http.StatusInternalServerError, CodeInternal, err.Error())
		return
	}

	OK(w, r, res, metaFrom(r))
}

// handleExportPortfolioCSV exports portfolio risk and positions to CSV.
// STRICTLY GATED by PermReportExport!
func (s *Server) handleExportPortfolioCSV(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "orgId")
	pfID := chi.URLParam(r, "portfolioId")

	// Verify PermReportExport
	if _, err := s.checkOrgPermission(r, orgID, services.PermReportExport); err != nil {
		Fail(w, r, http.StatusForbidden, CodeForbidden, "forbidden: role lacks PermReportExport export permission")
		return
	}

	pfs, _ := s.store.ListOrgPortfolios(r.Context(), orgID)
	var pf *portfolios.PortfolioState
	for _, p := range pfs {
		if p.ID == pfID {
			pf, _ = s.store.GetPortfolio(r.Context(), pfID, p.UserID)
			break
		}
	}
	if pf == nil {
		Fail(w, r, http.StatusNotFound, CodeNotFound, "portfolio not found")
		return
	}

	uid, _ := userIDFrom(r)
	_ = s.store.RecordInstitutionalAudit(r.Context(), services.AuditLogEntry{
		OrgID:        orgID,
		ActorID:      uid,
		Action:       "report.export_csv",
		ResourceType: "portfolio",
		ResourceID:   pfID,
	})

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"portfolio-%s.csv\"", pfID))

	fmt.Fprintf(w, "symbol,quantity,cost_basis,avg_cost,market_price,market_value,unrealized_pnl\n")
	for sym, pos := range pf.Positions {
		fmt.Fprintf(w, "%s,%s,%s,%s,%s,%s,%s\n",
			sym, pos.Quantity.String(), pos.CostBasis.String(), pos.AvgCostPerShare.String(),
			pos.MarketPrice.String(), pos.MarketValue.String(), pos.UnrealizedPnL.String())
	}
}

// handleListAuditLogs returns immutable audit logs. Requires PermAuditRead.
func (s *Server) handleListAuditLogs(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "orgId")
	if _, err := s.checkOrgPermission(r, orgID, services.PermAuditRead); err != nil {
		Fail(w, r, http.StatusForbidden, CodeForbidden, err.Error())
		return
	}

	entries, err := s.store.ListInstitutionalAudit(r.Context(), orgID, 100, 0)
	if err != nil {
		Fail(w, r, http.StatusInternalServerError, CodeInternal, err.Error())
		return
	}

	OK(w, r, entries, metaFrom(r))
}
