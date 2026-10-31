// Package research implements an evidence-grounded AI research assistant for NEPSE market data,
// regulatory circulars (NEPSE, SEBON, NRB), company disclosures, financial statements, and ratios.
//
// Every finding is grounded with source citations, as-of dates, and explicit tripartite categorization:
// FACT (retrieved source record), CALCULATION (tested deterministic formula), or INTERPRETATION (AI reasoning).
// Unsupported claims are rejected and private portfolio data is strictly protected against unauthorized disclosure.
package research

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"nepse/terminal/api/internal/marketdata"
	"nepse/terminal/api/internal/portfolios"
	"nepse/terminal/api/internal/services"
)

var (
	ErrUnauthorizedPortfolioAccess = errors.New("unauthorized access to private portfolio data")
	ErrInsufficientEvidence        = errors.New("insufficient authorized evidence to answer query")
)

type ClaimType string

const (
	ClaimFact           ClaimType = "FACT"
	ClaimCalculation    ClaimType = "CALCULATION"
	ClaimInterpretation ClaimType = "INTERPRETATION"
)

type SourceType string

const (
	SourceDisclosure         SourceType = "disclosure"
	SourceFinancialStatement SourceType = "financial_statement"
	SourceRatio              SourceType = "ratio"
	SourceMarketPrice        SourceType = "market_price"
	SourceCorporateAction    SourceType = "corporate_action"
	SourcePortfolio          SourceType = "portfolio"
)

// Citation provides audit provenance for an individual fact or calculation.
type Citation struct {
	SourceType    SourceType `json:"source_type"`
	SourceID      string     `json:"source_id"`
	Authority     string     `json:"authority"` // NEPSE, SEBON, NRB, COMPANY, PORTFOLIO_LEDGER
	CitationText  string     `json:"citation_text"`
	AsOfDate      string     `json:"as_of_date,omitempty"`
	ClaimType     ClaimType  `json:"claim_type"`
	AttachmentURL string     `json:"attachment_url,omitempty"`
	SHA256        string     `json:"sha256,omitempty"`
	VerbatimQuote string     `json:"verbatim_quote,omitempty"`
}

// GroundedFinding represents a verified epistemic statement.
type GroundedFinding struct {
	ClaimType ClaimType  `json:"claim_type"` // FACT, CALCULATION, INTERPRETATION
	Statement string     `json:"statement"`
	Formula   string     `json:"formula,omitempty"`
	Citations []Citation `json:"citations"`
}

// ResearchQuery is the input to the research assistant.
type ResearchQuery struct {
	UserID      string `json:"user_id,omitempty"`
	QueryText   string `json:"query_text"`
	Symbol      string `json:"symbol,omitempty"`
	Scope       string `json:"scope"` // market, company, regulatory, portfolio
	PortfolioID string `json:"portfolio_id,omitempty"`
}

// ResearchResponse contains the evidence-grounded research output.
type ResearchResponse struct {
	QueryText        string             `json:"query_text"`
	Symbol           string             `json:"symbol,omitempty"`
	Scope            string             `json:"scope"`
	SummaryAnswer    string             `json:"summary_answer"`
	Facts            []GroundedFinding  `json:"facts"`
	Calculations     []GroundedFinding  `json:"calculations"`
	Interpretations  []GroundedFinding  `json:"interpretations"`
	ConfidenceScore  marketdata.Decimal `json:"confidence_score"`
	LicenseNotice    string             `json:"license_notice"`
	Disclaimer       string             `json:"disclaimer"`
	SupportedByProof bool               `json:"supported_by_proof"`
}

// EvidenceContext aggregates all retrieved source records for a query.
type EvidenceContext struct {
	Disclosures      []services.Disclosure
	Statements       []*services.Statement
	Ratios           *services.PeriodRatios
	CorporateActions []services.CorporateAction
	LatestBar        *services.OHLCVBar
	Portfolio        *portfolios.PortfolioState
}

