import { z } from "zod";
import {
  BarSchema,
  CompanySchema,
  HealthSchema,
  IndexSchema,
  MarketStatusSchema,
  MetaSchema,
  MoverSchema,
  OverviewSchema,
  SectorSchema,
  SessionSchema,
  SignedOutSchema,
  SummarySchema,
  TradeSchema,
  BrokerResponseSchema,
  FloorsheetCoverageSchema,
  UserSchema,
  OHLCVBarSchema,
  IndicatorResultSchema,
  ChartLayoutSchema,
  IndexTrendPointSchema,
  SectorComparisonItemSchema,
  MarketBreadthSchema,
  envelope,
  type Bar,
  type Company,
  type Health,
  type IndexQuote,
  type MarketStatus,
  type Meta,
  type Mover,
  type Overview,
  type SectorOption,
  type Summary,
  type Trade,
  type BrokerSummary,
  type FloorsheetCoverage,
  type User,
  type UserSession,
  type OHLCVBar,
  type IndicatorResult,
  type ChartLayout,
  type IndexTrendPoint,
  type SectorComparisonItem,
  type MarketBreadth,
} from "../schemas";
import {
  AlertDeliverySchema,
  AlertEventSchema,
  AlertRuleSchema,
  ArchiveSummarySchema,
  DisclosureSchema,
  FreshnessReportSchema,
  IngestRunSchema,
  PortfolioAnalyticsSchema,
  PortfolioSchema,
  ResearchResponseSchema,
  SavedScreenSchema,
  ScreenRunSchema,
  ScreenerCoverageSchema,
  ScreenerMetricSchema,
  WatchlistSchema,
  type AlertDelivery,
  type AlertEvent,
  type AlertRule,
  type ArchiveSummary,
  type Disclosure,
  type FreshnessStream,
  type IngestRun,
  type Portfolio,
  type PortfolioAnalytics,
  type ResearchResponse,
  type SavedScreen,
  type ScreenRun,
  type ScreenerCoverage,
  type ScreenerMetric,
  type Watchlist,
} from "../schemas/workspace";

/**
 * Typed client for the Go API.
 *
 * Two entry points exist because the two runtimes have different cookie rules:
 *
 *   - `serverGet` runs in a React Server Component. It has no browser cookie jar,
 *     so it only fetches public market data and marks it `no-store` (market data
 *     is never cached at the edge: a stale quote is worse than a slow one).
 *   - `clientFetch` runs in the browser and includes credentials plus the CSRF
 *     header for mutations.
 *
 * Both validate the response with Zod before it reaches a component.
 */

export const API_BASE =
  process.env.NEXT_PUBLIC_API_BASE_URL?.replace(/\/$/, "") ?? "http://localhost:8081";

export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly requestId?: string;

  constructor(message: string, status: number, code: string, requestId?: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
    this.requestId = requestId;
  }

  /** True when the failure is a data gap rather than a bug. */
  get isDataGap() {
    return this.code === "NOT_FOUND" || this.code === "DATA_UNAVAILABLE";
  }
}

export type Result<T> = { data: T; meta: Meta };

function parseEnvelope<T extends z.ZodTypeAny>(
  schema: T,
  raw: unknown,
  status: number,
  requestId: string | undefined,
  // Some endpoints (portfolios, disclosures) answer with `data: null` when the
  // owner has no rows. That is an empty collection, not a contract violation or
  // a failure, so callers that expect it ask for a nullable result instead of
  // being told "the API returned no data".
  allowNull = false,
): Result<z.infer<T> | null> {
  const parsed = envelope(schema).safeParse(raw);
  if (!parsed.success) {
    throw new ApiError(
      "The API returned a payload that does not match the published contract.",
      status,
      "CONTRACT_MISMATCH",
      requestId,
    );
  }
  const payload = parsed.data as {
    data: z.infer<T> | null;
    meta: Meta;
    error: { code: string; message: string; retryable: boolean } | null | undefined;
  };
  if (payload.error) {
    throw new ApiError(
      payload.error.message,
      status,
      payload.error.code,
      payload.meta?.request_id,
    );
  }
  if (payload.data === null || payload.data === undefined) {
    if (allowNull) {
      return { data: null, meta: MetaSchema.parse(payload.meta) };
    }
    throw new ApiError("The API returned no data.", status, "NO_DATA", requestId);
  }
  return { data: payload.data, meta: MetaSchema.parse(payload.meta) };
}

