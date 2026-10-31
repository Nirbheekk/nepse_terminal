package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"nepse/terminal/api/internal/alerts"
	"nepse/terminal/api/internal/repositories"
	"nepse/terminal/api/internal/screener"
)

// userIDFrom returns the authenticated subject. Every handler below is mounted
// behind requireAuth, so a missing claim is a wiring bug rather than a client
// error — but it is still refused rather than defaulted, because a default would
// be a shared tenant.
func userIDFrom(r *http.Request) (string, bool) {
	claims, ok := ClaimsFrom(r.Context())
	if !ok || claims.Subject == "" {
		return "", false
	}
	return claims.Subject, true
}

// failStore maps the store's sentinel errors onto the public contract.
func (s *Server) failStore(w http.ResponseWriter, r *http.Request, op string, err error) {
	var verr *screener.ValidationError
	switch {
	case errors.As(err, &verr):
		Fail(w, r, http.StatusBadRequest, CodeValidation, verr.Error())
	case errors.Is(err, repositories.ErrNotFound):
		Fail(w, r, http.StatusNotFound, CodeNotFound, "not found")
	case errors.Is(err, repositories.ErrNameTaken):
		Fail(w, r, http.StatusConflict, CodeConflict, "a resource with that name already exists")
	case errors.Is(err, repositories.ErrLimitReached):
		Fail(w, r, http.StatusConflict, CodeConflict,
			fmt.Sprintf("watchlist is full (%d symbols)", repositories.MaxWatchlistItems))
	default:
		s.internal(w, r, op, err)
	}
}

// ---------------------------------------------------------------- metrics

// handleScreenerMetrics advertises the filter vocabulary and how much of it the
// stored data can actually answer today.
func (s *Server) handleScreenerMetrics(w http.ResponseWriter, r *http.Request) {
	coverage, err := s.store.ScreenerCoverage(r.Context())
	if err != nil {
		s.internal(w, r, "screener metrics", err)
		return
	}
	meta := metaFrom(r)
	meta.Source = "postgres:daily_ohlcv"
	meta.LicenseClass = "public_unverified"
	meta.Coverage = &Coverage{Covered: coverage.WithBars20, Eligible: coverage.Securities, Unit: "securities with 20+ sessions"}
	OK(w, r, map[string]any{
		"metrics":   screener.Metrics(),
		"operators": screener.OperatorKeys(),
		"coverage":  coverage,
	}, meta)
}

// ---------------------------------------------------------------- run

// decodeRule accepts either {"rule": {...}} or a bare rule object.
//
// Both are decoded with unknown fields rejected, so a misspelled key is a 400
// instead of a silently ignored filter that would widen the result.
func decodeRule(r *http.Request) (screener.Rule, error) {
	var rule screener.Rule
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	if err != nil {
		return rule, err
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return rule, errors.New("a rule is required")
	}

	raw := body
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(body, &envelope); err == nil {
		if inner, ok := envelope["rule"]; ok {
			raw = inner
		}
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&rule); err != nil {
		return rule, err
	}
	return rule, nil
}

// handleScreenerRun evaluates a rule without saving it.
func (s *Server) handleScreenerRun(w http.ResponseWriter, r *http.Request) {
	if _, ok := userIDFrom(r); !ok {
		unauthenticated(w, r)
		return
	}
	rule, err := decodeRule(r)
	if err != nil {
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, "invalid rule: "+err.Error())
		return
	}
	s.runAndRespond(w, r, rule)
}

func (s *Server) runAndRespond(w http.ResponseWriter, r *http.Request, rule screener.Rule) {
	normalised, err := screener.Normalize(rule)
	if err != nil {
		s.failStore(w, r, "screener run", err)
		return
	}
	rows, err := s.store.RunScreen(r.Context(), normalised)
	if err != nil {
		s.failStore(w, r, "screener run", err)
		return
	}
	coverage, err := s.store.ScreenerCoverage(r.Context())
	if err != nil {
		s.internal(w, r, "screener coverage", err)
		return
	}

	meta := metaFrom(r)
	meta.Source = "postgres:daily_ohlcv"
	meta.LicenseClass = "public_unverified"
	meta.Method = "filtered over the latest stored session"
	total := len(rows)
	meta.Total = &total
	if coverage.LatestSession != "" {
		meta.SessionDate = coverage.LatestSession
	}
	OK(w, r, map[string]any{
		"rule":     normalised,
		"rows":     rows,
		"returned": len(rows),
		"coverage": coverage,
	}, meta)
}

