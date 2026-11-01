// Package httpapi wires the HTTP surface: routing, middleware and handlers.
package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"nepse/terminal/api/internal/auth"
	"nepse/terminal/api/internal/config"
	"nepse/terminal/api/internal/contracts"
	"nepse/terminal/api/internal/repositories"
)

const (
	accessCookie  = "np_at"
	refreshCookie = "np_rt"
	csrfCookie    = "np_csrf"
	maxBodyBytes  = 1 << 16 // 64 KiB is ample for auth payloads
)

// Server holds dependencies shared by all handlers.
type Server struct {
	cfg      *config.Config
	store    *repositories.Store
	tokens   *auth.Manager
	logger   *slog.Logger
	limiters *limiterMap
	schema   repositories.MigrationStatus
	started  time.Time
	dummyPwd string // verified against when an email is unknown, to equalise timing
}

// New builds a Server. It never panics on a missing dependency; it returns an
// error so main can exit with a clear message.
func New(cfg *config.Config, st *repositories.Store, logger *slog.Logger, schema repositories.MigrationStatus) (*Server, error) {
	dummy, err := auth.HashPassword("timing-equaliser-not-a-real-password")
	if err != nil {
		return nil, err
	}
	return &Server{
		cfg:      cfg,
		store:    st,
		tokens:   auth.NewManager(cfg.JWTSecret, cfg.JWTIssuer, cfg.AccessTTL),
		logger:   logger,
		limiters: newLimiterMap(cfg.RateLimitRPM),
		schema:   schema,
		started:  time.Now().UTC(),
		dummyPwd: dummy,
	}, nil
}