/**
 * Fetch with a deadline.
 *
 * Without one, an API process that accepts the connection but never answers
 * hangs the whole page render instead of showing the connection warning the UI
 * is built to show.
 */
async function timedFetch(url: string, init: RequestInit = {}): Promise<Response> {
  try {
    return await fetch(url, { ...init, signal: AbortSignal.timeout(8000) });
  } catch (err) {
    const timedOut = err instanceof Error && err.name === "TimeoutError";
    throw new ApiError(
      timedOut
        ? "The API did not answer within 8 seconds."
        : `The API at ${API_BASE} is not reachable.`,
      0,
      "API_UNREACHABLE",
    );
  }
}

async function readJson(res: Response): Promise<unknown> {
  const text = await res.text();
  if (!text) return null;
  try {
    return JSON.parse(text);
  } catch {
    return null;
  }
}

/** Server-side fetch of public data. Throws ApiError on any non-success. */
export async function serverGet<T extends z.ZodTypeAny>(
  path: string,
  schema: T,
): Promise<Result<z.infer<T>>> {
  const res = await timedFetch(`${API_BASE}${path}`, {
    cache: "no-store",
    headers: { Accept: "application/json" },
  });
  const body = await readJson(res);
  const requestId = res.headers.get("X-Request-Id") ?? undefined;

  if (!res.ok) {
    const parsed = z
      .object({ error: z.object({ code: z.string(), message: z.string() }).nullable() })
      .safeParse(body);
    throw new ApiError(
      parsed.success && parsed.data.error ? parsed.data.error.message : `Request failed (${res.status})`,
      res.status,
      parsed.success && parsed.data.error ? parsed.data.error.code : "HTTP_ERROR",
      requestId,
    );
  }	return parseEnvelope(schema, body, res.status, requestId) as Result<z.infer<T>>;
}

export function readCsrfCookie(): string {
  if (typeof document === "undefined") return "";
  const match = document.cookie.match(/(?:^|;\s*)np_csrf=([^;]+)/);
  const value = match?.[1];
  return value ? decodeURIComponent(value) : "";
}

/**
 * Exchange the refresh cookie for a new access cookie.
 *
 * Access tokens are short-lived by design; without this call a signed-in reader
 * silently became anonymous fifteen minutes after signing in, with the UI
 * showing "sign in" on pages they already had access to.
 *
 * Returns true when a retry is worth attempting. A failed refresh is not an
 * error to surface by itself: the caller then reports the original 401, which is
 * the honest state (the session really is over).
 */
async function refreshSession(): Promise<boolean> {
  try {
    const res = await timedFetch(`${API_BASE}/api/v1/auth/refresh`, {
      method: "POST",
      credentials: "include",
      cache: "no-store",
      headers: { Accept: "application/json", "X-CSRF-Token": readCsrfCookie() },
    });
    return res.ok;
  } catch {
    return false;
  }
}

/** Browser fetch: sends cookies, and the CSRF header for unsafe methods. */
export async function clientFetch<T extends z.ZodTypeAny>(
  path: string,
  schema: T,
  init: RequestInit = {},
): Promise<Result<z.infer<T>>> {
  return clientFetchRetrying(path, schema, init, true);
}

async function clientFetchRetrying<T extends z.ZodTypeAny>(
  path: string,
  schema: T,
  init: RequestInit,
  allowRefresh: boolean,
): Promise<Result<z.infer<T>>> {
  const method = (init.method ?? "GET").toUpperCase();
  const headers = new Headers(init.headers);
  headers.set("Accept", "application/json");
  if (init.body) headers.set("Content-Type", "application/json");
  if (!["GET", "HEAD", "OPTIONS"].includes(method)) {
    headers.set("X-CSRF-Token", readCsrfCookie());
  }

  const res = await timedFetch(`${API_BASE}${path}`, {
    ...init,
    method,
    headers,
    credentials: "include",
    cache: "no-store",
  });  const body = await readJson(res);
  const requestId = res.headers.get("X-Request-Id") ?? undefined;

  // One silent renewal per request. A 401 means the request was rejected before
  // it ran, so replaying a mutation here cannot apply it twice. Auth routes are
  // excluded: a failed login must not trigger a refresh of the session it failed
  // to create.
  if (res.status === 401 && allowRefresh && !path.startsWith("/api/v1/auth/")) {
    if (await refreshSession()) {
      return clientFetchRetrying(path, schema, init, false);
    }
  }

  if (!res.ok) {
    const parsed = z
      .object({ error: z.object({ code: z.string(), message: z.string() }).nullable() })
      .safeParse(body);
    throw new ApiError(
      parsed.success && parsed.data.error ? parsed.data.error.message : `Request failed (${res.status})`,
      res.status,
      parsed.success && parsed.data.error ? parsed.data.error.code : "HTTP_ERROR",
      requestId,
    );
  }
  return parseEnvelope(schema, body, res.status, requestId) as Result<z.infer<T>>;
}