// ---------------------------------------------------------------- saved screens

type screenRequest struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Definition  json.RawMessage `json:"definition"`
}

// decodeScreen validates the envelope and the rule inside it.
func decodeScreen(r *http.Request) (string, string, []byte, screener.Rule, error) {
	var req screenRequest
	if err := decodeJSON(r, &req); err != nil {
		return "", "", nil, screener.Rule{}, &screener.ValidationError{Field: "body", Message: err.Error()}
	}
	name := strings.TrimSpace(req.Name)
	if name == "" || len(name) > 80 {
		return "", "", nil, screener.Rule{}, &screener.ValidationError{
			Field: "name", Message: "must be 1–80 characters"}
	}
	desc := strings.TrimSpace(req.Description)
	if len(desc) > 500 {
		return "", "", nil, screener.Rule{}, &screener.ValidationError{
			Field: "description", Message: "must be at most 500 characters"}
	}
	var rule screener.Rule
	if len(req.Definition) == 0 {
		return "", "", nil, rule, &screener.ValidationError{
			Field: "definition", Message: "is required"}
	}
	if err := json.Unmarshal(req.Definition, &rule); err != nil {
		return "", "", nil, rule, &screener.ValidationError{
			Field: "definition", Message: err.Error()}
	}
	normalised, err := screener.Normalize(rule)
	if err != nil {
		return "", "", nil, rule, err
	}
	body, err := json.Marshal(normalised)
	if err != nil {
		return "", "", nil, rule, err
	}
	return name, desc, body, normalised, nil
}

func (s *Server) handleListScreens(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r)
	if !ok {
		unauthenticated(w, r)
		return
	}
	screens, err := s.store.ListScreens(r.Context(), uid)
	if err != nil {
		s.internal(w, r, "list screens", err)
		return
	}
	total := len(screens)
	meta := metaFrom(r)
	meta.Total = &total
	OK(w, r, screens, meta)
}

func (s *Server) handleCreateScreen(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r)
	if !ok {
		unauthenticated(w, r)
		return
	}
	name, desc, body, _, err := decodeScreen(r)
	if err != nil {
		s.failStore(w, r, "create screen", err)
		return
	}
	screen, err := s.store.CreateScreen(r.Context(), uid, name, desc, body)
	if err != nil {
		s.failStore(w, r, "create screen", err)
		return
	}
	Succeed(w, r, http.StatusCreated, screen, metaFrom(r))
}

func (s *Server) handleGetScreen(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r)
	if !ok {
		unauthenticated(w, r)
		return
	}
	screen, err := s.store.GetScreen(r.Context(), uid, pathParam(r, "id"))
	if err != nil {
		s.failStore(w, r, "get screen", err)
		return
	}
	OK(w, r, screen, metaFrom(r))
}

func (s *Server) handleUpdateScreen(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r)
	if !ok {
		unauthenticated(w, r)
		return
	}
	name, desc, body, _, err := decodeScreen(r)
	if err != nil {
		s.failStore(w, r, "update screen", err)
		return
	}
	screen, err := s.store.UpdateScreen(r.Context(), uid, pathParam(r, "id"), name, desc, body)
	if err != nil {
		s.failStore(w, r, "update screen", err)
		return
	}
	OK(w, r, screen, metaFrom(r))
}

func (s *Server) handleDeleteScreen(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r)
	if !ok {
		unauthenticated(w, r)
		return
	}
	deleted, err := s.store.DeleteScreen(r.Context(), uid, pathParam(r, "id"))
	if err != nil {
		s.internal(w, r, "delete screen", err)
		return
	}
	if !deleted {
		Fail(w, r, http.StatusNotFound, CodeNotFound, "not found")
		return
	}
	OK(w, r, map[string]any{"deleted": true}, metaFrom(r))
}

// handleRunSavedScreen runs a stored screen, so the UI can refresh one without
// sending its definition back.
func (s *Server) handleRunSavedScreen(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r)
	if !ok {
		unauthenticated(w, r)
		return
	}
	saved, err := s.store.GetScreen(r.Context(), uid, pathParam(r, "id"))
	if err != nil {
		s.failStore(w, r, "run saved screen", err)
		return
	}
	var rule screener.Rule
	if err := json.Unmarshal(saved.Definition, &rule); err != nil {
		s.internal(w, r, "run saved screen", err)
		return
	}
	s.runAndRespond(w, r, rule)
}