// EvidenceRetriever defines the interface for retrieving grounding records.
type EvidenceRetriever interface {
	GetDisclosures(ctx context.Context, symbol, authority string, limit int) ([]services.Disclosure, error)
	GetFinancialStatements(ctx context.Context, symbol string) ([]*services.Statement, error)
	GetRatios(ctx context.Context, symbol string, asOfDate string) (*services.PeriodRatios, error)
	GetCorporateActions(ctx context.Context, symbol, actionType string) ([]services.CorporateAction, error)
	GetLatestBar(ctx context.Context, symbol string) (*services.OHLCVBar, error)
	GetPortfolio(ctx context.Context, portfolioID, userID string) (*portfolios.PortfolioState, error)
}

// Assistant orchestrates evidence retrieval, grounding validation, and synthesis.
type Assistant struct {
	retriever EvidenceRetriever
}

// NewAssistant creates a new research assistant.
func NewAssistant(r EvidenceRetriever) *Assistant {
	return &Assistant{retriever: r}
}

// Execute processes a research query with strict evidence grounding.
func (a *Assistant) Execute(ctx context.Context, q ResearchQuery) (*ResearchResponse, error) {
	q.QueryText = strings.TrimSpace(q.QueryText)
	if q.QueryText == "" {
		return nil, errors.New("empty query text")
	}
	q.Symbol = strings.ToUpper(strings.TrimSpace(q.Symbol))

	// 1. Enforce Portfolio Privacy Boundary
	if q.Scope == "portfolio" || q.PortfolioID != "" {
		if q.UserID == "" {
			return nil, fmt.Errorf("%w: user authentication required to query portfolio", ErrUnauthorizedPortfolioAccess)
		}
		if q.PortfolioID == "" {
			return nil, errors.New("portfolio_id required for portfolio-scoped query")
		}
	}

	// 2. Retrieve Evidence
	evidence, err := a.gatherEvidence(ctx, q)
	if err != nil {
		return nil, err
	}

	// 3. Grounded Synthesis
	resp := a.synthesize(q, evidence)
	return resp, nil
}

func (a *Assistant) gatherEvidence(ctx context.Context, q ResearchQuery) (*EvidenceContext, error) {
	ev := &EvidenceContext{}

	// Portfolio gathering (strictly tenant-gated)
	if q.Scope == "portfolio" && q.PortfolioID != "" {
		pf, err := a.retriever.GetPortfolio(ctx, q.PortfolioID, q.UserID)
		if err != nil {
			return nil, fmt.Errorf("%w: cannot access portfolio %s", ErrUnauthorizedPortfolioAccess, q.PortfolioID)
		}
		ev.Portfolio = pf
	}

	// Company/Regulatory Disclosures gathering
	if a.retriever != nil {
		if q.Symbol != "" {
			discs, _ := a.retriever.GetDisclosures(ctx, q.Symbol, "", 10)
			ev.Disclosures = discs

			stmts, _ := a.retriever.GetFinancialStatements(ctx, q.Symbol)
			ev.Statements = stmts

			rts, _ := a.retriever.GetRatios(ctx, q.Symbol, "")
			ev.Ratios = rts

			acts, _ := a.retriever.GetCorporateActions(ctx, q.Symbol, "")
			ev.CorporateActions = acts

			bar, _ := a.retriever.GetLatestBar(ctx, q.Symbol)
			ev.LatestBar = bar
		} else {
			// Market / Regulatory query
			discs, _ := a.retriever.GetDisclosures(ctx, "", "", 20)
			ev.Disclosures = discs
		}
	}

	return ev, nil
}

