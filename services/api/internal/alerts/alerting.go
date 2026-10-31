// Package alerting evaluates standing alert rules and delivers what fires.
//
// # The three guards, and why all three are needed
//
//   - Deduplication. The fingerprint of (rule, symbol, session, metric, value) is
//     unique in the database. Evaluating a rule twice — because two workers ran,
//     because a job was retried, or because the next cycle saw the same session —
//     cannot produce a second event. This is enforced by an index rather than by
//     reading first and writing second, because the read/write version loses
//     exactly the race it exists to prevent.
//   - Cooldown. A rule about a live condition ("price above 500") is true on
//     every tick. Once it has been delivered, it is not delivered again until the
//     cooldown elapses; the observation is still recorded and counted, so the
//     history shows the condition persisted rather than vanishing.
//   - Attempts. A delivery that fails is retried with exponential backoff, and
//     after max_attempts it is left failed. A permanent failure must be visible
//     in the history, not retried forever in the dark.
//
// # What delivery is
//
// This platform has no email or push provider configured, and inventing one
// would make the feature look finished while sending nothing. Delivery is
// therefore in-app: the delivery row is the message, readable through
// /api/v1/alerts/deliveries by its owner. The Delivery channel column exists so a
// second channel can be added without changing the event model.
package alerts

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"nepse/terminal/api/internal/repositories"
	"nepse/terminal/api/internal/screener"
)

// Defaults. A cooldown of an hour and three attempts are deliberately modest:
// loud enough to be noticed, quiet enough not to spam.
const (
	DefaultCooldown = time.Hour
	MaxCooldown     = 7 * 24 * time.Hour
	DefaultAttempts = 3
	MaxAttempts     = 10
	// RetryBatch bounds how many due deliveries one cycle attempts, so a backlog
	// cannot turn a cycle into an unbounded loop.
	RetryBatch = 100
)

// Backoff is the wait before the next attempt after attempt n (1-based):
// 30s, 1m, 2m, 4m … capped at an hour. It is a pure function so the schedule can
// be asserted without waiting for it.
func Backoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	d := 30 * time.Second << (attempt - 1)
	if d > time.Hour || d <= 0 {
		d = time.Hour
	}
	return d
}

// NextAttempt decides what happens after a failed attempt: another scheduled
// attempt, or a terminal failure once the allowance is spent.
func NextAttempt(attempt, max int, now time.Time) (nextAt *time.Time, terminal bool) {
	if attempt >= max {
		return nil, true
	}
	t := now.Add(Backoff(attempt))
	return &t, false
}

// Fingerprint identifies one observation. It deliberately includes the metric
// value: "close crossed 500" observed at 501 and at 640 are different facts, and
// collapsing them would hide the second move.
func Fingerprint(ruleID, symbol, session, metric, value string) string {
	sum := sha256.Sum256([]byte(ruleID + "|" + symbol + "|" + session + "|" + metric + "|" + value))
	return hex.EncodeToString(sum[:])
}

// ShouldDeliver decides whether a fresh observation may be sent, given when this
// rule and symbol was last delivered.
//
// "Outside the window" is measured from the last delivery, not from the last
// event: a suppressed observation must not extend its own suppression, or a
// condition that stays true would never be repeated.
func ShouldDeliver(now, lastDelivery time.Time, hasDelivery bool, cooldown time.Duration) bool {
	if cooldown <= 0 || !hasDelivery {
		return true
	}
	return now.Sub(lastDelivery) >= cooldown
}

// Delivery is what a Deliverer is asked to send.
type Delivery struct {
	ID      string
	EventID string
	RuleID  string
	UserID  string
	Attempt int
	Payload []byte
}

// Deliverer sends one delivery. An error means "try again later", not "give up":
// the engine decides when the attempts are spent.
type Deliverer interface {
	Deliver(ctx context.Context, d Delivery) error
}

// InAppDeliverer is the default channel. An in-app message is the delivery row
// itself, which is already persisted before it is handed here, so the delivery is
// complete the moment it is recorded.
type InAppDeliverer struct{}

// Deliver implements Deliverer.
func (InAppDeliverer) Deliver(context.Context, Delivery) error { return nil }

// Engine evaluates rules and drains the retry queue.
type Engine struct {
	Store     *repositories.Store
	Deliverer Deliverer
	Logger    *slog.Logger
	// Now is injectable so cooldown and backoff behaviour can be tested without
	// sleeping. Nil means the wall clock.
	Now func() time.Time
}

