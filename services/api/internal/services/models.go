package services

import (
	"time"

	"nepse/terminal/api/internal/marketdata"
)

type OrganizationPlan string

const (
	PlanTrial         OrganizationPlan = "trial"
	PlanPro           OrganizationPlan = "pro"
	PlanInstitutional OrganizationPlan = "institutional"
	PlanEnterprise    OrganizationPlan = "enterprise"
)

// Organization represents an institutional customer tenant.
type Organization struct {
	ID        string           `json:"id"`
	Name      string           `json:"name"`
	Slug      string           `json:"slug"`
	Plan      OrganizationPlan `json:"plan"`
	Status    string           `json:"status"` // active, suspended, archived
	CreatedAt time.Time        `json:"created_at"`
	UpdatedAt time.Time        `json:"updated_at"`
}

// OrganizationMember represents a user within an organization with an assigned RBAC role.
type OrganizationMember struct {
	OrganizationID string    `json:"organization_id"`
	UserID         string    `json:"user_id"`
	Email          string    `json:"email,omitempty"`
	DisplayName    string    `json:"display_name,omitempty"`
	Role           Role      `json:"role"`
	JoinedAt       time.Time `json:"joined_at"`
}

// ResearchNote represents an institutional investment thesis or stock note.
type ResearchNote struct {
	ID          string    `json:"id"`
	OrgID       string    `json:"org_id"`
	AuthorID    string    `json:"author_id"`
	AuthorName  string    `json:"author_name,omitempty"`
	Symbol      string    `json:"symbol,omitempty"`
	PortfolioID string    `json:"portfolio_id,omitempty"`
	Title       string    `json:"title"`
	Summary     string    `json:"summary"`
	Body        string    `json:"body"`
	Tags        []string  `json:"tags"`
	Visibility  string    `json:"visibility"` // internal_team, restricted, published
	Status      string    `json:"status"`     // draft, under_review, approved, archived
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ApprovalActionType names the kind of change an approval request covers.
//
// It is deliberately distinct from ActionType, which describes a corporate
// action (dividend, split, ...): the two merged into one package from separate
// subsystems and answer different questions.
type ApprovalActionType string

const (
	ActionLargeTrade        ApprovalActionType = "large_trade"
	ActionRebalance         ApprovalActionType = "rebalance"
	ActionNotePublish       ApprovalActionType = "note_publish"
	ActionCashWithdrawal    ApprovalActionType = "cash_withdrawal"
	ActionRiskLimitOverride ApprovalActionType = "risk_limit_override"
)

type ApprovalStatus string

const (
	ApprovalPending   ApprovalStatus = "pending"
	ApprovalApproved  ApprovalStatus = "approved"
	ApprovalRejected  ApprovalStatus = "rejected"
	ApprovalCancelled ApprovalStatus = "cancelled"
)

// ApprovalRequest implements four-eyes / maker-checker workflows.
type ApprovalRequest struct {
	ID            string             `json:"id"`
	OrgID         string             `json:"org_id"`
	PortfolioID   string             `json:"portfolio_id,omitempty"`
	RequesterID   string             `json:"requester_id"`
	RequesterName string             `json:"requester_name,omitempty"`
	ActionType    ApprovalActionType `json:"action_type"`
	Payload       map[string]any     `json:"payload"`
	Status        ApprovalStatus     `json:"status"`
	ReviewerID    string             `json:"reviewer_id,omitempty"`
	ReviewerName  string             `json:"reviewer_name,omitempty"`
	ReviewedAt    *time.Time         `json:"reviewed_at,omitempty"`
	ReviewNotes   string             `json:"review_notes,omitempty"`
	CreatedAt     time.Time          `json:"created_at"`
}

// ScheduledReport defines recurring institutional reports.
type ScheduledReport struct {
	ID          string         `json:"id"`
	OrgID       string         `json:"org_id"`
	PortfolioID string         `json:"portfolio_id"`
	CreatedBy   string         `json:"created_by"`
	Name        string         `json:"name"`
	ReportType  string         `json:"report_type"` // daily_pnl, weekly_risk, monthly_executive_summary, compliance_audit, concentration_liquidity
	Schedule    string         `json:"schedule"`    // daily, weekly, monthly
	Recipients  []string       `json:"recipients"`
	Parameters  map[string]any `json:"parameters,omitempty"`
	IsActive    bool           `json:"is_active"`
	LastRunAt   *time.Time     `json:"last_run_at,omitempty"`
	NextRunAt   *time.Time     `json:"next_run_at,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
}

// AuditLogEntry records an immutable action in an institutional workspace.
type AuditLogEntry struct {
	ID           int64          `json:"id"`
	OrgID        string         `json:"org_id"`
	ActorID      string         `json:"actor_id"`
	ActorName    string         `json:"actor_name,omitempty"`
	Action       string         `json:"action"`
	ResourceType string         `json:"resource_type"`
	ResourceID   string         `json:"resource_id"`
	Detail       map[string]any `json:"detail,omitempty"`
	IPAddress    string         `json:"ip_address,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
}

// ReportSnapshot holds the generated contents of an automated scheduled report.
type ReportSnapshot struct {
	OrgID         string             `json:"org_id"`
	PortfolioID   string             `json:"portfolio_id"`
	GeneratedAt   string             `json:"generated_at"`
	ReportType    string             `json:"report_type"`
	TotalNAV      marketdata.Decimal `json:"total_nav"`
	CashBalance   marketdata.Decimal `json:"cash_balance"`
	TopHolding    string             `json:"top_holding"`
	Top1Pct       marketdata.Decimal `json:"top1_pct"`
	HHI           marketdata.Decimal `json:"hhi"`
	WeightedDTL   marketdata.Decimal `json:"weighted_dtl"`
	TotalBreaches int                `json:"total_breaches"`
	Breaches      []LimitBreach      `json:"breaches,omitempty"`
	StressLoss    marketdata.Decimal `json:"stress_loss,omitempty"`
}
