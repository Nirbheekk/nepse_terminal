import { z } from "zod";

/**
 * Runtime schemas for every payload the UI renders.
 *
 * The Go API is the contract owner (see api/openapi.yaml). These schemas are the
 * client's half of it: a response that drifts from the contract fails here, at
 * the boundary, instead of rendering "NaN" somewhere in a table three clicks
 * later. Numbers that the database stores as NUMERIC arrive as strings on
 * purpose — parsing them to float64 in the API would lose precision — so the
 * schema keeps them as strings and formatting happens at render time.
 */

export const MetaSchema = z.object({
  request_id: z.string().optional(),
  as_of: z.string().optional(),
  source: z.string().optional(),
  license_class: z.string().optional(),
  // "final" means the exchange is closed and this session will not change
  // again. It is deliberately distinct from "stale", which means a capture is
  // older than the session it should describe.
  freshness: z.enum(["live", "delayed", "stale", "final"]).optional(),
  freshness_reason: z.string().optional(),
  market_state: z.string().optional(),
  session_date: z.string().optional(),
  age_seconds: z.number().optional(),
  calculation: z.string().optional(),
  coverage: z
    .object({
      covered: z.number(),
      eligible: z.number(),
      unit: z.string(),
    })
    .optional(),
  total: z.number().optional(),
  next_cursor: z.string().optional(),
});

export const ApiErrorSchema = z.object({
  code: z.string(),
  message: z.string(),
  retryable: z.boolean(),
});

export type Meta = z.infer<typeof MetaSchema>;

/** Envelope factory: every endpoint returns { data, meta, error }. */
export function envelope<T extends z.ZodTypeAny>(data: T) {
  return z.object({
    data: data.nullable(),
    meta: MetaSchema.default({}),
    error: ApiErrorSchema.nullable().optional(),
  });
}

const numericString = z.union([z.string(), z.number()]).transform((v) => String(v));

export const SummarySchema = z.object({
  trading_date: z.string(),
  nepse_index: numericString,
  point_change: numericString,
  percent_change: numericString,
  turnover: numericString,
  transactions: z.number(),
  traded_shares: numericString,
  traded_securities: z.number(),
  advanced: z.number(),
  declined: z.number(),
  unchanged: z.number(),
  market_cap: numericString,
  float_market_cap: numericString,
  cap_covered: z.number(),
  cap_eligible: z.number(),
  cap_method: z.string(),
  ingested_at: z.string(),
});
export type Summary = z.infer<typeof SummarySchema>;

export const MoverSchema = z.object({
  symbol: z.string(),
  name: z.string(),
  close: numericString,
  previous_close: numericString,
  change: numericString,
  percent_change: numericString,
  volume: numericString,
  turnover: numericString,
});
export type Mover = z.infer<typeof MoverSchema>;

export const CompanySchema = z.object({
  symbol: z.string(),
  name: z.string(),
  sector: z.string(),
  sector_code: z.string().optional(),
  status: z.string(),
  instrument: z.string(),
  close: numericString.optional(),
  previous_close: numericString.optional(),
  change: numericString.optional(),
  percent_change: numericString.optional(),
  volume: numericString.optional(),
  turnover: numericString.optional(),
  transactions: numericString.optional(),
  listed_shares: numericString.optional(),
  public_shares: numericString.optional(),
  market_cap: numericString.optional(),
  market_cap_method: z.string(),
  market_cap_reported: numericString.optional(),
  market_cap_reported_basis: z.string().optional(),
  float_market_cap: numericString.optional(),
  float_market_cap_method: z.string(),
  trading_date: z.string().optional(),
  ingested_at: z.string().optional(),
});
export type Company = z.infer<typeof CompanySchema>;

export const BarSchema = z.object({
  trading_date: z.string(),
  open: numericString,
  high: numericString,
  low: numericString,
  close: numericString,
  previous_close: numericString,
  volume: numericString,
  turnover: numericString,
  transactions: numericString,
  market_cap: numericString.optional(),
});
export type Bar = z.infer<typeof BarSchema>;

export const IndexSchema = z.object({
  code: z.string(),
  name: z.string(),
  kind: z.string(),
  value: numericString,
  point_change: numericString,
  percent_change: numericString,
  trading_date: z.string(),
});
export type IndexQuote = z.infer<typeof IndexSchema>;

export const TradeSchema = z.object({
  contract_id: z.string(),
  trading_date: z.string(),
  symbol: z.string(),
  buyer_broker: z.string().nullable(),
  seller_broker: z.string().nullable(),
  quantity: numericString,
  rate: numericString,
  amount: numericString,
  executed_at: z.string().nullable(),
});
export type Trade = z.infer<typeof TradeSchema>;