func (e *Engine) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now().UTC()
}

func (e *Engine) deliverer() Deliverer {
	if e.Deliverer != nil {
		return e.Deliverer
	}
	return InAppDeliverer{}
}

// Result is a cycle's summary. It is logged and returned, so an operator can tell
// "nothing to do" from "everything failed".
type Result struct {
	RulesEvaluated int      `json:"rules_evaluated"`
	RowsExamined   int      `json:"rows_examined"`
	Observations   int      `json:"observations"`
	NewEvents      int      `json:"new_events"`
	Duplicates     int      `json:"duplicates"`
	Suppressed     int      `json:"suppressed"`
	Deliveries     int      `json:"deliveries"`
	Retried        int      `json:"retried"`
	Failed         int      `json:"failed"`
	Unavailable    int      `json:"unavailable_metrics"`
	Errors         []string `json:"errors,omitempty"`
}

// RunOnce evaluates every enabled rule, then drains due retries.
//
// One rule failing does not stop the others: a malformed saved screen should not
// silence every other tenant's alerts. The error is collected and reported.
func (e *Engine) RunOnce(ctx context.Context) (Result, error) {
	res := Result{}

	rules, err := e.Store.EnabledAlertRules(ctx)
	if err != nil {
		return res, err
	}
	for _, owned := range rules {
		res.RulesEvaluated++
		if err := e.evaluateRule(ctx, owned, &res); err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("rule %s: %v", owned.ID, err))
			if e.Logger != nil {
				e.Logger.Warn("alert rule evaluation failed", "rule", owned.ID, "error", err)
			}
		}
	}

	if err := e.retryPending(ctx, &res); err != nil {
		res.Errors = append(res.Errors, "retry pass: "+err.Error())
	}
	return res, nil
}

// EvaluateForUser runs only the caller's rules, so the API can offer "check my
// alerts now" without letting a request touch another tenant's evaluation.
func (e *Engine) EvaluateForUser(ctx context.Context, userID string) (Result, error) {
	res := Result{}
	rules, err := e.Store.ListAlertRules(ctx, userID)
	if err != nil {
		return res, err
	}
	for _, r := range rules {
		if !r.Enabled {
			continue
		}
		res.RulesEvaluated++
		if err := e.evaluateRule(ctx, repositories.OwnedAlertRule{UserID: userID, AlertRule: r}, &res); err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("rule %s: %v", r.ID, err))
		}
	}
	if err := e.retryPending(ctx, &res); err != nil {
		res.Errors = append(res.Errors, "retry pass: "+err.Error())
	}
	return res, nil
}

// evaluateRule resolves the rule's subjects and checks the condition for each.
func (e *Engine) evaluateRule(ctx context.Context, owned repositories.OwnedAlertRule, res *Result) error {
	rule := owned.AlertRule
	metric, ok := screener.LookupMetric(rule.Metric)
	if !ok {
		return fmt.Errorf("metric %q is no longer filterable", rule.Metric)
	}

	rows, err := e.subjects(ctx, rule)
	if err != nil {
		return err
	}
	res.RowsExamined += len(rows)

	for _, row := range rows {
		value, ok := row.Value(rule.Metric)
		if !ok {
			// The metric is absent for this security — no session, or not enough
			// history. It cannot satisfy a threshold, and it is counted so a rule
			// that never fires can be explained.
			res.Unavailable++
			continue
		}
		if metric.MinBars > 0 && row.Bars < metric.MinBars {
			res.Unavailable++
			continue
		}
		satisfied, err := screener.Satisfies(rule.Comparator, value, rule.Threshold)
		if err != nil {
			return err
		}
		if !satisfied {
			continue
		}
		res.Observations++

		fingerprint := Fingerprint(rule.ID, row.Symbol, row.TradingDate, rule.Metric, value)
		event := &repositories.AlertEvent{
			RuleID:      rule.ID,
			Symbol:      row.Symbol,
			SessionDate: row.TradingDate,
			Metric:      rule.Metric,
			MetricValue: &value,
			Threshold:   rule.Threshold,
			Comparator:  rule.Comparator,
		}
		eventID, err := e.Store.InsertAlertEvent(ctx, event, owned.UserID, fingerprint)
		if errors.Is(err, repositories.ErrEventDuplicate) {
			res.Duplicates++
			continue
		}
		if err != nil {
			return err
		}
		res.NewEvents++

		cooldown := time.Duration(rule.CooldownSeconds) * time.Second
		last, hasLast, err := e.Store.LastDeliveryAt(ctx, rule.ID, row.Symbol)
		if err != nil {
			return err
		}
		if !ShouldDeliver(e.now(), last, hasLast, cooldown) {
			res.Suppressed++
			if err := e.Store.SuppressEvent(ctx, eventID); err != nil {
				return err
			}
			continue
		}

		payload, _ := json.Marshal(map[string]any{
			"rule_id":      rule.ID,
			"rule_name":    rule.Name,
			"symbol":       row.Symbol,
			"session_date": row.TradingDate,
			"metric":       rule.Metric,
			"value":        value,
			"comparator":   rule.Comparator,
			"threshold":    rule.Threshold,
			"observed_at":  e.now().Format(time.RFC3339),
		})

		deliveryID, err := e.Store.CreateDelivery(ctx, eventID, rule.ID, owned.UserID, rule.MaxAttempts, payload)
		if err != nil {
			return err
		}
		res.Deliveries++
		e.attempt(ctx, repositories.PendingDelivery{
			ID: deliveryID, EventID: eventID, RuleID: rule.ID, UserID: owned.UserID,
			Attempts: 0, MaxAttempts: rule.MaxAttempts,
		}, payload, res)
	}
	return nil
}