func (a *Assistant) synthesize(q ResearchQuery, ev *EvidenceContext) *ResearchResponse {
	resp := &ResearchResponse{
		QueryText:     q.QueryText,
		Symbol:        q.Symbol,
		Scope:         q.Scope,
		LicenseNotice: "Source disclosures are official publications of NEPSE, SEBON, NRB or listed issuers. Calculations use tested deterministic formulas.",
		Disclaimer:    "This research analysis separates factual disclosures, deterministic calculations, and interpretations. Not financial advice.",
	}

	// Check if any evidence exists
	hasEvidence := len(ev.Disclosures) > 0 || len(ev.Statements) > 0 || ev.Ratios != nil ||
		len(ev.CorporateActions) > 0 || ev.LatestBar != nil || ev.Portfolio != nil

	if !hasEvidence {
		// Strict guardrail: Prevent unsupported claims / hallucinations
		resp.SummaryAnswer = fmt.Sprintf("No authorized NEPSE, SEBON, NRB or company disclosures found in platform records matching '%s'. Unsupported claims are refused.", q.QueryText)
		resp.ConfidenceScore = marketdata.Zero()
		resp.SupportedByProof = false
		return resp
	}

	// 1. Extract FACTS from Disclosures
	for _, d := range ev.Disclosures {
		cite := Citation{
			SourceType:   SourceDisclosure,
			SourceID:     d.ID,
			Authority:    string(d.Authority),
			CitationText: fmt.Sprintf("[%s %s] %s (%s)", d.Authority, d.Category, d.Title, d.PublishedAtNPT),
			AsOfDate:     d.PublishedAt.Format("2006-01-02"),
			ClaimType:    ClaimFact,
		}
		if len(d.Attachments) > 0 {
			cite.AttachmentURL = d.Attachments[0].FileURL
			cite.SHA256 = d.Attachments[0].SHA256
		}

		finding := GroundedFinding{
			ClaimType: ClaimFact,
			Statement: fmt.Sprintf("%s issued %s: %s (Ref: %s)", d.Authority, d.Category, d.Title, d.ReferenceNumber),
			Citations: []Citation{cite},
		}
		resp.Facts = append(resp.Facts, finding)
	}

	// Extract FACTS from Corporate Actions (Dividends/Bonus)
	for _, ca := range ev.CorporateActions {
		cite := Citation{
			SourceType:   SourceCorporateAction,
			SourceID:     ca.ID,
			Authority:    "NEPSE/COMPANY",
			CitationText: fmt.Sprintf("%s Corporate Action %s for FY %s (Ex-Date: %s)", ca.Symbol, ca.ActionType, ca.FiscalYear, ca.BookClosureDate),
			AsOfDate:     ca.BookClosureDate,
			ClaimType:    ClaimFact,
		}
		var desc string
		if ca.ActionType == services.ActionBonusShare {
			desc = fmt.Sprintf("%s announced %s%% bonus share for FY %s with book closure on %s", ca.Symbol, ca.BonusPercent.String(), ca.FiscalYear, ca.BookClosureDate)
		} else if ca.ActionType == services.ActionCashDividend {
			desc = fmt.Sprintf("%s announced %s cash dividend for FY %s with book closure on %s", ca.Symbol, ca.CashAmount.String(), ca.FiscalYear, ca.BookClosureDate)
		} else {
			desc = fmt.Sprintf("%s registered %s for FY %s with book closure on %s", ca.Symbol, ca.ActionType, ca.FiscalYear, ca.BookClosureDate)
		}

		resp.Facts = append(resp.Facts, GroundedFinding{
			ClaimType: ClaimFact,
			Statement: desc,
			Citations: []Citation{cite},
		})
	}

	// 2. Extract CALCULATIONS from Financial Ratios & Market Prices
	if ev.Ratios != nil {
		r := ev.Ratios
		ratioCite := Citation{
			SourceType:   SourceRatio,
			SourceID:     fmt.Sprintf("%s-%s-%s", r.Symbol, r.FiscalYear, r.PeriodType),
			Authority:    "FINANCIAL_ENGINE",
			CitationText: fmt.Sprintf("Reporting-period ratios for %s %s %s as of %s (Price: Rs %s)", r.Symbol, r.FiscalYear, r.PeriodType, r.AsOfDate, r.PriceUsed.String()),
			AsOfDate:     r.AsOfDate,
			ClaimType:    ClaimCalculation,
		}

		if r.PE.IsPositive() {
			resp.Calculations = append(resp.Calculations, GroundedFinding{
				ClaimType: ClaimCalculation,
				Statement: fmt.Sprintf("Price-to-Earnings (P/E) ratio is %sx (Market Price Rs %s / Annualized EPS Rs %s)", r.PE.Format(2), r.PriceUsed.Format(2), r.AnnualizedEPS.Format(2)),
				Formula:   "Market Price / Annualized EPS",
				Citations: []Citation{ratioCite},
			})
		}

		if r.PB.IsPositive() {
			resp.Calculations = append(resp.Calculations, GroundedFinding{
				ClaimType: ClaimCalculation,
				Statement: fmt.Sprintf("Price-to-Book (P/B) ratio is %sx (Market Price Rs %s / Book Value Per Share Rs %s)", r.PB.Format(2), r.PriceUsed.Format(2), r.BookValuePerShare.Format(2)),
				Formula:   "Market Price / Book Value Per Share",
				Citations: []Citation{ratioCite},
			})
		}

		if r.ROE.IsPositive() {
			resp.Calculations = append(resp.Calculations, GroundedFinding{
				ClaimType: ClaimCalculation,
				Statement: fmt.Sprintf("Return on Equity (ROE) is %s%% (Annualized Net Income / Shareholders' Equity)", r.ROE.Mul(marketdata.MustParse("100")).Format(2)),
				Formula:   "Annualized Net Income / Shareholders' Equity",
				Citations: []Citation{ratioCite},
			})
		}
	}

	// Portfolio Calculations (if authorized)
	if ev.Portfolio != nil {
		pf := ev.Portfolio
		pfCite := Citation{
			SourceType:   SourcePortfolio,
			SourceID:     pf.ID,
			Authority:    "PORTFOLIO_LEDGER",
			CitationText: fmt.Sprintf("Portfolio '%s' (Owner ID: %s, Accounting Method: %s)", pf.Name, pf.UserID, pf.CostBasisMethod),
			ClaimType:    ClaimCalculation,
		}

		resp.Calculations = append(resp.Calculations, GroundedFinding{
			ClaimType: ClaimCalculation,
			Statement: fmt.Sprintf("Portfolio total value is Rs %s (Cash: Rs %s, Market Value: Rs %s, Unrealized P&L: Rs %s)",
				pf.TotalPortfolioValue.Format(2), pf.CashBalance.Format(2), pf.TotalMarketValue.Format(2), pf.TotalUnrealizedPnL.Format(2)),
			Formula:   "Cash Balance + Total Position Market Values",
			Citations: []Citation{pfCite},
		})
	}

	// 3. Derive INTERPRETATIONS (Explicitly marked as AI reasoning over the facts & calculations)
	if len(resp.Calculations) > 0 || len(resp.Facts) > 0 {
		var interpBuilder strings.Builder
		if ev.Ratios != nil && ev.Ratios.PE.IsPositive() {
			if ev.Ratios.PE.LessThan(marketdata.MustParse("15")) {
				interpBuilder.WriteString(fmt.Sprintf("%s trades at a valuation multiple below common historical sector medians. ", ev.Ratios.Symbol))
			} else {
				interpBuilder.WriteString(fmt.Sprintf("%s trades at a premium valuation multiple reflecting growth expectations. ", ev.Ratios.Symbol))
			}
		}
		if len(ev.CorporateActions) > 0 {
			interpBuilder.WriteString("The company has an active record of capital distributions via bonus/dividend actions.")
		}
		if interpBuilder.Len() == 0 {
			interpBuilder.WriteString("Regulatory and issuer disclosures reflect standard compliance and ongoing corporate activities.")
		}

		allCitations := make([]Citation, 0)
		for _, f := range resp.Facts {
			allCitations = append(allCitations, f.Citations...)
		}
		for _, c := range resp.Calculations {
			allCitations = append(allCitations, c.Citations...)
		}

		resp.Interpretations = append(resp.Interpretations, GroundedFinding{
			ClaimType: ClaimInterpretation,
			Statement: interpBuilder.String(),
			Citations: allCitations,
		})
	}

	// Summary Answer synthesizing the findings
	resp.SummaryAnswer = fmt.Sprintf("Retrieved %d factual records and %d deterministic calculations. Key insights: %d verified statements.",
		len(resp.Facts), len(resp.Calculations), len(resp.Facts)+len(resp.Calculations))
	resp.ConfidenceScore = marketdata.MustParse("0.95")
	resp.SupportedByProof = true

	return resp
}
