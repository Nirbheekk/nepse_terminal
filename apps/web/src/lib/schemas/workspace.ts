import { z } from "zod";

/**
 * Runtime schemas for the account-scoped and operational surfaces: screener,
 * watchlists, alerts, portfolios, disclosures, research and data health.
 *
 * These were written against payloads captured from the running API, not from
 * the Go structs, so the shapes here are what the wire actually carries. Decimal
 * columns still arrive as strings; they are kept as strings and formatted at
 * render time so a price never loses precision on its way to the screen.
 */

const decimal = z.union([z.string(), z.number()]).transform((v) => String(v));
const optionalDecimal = decimal.nullish().transform((v) => (v === null || v === undefined ? undefined : v));

// ------------------------------------------------------------------ screener

export const ScreenerMetricSchema = z.object({
  key: z.string(),
  label: z.string(),
  kind: z.enum(["price", "volume", "fundamental", "technical"]),
  unit: z.string().optional(),
  description: z.string().optional(),
  min_bars: z.number(),
});

export const ScreenerCoverageSchema = z.object({
  securities: z.number(),
  with_20_sessions: z.number(),
  with_50_sessions: z.number(),
  with_252_sessions: z.number(),
  with_float_cap: z.number(),
  with_free_float: z.number(),
  max_sessions: z.number(),
  latest_session: z.string(),
});

export const ScreenerRowSchema = z.object({
  security_id: z.string(),
  symbol: z.string(),
  name: z.string(),
  sector: z.string(),
  sector_code: z.string(),
  instrument: z.string(),
  status: z.string(),
  trading_date: z.string(),
  close: optionalDecimal,
  previous_close: optionalDecimal,
  change_pct: optionalDecimal,
  volume: optionalDecimal,
  turnover: optionalDecimal,
  transactions: z.number().nullish(),
  market_cap: optionalDecimal,
  float_market_cap: optionalDecimal,
  free_float_pct: optionalDecimal,
  listed_shares: optionalDecimal,
  public_shares: optionalDecimal,
  sma_20: optionalDecimal,
  sma_50: optionalDecimal,
  high_52w: optionalDecimal,
  low_52w: optionalDecimal,
  pct_from_52w_high: optionalDecimal,
  volume_ratio_20: optionalDecimal,
  return_5d: optionalDecimal,
  return_20d: optionalDecimal,
  bars: z.number(),
});

export const ScreenRunSchema = z.object({
  coverage: ScreenerCoverageSchema,
  returned: z.number(),
  rows: z.array(ScreenerRowSchema),
});

export const SavedScreenSchema = z.object({
  id: z.string(),
  name: z.string(),
  description: z.string().optional(),
  definition: z.unknown(),
  created_at: z.string(),
  updated_at: z.string(),
});

// ---------------------------------------------------------------- watchlists

export const WatchlistItemSchema = z.object({
  symbol: z.string(),
  name: z.string(),
  note: z.string().optional(),
  added_at: z.string(),
  trading_date: z.string().optional(),
  close: optionalDecimal,
  change_pct: optionalDecimal,
  volume: optionalDecimal,
});

export const WatchlistSchema = z.object({
  id: z.string(),
  name: z.string(),
  item_count: z.number(),
  items: z.array(WatchlistItemSchema),
  created_at: z.string(),
  updated_at: z.string(),
});

// -------------------------------------------------------------------- alerts

export const AlertRuleSchema = z.object({
  id: z.string(),
  name: z.string(),
  symbol: z.string().nullish(),
  metric: z.string(),
  comparator: z.string(),
  threshold: decimal,
  cooldown_seconds: z.number(),
  max_attempts: z.number(),
  enabled: z.boolean(),
  created_at: z.string(),
  updated_at: z.string(),
});

export const AlertEventSchema = z.object({
  id: z.string(),
  rule_id: z.string(),
  rule_name: z.string(),
  symbol: z.string(),
  session_date: z.string(),
  metric: z.string(),
  metric_value: optionalDecimal,
  threshold: decimal,
  comparator: z.string(),
  first_seen_at: z.string(),
  last_seen_at: z.string(),
  suppressed_count: z.number(),
});

export const AlertDeliverySchema = z.object({
  id: z.string(),
  event_id: z.string(),
  rule_id: z.string(),
  rule_name: z.string(),
  symbol: z.string(),
  channel: z.string(),
  status: z.string(),
  attempts: z.number(),
  max_attempts: z.number(),
  last_error: z.string().nullish(),
  created_at: z.string(),
  next_attempt_at: z.string().nullish(),
  delivered_at: z.string().nullish(),
});

