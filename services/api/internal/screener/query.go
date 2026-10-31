package screener

import (
	"fmt"
	"strings"
)

// ctePrefix builds the row set every part of the screener reads: one row per
// listed security with the latest session's price, volume and capitalisation,
// its share counts, and the history-derived technicals.
//
// The window is capped at 252 sessions because a "52-week" metric over more than
// a year would be a different claim than the one the column name makes.
const ctePrefix = `
WITH latest AS (
    SELECT max(trading_date) AS dt FROM daily_ohlcv
),
b AS (
    SELECT sec.id            AS security_id,
           c.symbol,
           c.name,
           c.status,
           c.instrument,
           COALESCE(s.code, 'unclassified') AS sector_code,
           COALESCE(s.name, 'Unclassified') AS sector,
           o.trading_date,
           o.close,
           o.previous_close,
           o.volume,
           o.turnover,
           o.transactions,
           o.market_cap,
           sec.listed_shares,
           sec.public_shares,
           CASE WHEN o.close IS NOT NULL AND sec.public_shares IS NOT NULL
                THEN o.close * sec.public_shares END                          AS float_market_cap,
           CASE WHEN sec.listed_shares > 0 AND sec.public_shares IS NOT NULL
                THEN 100.0 * sec.public_shares / sec.listed_shares END        AS free_float_pct,
           CASE WHEN o.previous_close > 0
                THEN 100.0 * (o.close - o.previous_close) / o.previous_close END AS change_pct,
           st.bars,
           st.sma_20,
           st.sma_50,
           st.high_52w,
           st.low_52w,
           st.avg_vol_20,
           CASE WHEN st.close_5 > 0
                THEN 100.0 * (o.close - st.close_5) / st.close_5 END          AS return_5d,
           CASE WHEN st.close_20 > 0
                THEN 100.0 * (o.close - st.close_20) / st.close_20 END        AS return_20d,
           CASE WHEN st.high_52w > 0
                THEN 100.0 * (o.close - st.high_52w) / st.high_52w END        AS pct_from_52w_high,
           CASE WHEN st.avg_vol_20 > 0
                THEN o.volume / st.avg_vol_20 END                             AS volume_ratio_20
    FROM companies c
    JOIN securities sec ON sec.company_id = c.id
    LEFT JOIN sectors s ON s.id = c.sector_id
    JOIN daily_ohlcv o ON o.security_id = sec.id
    JOIN latest l ON o.trading_date = l.dt
    LEFT JOIN LATERAL (
        SELECT count(*)                                        AS bars,
               avg(w.close)   FILTER (WHERE w.rn <= 20)        AS sma_20,
               avg(w.close)   FILTER (WHERE w.rn <= 50)        AS sma_50,
               max(w.high)    FILTER (WHERE w.rn <= 252)       AS high_52w,
               min(w.low)     FILTER (WHERE w.rn <= 252)       AS low_52w,
               avg(w.volume)  FILTER (WHERE w.rn <= 20)        AS avg_vol_20,
               max(w.close)   FILTER (WHERE w.rn = 5)          AS close_5,
               max(w.close)   FILTER (WHERE w.rn = 20)         AS close_20
        FROM (
            SELECT trading_date, close, high, low, volume,
                   row_number() OVER (ORDER BY trading_date DESC) AS rn
            FROM daily_ohlcv
            WHERE security_id = sec.id AND trading_date <= l.dt
            ORDER BY trading_date DESC
            LIMIT 252
        ) w
    ) st ON true
)`

// selectColumns is the single result shape. Every numeric is cast to text so a
// DECIMAL reaches the client without a float64 round trip; a NULL stays NULL and
// arrives as a missing value rather than a zero.
const selectColumns = `
    b.security_id::text,
    b.symbol,
    b.name,
    b.sector,
    b.sector_code,
    b.instrument,
    b.status,
    b.trading_date::text,
    b.close::text,
    b.previous_close::text,
    b.change_pct::text,
    b.volume::text,
    b.turnover::text,
    b.transactions,
    b.market_cap::text,
    b.float_market_cap::text,
    b.free_float_pct::text,
    b.listed_shares::text,
    b.public_shares::text,
    b.sma_20::text,
    b.sma_50::text,
    b.high_52w::text,
    b.low_52w::text,
    b.pct_from_52w_high::text,
    b.volume_ratio_20::text,
    b.return_5d::text,
    b.return_20d::text,
    b.bars`

// orderColumns maps a sort key to SQL. Only allowlisted keys reach the query;
// every ordering ends in symbol so pagination and repeated runs are stable.
func orderSQL(sortKey, order string) string {
	dir := "ASC"
	if order == "desc" {
		dir = "DESC"
	}
	switch sortKey {
	case "name":
		return "b.name " + dir + " NULLS LAST, b.symbol ASC"
	case "sector":
		return "b.sector " + dir + " NULLS LAST, b.symbol ASC"
	case "symbol":
		return "b.symbol " + dir
	}
	m, ok := metrics[sortKey]
	if !ok {
		return "b.symbol ASC"
	}
	return m.expr + " " + dir + " NULLS LAST, b.symbol ASC"
}

