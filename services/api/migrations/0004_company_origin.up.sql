-- Reference data needs provenance too.
--
-- The exchange's company registry only lists ordinary equities, but its security
-- list also carries promoter and preference classes whose issuer is absent from
-- that registry. Those securities are still tradeable, so dropping them would
-- lose real contracts. The platform therefore creates a placeholder issuer for
-- the handful that cannot be resolved to a registered company, and marks where
-- each row came from so a placeholder is never counted as a listed company.

ALTER TABLE companies
    ADD COLUMN origin text NOT NULL DEFAULT 'registry';

ALTER TABLE companies
    ADD CONSTRAINT companies_origin_check
        CHECK (origin IN ('registry', 'listed_security'));

COMMENT ON COLUMN companies.origin IS
    'registry: present in the exchange company registry. listed_security: '
    'placeholder issuer created because a listed security referenced it and no '
    'registered company matched. Placeholders must not be counted as issuers.';

CREATE INDEX companies_origin_idx ON companies (origin) WHERE origin <> 'registry';

-- Promoter and preference classes trade under their issuer's ordinary symbol
-- plus a suffix (NABILP, CFCLPO). Stripping the suffix is only a lookup hint:
-- callers join the result against companies, so a mis-suffixed symbol resolves
-- to nothing rather than to a wrong issuer. A symbol with no class suffix
-- returns an empty string, which matches no company.
CREATE FUNCTION class_parent_symbol(symbol text) RETURNS text
LANGUAGE sql IMMUTABLE STRICT AS $$
    SELECT CASE
        WHEN length($1) > 3 AND $1 LIKE '%PO' THEN left($1, length($1) - 2)
        WHEN length($1) > 3 AND $1 LIKE '%P'  THEN left($1, length($1) - 1)
        ELSE ''
    END
$$;