/** Server fetch for endpoints that answer `data: null` for an empty set. */
export async function serverGetOptional<T extends z.ZodTypeAny>(
  path: string,
  schema: T,
): Promise<Result<z.infer<T> | null>> {
  const res = await timedFetch(`${API_BASE}${path}`, {
    cache: "no-store",
    headers: { Accept: "application/json" },
  });
  const body = await readJson(res);
  const requestId = res.headers.get("X-Request-Id") ?? undefined;
  if (!res.ok) {
    const parsed = z
      .object({ error: z.object({ code: z.string(), message: z.string() }).nullable() })
      .safeParse(body);
    throw new ApiError(
      parsed.success && parsed.data.error ? parsed.data.error.message : `Request failed (${res.status})`,
      res.status,
      parsed.success && parsed.data.error ? parsed.data.error.code : "HTTP_ERROR",
      requestId,
    );
  }
  return parseEnvelope(schema, body, res.status, requestId, true);
}

/** Browser fetch for endpoints that answer `data: null` for an empty set. */
export async function clientFetchOptional<T extends z.ZodTypeAny>(
  path: string,
  schema: T,
  init: RequestInit = {},
): Promise<Result<z.infer<T> | null>> {
  return clientFetchOptionalRetrying(path, schema, init, true);
}

async function clientFetchOptionalRetrying<T extends z.ZodTypeAny>(
  path: string,
  schema: T,
  init: RequestInit,
  allowRefresh: boolean,
): Promise<Result<z.infer<T> | null>> {
  const res = await timedFetch(`${API_BASE}${path}`, {
    ...init,
    credentials: "include",
    cache: "no-store",
    headers: { Accept: "application/json", ...(init.headers as Record<string, string>) },
  });
  const body = await readJson(res);
  const requestId = res.headers.get("X-Request-Id") ?? undefined;
  if (res.status === 401 && allowRefresh) {
    if (await refreshSession()) {
      return clientFetchOptionalRetrying(path, schema, init, false);
    }
  }
  if (!res.ok) {
    const parsed = z
      .object({ error: z.object({ code: z.string(), message: z.string() }).nullable() })
      .safeParse(body);
    throw new ApiError(
      parsed.success && parsed.data.error ? parsed.data.error.message : `Request failed (${res.status})`,
      res.status,
      parsed.success && parsed.data.error ? parsed.data.error.code : "HTTP_ERROR",
      requestId,
    );
  }
  return parseEnvelope(schema, body, res.status, requestId, true);
}

/** Company-screen query, mirrored by parseCompanyFilter on the API. */
export type CompanyQuery = {
  q?: string;
  sector?: string;
  instrument?: string;
  status?: string;
  sort?: string;
  order?: "asc" | "desc";
  limit?: number;
  offset?: number;
};

function companyQuery(p: CompanyQuery): string {
  const qs = new URLSearchParams();
  if (p.q) qs.set("q", p.q);
  if (p.sector) qs.set("sector", p.sector);
  if (p.instrument) qs.set("instrument", p.instrument);
  if (p.status) qs.set("status", p.status);
  if (p.sort) qs.set("sort", p.sort);
  if (p.order) qs.set("order", p.order);
  qs.set("limit", String(p.limit ?? 50));
  if (p.offset) qs.set("offset", String(p.offset));
  return qs.toString();
}

/**
 * Floorsheet query, mirrored by parseTradeFilter on the API (see
 * internal/httpapi/market_handlers.go). The page and the CSV export build their
 * parameters from the same object, so a download always matches the table.
 */
export type FloorsheetQuery = {
  date?: string;
  symbol?: string;
  broker?: string;
  side?: "buy" | "sell" | "";
  min_amount?: string;
  max_amount?: string;
  sort?: "contract_desc" | "amount_desc" | "amount_asc" | "time_desc" | "time_asc";
  cursor?: string;
  page?: number;
  limit?: number;
};