// Handler returns the fully wired router.
func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(requestID)
	r.Use(securityHeaders)
	r.Use(recoverer(s.logger))
	r.Use(requestLogger(s.logger))
	r.Use(cors(s.cfg))
	r.Use(rateLimit(s.limiters))

	// Operational endpoints are unauthenticated and outside /api so probes and
	// proxies can reach them without a token.
	r.Get("/healthz", s.handleLiveness)
	r.Get("/readyz", s.handleReadiness)
	r.Get("/openapi.yaml", s.handleOpenAPI)

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/openapi.yaml", s.handleOpenAPI)
		r.Get("/health", s.handleDataHealth)
		r.Get("/health/freshness", s.handleFreshness)
		r.Get("/health/ingest-runs", s.handleIngestRuns)
		r.Get("/health/archives", s.handleArchives)

		r.Get("/market/overview", s.handleOverview)
		r.Get("/market/status", s.handleMarketStatus)
		r.Get("/market/summary", s.handleSummary)
		r.Get("/market/top-movers", s.handleTopMovers)
		r.Get("/market/breadth", s.handleMarketBreadth)
		r.Get("/indices", s.handleIndices)
		r.Get("/indices/{code}/history", s.handleIndexHistory)
		r.Get("/indices/{code}/intraday", s.handleIndexIntraday)
		r.Get("/sectors", s.handleSectors)
		r.Get("/sectors/comparison", s.handleSectorComparison)
		r.Get("/floorsheet", s.handleFloorsheet)
		r.Get("/floorsheet/brokers", s.handleFloorsheetBrokers)
		r.Get("/floorsheet/coverage", s.handleFloorsheetCoverage)
		r.Get("/floorsheet/export.csv", s.handleFloorsheetExport)

		r.Get("/charts/ohlcv", s.handleOHLCV)
		r.Get("/charts/indicators", s.handleIndicators)
		r.Get("/charts/layouts", s.handleGetChartLayouts)
		r.Post("/charts/layouts", s.handleSaveChartLayout)
		r.Delete("/charts/layouts/{id}", s.handleDeleteChartLayout)

		r.Get("/companies", s.handleCompanies)
		r.Get("/companies/export.csv", s.handleCompaniesExport)
		r.Get("/companies/{symbol}", s.handleCompany)
		r.Get("/companies/{symbol}/prices", s.handlePrices)
		r.Get("/companies/{symbol}/prices.csv", s.handlePricesExport)
		r.Get("/companies/{symbol}/financials", s.handleGetFinancials)
		r.Post("/companies/{symbol}/financials", s.handlePostFinancials)
		r.Get("/companies/{symbol}/ratios", s.handleGetRatios)
		r.Get("/companies/{symbol}/corporate-actions", s.handleGetCorporateActions)
		r.Post("/companies/{symbol}/corporate-actions", s.handlePostCorporateAction)
		r.Get("/companies/{symbol}/adjusted-prices", s.handleGetAdjustedPrices)
		r.Get("/corporate-actions", s.handleGetCorporateActions)
		r.Get("/companies/{symbol}/profile", s.handleGetCompanyProfile)
		r.Get("/companies/{symbol}/board", s.handleGetBoard)
		r.Get("/companies/{symbol}/ownership", s.handleGetOwnership)
		r.Get("/mutual-funds", s.handleListMutualFunds)
		r.Get("/mutual-funds/{id}/nav", s.handleGetMutualFundNAV)
		r.Get("/data/coverage", s.handleDataCoverage)
		r.Get("/data/freshness", s.handleFreshness)
		r.Get("/disclosures", s.handleListDisclosures)
		r.Get("/disclosures/{id}", s.handleGetDisclosure)
		r.Post("/disclosures", s.handlePostDisclosure)
		r.Post("/research/query", s.handleResearchQuery)
		r.Get("/developer/licensing-matrix", s.handleGetLicensingMatrix)

		// Screener, saved screens, watchlists and alerts are per-user: they are
		// mounted inside one authenticated group, so there is no way to reach a
		// tenant's rows by guessing an id and no unauthenticated surface at all.
		r.Group(func(r chi.Router) {
			r.Use(s.requireAuth)

			r.Get("/screener/metrics", s.handleScreenerMetrics)
			r.With(requireCSRF).Post("/screener/run", s.handleScreenerRun)
			r.Get("/screener/screens", s.handleListScreens)
			r.With(requireCSRF).Post("/screener/screens", s.handleCreateScreen)
			r.Get("/screener/screens/{id}", s.handleGetScreen)
			r.With(requireCSRF).Put("/screener/screens/{id}", s.handleUpdateScreen)
			r.With(requireCSRF).Delete("/screener/screens/{id}", s.handleDeleteScreen)
			r.With(requireCSRF).Post("/screener/screens/{id}/run", s.handleRunSavedScreen)

			r.Get("/watchlists", s.handleListWatchlists)
			r.With(requireCSRF).Post("/watchlists", s.handleCreateWatchlist)
			r.Get("/watchlists/{id}", s.handleGetWatchlist)
			r.With(requireCSRF).Put("/watchlists/{id}", s.handleRenameWatchlist)
			r.With(requireCSRF).Delete("/watchlists/{id}", s.handleDeleteWatchlist)
			r.With(requireCSRF).Post("/watchlists/{id}/items", s.handleAddWatchlistItem)
			r.With(requireCSRF).Delete("/watchlists/{id}/items/{symbol}", s.handleRemoveWatchlistItem)

			r.Get("/alerts", s.handleListAlerts)
			r.With(requireCSRF).Post("/alerts", s.handleCreateAlert)
			r.With(requireCSRF).Put("/alerts/{id}", s.handleUpdateAlert)
			r.With(requireCSRF).Delete("/alerts/{id}", s.handleDeleteAlert)
			r.Get("/alerts/events", s.handleAlertEvents)
			r.Get("/alerts/deliveries", s.handleAlertDeliveries)
			r.With(requireCSRF).Post("/alerts/evaluate", s.handleEvaluateAlerts)

			r.Get("/portfolios", s.handleListPortfolios)
			r.With(requireCSRF).Post("/portfolios", s.handleCreatePortfolio)
			r.Get("/portfolios/{id}", s.handleGetPortfolio)
			r.With(requireCSRF).Delete("/portfolios/{id}", s.handleDeletePortfolio)
			r.Get("/portfolios/{id}/transactions", s.handleGetTransactions)
			r.With(requireCSRF).Post("/portfolios/{id}/transactions", s.handleRecordTransaction)
			r.Get("/portfolios/{id}/lots", s.handleGetLots)
			r.Get("/portfolios/{id}/analytics", s.handleGetPortfolioAnalytics)

			// Institutional Workspaces & Multi-tenant RBAC
			r.Get("/workspaces", s.handleListWorkspaces)
			r.With(requireCSRF).Post("/workspaces", s.handleCreateWorkspace)
			r.Get("/workspaces/{orgId}/members", s.handleListOrgMembers)
			r.With(requireCSRF).Post("/workspaces/{orgId}/members", s.handleAddOrgMember)
			r.Get("/workspaces/{orgId}/portfolios", s.handleListOrgPortfolios)
			r.With(requireCSRF).Post("/workspaces/{orgId}/portfolios", s.handleCreateOrgPortfolio)
			r.Get("/workspaces/{orgId}/notes", s.handleListNotes)
			r.With(requireCSRF).Post("/workspaces/{orgId}/notes", s.handleCreateNote)
			r.Get("/workspaces/{orgId}/approvals", s.handleListApprovals)
			r.With(requireCSRF).Post("/workspaces/{orgId}/approvals/{id}/review", s.handleReviewApproval)
			r.Get("/workspaces/{orgId}/portfolios/{portfolioId}/liquidity", s.handleLiquidityRisk)
			r.Get("/workspaces/{orgId}/portfolios/{portfolioId}/concentration", s.handleConcentrationRisk)
			r.With(requireCSRF).Post("/workspaces/{orgId}/portfolios/{portfolioId}/stress", s.handleStressTesting)
			r.Get("/workspaces/{orgId}/portfolios/{portfolioId}/export.csv", s.handleExportPortfolioCSV)
			r.Get("/workspaces/{orgId}/audit", s.handleListAuditLogs)

			// Developer API Keys & Exports
			r.Get("/developer/keys", s.handleListDeveloperKeys)
			r.With(requireCSRF).Post("/developer/keys", s.handleCreateDeveloperKey)
			r.With(requireCSRF).Delete("/developer/keys/{id}", s.handleRevokeDeveloperKey)
			r.With(requireCSRF).Post("/exports", s.handleCreateExportJob)
			r.Get("/exports", s.handleListExportJobs)
			r.Get("/exports/{id}", s.handleGetExportJob)
			r.Get("/exports/{id}/download", s.handleDownloadExport)
		})

		r.Route("/auth", func(r chi.Router) {
			r.Post("/register", s.handleRegister)
			r.Post("/login", s.handleLogin)
			r.Post("/refresh", s.handleRefresh)
			r.Post("/logout", s.handleLogout)

			r.Group(func(r chi.Router) {
				r.Use(s.requireAuth)
				r.Get("/me", s.handleMe)
				r.Get("/sessions", s.handleSessions)
				r.With(requireCSRF).Delete("/sessions/{id}", s.handleDeleteSession)
			})
		})
	})

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		Fail(w, r, http.StatusNotFound, CodeNotFound, "no such endpoint")
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		Fail(w, r, http.StatusMethodNotAllowed, CodeBadRequest, "method not allowed")
	})
	return r
}