// ---------------------------------------------------------------- portfolios

export const PortfolioSchema = z.object({
  id: z.string(),
  user_id: z.string(),
  name: z.string(),
  currency: z.string(),
  cost_basis_method: z.string(),
  cash_balance: decimal,
  // A portfolio with no open positions sends null, not an absent key.
  positions: z
    .record(z.string(), z.unknown())
    .nullish()
    .transform((v) => v ?? undefined),
});

export const PortfolioAnalyticsSchema = z.object({
  performance: z
    .object({
      total_days: z.number(),
      start_date: z.string(),
      end_date: z.string(),
      portfolio_twr: decimal,
      portfolio_cagr: decimal,
      benchmark_return: decimal,
      benchmark_cagr: decimal,
      excess_return: decimal,
    })
    .partial()
    .optional(),
  risk: z.record(z.string(), z.unknown()).optional(),
  drawdown: z.record(z.string(), z.unknown()).optional(),
});

// --------------------------------------------------------------- disclosures

export const DisclosureSchema = z.object({
  id: z.string(),
  authority: z.string(),
  symbol: z.string().optional(),
  category: z.string(),
  title: z.string(),
  summary: z.string().optional(),
  published_at: z.string(),
  published_at_npt: z.string().optional(),
  effective_date: z.string().optional(),
  source_url: z.string().optional(),
});

// ------------------------------------------------------------------ research

export const GroundedFindingSchema = z.object({
  claim_type: z.string(),
  statement: z.string(),
  formula: z.string().optional(),
  citations: z
    .array(
      z.object({
        source_type: z.string(),
        authority: z.string(),
        citation_text: z.string(),
        as_of_date: z.string().optional(),
        attachment_url: z.string().optional(),
      }),
    )
    .optional(),
});

export const ResearchResponseSchema = z.object({
  query_text: z.string(),
  scope: z.string(),
  summary_answer: z.string(),
  facts: z.array(GroundedFindingSchema),
  calculations: z.array(GroundedFindingSchema),
  interpretations: z.array(GroundedFindingSchema),
  confidence_score: decimal,
  license_notice: z.string(),
  disclaimer: z.string(),
  supported_by_proof: z.boolean(),
});

// ------------------------------------------------------------ data health

export const FreshnessStreamSchema = z.object({
  stream: z.string(),
  description: z.string().optional(),
  owner: z.string().optional(),
  severity: z.string().optional(),
  max_age_seconds: z.number().optional(),
  rows: z.number().optional(),
  latest_at: z.string().nullish(),
  age_seconds: z.number().optional(),
  status: z.string(),
});

export const FreshnessReportSchema = z.object({
  datasets: z.array(FreshnessStreamSchema),
});

export const IngestRunSchema = z.object({
  source: z.string(),
  stream: z.string(),
  status: z.string(),
  rows_read: z.number(),
  rows_written: z.number(),
  rows_rejected: z.number(),
  archive_sha256: z.string().optional(),
  started_at: z.string(),
  finished_at: z.string().nullish(),
});

export const ArchiveSummarySchema = z.object({
  stream: z.string(),
  archives: z.number(),
  rows: z.number(),
  bytes: z.number(),
  latest_at: z.string().nullish(),
});

export type ScreenerMetric = z.infer<typeof ScreenerMetricSchema>;
export type ScreenerCoverage = z.infer<typeof ScreenerCoverageSchema>;
export type ScreenerRow = z.infer<typeof ScreenerRowSchema>;
export type ScreenRun = z.infer<typeof ScreenRunSchema>;
export type SavedScreen = z.infer<typeof SavedScreenSchema>;
export type Watchlist = z.infer<typeof WatchlistSchema>;
export type WatchlistItem = z.infer<typeof WatchlistItemSchema>;
export type AlertRule = z.infer<typeof AlertRuleSchema>;
export type AlertEvent = z.infer<typeof AlertEventSchema>;
export type AlertDelivery = z.infer<typeof AlertDeliverySchema>;
export type Portfolio = z.infer<typeof PortfolioSchema>;
export type PortfolioAnalytics = z.infer<typeof PortfolioAnalyticsSchema>;
export type Disclosure = z.infer<typeof DisclosureSchema>;
export type ResearchResponse = z.infer<typeof ResearchResponseSchema>;
export type FreshnessStream = z.infer<typeof FreshnessStreamSchema>;
export type IngestRun = z.infer<typeof IngestRunSchema>;
export type ArchiveSummary = z.infer<typeof ArchiveSummarySchema>;