export function floorsheetQuery(p: FloorsheetQuery): string {
  const qs = new URLSearchParams();
  if (p.date) qs.set("date", p.date);
  if (p.symbol) qs.set("symbol", p.symbol);
  if (p.broker) qs.set("broker", p.broker);
  if (p.broker && p.side) qs.set("side", p.side);
  if (p.min_amount) qs.set("min_amount", p.min_amount);
  if (p.max_amount) qs.set("max_amount", p.max_amount);
  if (p.sort) qs.set("sort", p.sort);
  if (p.cursor) qs.set("cursor", p.cursor);
  if (p.page && p.page > 1) qs.set("page", String(p.page));
  qs.set("limit", String(p.limit ?? 50));
  return qs.toString();
}

/** Absolute URL of the CSV export for a filter, so a link can download it. */
export function floorsheetExportUrl(p: FloorsheetQuery): string {
  return `${API_BASE}/api/v1/floorsheet/export.csv?${floorsheetQuery({ ...p, cursor: undefined })}`;
}

export const api = {
  overview: () => serverGet("/api/v1/market/overview", OverviewSchema),
  summary: () => serverGet("/api/v1/market/summary", SummarySchema),
  status: () => serverGet("/api/v1/market/status", MarketStatusSchema),
  movers: (type: "gainers" | "losers" | "turnover" | "volume", limit = 8) =>
    serverGet(`/api/v1/market/top-movers?type=${type}&limit=${limit}`, z.array(MoverSchema)),
  indices: () => serverGet("/api/v1/indices", z.array(IndexSchema)),
  sectors: () => serverGet("/api/v1/sectors", z.array(SectorSchema)),
  health: () => serverGet("/api/v1/health", HealthSchema),
  companies: (params: CompanyQuery = {}) =>
    serverGet(`/api/v1/companies?${companyQuery(params)}`, z.array(CompanySchema)),
  company: (symbol: string) =>
    serverGet(`/api/v1/companies/${encodeURIComponent(symbol)}`, CompanySchema),
  prices: (symbol: string, limit = 120) =>
    serverGet(
      `/api/v1/companies/${encodeURIComponent(symbol)}/prices?limit=${limit}`,
      z.array(BarSchema),
    ),
  floorsheet: (params: FloorsheetQuery = {}) =>
    serverGet(`/api/v1/floorsheet?${floorsheetQuery(params)}`, z.array(TradeSchema)),
  floorsheetBrokers: (params: { date?: string; limit?: number } = {}) =>{
    const qs = new URLSearchParams();
    if (params.date) qs.set("date", params.date);
    qs.set("limit", String(params.limit ?? 10));
    return serverGet(`/api/v1/floorsheet/brokers?${qs.toString()}`, BrokerResponseSchema);
  },
  floorsheetCoverage: () =>
    serverGet("/api/v1/floorsheet/coverage", FloorsheetCoverageSchema),
  ohlcv: (symbol: string, interval = "1d", from = "", to = "") => {
    const qs = new URLSearchParams({ symbol, interval });
    if (from) qs.set("from", from);
    if (to) qs.set("to", to);
    return serverGet(`/api/v1/charts/ohlcv?${qs.toString()}`, z.array(OHLCVBarSchema));
  },
  indicators: (symbol: string, interval = "1d", set = "sma:20,ema:50,rsi:14") => {
    const qs = new URLSearchParams({ symbol, interval, set });
    return serverGet(`/api/v1/charts/indicators?${qs.toString()}`, z.array(IndicatorResultSchema));
  },
  indexHistory: (code: string, from = "", to = "") => {
    const qs = new URLSearchParams({ granularity: "daily" });
    if (from) qs.set("from", from);
    if (to) qs.set("to", to);
    return serverGet(`/api/v1/indices/${encodeURIComponent(code)}/history?${qs.toString()}`, z.array(IndexTrendPointSchema));
  },
  indexIntraday: (code: string, date = "") => {
    const qs = new URLSearchParams();
    if (date) qs.set("date", date);
    return serverGet(`/api/v1/indices/${encodeURIComponent(code)}/intraday?${qs.toString()}`, z.array(IndexTrendPointSchema));
  },
  sectorComparison: (period = "1d") =>
    serverGet(`/api/v1/sectors/comparison?period=${encodeURIComponent(period)}`, z.array(SectorComparisonItemSchema)),
  marketBreadth: () =>
    serverGet("/api/v1/market/breadth", MarketBreadthSchema),
  chartLayouts: () =>
    clientFetch("/api/v1/charts/layouts", z.array(ChartLayoutSchema)),
  saveChartLayout: (body: { name: string; symbol: string; interval: string; chart_type: string; indicators: any; settings: any }) =>
    clientFetch("/api/v1/charts/layouts", ChartLayoutSchema, {
      method: "POST",
      body: JSON.stringify(body),
    }),
  deleteChartLayout: (id: string) =>
    clientFetch(`/api/v1/charts/layouts/${encodeURIComponent(id)}`, z.object({ status: z.string() }), {
      method: "DELETE",
    }),
};