func (s *Server) handleOpenAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(contracts.Spec)
}

// ---------------------------------------------------------------- health

func (s *Server) handleLiveness(w http.ResponseWriter, r *http.Request) {
	OK(w, r, map[string]any{
		"status":         "ok",
		"version":        contracts.Version,
		"uptime_seconds": int(time.Since(s.started).Seconds()),
	}, metaFrom(r))
}

func (s *Server) handleReadiness(w http.ResponseWriter, r *http.Request) {
	dbOK := s.store.Ping(r.Context()) == nil
	body := map[string]any{
		"database":   map[bool]string{true: "reachable", false: "unreachable"}[dbOK],
		"migrations": map[string]any{"applied": s.schema.Applied, "latest": s.schema.Latest},
	}
	if !dbOK {
		writeJSON(w, http.StatusServiceUnavailable, Envelope{
			Meta:  metaFrom(r),
			Error: &APIError{Code: CodeUnavailable, Message: "database unreachable", Retryable: true},
		})
		return
	}
	body["status"] = "ready"
	OK(w, r, body, metaFrom(r))
}

func (s *Server) handleDataHealth(w http.ResponseWriter, r *http.Request) {
	h, err := s.store.DataHealth(r.Context())
	if err != nil {
		s.internal(w, r, "data health", err)
		return
	}
	meta := metaFrom(r)
	meta.Source = "postgres"
	meta.LicenseClass = "public_unverified"
	if h.LatestIngest != "" {
		meta.AsOf = h.LatestIngest
	}
	OK(w, r, h, meta)
}

// ---------------------------------------------------------------- helpers

func (s *Server) internal(w http.ResponseWriter, r *http.Request, op string, err error) {
	// Log the cause, return a stable code: internal text never reaches the client.
	s.logger.Error("handler failed", "op", op, "error", err, "request_id", RequestIDFrom(r.Context()))
	Fail(w, r, http.StatusInternalServerError, CodeInternal, "internal error")
}

func decodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return errors.New("body must contain a single JSON object")
	}
	return nil
}

// pathParam reads a chi URL parameter.
func pathParam(r *http.Request, key string) string {
	return chi.URLParam(r, key)
}

func queryInt(r *http.Request, key string, def, min, max int) (int, error) {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return def, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, errors.New(key + " must be an integer")
	}
	if n < min || n > max {
		return 0, errors.New(key + " must be between " + strconv.Itoa(min) + " and " + strconv.Itoa(max))
	}
	return n, nil
}
