package research

import (
	"context"
	"strings"
	"testing"
	"time"

	"nepse/terminal/api/internal/marketdata"
	"nepse/terminal/api/internal/portfolios"
	"nepse/terminal/api/internal/services"
)

type mockRetriever struct {
	disclosures []services.Disclosure
	ratios      *services.PeriodRatios
	portfolio   *portfolios.PortfolioState
}

func (m *mockRetriever) GetDisclosures(ctx context.Context, symbol, authority string, limit int) ([]services.Disclosure, error) {
	return m.disclosures, nil
}
func (m *mockRetriever) GetFinancialStatements(ctx context.Context, symbol string) ([]*services.Statement, error) {
	return nil, nil
}
func (m *mockRetriever) GetRatios(ctx context.Context, symbol string, asOfDate string) (*services.PeriodRatios, error) {
	return m.ratios, nil
}
func (m *mockRetriever) GetCorporateActions(ctx context.Context, symbol, actionType string) ([]services.CorporateAction, error) {
	return nil, nil
}
func (m *mockRetriever) GetLatestBar(ctx context.Context, symbol string) (*services.OHLCVBar, error) {
	return nil, nil
}
func (m *mockRetriever) GetPortfolio(ctx context.Context, portfolioID, userID string) (*portfolios.PortfolioState, error) {
	if m.portfolio != nil && m.portfolio.ID == portfolioID && m.portfolio.UserID == userID {
		return m.portfolio, nil
	}
	return nil, ErrUnauthorizedPortfolioAccess
}

func TestTripartiteEvidenceGrounding(t *testing.T) {
	retriever := &mockRetriever{
		disclosures: []services.Disclosure{
			{
				ID:             "d1",
				Authority:      services.AuthoritySEBON,
				Category:       services.CategoryCircular,
				Title:          "Directive on IPO Allotment",
				PublishedAt:    time.Date(2024, 6, 1, 10, 0, 0, 0, time.UTC),
				PublishedAtNPT: "2024-06-01 15:45:00 NPT",
			},
		},
		ratios: &services.PeriodRatios{
			Symbol:        "NABIL",
			FiscalYear:    "2080/81",
			PeriodType:    services.PeriodAnnual,
			PriceUsed:     marketdata.MustParse("240"),
			AnnualizedEPS: marketdata.MustParse("12"),
			PE:            marketdata.MustParse("20"),
			AsOfDate:      "2024-07-20",
		},
	}

	assistant := NewAssistant(retriever)
	resp, err := assistant.Execute(context.Background(), ResearchQuery{
		QueryText: "Analyze NABIL valuation and SEBON rules",
		Symbol:    "NABIL",
		Scope:     "company",
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !resp.SupportedByProof {
		t.Error("expected SupportedByProof to be true")
	}

	// 1. Verify FACTS
	if len(resp.Facts) == 0 {
		t.Error("expected at least 1 FACT from disclosures")
	}
	for _, f := range resp.Facts {
		if f.ClaimType != ClaimFact {
			t.Errorf("expected ClaimFact, got %s", f.ClaimType)
		}
		if len(f.Citations) == 0 {
			t.Error("FACT must have at least one citation")
		}
	}

	// 2. Verify CALCULATIONS
	if len(resp.Calculations) == 0 {
		t.Error("expected at least 1 CALCULATION from ratios")
	}
	for _, c := range resp.Calculations {
		if c.ClaimType != ClaimCalculation {
			t.Errorf("expected ClaimCalculation, got %s", c.ClaimType)
		}
		if c.Formula == "" {
			t.Error("CALCULATION must specify its tested formula")
		}
	}

	// 3. Verify INTERPRETATIONS
	if len(resp.Interpretations) == 0 {
		t.Error("expected INTERPRETATION based on facts/calculations")
	}
	for _, i := range resp.Interpretations {
		if i.ClaimType != ClaimInterpretation {
			t.Errorf("expected ClaimInterpretation, got %s", i.ClaimType)
		}
	}
}

func TestPreventsUnsupportedClaims(t *testing.T) {
	// Empty retriever with zero evidence
	emptyRetriever := &mockRetriever{}
	assistant := NewAssistant(emptyRetriever)

	resp, err := assistant.Execute(context.Background(), ResearchQuery{
		QueryText: "What is the secret merger between Bank A and Bank B?",
		Symbol:    "BANKA",
		Scope:     "company",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.SupportedByProof {
		t.Error("expected SupportedByProof to be false when no records exist")
	}
	if !resp.ConfidenceScore.IsZero() {
		t.Errorf("expected 0 confidence score, got %s", resp.ConfidenceScore.String())
	}
	if !strings.Contains(resp.SummaryAnswer, "No authorized NEPSE, SEBON, NRB or company disclosures found") {
		t.Errorf("expected hallucination refusal message, got: %s", resp.SummaryAnswer)
	}
}

func TestPreventsUnauthorizedPortfolioDisclosure(t *testing.T) {
	secretPf := portfolios.NewPortfolioState("pf-secret-99", "user-victim", "Private Wealth", portfolios.MethodWAC)
	secretPf.TotalPortfolioValue = marketdata.MustParse("50000000.00") // Rs 5 Crore

	retriever := &mockRetriever{
		portfolio: secretPf,
	}
	assistant := NewAssistant(retriever)

	// Attacker trying to query victim's portfolio
	_, err := assistant.Execute(context.Background(), ResearchQuery{
		UserID:      "user-attacker", // Wrong user ID
		QueryText:   "How much money does user-victim have in pf-secret-99?",
		Scope:       "portfolio",
		PortfolioID: "pf-secret-99",
	})

	if err == nil {
		t.Fatal("expected unauthorized portfolio access error, got nil")
	}
	if !strings.Contains(err.Error(), "unauthorized access to private portfolio data") {
		t.Errorf("expected ErrUnauthorizedPortfolioAccess, got: %v", err)
	}
}
