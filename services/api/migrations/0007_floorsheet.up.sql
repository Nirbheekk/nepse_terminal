-- 0007_floorsheet: page-level provenance, run receipts and broker summaries for
-- the trade-level archive.
--
-- The trade table already stores one row per contract with a unique contract id,
-- so re-ingestion is idempotent. What it cannot express is *whether the archive
-- it came from was complete*: a session served in 140 pages may have been
-- captured as 100 pages and still look like a valid, if smaller, dataset. These
-- tables record the pagination the exchange reported, the pages actually
-- received, and the verdict of the completeness check, so an incomplete capture
-- is stored as incomplete rather than as a quieter market.

-- ---------------------------------------------------------------- pages
-- One row per raw page payload received. The four-part key identifies the byte
-- stream: a re-sweep of the same session produces a new sweep number rather than
-- overwriting the first attempt, which is what makes page drift measurable.
CREATE TABLE floorsheet_pages (
    trading_date   date NOT NULL,
    session_id     text NOT NULL,
    sweep          integer NOT NULL CHECK (sweep >= 1),
    page_no        integer NOT NULL CHECK (page_no >= 0),
    page_rows      integer NOT NULL CHECK (page_rows >= 0),
    first_contract NUMERIC(30,0),
    last_contract  NUMERIC(30,0),
    total_pages    integer,
    total_elements bigint,
    sha256         text NOT NULL CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    rel_path       text NOT NULL,
    bytes          bigint NOT NULL CHECK (bytes >= 0),
    fetched_at     timestamptz,
    ingested_at    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (trading_date, session_id, sweep, page_no)
);

CREATE INDEX floorsheet_pages_date_idx ON floorsheet_pages (trading_date, sweep, page_no);

-- ---------------------------------------------------------------- runs
-- The verdict of one ingestion of one trading date, kept so that a later claim
-- of completeness can be checked against the evidence that produced it.
CREATE TABLE floorsheet_runs (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    trading_date        date NOT NULL,
    source              text NOT NULL,
    status              text NOT NULL
                        CHECK (status IN ('success', 'partial', 'failed')),
    pages_expected      integer NOT NULL DEFAULT 0,
    pages_present       integer NOT NULL DEFAULT 0,
    pages_missing       integer NOT NULL DEFAULT 0,
    rows_in_pages       integer NOT NULL DEFAULT 0,
    unique_contracts    integer NOT NULL DEFAULT 0,
    duplicate_contracts integer NOT NULL DEFAULT 0,
    total_elements      bigint NOT NULL DEFAULT 0,
    missing_contracts   integer NOT NULL DEFAULT 0,
    converged           boolean NOT NULL DEFAULT false,
    date_mismatches     integer NOT NULL DEFAULT 0,
    digest_failures     integer NOT NULL DEFAULT 0,
    rows_written        integer NOT NULL DEFAULT 0,
    rows_unchanged      integer NOT NULL DEFAULT 0,
    timestamp_precision text,
    timezone_basis      text,
    started_at          timestamptz NOT NULL DEFAULT now(),
    finished_at         timestamptz,
    detail              jsonb NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX floorsheet_runs_date_idx ON floorsheet_runs (trading_date, started_at DESC);
CREATE UNIQUE INDEX floorsheet_runs_date_source_key
    ON floorsheet_runs (trading_date, source, started_at);

-- ---------------------------------------------------------------- brokers
-- Turnover per broker per session, summed from the contracts themselves.
--
-- Only contracts that actually carry a member id contribute. The exchange leaves
-- buyerMemberId and sellerMemberId null on a large share of rows and supplies no
-- broker name at all, so this table covers a subset of the session; the coverage
-- is returned with the summary rather than implied by an empty column.
CREATE TABLE floorsheet_broker_summaries (
    trading_date     date NOT NULL,
    broker_code      text NOT NULL,
    buy_contracts    integer NOT NULL DEFAULT 0,
    sell_contracts   integer NOT NULL DEFAULT 0,
    buy_quantity     NUMERIC(24,4) NOT NULL DEFAULT 0,
    sell_quantity    NUMERIC(24,4) NOT NULL DEFAULT 0,
    buy_turnover     NUMERIC(24,4) NOT NULL DEFAULT 0,
    sell_turnover    NUMERIC(24,4) NOT NULL DEFAULT 0,
    net_turnover     NUMERIC(24,4) NOT NULL DEFAULT 0,
    computed_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (trading_date, broker_code)
);

CREATE INDEX floorsheet_broker_turnover_idx
    ON floorsheet_broker_summaries (trading_date, buy_turnover DESC);