// ------------------------------------------------------------- account tools

/**
 * Acknowledgement body for deletes. The API answers {deleted:true} or
 * {removed:true} depending on the resource, so the key is not asserted; the
 * status code is what says the delete happened.
 */
const AckSchema = z.record(z.string(), z.unknown());

/** A screener rule, in the shape the API's validator accepts. */
export type ScreenRule = {
  universe?: { sectors?: string[]; instruments?: string[]; symbols?: string[]; search?: string };
  all?: { metric: string; op: string; value: string | number; value_to?: string | number }[];
  any?: { metric: string; op: string; value: string | number }[];
  sort?: string;
  order?: "asc" | "desc";
  limit?: number;
};

export const screener = {
  metrics: () =>
    clientFetch(
      "/api/v1/screener/metrics",
      z.object({ coverage: ScreenerCoverageSchema, metrics: z.array(ScreenerMetricSchema) }),
    ),
  run: (rule: ScreenRule) =>
    clientFetch("/api/v1/screener/run", ScreenRunSchema, {
      method: "POST",
      body: JSON.stringify({ rule }),
    }),
  screens: () => clientFetch("/api/v1/screener/screens", z.array(SavedScreenSchema)),
  save: (body: { name: string; description?: string; definition: ScreenRule }) =>
    clientFetch("/api/v1/screener/screens", SavedScreenSchema, {
      method: "POST",
      body: JSON.stringify(body),
    }),
  remove: (id: string) =>
    clientFetch(`/api/v1/screener/screens/${encodeURIComponent(id)}`, AckSchema, {
      method: "DELETE",
    }),
};

export const watchlists = {
  list: () => clientFetch("/api/v1/watchlists", z.array(WatchlistSchema)),
  create: (name: string) =>
    clientFetch("/api/v1/watchlists", WatchlistSchema, {
      method: "POST",
      body: JSON.stringify({ name }),
    }),
  get: (id: string) => clientFetch(`/api/v1/watchlists/${encodeURIComponent(id)}`, WatchlistSchema),
  add: (id: string, symbol: string, note = "") =>
    clientFetch(
      `/api/v1/watchlists/${encodeURIComponent(id)}/items`,
      z.object({ added: z.boolean(), symbol: z.string() }),
      { method: "POST", body: JSON.stringify({ symbol, note }) },
    ),
  remove: (id: string, symbol: string) =>
    clientFetch(
      `/api/v1/watchlists/${encodeURIComponent(id)}/items/${encodeURIComponent(symbol)}`,
      AckSchema,
      { method: "DELETE" },
    ),
  destroy: (id: string) =>
    clientFetch(`/api/v1/watchlists/${encodeURIComponent(id)}`, AckSchema, { method: "DELETE" }),
};

export const alerts = {
  list: () => clientFetch("/api/v1/alerts", z.array(AlertRuleSchema)),
  create: (body: {
    name: string;
    symbol?: string;
    metric: string;
    comparator: string;
    threshold: number | string;
    cooldown_seconds?: number;
  }) =>
    clientFetch("/api/v1/alerts", AlertRuleSchema, { method: "POST", body: JSON.stringify(body) }),
  // PUT is a full replacement, but the API fills every omitted field from the
  // stored rule, so sending only {"enabled": false} flips the switch without
  // blanking the metric.
  toggle: (id: string, enabled: boolean) =>
    clientFetch(`/api/v1/alerts/${encodeURIComponent(id)}`, AlertRuleSchema, {
      method: "PUT",
      body: JSON.stringify({ enabled }),
    }),
  remove: (id: string) =>
    clientFetch(`/api/v1/alerts/${encodeURIComponent(id)}`, AckSchema, { method: "DELETE" }),
  evaluate: () =>
    clientFetch("/api/v1/alerts/evaluate", z.record(z.string(), z.unknown()), { method: "POST" }),
  events: () => clientFetch("/api/v1/alerts/events", z.array(AlertEventSchema)),
  deliveries: () => clientFetch("/api/v1/alerts/deliveries", z.array(AlertDeliverySchema)),
};