// subjects resolves which securities a rule watches.
func (e *Engine) subjects(ctx context.Context, rule repositories.AlertRule) ([]screener.Row, error) {
	var screen screener.Rule
	if len(rule.Screen) > 0 {
		if err := json.Unmarshal(rule.Screen, &screen); err != nil {
			return nil, fmt.Errorf("stored screen is not a valid rule: %w", err)
		}
		normalised, err := screener.Normalize(screen)
		if err != nil {
			return nil, fmt.Errorf("stored screen is invalid: %w", err)
		}
		screen = normalised
	}
	if rule.Symbol != nil && *rule.Symbol != "" {
		// A symbol-scoped rule ignores the screen's symbol list but keeps its
		// other narrowing, so "watch NABIL when banks are above 4000" works.
		screen.Universe.Symbols = []string{*rule.Symbol}
	}
	if len(screen.Universe.Symbols) == 0 && rule.Screen == nil {
		return nil, nil
	}
	if screen.Limit == 0 || screen.Limit < screener.MaxLimit {
		screen.Limit = screener.MaxLimit
	}
	return e.Store.RunScreen(ctx, screen)
}

// attempt performs one delivery attempt and records the outcome.
func (e *Engine) attempt(ctx context.Context, d repositories.PendingDelivery, payload []byte, res *Result) {
	attempt := d.Attempts + 1
	err := e.deliverer().Deliver(ctx, Delivery{
		ID: d.ID, EventID: d.EventID, RuleID: d.RuleID, UserID: d.UserID,
		Attempt: attempt, Payload: payload,
	})
	if err == nil {
		if markErr := e.Store.MarkDelivered(ctx, d.ID, attempt); markErr != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("delivery %s: %v", d.ID, markErr))
			return
		}
		return
	}

	res.Failed++
	nextAt, terminal := NextAttempt(attempt, d.MaxAttempts, e.now())
	if terminal && e.Logger != nil {
		e.Logger.Warn("alert delivery exhausted its attempts",
			"delivery", d.ID, "rule", d.RuleID, "attempts", attempt, "error", err)
	}
	if markErr := e.Store.MarkDeliveryFailed(ctx, d.ID, attempt, nextAt, err.Error()); markErr != nil {
		res.Errors = append(res.Errors, fmt.Sprintf("delivery %s: %v", d.ID, markErr))
	}
}

// retryPending drains deliveries whose next attempt is due.
func (e *Engine) retryPending(ctx context.Context, res *Result) error {
	due, err := e.Store.DueDeliveries(ctx, e.now(), RetryBatch)
	if err != nil {
		return err
	}
	for _, d := range due {
		res.Retried++
		// The payload is rebuilt from the stored row rather than carried in the
		// queue, so a retry sends what the event says now.
		payload, _ := json.Marshal(map[string]any{
			"delivery_id": d.ID,
			"event_id":    d.EventID,
			"rule_id":     d.RuleID,
			"retry":       d.Attempts + 1,
		})
		e.attempt(ctx, d, payload, res)
	}
	return nil
}
