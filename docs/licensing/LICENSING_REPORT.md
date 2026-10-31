# LICENSING REPORT — Data Rights Gap Analysis

**Phase 0 deliverable. This is a blocker, not a caveat.**

Date: 2026-10-29. This document records what could and could not be verified. It
is an engineering risk assessment, not legal advice; the operative documents must
be obtained from NEPSE before any public, commercial, or paid release.

---

## 1. The finding that matters

The platform currently obtains market data from `nepalstock.com.np` through
`nepse-client`, a **third-party library that explicitly states it is unofficial**:

> "NOT affiliated with, endorsed by, or officially connected to" NEPSE
> — [nepseclient.readthedocs.io](https://nepseclient.readthedocs.io/),
> [github.com/4mritgiri/NepseClient](https://github.com/4mritgiri/NepseClient)

NEPSE separately operates a **formal, paid data-access procedure** — the
*Data Usage Procedure 2077*, approved by SEBON — under which applicants disclose
their intended use, provide technical access details, pay a fee, and are subject
to penalties if data is used for unauthorised purposes:

> "If the data provided is used for unauthorized purposes, NEPSE has the right to
> discontinue the service and even impose a fine as a penalty."
> — [ShareSansar, 27 Dec 2020](https://www.sharesansar.com/newsdetail/another-milestone-nepse-finally-brings-data-api-after-a-long-wait-real-time-data-with-only-30-second-delay-2020-12-27)

Reporting indicates the fee is **highest for recipients intending to redistribute
data**. One documented commercial arrangement (2023) was reported at
**NPR 40,000/month plus NPR 200,000 installation**
([ShareSansar, 23 May 2023](https://www.sharesansar.com/newsdetail/nepse-begins-commercial-sale-of-official-statistics-2023-05-23));
that is one agreement, not a published tariff.

**Conclusion:** the current ingestion path is not an authorised commercial data
feed. Building a professional terminal on it and exposing data through a public
API, exports, or paid tiers would carry material legal risk.

## 2. What could not be verified

| Question | Status |
|---|---|
| Full text of NEPSE's Data Usage Procedure 2077 and its fee schedule | **Not retrieved** — the procedure was linked in reporting as embedded PDFs that could not be read |
| A NEPSE-hosted public page expressly prohibiting scraping or automated access | **Not located.** Absence of a readable page is not evidence of permission |
| Current NEPSE tariff | **Not verified** — only a 2023 example |
| A NEPSE-hosted list of authorised resellers | **Not located** |
| Whether derived analytics (market cap, indicators, screener output) may be redistributed | **Not verified** |
| News/document redistribution rights for NEPSE notices and company PDFs | **Not verified** |
| SEBON rules specifically governing third-party redistribution | No standalone rule found; control appears to run through the NEPSE procedure and the applicant's agreement ([SEBON clarification, 27 Sep 2022](https://www.sharesansar.com/newsdetail/sebon-clarifies-that-nepses-api-data-usage-procedure-fees-have-not-increased-2022-09-27)) |

## 3. Someone claims to be licensed — treat as unverified

**Source Code Pvt. Ltd.** markets a Market Data Provider product and states on its
own pages that it is officially licensed by NEPSE and an official data
distributor ([sourcecode.com.np/product/mdp](https://sourcecode.com.np/product/mdp),
[data.smartwealthpro.com/about](https://data.smartwealthpro.com/about/)).

That is a **vendor claim**. No NEPSE-published confirmation was found, so the
scope, duration and current status of any agreement behind it are unverified.
It is nonetheless the most credible route to a lawful feed and should be
evaluated directly. ShareHub's commercial API exists but no licensing claim was
located, so it should not be treated as authorised on current evidence.

## 4. Data classification for everything currently held

Every dataset in [DATA_SOURCE_MATRIX.md](DATA_SOURCE_MATRIX.md) is classified:

```
publicly accessible, licence unverified
```

That classification governs what the product may do with it:

| Use | Permitted today? |
|---|---|
| Local single-user research dashboard | Lowest risk; still subject to NEPSE terms |
| Public read-only website | **Not cleared** |
| Public REST API for third parties | **Not cleared** |
| Bulk CSV/Parquet downloads of trade or price data | **Not cleared** |
| Paid subscription tiers | **Not cleared** |
| Derived indicators computed locally | Unclear — depends on the agreement |
| Redistribution of news text or company PDFs | **Not cleared** |

## 5. Required before production release

1. Obtain the **Data Usage Procedure 2077** text and the current fee schedule
   directly from NEPSE, and the application checklist.
2. Apply for a data licence stating the intended use: terminal, public API, and/or
   redistribution — the fee differs by category.
3. Or contract a verified licensed vendor, obtaining written confirmation of
   redistribution rights for API and export use.
4. Confirm in writing whether derived analytics may be redistributed.
5. Confirm automated polling limits and permitted frequency (the current build
   polls floorsheet every second, which NEPSE has not sanctioned).
6. Confirm news and document redistribution terms.

## 6. Engineering consequences (already designed in)

These are not future concerns; they change the architecture:

1. **A source registry with entitlement flags** is a first-class backend concern,
   not a comment. Every record carries licence classification and redistribution
   permission (brief §16).
2. **The ingestion adapter must be swappable.** Unofficial client today, licensed
   feed later, with no schema change — that is why §15 of the brief puts adapters
   behind an interface.
3. **Exports and the public API must be entitlement-gated** at the API layer, not
   only in the UI.
4. **The AI assistant must not ship** before redistribution terms are settled,
   because retrieval-augmented answers re-publish the underlying data.
5. **Polling cadence must be configurable down to zero**, so ingestion can be
   throttled or disabled without a code change if NEPSE objects.

## 7. Recommendation

Proceed with the migration on the **engineering** axis (Go, PostgreSQL, Next.js,
auth, schema, tests) because that work is transport-agnostic and preserves this
archive. Treat **publication** as gated: build the entitlement layer from Phase 1,
keep preview deployments private, and do not open a public API, bulk export, or
paid tier until item 5.1–5.3 is resolved in writing.