export const portfolios = {
  list: () => clientFetchOptional("/api/v1/portfolios", z.array(PortfolioSchema)),
  create: (body: { name: string; description?: string; cost_basis_method?: string }) =>
    clientFetch("/api/v1/portfolios", PortfolioSchema, { method: "POST", body: JSON.stringify(body) }),
  destroy: (id: string) =>
    clientFetch(`/api/v1/portfolios/${encodeURIComponent(id)}`, AckSchema, { method: "DELETE" }),
  get: (id: string) => clientFetch(`/api/v1/portfolios/${encodeURIComponent(id)}`, PortfolioSchema),
  analytics: (id: string) =>
    clientFetch(`/api/v1/portfolios/${encodeURIComponent(id)}/analytics`, PortfolioAnalyticsSchema),
};

export const research = {
  query: (body: { query_text: string; scope?: string; symbol?: string }) =>
    clientFetch("/api/v1/research/query", ResearchResponseSchema, {
      method: "POST",
      body: JSON.stringify(body),
    }),
};

export const ops = {
  freshness: () => serverGet("/api/v1/health/freshness", FreshnessReportSchema),
  ingestRuns: (limit = 10) =>
    serverGet(`/api/v1/health/ingest-runs?limit=${limit}`, z.array(IngestRunSchema)),
  archives: () => serverGet("/api/v1/health/archives", z.array(ArchiveSummarySchema)),
  health: () => serverGet("/api/v1/health", HealthSchema),
};

export const disclosures = {
  list: (limit = 25) =>
    serverGetOptional(`/api/v1/disclosures?limit=${limit}`, z.array(DisclosureSchema)),
};

export const auth = {
  me: () => clientFetch("/api/v1/auth/me", UserSchema),
  login: (body: { email: string; password: string }) =>
    clientFetch("/api/v1/auth/login", UserSchema, {
      method: "POST",
      body: JSON.stringify(body),
    }),
  register: (body: { email: string; password: string; display_name: string }) =>
    clientFetch("/api/v1/auth/register", UserSchema, {
      method: "POST",
      body: JSON.stringify(body),
    }),
  refresh: () => clientFetch("/api/v1/auth/refresh", UserSchema, { method: "POST" }),
  logout: () =>
    clientFetch("/api/v1/auth/logout", SignedOutSchema, { method: "POST" }),
  sessions: () => clientFetch("/api/v1/auth/sessions", z.array(SessionSchema)),
  revokeSession: (id: string) =>
    clientFetch(`/api/v1/auth/sessions/${encodeURIComponent(id)}`, z.object({ revoked: z.boolean() }), {
      method: "DELETE",
    }),
};

/**
 * Absolute URLs for CSV downloads.
 *
 * Exports are fetched by the browser, not proxied through the web tier: a large
 * file should stream from the API that owns the data.
 */
export const exportUrl = {
  companies: (params: CompanyQuery = {}) => `${API_BASE}/api/v1/companies/export.csv?${companyQuery(params)}`,
  prices: (symbol: string) =>
    `${API_BASE}/api/v1/companies/${encodeURIComponent(symbol)}/prices.csv`,
};

export type {
  Bar,
  Company,
  Health,
  IndexQuote,
  MarketStatus,
  Mover,
  Overview,
  SectorOption,
  Summary,
  Trade,
  User,
  UserSession,
  OHLCVBar,
  IndicatorResult,
  ChartLayout,
  IndexTrendPoint,
  SectorComparisonItem,
  MarketBreadth,
  ScreenerMetric,
  ScreenerCoverage,
  ScreenRun,
  SavedScreen,
  Watchlist,
  AlertRule,
  AlertEvent,
  AlertDelivery,
  Portfolio,
  PortfolioAnalytics,
  Disclosure,
  ResearchResponse,
  FreshnessStream,
  IngestRun,
  ArchiveSummary,
};