/** One member's session totals. Coverage is stated separately, never assumed. */
export const BrokerSummarySchema = z.object({
  broker_code: z.string(),
  broker_name: z.string().nullable(),
  buy_contracts: z.number(),
  sell_contracts: z.number(),
  buy_quantity: numericString,
  sell_quantity: numericString,
  buy_turnover: numericString,
  sell_turnover: numericString,
  net_turnover: numericString,
});
export type BrokerSummary = z.infer<typeof BrokerSummarySchema>;

/** How much of a session can be attributed to a member id at all. */
export const BrokerCoverageSchema = z.object({
  trading_date: z.string(),
  contracts: z.number(),
  with_buyer: z.number(),
  with_seller: z.number(),
  with_buyer_id: z.number(),
  with_seller_id: z.number(),
  attributed: z.number(),
  unattributed: z.number(),
  brokers: z.number(),
  turnover: numericString.optional(),
  coverage_ratio: z.string().optional(),
});
export type BrokerCoverage = z.infer<typeof BrokerCoverageSchema>;

export const BrokerResponseSchema = z.object({
  brokers: z.array(BrokerSummarySchema),
  coverage: BrokerCoverageSchema.nullable(),
});

/** Evidence for one session: what the archive holds and whether it was proven. */
export const DateCoverageSchema = z.object({
  trading_date: z.string(),
  contracts: z.number(),
  contracts_reported: z.number().optional(),
  missing_contracts: z.number().optional(),
  turnover: numericString,
  symbols: z.number(),
  pages_present: z.number(),
  pages_expected: z.number(),
  converged: z.boolean().optional(),
  run_status: z.string().optional(),
  verified: z.boolean(),
  status: z.enum(["complete", "incomplete", "missing"]),
  timestamp_precision: z.string().optional(),
  timezone_basis: z.string().optional(),
  unattributed_contracts: z.number(),
});
export type DateCoverage = z.infer<typeof DateCoverageSchema>;

export const FloorsheetRunSchema = z.object({
  id: z.string(),
  trading_date: z.string(),
  source: z.string(),
  status: z.string(),
  pages_expected: z.number(),
  pages_present: z.number(),
  pages_missing: z.number(),
  rows_in_pages: z.number(),
  unique_contracts: z.number(),
  duplicate_contracts: z.number(),
  total_elements: z.number(),
  missing_contracts: z.number(),
  converged: z.boolean(),
  date_mismatches: z.number(),
  digest_failures: z.number(),
  rows_written: z.number(),
  rows_unchanged: z.number(),
  timestamp_precision: z.string().optional(),
  timezone_basis: z.string().optional(),
  started_at: z.string(),
  finished_at: z.string().nullable(),
});
export type FloorsheetRun = z.infer<typeof FloorsheetRunSchema>;

export const FloorsheetCoverageSchema = z.object({
  coverage: z.object({
    earliest_verified: z.string(),
    latest_verified: z.string(),
    earliest_present: z.string(),
    latest_present: z.string(),
    sessions_tracked: z.number(),
    missing_dates: z.array(z.string()),
    dates: z.array(DateCoverageSchema),
  }),
  runs: z.array(FloorsheetRunSchema),
});
export type FloorsheetCoverage = z.infer<typeof FloorsheetCoverageSchema>;

export const MarketStatusSchema = z.object({
  state: z.string(),
  reported_status: z.string(),
  as_of: z.string().nullable(),
  observed_at: z.string().nullable(),
  age_seconds: z.number().nullable(),
  session_date: z.string(),
  is_open: z.boolean(),
  is_current_session: z.boolean(),
});
export type MarketStatus = z.infer<typeof MarketStatusSchema>;

export const OverviewSchema = z.object({
  summary: SummarySchema,
  status: MarketStatusSchema.nullable(),
  movers: z.object({
    gainers: z.array(MoverSchema),
    losers: z.array(MoverSchema),
    turnover: z.array(MoverSchema),
  }),
  indices: z.array(IndexSchema),
  generated_at: z.string(),
});
export type Overview = z.infer<typeof OverviewSchema>;

export const SectorSchema = z.object({
  code: z.string(),
  name: z.string(),
  companies: z.number(),
});
export type SectorOption = z.infer<typeof SectorSchema>;

