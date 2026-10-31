package repositories

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// AlertRule is one tenant's standing condition: "tell me when <metric> crosses
// <threshold>", either for a named symbol or for everything a screen matches.
type AlertRule struct {
	ID              string          `json:"id"`
	Name            string          `json:"name"`
	Symbol          *string         `json:"symbol,omitempty"`
	Screen          json.RawMessage `json:"screen,omitempty"`
	Metric          string          `json:"metric"`
	Comparator      string          `json:"comparator"`
	Threshold       string          `json:"threshold"`
	CooldownSeconds int             `json:"cooldown_seconds"`
	MaxAttempts     int             `json:"max_attempts"`
	Enabled         bool            `json:"enabled"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

// AlertEvent is a deduplicated observation: a rule, a symbol, a session and the
// value that satisfied it. The fingerprint unique constraint means re-evaluating
// the same rule against the same session cannot create a second event.
type AlertEvent struct {
	ID              string    `json:"id"`
	RuleID          string    `json:"rule_id"`
	RuleName        string    `json:"rule_name"`
	Symbol          string    `json:"symbol"`
	SessionDate     string    `json:"session_date"`
	Metric          string    `json:"metric"`
	MetricValue     *string   `json:"metric_value,omitempty"`
	Threshold       string    `json:"threshold"`
	Comparator      string    `json:"comparator"`
	FirstSeenAt     time.Time `json:"first_seen_at"`
	LastSeenAt      time.Time `json:"last_seen_at"`
	SuppressedCount int       `json:"suppressed_count"`
}

// AlertDelivery is one attempt to tell the owner about an event.
type AlertDelivery struct {
	ID            string          `json:"id"`
	EventID       string          `json:"event_id"`
	RuleID        string          `json:"rule_id"`
	RuleName      string          `json:"rule_name"`
	Symbol        string          `json:"symbol"`
	Channel       string          `json:"channel"`
	Status        string          `json:"status"`
	Attempts      int             `json:"attempts"`
	MaxAttempts   int             `json:"max_attempts"`
	LastError     *string         `json:"last_error,omitempty"`
	Payload       json.RawMessage `json:"payload"`
	CreatedAt     time.Time       `json:"created_at"`
	NextAttemptAt *time.Time      `json:"next_attempt_at,omitempty"`
	DeliveredAt   *time.Time      `json:"delivered_at,omitempty"`
}

const alertRuleColumns = `id::text, name, symbol, screen, metric, comparator,
	threshold::text, cooldown_seconds, max_attempts, enabled, created_at, updated_at`

func scanAlertRule(row pgx.Row) (*AlertRule, error) {
	var (
		r      AlertRule
		symbol sql.NullString
		screen []byte
	)
	if err := row.Scan(&r.ID, &r.Name, &symbol, &screen, &r.Metric, &r.Comparator,
		&r.Threshold, &r.CooldownSeconds, &r.MaxAttempts, &r.Enabled,
		&r.CreatedAt, &r.UpdatedAt); err != nil {
		return nil, err
	}
	r.Symbol = strPtr(symbol)
	if len(screen) > 0 {
		r.Screen = screen
	}
	return &r, nil
}

// CreateAlertRule stores a rule for one owner.
func (s *Store) CreateAlertRule(ctx context.Context, userID string, r *AlertRule) (*AlertRule, error) {
	row := s.pool.QueryRow(ctx, `
		INSERT INTO alert_rules (user_id, name, symbol, screen, metric, comparator,
		                         threshold, cooldown_seconds, max_attempts, enabled)
		VALUES ($1::uuid, $2, $3, $4, $5, $6, $7::numeric, $8, $9, $10)
		RETURNING `+alertRuleColumns,
		userID, r.Name, r.Symbol, nullJSON(r.Screen), r.Metric, r.Comparator,
		r.Threshold, r.CooldownSeconds, r.MaxAttempts, r.Enabled)
	out, err := scanAlertRule(row)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// UpdateAlertRule replaces a rule's mutable fields for its owner.
func (s *Store) UpdateAlertRule(ctx context.Context, userID, id string, r *AlertRule) (*AlertRule, error) {
	row := s.pool.QueryRow(ctx, `
		UPDATE alert_rules
		SET name = $3, symbol = $4, screen = $5, metric = $6, comparator = $7,
		    threshold = $8::numeric, cooldown_seconds = $9, max_attempts = $10,
		    enabled = $11, updated_at = now()
		WHERE id = $1::uuid AND user_id = $2::uuid
		RETURNING `+alertRuleColumns,
		id, userID, r.Name, r.Symbol, nullJSON(r.Screen), r.Metric, r.Comparator,
		r.Threshold, r.CooldownSeconds, r.MaxAttempts, r.Enabled)
	out, err := scanAlertRule(row)
	if err != nil {
		return nil, wrapNoRows(err)
	}
	return out, nil
}

// ListAlertRules returns the owner's rules.
func (s *Store) ListAlertRules(ctx context.Context, userID string) ([]AlertRule, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+alertRuleColumns+` FROM alert_rules
		WHERE user_id = $1::uuid ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []AlertRule{}
	for rows.Next() {
		r, err := scanAlertRule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	return out, rows.Err()
}

// GetAlertRule returns one rule only when the caller owns it.
func (s *Store) GetAlertRule(ctx context.Context, userID, id string) (*AlertRule, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT `+alertRuleColumns+` FROM alert_rules
		WHERE id = $1::uuid AND user_id = $2::uuid`, id, userID)
	out, err := scanAlertRule(row)
	if err != nil {
		return nil, wrapNoRows(err)
	}
	return out, nil
}

// DeleteAlertRule removes a rule and, by cascade, its events and deliveries.
func (s *Store) DeleteAlertRule(ctx context.Context, userID, id string) (bool, error) {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM alert_rules WHERE id = $1::uuid AND user_id = $2::uuid`, id, userID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// OwnedAlertRule carries the tenant a rule belongs to. Only the worker, which
// evaluates every tenant's rules, needs it; request handlers always operate from
// the authenticated subject instead.
type OwnedAlertRule struct {
	UserID string
	AlertRule
}

// EnabledAlertRules returns every enabled rule across tenants. It is used by the
// worker, which is a trusted process: it evaluates rules but never widens what a
// rule can see, because the rule's own user_id scopes its evaluation.
func (s *Store) EnabledAlertRules(ctx context.Context) ([]OwnedAlertRule, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT user_id::text, `+alertRuleColumns+` FROM alert_rules
		WHERE enabled ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []OwnedAlertRule{}
	for rows.Next() {
		var (
			uid    string
			r      AlertRule
			symbol sql.NullString
			screen []byte
		)
		if err := rows.Scan(&uid, &r.ID, &r.Name, &symbol, &screen, &r.Metric,
			&r.Comparator, &r.Threshold, &r.CooldownSeconds, &r.MaxAttempts,
			&r.Enabled, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		r.Symbol = strPtr(symbol)
		if len(screen) > 0 {
			r.Screen = screen
		}
		out = append(out, OwnedAlertRule{UserID: uid, AlertRule: r})
	}
	return out, rows.Err()
}

func nullJSON(raw json.RawMessage) []byte {
	if len(raw) == 0 {
		return nil
	}
	return raw
}

// ---------------------------------------------------------------- events

// ErrEventDuplicate signals that the observation already existed. Callers treat
// it as "nothing new to deliver", not as a failure.
var ErrEventDuplicate = errors.New("event already recorded")

// InsertAlertEvent records an observation, or returns ErrEventDuplicate when the
// same fingerprint already exists. The decision is made by the unique index, so
// two workers racing on the same session cannot both deliver.
func (s *Store) InsertAlertEvent(ctx context.Context, e *AlertEvent, userID, fingerprint string) (string, error) {
	var id string
	var value any
	if e.MetricValue != nil {
		value = *e.MetricValue
	}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO alert_events (rule_id, user_id, symbol, session_date, metric,
		                          metric_value, threshold, comparator, fingerprint)
		VALUES ($1::uuid, $2::uuid, $3, $4::date, $5, $6::numeric, $7::numeric, $8, $9)
		ON CONFLICT (rule_id, fingerprint) DO NOTHING
		RETURNING id::text`,
		e.RuleID, userID, e.Symbol, e.SessionDate, e.Metric, value,
		e.Threshold, e.Comparator, fingerprint).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrEventDuplicate
		}
		return "", err
	}
	return id, nil
}

// LastDeliveryAt returns when the most recent delivery for this rule and symbol
// was created, which is what the cooldown is measured against.
// max() over an empty set is NULL rather than no rows, so the value is read
// through a nullable type: a rule that has never delivered is a normal state,
// not an error.
func (s *Store) LastDeliveryAt(ctx context.Context, ruleID, symbol string) (time.Time, bool, error) {
	var t sql.NullTime
	err := s.pool.QueryRow(ctx, `
		SELECT max(d.created_at)
		FROM alert_deliveries d
		JOIN alert_events e ON e.id = d.event_id
		WHERE e.rule_id = $1::uuid AND e.symbol = $2`, ruleID, symbol).Scan(&t)
	if err != nil {
		return time.Time{}, false, err
	}
	if !t.Valid {
		return time.Time{}, false, nil
	}
	return t.Time, true, nil
}

// SuppressEvent records that this observation satisfied its rule but was not
// sent, because the cooldown was still running. Keeping the count means the
// history explains a silence instead of leaving a gap that looks like a bug.
func (s *Store) SuppressEvent(ctx context.Context, eventID string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE alert_events
		SET suppressed_count = suppressed_count + 1, last_seen_at = now()
		WHERE id = $1::uuid`, eventID)
	return err
}

// ---------------------------------------------------------------- deliveries

// CreateDelivery queues one delivery for an event.
func (s *Store) CreateDelivery(ctx context.Context, eventID, ruleID, userID string, maxAttempts int, payload json.RawMessage) (string, error) {
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	var id string
	err := s.pool.QueryRow(ctx, `
		INSERT INTO alert_deliveries (event_id, rule_id, user_id, max_attempts, payload)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5)
		RETURNING id::text`,
		eventID, ruleID, userID, maxAttempts, nullJSON(payload)).Scan(&id)
	if err != nil {
		return "", err
	}
	return id, nil
}

// PendingDelivery is the minimal shape the retry loop needs.
type PendingDelivery struct {
	ID          string
	EventID     string
	RuleID      string
	UserID      string
	Attempts    int
	MaxAttempts int
}

// DueDeliveries returns deliveries that are pending and due for another attempt.
//
// The clock is a parameter rather than now() so a retry schedule can be tested
// without sleeping: the caller decides what "now" means.
func (s *Store) DueDeliveries(ctx context.Context, now time.Time, limit int) ([]PendingDelivery, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id::text, event_id::text, rule_id::text, user_id::text, attempts, max_attempts
		FROM alert_deliveries
		WHERE status = 'pending' AND (next_attempt_at IS NULL OR next_attempt_at <= $1)
		ORDER BY created_at
		LIMIT $2`, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []PendingDelivery{}
	for rows.Next() {
		var d PendingDelivery
		if err := rows.Scan(&d.ID, &d.EventID, &d.RuleID, &d.UserID,
			&d.Attempts, &d.MaxAttempts); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// MarkDelivered records a successful attempt.
func (s *Store) MarkDelivered(ctx context.Context, id string, attempts int) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE alert_deliveries
		SET status = 'delivered', attempts = $2, delivered_at = now(),
		    last_error = NULL, next_attempt_at = NULL
		WHERE id = $1::uuid`, id, attempts)
	return err
}

// MarkDeliveryFailed records a failed attempt and the time of the next one. A
// delivery that has spent its attempts is left failed rather than retried
// forever, so a permanent error is visible in the history instead of invisible.
func (s *Store) MarkDeliveryFailed(ctx context.Context, id string, attempts int, nextAt *time.Time, msg string) error {
	status := "pending"
	if nextAt == nil {
		status = "failed"
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE alert_deliveries
		SET status = $3, attempts = $2, next_attempt_at = $4, last_error = $5
		WHERE id = $1::uuid`, id, attempts, status, nextAt, msg)
	return err
}

// ListAlertEvents returns the owner's event history, newest first.
func (s *Store) ListAlertEvents(ctx context.Context, userID string, limit int) ([]AlertEvent, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `
		SELECT e.id::text, e.rule_id::text, r.name, e.symbol, e.session_date::text,
		       e.metric, e.metric_value::text, e.threshold::text, e.comparator,
		       e.first_seen_at, e.last_seen_at, e.suppressed_count
		FROM alert_events e
		JOIN alert_rules r ON r.id = e.rule_id
		WHERE e.user_id = $1::uuid
		ORDER BY e.first_seen_at DESC
		LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []AlertEvent{}
	for rows.Next() {
		var (
			e     AlertEvent
			value sql.NullString
		)
		if err := rows.Scan(&e.ID, &e.RuleID, &e.RuleName, &e.Symbol, &e.SessionDate,
			&e.Metric, &value, &e.Threshold, &e.Comparator,
			&e.FirstSeenAt, &e.LastSeenAt, &e.SuppressedCount); err != nil {
			return nil, err
		}
		e.MetricValue = strPtr(value)
		out = append(out, e)
	}
	return out, rows.Err()
}

// ListDeliveries returns the owner's delivery history, newest first.
func (s *Store) ListDeliveries(ctx context.Context, userID string, limit int) ([]AlertDelivery, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `
		SELECT d.id::text, d.event_id::text, d.rule_id::text, r.name, e.symbol,
		       d.channel, d.status, d.attempts, d.max_attempts, d.last_error,
		       d.payload, d.created_at, d.next_attempt_at, d.delivered_at
		FROM alert_deliveries d
		JOIN alert_rules r ON r.id = d.rule_id
		JOIN alert_events e ON e.id = d.event_id
		WHERE d.user_id = $1::uuid
		ORDER BY d.created_at DESC
		LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []AlertDelivery{}
	for rows.Next() {
		var (
			d       AlertDelivery
			lastErr sql.NullString
			next    sql.NullTime
			done    sql.NullTime
			payload []byte
		)
		if err := rows.Scan(&d.ID, &d.EventID, &d.RuleID, &d.RuleName, &d.Symbol,
			&d.Channel, &d.Status, &d.Attempts, &d.MaxAttempts, &lastErr,
			&payload, &d.CreatedAt, &next, &done); err != nil {
			return nil, err
		}
		d.LastError = strPtr(lastErr)
		d.Payload = payload
		if next.Valid {
			t := next.Time
			d.NextAttemptAt = &t
		}
		if done.Valid {
			t := done.Time
			d.DeliveredAt = &t
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
