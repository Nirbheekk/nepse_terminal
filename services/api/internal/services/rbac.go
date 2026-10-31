// Package institutional implements organization-aware institutional workspaces,
// role-based access control (RBAC), multi-portfolio management, research notes,
// concentration and liquidity risk engines, stress-testing scenarios, approvals,
// scheduled reporting, and audit trails.
package services

type Role string

const (
	RoleOrgAdmin          Role = "org_admin"
	RolePortfolioManager  Role = "portfolio_manager"
	RoleAnalyst           Role = "analyst"
	RoleComplianceOfficer Role = "compliance_officer"
	RoleViewer            Role = "viewer"
)

type Permission string

const (
	PermPortfolioRead  Permission = "portfolio:read"
	PermPortfolioWrite Permission = "portfolio:write"
	PermTradeRecord    Permission = "trade:record"
	PermTradeApprove   Permission = "trade:approve"
	PermNoteCreate     Permission = "note:create"
	PermNotePublish    Permission = "note:publish"
	PermRiskView       Permission = "risk:view"
	PermScenarioRun    Permission = "scenario:run"
	PermReportSchedule Permission = "report:schedule"
	PermReportExport   Permission = "report:export" // Export permission gate
	PermAuditRead      Permission = "audit:read"
	PermMemberManage   Permission = "member:manage"
)

// rolePermissions maps each role to its granted permissions.
var rolePermissions = map[Role]map[Permission]bool{
	RoleOrgAdmin: {
		PermPortfolioRead:  true,
		PermPortfolioWrite: true,
		PermTradeRecord:    true,
		PermTradeApprove:   true,
		PermNoteCreate:     true,
		PermNotePublish:    true,
		PermRiskView:       true,
		PermScenarioRun:    true,
		PermReportSchedule: true,
		PermReportExport:   true,
		PermAuditRead:      true,
		PermMemberManage:   true,
	},
	RolePortfolioManager: {
		PermPortfolioRead:  true,
		PermPortfolioWrite: true,
		PermTradeRecord:    true,
		PermTradeApprove:   false, // Cannot approve own or others' trades (compliance/admin only)
		PermNoteCreate:     true,
		PermNotePublish:    true,
		PermRiskView:       true,
		PermScenarioRun:    true,
		PermReportSchedule: true,
		PermReportExport:   true,
		PermAuditRead:      true,
		PermMemberManage:   false,
	},
	RoleAnalyst: {
		PermPortfolioRead:  true,
		PermPortfolioWrite: false,
		PermTradeRecord:    false,
		PermTradeApprove:   false,
		PermNoteCreate:     true,
		PermNotePublish:    false, // Notes require compliance/manager review to publish
		PermRiskView:       true,
		PermScenarioRun:    true,
		PermReportSchedule: false,
		PermReportExport:   false,
		PermAuditRead:      false,
		PermMemberManage:   false,
	},
	RoleComplianceOfficer: {
		PermPortfolioRead:  true,
		PermPortfolioWrite: false,
		PermTradeRecord:    false,
		PermTradeApprove:   true, // Dedicated approval authority
		PermNoteCreate:     false,
		PermNotePublish:    true, // Can approve and publish research notes
		PermRiskView:       true,
		PermScenarioRun:    true,
		PermReportSchedule: true,
		PermReportExport:   true,
		PermAuditRead:      true,
		PermMemberManage:   false,
	},
	RoleViewer: {
		PermPortfolioRead:  true,
		PermPortfolioWrite: false,
		PermTradeRecord:    false,
		PermTradeApprove:   false,
		PermNoteCreate:     false,
		PermNotePublish:    false,
		PermRiskView:       true,
		PermScenarioRun:    false,
		PermReportSchedule: false,
		PermReportExport:   false,
		PermAuditRead:      false,
		PermMemberManage:   false,
	},
}

// HasPermission checks whether a given role holds the specified permission.
func HasPermission(role Role, perm Permission) bool {
	perms, ok := rolePermissions[role]
	if !ok {
		return false
	}
	return perms[perm]
}