export const HealthSchema = z.object({
  companies: z.number(),
  securities: z.number(),
  sessions: z.number(),
  trades: z.number(),
  bars: z.number(),
  latest_session: z.string(),
  latest_ingest: z.string(),
  schema_version: z.string(),
});
export type Health = z.infer<typeof HealthSchema>;

export const UserSchema = z.object({
  id: z.string(),
  email: z.string(),
  display_name: z.string(),
  status: z.string(),
  roles: z.array(z.string()),
  created_at: z.string(),
  last_login_at: z.string().nullable(),
});
export type User = z.infer<typeof UserSchema>;

export const SessionSchema = z.object({
  id: z.string(),
  family_id: z.string(),
  user_agent: z.string(),
  created_at: z.string(),
  expires_at: z.string(),
  revoked_at: z.string().nullable(),
});
export type UserSession = z.infer<typeof SessionSchema>;

export const SignedOutSchema = z.object({ signed_out: z.boolean() });

/** Credential rules, mirrored from internal/httpapi validateCredentials. */
export const CredentialsSchema = z.object({
  email: z.string().trim().email("Enter a valid email address").max(254),
  password: z
    .string()
    .min(10, "Use at least 10 characters")
    .max(128, "Use at most 128 characters"),
  display_name: z.string().trim().max(120).optional(),
});

export const RegisterSchema = CredentialsSchema.extend({
  display_name: z.string().trim().min(1, "Tell us what to call you").max(120),
});

export type RegisterInput = z.infer<typeof RegisterSchema>;
export type LoginInput = z.infer<typeof CredentialsSchema>;

// ------------------------------------------------------------------ charts & indicators

export const OHLCVBarSchema = z.object({
  time: z.string(),
  open: numericString,
  high: numericString,
  low: numericString,
  close: numericString,
  volume: numericString,
  turnover: numericString.optional(),
  transactions: numericString.optional(),
  previous_close: numericString.optional(),
});
export type OHLCVBar = z.infer<typeof OHLCVBarSchema>;

export const IndicatorPointSchema = z.object({
  t: z.string(),
  v: z.number().nullable().optional(),
  macd: z.number().nullable().optional(),
  signal: z.number().nullable().optional(),
  hist: z.number().nullable().optional(),
  upper: z.number().nullable().optional(),
  middle: z.number().nullable().optional(),
  lower: z.number().nullable().optional(),
  k: z.number().nullable().optional(),
  d: z.number().nullable().optional(),
});
export type IndicatorPoint = z.infer<typeof IndicatorPointSchema>;

export const IndicatorResultSchema = z.object({
  indicator: z.string(),
  params: z.record(z.string(), z.any()),
  warmup_bars: z.number(),
  series: z.array(IndicatorPointSchema),
});
export type IndicatorResult = z.infer<typeof IndicatorResultSchema>;

export const ChartLayoutSchema = z.object({
  id: z.string(),
  user_id: z.string().nullable().optional(),
  name: z.string(),
  symbol: z.string(),
  interval: z.string(),
  chart_type: z.string(),
  indicators: z.any(),
  settings: z.any(),
  created_at: z.string(),
  updated_at: z.string(),
});
export type ChartLayout = z.infer<typeof ChartLayoutSchema>;

export const IndexTrendPointSchema = z.object({
  time: z.string(),
  value: numericString,
  point_change: numericString.optional(),
  percent_change: numericString.optional(),
  granularity: z.string(),
});
export type IndexTrendPoint = z.infer<typeof IndexTrendPointSchema>;

export const SectorComparisonItemSchema = z.object({
  code: z.string(),
  name: z.string(),
  current_value: numericString,
  start_value: numericString,
  point_change: numericString,
  percent_change: z.number(),
  turnover: numericString.optional(),
  market_cap: numericString.optional(),
  scrips_count: z.number(),
  weight_pct: z.number().optional(),
});
export type SectorComparisonItem = z.infer<typeof SectorComparisonItemSchema>;

export const MarketBreadthSchema = z.object({
  advanced: z.number(),
  declined: z.number(),
  unchanged: z.number(),
  total: z.number(),
  advance_ratio: z.number(),
  distribution: z.record(z.string(), z.number()),
  // The API sends null, not an absent key, when no sector breakdown was
  // computed. `optional()` only accepts undefined, so this is nullish and
  // normalised to undefined for the renderers.
  by_sector: z
    .array(
      z.object({
        sector: z.string(),
        advanced: z.number(),
        declined: z.number(),
        unchanged: z.number(),
      }),
    )
    .nullish()
    .transform((v) => v ?? undefined),
});
export type MarketBreadth = z.infer<typeof MarketBreadthSchema>;