// ---------------------------------------------------------------- watchlists

func (s *Server) handleListWatchlists(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r)
	if !ok {
		unauthenticated(w, r)
		return
	}
	lists, err := s.store.ListWatchlists(r.Context(), uid)
	if err != nil {
		s.internal(w, r, "list watchlists", err)
		return
	}
	total := len(lists)
	meta := metaFrom(r)
	meta.Total = &total
	OK(w, r, lists, meta)
}

func (s *Server) handleCreateWatchlist(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r)
	if !ok {
		unauthenticated(w, r)
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(r, &req); err != nil {
		badRequest(w, r, "invalid payload: "+err.Error())
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" || len(name) > 80 {
		badRequest(w, r, "name must be 1–80 characters")
		return
	}
	list, err := s.store.CreateWatchlist(r.Context(), uid, name)
	if err != nil {
		s.failStore(w, r, "create watchlist", err)
		return
	}
	Succeed(w, r, http.StatusCreated, list, metaFrom(r))
}

func (s *Server) handleGetWatchlist(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r)
	if !ok {
		unauthenticated(w, r)
		return
	}
	list, err := s.store.GetWatchlist(r.Context(), uid, pathParam(r, "id"))
	if err != nil {
		s.failStore(w, r, "get watchlist", err)
		return
	}
	OK(w, r, list, metaFrom(r))
}

func (s *Server) handleRenameWatchlist(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r)
	if !ok {
		unauthenticated(w, r)
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(r, &req); err != nil {
		badRequest(w, r, "invalid payload: "+err.Error())
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" || len(name) > 80 {
		badRequest(w, r, "name must be 1–80 characters")
		return
	}
	list, err := s.store.RenameWatchlist(r.Context(), uid, pathParam(r, "id"), name)
	if err != nil {
		s.failStore(w, r, "rename watchlist", err)
		return
	}
	OK(w, r, list, metaFrom(r))
}

func (s *Server) handleDeleteWatchlist(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r)
	if !ok {
		unauthenticated(w, r)
		return
	}
	deleted, err := s.store.DeleteWatchlist(r.Context(), uid, pathParam(r, "id"))
	if err != nil {
		s.internal(w, r, "delete watchlist", err)
		return
	}
	if !deleted {
		Fail(w, r, http.StatusNotFound, CodeNotFound, "not found")
		return
	}
	OK(w, r, map[string]any{"deleted": true}, metaFrom(r))
}

func (s *Server) handleAddWatchlistItem(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r)
	if !ok {
		unauthenticated(w, r)
		return
	}
	var req struct {
		Symbol string `json:"symbol"`
		Note   string `json:"note"`
	}
	if err := decodeJSON(r, &req); err != nil {
		badRequest(w, r, "invalid payload: "+err.Error())
		return
	}
	symbol := strings.ToUpper(strings.TrimSpace(req.Symbol))
	if !symbolRe.MatchString(symbol) {
		badRequest(w, r, "symbol is not valid")
		return
	}
	note := strings.TrimSpace(req.Note)
	if len(note) > 200 {
		badRequest(w, r, "note must be at most 200 characters")
		return
	}
	added, err := s.store.AddWatchlistItem(r.Context(), uid, pathParam(r, "id"), symbol, note)
	if err != nil {
		s.failStore(w, r, "add watchlist item", err)
		return
	}
	status := http.StatusOK
	if added {
		status = http.StatusCreated
	}
	Succeed(w, r, status, map[string]any{"symbol": symbol, "added": added}, metaFrom(r))
}

func (s *Server) handleRemoveWatchlistItem(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r)
	if !ok {
		unauthenticated(w, r)
		return
	}
	symbol := strings.ToUpper(strings.TrimSpace(pathParam(r, "symbol")))
	removed, err := s.store.RemoveWatchlistItem(r.Context(), uid, pathParam(r, "id"), symbol)
	if err != nil {
		s.failStore(w, r, "remove watchlist item", err)
		return
	}
	if !removed {
		Fail(w, r, http.StatusNotFound, CodeNotFound, "symbol is not in this watchlist")
		return
	}
	OK(w, r, map[string]any{"symbol": symbol, "removed": true}, metaFrom(r))
}

// ---------------------------------------------------------------- alerts