// historyGate is the number of stored sessions a rule requires before any of its
// technical filters may be evaluated. A metric's own requirement applies unless
// the condition overrides it, and the rule-wide gate applies on top.
func historyGate(r Rule) int {
	gate := 0
	if r.MinBars > 0 {
		gate = r.MinBars
	}
	for _, c := range append(append([]Condition(nil), r.All...), r.Any...) {
		m, ok := metrics[c.Metric]
		if !ok {
			continue
		}
		need := m.MinBars
		if c.MinBars > 0 {
			need = c.MinBars
		}
		if need > gate {
			gate = need
		}
	}
	return gate
}

// escapeLike neutralises the wildcards in a caller-supplied search term, so a
// search for "100%" looks for that text rather than matching everything.
func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

type args struct {
	list []any
}

func (a *args) add(v any) string {
	a.list = append(a.list, v)
	return fmt.Sprintf("$%d", len(a.list))
}

// Query assembles the screening SQL for a normalised rule.
//
// It returns an error only for a rule that Normalize would have rejected; every
// fragment interpolated into the string comes from the metric registry or a
// fixed operator map, and every caller-supplied value is a parameter.
func Query(r Rule) (string, []any, error) {
	var where []string
	a := &args{}

	if len(r.Universe.Sectors) > 0 {
		where = append(where, "b.sector_code = ANY("+a.add(r.Universe.Sectors)+")")
	}
	if len(r.Universe.Instruments) > 0 {
		where = append(where, "b.instrument = ANY("+a.add(r.Universe.Instruments)+")")
	}
	if len(r.Universe.Statuses) > 0 {
		where = append(where, "b.status = ANY("+a.add(r.Universe.Statuses)+")")
	}
	if len(r.Universe.Symbols) > 0 {
		where = append(where, "b.symbol = ANY("+a.add(r.Universe.Symbols)+")")
	}
	if r.Universe.Search != "" {
		p := "%" + escapeLike(r.Universe.Search) + "%"
		where = append(where, "(b.symbol ILIKE "+a.add(p)+" OR b.name ILIKE "+a.add(p)+")")
	}
	if gate := historyGate(r); gate > 0 {
		where = append(where, "b.bars >= "+a.add(gate)+"")
	}

	all, err := conditionSQL(r.All, a)
	if err != nil {
		return "", nil, err
	}
	if len(all) > 0 {
		where = append(where, "("+strings.Join(all, " AND ")+")")
	}
	any, err := conditionSQL(r.Any, a)
	if err != nil {
		return "", nil, err
	}
	if len(any) > 0 {
		// "any" is a union inside the rule, never a substitute for "all": the two
		// lists are ANDed, so a caller can require a sector and then accept any of
		// several price conditions.
		where = append(where, "("+strings.Join(any, " OR ")+")")
	}

	whereSQL := ""
	if len(where) > 0 {
		whereSQL = "\n WHERE " + strings.Join(where, "\n   AND ")
	}
	limit := r.Limit
	if limit <= 0 {
		limit = DefaultLimit
	}

	q := ctePrefix + "\nSELECT" + selectColumns + "\nFROM b" + whereSQL +
		"\n ORDER BY " + orderSQL(r.Sort, r.Order) +
		"\n LIMIT " + a.add(limit)
	return q, a.list, nil
}

func conditionSQL(conds []Condition, a *args) ([]string, error) {
	out := make([]string, 0, len(conds))
	for _, c := range conds {
		m, ok := metrics[c.Metric]
		if !ok {
			return nil, invalid("metric", "%q is not a filterable metric", c.Metric)
		}
		if c.Op == "between" {
			if c.ValueTo == nil {
				return nil, invalid("value_to", "is required when op is between")
			}
			out = append(out, fmt.Sprintf("(%s BETWEEN %s::numeric AND %s::numeric)",
				m.expr, a.add(string(c.Value)), a.add(string(*c.ValueTo))))
			continue
		}
		sym, ok := operators[c.Op]
		if !ok {
			return nil, invalid("op", "%q is not a comparator", c.Op)
		}
		out = append(out, fmt.Sprintf("(%s %s %s::numeric)", m.expr, sym, a.add(string(c.Value))))
	}
	return out, nil
}

// CoverageQuery counts how many securities can actually answer each metric, so
// a screen that leans on history can say "prices cover 428 scrips, 20-day
// averages cover 2" instead of returning an unexplained empty list.
func CoverageQuery() string {
	return ctePrefix + `
SELECT count(*),
       count(*) FILTER (WHERE b.bars >= 20),
       count(*) FILTER (WHERE b.bars >= 50),
       count(*) FILTER (WHERE b.bars >= 252),
       count(*) FILTER (WHERE b.float_market_cap IS NOT NULL),
       count(*) FILTER (WHERE b.free_float_pct IS NOT NULL),
       max(b.bars),
       COALESCE(max(b.trading_date)::text, '')
FROM b`
}