type alertRequest struct {
	Name       string          `json:"name"`
	Symbol     *string         `json:"symbol"`
	Screen     json.RawMessage `json:"screen"`
	Metric     string          `json:"metric"`
	Comparator string          `json:"comparator"`
	// Threshold accepts a JSON number or a numeric string: a client writing
	// {"threshold": 415.25} is doing the natural thing, and refusing it because
	// the column is NUMERIC would be an implementation detail leaking out.
	Threshold       screener.Number `json:"threshold"`
	CooldownSeconds int             `json:"cooldown_seconds"`
	MaxAttempts     int             `json:"max_attempts"`
	Enabled         *bool           `json:"enabled"`
}

// buildAlertRule validates an alert request into a storable rule.
func buildAlertRule(req alertRequest) (*repositories.AlertRule, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" || len(name) > 80 {
		return nil, &screener.ValidationError{Field: "name", Message: "must be 1–80 characters"}
	}
	metric, ok := screener.LookupMetric(strings.TrimSpace(req.Metric))
	if !ok {
		return nil, &screener.ValidationError{Field: "metric", Message: "is not a filterable metric"}
	}
	comparator := strings.ToLower(strings.TrimSpace(req.Comparator))
	switch comparator {
	case "gt", "gte", "lt", "lte", "eq":
	default:
		return nil, &screener.ValidationError{Field: "comparator",
			Message: "must be one of gt, gte, lt, lte, eq"}
	}
	threshold := strings.TrimSpace(string(req.Threshold))
	if threshold == "" {
		return nil, &screener.ValidationError{Field: "threshold", Message: "is required"}
	}
	if _, err := screener.Satisfies(comparator, threshold, threshold); err != nil {
		return nil, &screener.ValidationError{Field: "threshold", Message: "must be numeric"}
	}

	rule := &repositories.AlertRule{
		Name: name, Metric: metric.Key, Comparator: comparator, Threshold: threshold,
		CooldownSeconds: req.CooldownSeconds, MaxAttempts: req.MaxAttempts, Enabled: true,
	}
	if req.Enabled != nil {
		rule.Enabled = *req.Enabled
	}
	if rule.CooldownSeconds == 0 {
		rule.CooldownSeconds = int(alerts.DefaultCooldown.Seconds())
	}
	if rule.CooldownSeconds < 0 || time.Duration(rule.CooldownSeconds)*1e9 > alerts.MaxCooldown {
		return nil, &screener.ValidationError{Field: "cooldown_seconds",
			Message: "must be between 0 and 604800"}
	}
	if rule.MaxAttempts == 0 {
		rule.MaxAttempts = alerts.DefaultAttempts
	}
	if rule.MaxAttempts < 1 || rule.MaxAttempts > alerts.MaxAttempts {
		return nil, &screener.ValidationError{Field: "max_attempts",
			Message: "must be between 1 and 10"}
	}

	if req.Symbol != nil {
		sym := strings.ToUpper(strings.TrimSpace(*req.Symbol))
		if sym != "" {
			if !symbolRe.MatchString(sym) {
				return nil, &screener.ValidationError{Field: "symbol", Message: "is not a valid symbol"}
			}
			rule.Symbol = &sym
		}
	}
	if len(req.Screen) > 0 && string(req.Screen) != "null" {
		var screen screener.Rule
		if err := json.Unmarshal(req.Screen, &screen); err != nil {
			return nil, &screener.ValidationError{Field: "screen", Message: err.Error()}
		}
		normalised, err := screener.Normalize(screen)
		if err != nil {
			return nil, err
		}
		body, err := json.Marshal(normalised)
		if err != nil {
			return nil, err
		}
		rule.Screen = body
	}
	if rule.Symbol == nil && rule.Screen == nil {
		return nil, &screener.ValidationError{Field: "symbol",
			Message: "an alert needs a symbol or a screen"}
	}
	return rule, nil
}

func (s *Server) handleListAlerts(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r)
	if !ok {
		unauthenticated(w, r)
		return
	}
	rules, err := s.store.ListAlertRules(r.Context(), uid)
	if err != nil {
		s.internal(w, r, "list alerts", err)
		return
	}
	total := len(rules)
	meta := metaFrom(r)
	meta.Total = &total
	OK(w, r, rules, meta)
}

func (s *Server) handleCreateAlert(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r)
	if !ok {
		unauthenticated(w, r)
		return
	}
	var req alertRequest
	if err := decodeJSON(r, &req); err != nil {
		badRequest(w, r, "invalid payload: "+err.Error())
		return
	}
	rule, err := buildAlertRule(req)
	if err != nil {
		s.failStore(w, r, "create alert", err)
		return
	}
	created, err := s.store.CreateAlertRule(r.Context(), uid, rule)
	if err != nil {
		s.failStore(w, r, "create alert", err)
		return
	}
	Succeed(w, r, http.StatusCreated, created, metaFrom(r))
}

func (s *Server) handleUpdateAlert(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r)
	if !ok {
		unauthenticated(w, r)
		return
	}
	// The update is a full replacement so a partial payload cannot silently
	// change a rule's subject: "enabled: false" alone must not blank the metric.
	existing, err := s.store.GetAlertRule(r.Context(), uid, pathParam(r, "id"))
	if err != nil {
		s.failStore(w, r, "get alert", err)
		return
	}
	var req alertRequest
	if err := decodeJSON(r, &req); err != nil {
		badRequest(w, r, "invalid payload: "+err.Error())
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		req.Name = existing.Name
	}
	if strings.TrimSpace(req.Metric) == "" {
		req.Metric = existing.Metric
	}
	if strings.TrimSpace(req.Comparator) == "" {
		req.Comparator = existing.Comparator
	}
	if strings.TrimSpace(string(req.Threshold)) == "" {
		req.Threshold = screener.Number(existing.Threshold)
	}
	if req.Symbol == nil {
		req.Symbol = existing.Symbol
	}
	if len(req.Screen) == 0 {
		req.Screen = existing.Screen
	}
	if req.CooldownSeconds == 0 {
		req.CooldownSeconds = existing.CooldownSeconds
	}
	if req.MaxAttempts == 0 {
		req.MaxAttempts = existing.MaxAttempts
	}
	if req.Enabled == nil {
		enabled := existing.Enabled
		req.Enabled = &enabled
	}
	rule, err := buildAlertRule(req)
	if err != nil {
		s.failStore(w, r, "update alert", err)
		return
	}
	updated, err := s.store.UpdateAlertRule(r.Context(), uid, existing.ID, rule)
	if err != nil {
		s.failStore(w, r, "update alert", err)
		return
	}
	OK(w, r, updated, metaFrom(r))
}

func (s *Server) handleDeleteAlert(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r)
	if !ok {
		unauthenticated(w, r)
		return
	}
	deleted, err := s.store.DeleteAlertRule(r.Context(), uid, pathParam(r, "id"))
	if err != nil {
		s.internal(w, r, "delete alert", err)
		return
	}
	if !deleted {
		Fail(w, r, http.StatusNotFound, CodeNotFound, "not found")
		return
	}
	OK(w, r, map[string]any{"deleted": true}, metaFrom(r))
}

func (s *Server) handleAlertEvents(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r)
	if !ok {
		unauthenticated(w, r)
		return
	}
	limit, err := queryInt(r, "limit", 100, 1, 500)
	if err != nil {
		badRequest(w, r, err.Error())
		return
	}
	events, err := s.store.ListAlertEvents(r.Context(), uid, limit)
	if err != nil {
		s.internal(w, r, "alert events", err)
		return
	}
	total := len(events)
	meta := metaFrom(r)
	meta.Total = &total
	OK(w, r, events, meta)
}

func (s *Server) handleAlertDeliveries(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r)
	if !ok {
		unauthenticated(w, r)
		return
	}
	limit, err := queryInt(r, "limit", 100, 1, 500)
	if err != nil {
		badRequest(w, r, err.Error())
		return
	}
	deliveries, err := s.store.ListDeliveries(r.Context(), uid, limit)
	if err != nil {
		s.internal(w, r, "alert deliveries", err)
		return
	}
	total := len(deliveries)
	meta := metaFrom(r)
	meta.Total = &total
	OK(w, r, deliveries, meta)
}

// handleEvaluateAlerts runs the caller's own rules now. The worker runs every
// tenant's rules on a schedule; this endpoint exists so a user can confirm their
// rule works without waiting for the next cycle — and it can only ever see their
// own data, because the engine is handed one user id.
func (s *Server) handleEvaluateAlerts(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r)
	if !ok {
		unauthenticated(w, r)
		return
	}
	engine := &alerts.Engine{Store: s.store, Logger: s.logger}
	res, err := engine.EvaluateForUser(r.Context(), uid)
	if err != nil {
		s.internal(w, r, "evaluate alerts", err)
		return
	}
	meta := metaFrom(r)
	meta.Method = "in-app delivery"
	OK(w, r, res, meta)
}
