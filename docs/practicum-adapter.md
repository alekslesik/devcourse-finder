# Yandex Practicum adapter

The API collector's `yandex` adapter discovers candidates from the sitemap
advertised by the official robots file:
<https://practicum.yandex.ru/lang-static/sitemap/>. Only canonical, one-segment
course URLs with a supported language hint are queued. Sitemap entries are
candidates, not verified courses.

## Verification contract

Each candidate requires four independent anonymous GET responses on
`practicum.yandex.ru`, with the existing TLS, host, redirect and body safeguards:

1. The Russian landing page must have one matching canonical URL, one matching
   JSON-LD Online Course Product with the exact SKU, and one `prices-config`
   JSON script mapping that SKU to the exact fixed official price endpoint.
   HTML-supplied URLs are compared, never followed. SEO prices and availability
   are not used; recorded SEO prices can be expired monthly payment figures.
2. `/api/v2/professions-by-slugs/?slugs=<slug>` must return exactly one matching
   profession, a valid ID, a base/plus full-program tariff, explicit purchase and
   visibility flags, RUB base price and a matching landing path if supplied.
   Demand-test pages and interactive textbooks are rejected.
3. `/api/v3/professions/prices/?slugs=<slug>` must bind the same slug to a RUB
   product with an explicit non-recurring charge flag. `profession_price` must
   match the profession's full price. `profession_final_price` supplies the full
   payable amount; partial payments, bank installments, credit and B2B amounts
   never supply it. Public discounts must reconcile with the original amount,
   absolute discount and percentage (up to one ruble of integer rounding), and
   have a future deadline. Personalized promocodes/certificates and special
   promotional tariffs are rejected. Zero prices cannot establish that a full
   program is free, even when a free introductory track exists.
4. `/api/professions/<slug>/nearest_squads/` must return a valid array of explicitly
   bound cohorts, with start/payment dates and seat counts/limits. An open offer
   requires purchase availability, a future payment deadline and available seats.
   Empty/full/expired cohorts produce a closed observation; existing two-run
   confirmation still applies, and a new closed program is not published.
   Zone-less timestamps are interpreted as Moscow time. Offer validity is capped
   by the discount deadline and the last available cohort's payment deadline.

The page, profession, price and cohort hashes all contribute to the stored evidence
hash. Collection reserves 140 seconds before claiming a four-read candidate,
within the existing ten-minute run. The publisher does not make HTTP requests.

Profession IDs and billable product IDs are separate: actual Java extended and
frontend responses demonstrate that they need not be equal. Both identities are
persisted with the catalog identity in the publication transaction. A subsequent
ID change is protected instead of updating a different program at the same URL.
Such a rejected read also clears pending price/closure confirmations in the same
transaction, including identities that reuse operator-curated course IDs. The
original identity must then provide a new consecutive pair of observations.
Database migration adds nullable columns; existing identities are bound on their
next successful verified publication.

## Coverage and limitations

Recorded public API fixtures cover Python, Java, Go and frontend, plus an extended
Java tariff. The landing fixture is a reduced authentic Python page; other-language
parser tests explicitly substitute its identity to exercise the contract. Those
substitutions are not claimed as successful live page verification.

A single canonical landing produces one complete program offer. Dedicated plus
pages can be accepted when they meet the same contract. A plus SKU whose official
landing is the base course's page, a redirected page, or a page without a matching
`prices-config` mapping is skipped. Shared-page tariff grouping is tracked separately in
[issue #49](https://github.com/alekslesik/devcourse-finder/issues/49). This avoids assigning a base price to an extended program or creating
synthetic URLs to force an identity match.

On October 9, anonymous source research returned all four language price and
cohort APIs, while some landing pages returned protective HTML instead of course
content. Such pages fail before any extra API reads. They are not published, and
normal persisted retries continue; access on the VDS may differ. The API used by
the official frontend is not a documented stability guarantee: changed or missing
fields prevent refresh instead of weakening verification.

A fresh end-to-end verification on October 9 used the actual updater HTTP client
and all four current official responses, then queued and published the Python
program in a disposable PostgreSQL database: 137,000 RUB full price, open
enrollment, not free. No production database or VDS was accessed. Other-language
unit cases do not add to this measured live count.

## Operation

After merging, deploy the published release through **Deploy release**, then use
**Collect catalog** to request an extra run. The same twice-daily API-worker schedule
continues. Status shows actual published coverage and pending observations;
discovery totals and fixture counts are not production course counts. No new
secret, browser process, external queue or deployment trigger is required.

Fixed failure codes are `invalid_practicum_page`, `invalid_practicum_payload`,
`invalid_practicum_price`, `identity_mismatch`, `unverified_enrollment` and
`source_unavailable`. Existing publications remain intact on failed verification;
failed reads reset outstanding anomaly confirmations through the publisher.

## Verification commands

Normal CI uses deterministic reduced public fixtures and PostgreSQL integration
tests, without depending on provider availability. An optional live check is:

```bash
PRACTICUM_LIVE_CHECK=1 TEST_DATABASE_URL='<disposable database ending in _test>' \
  go test -v ./backend/updater -run TestPracticumLiveVerificationAndPublication
```

It performs four bounded official GETs and publishes only into an isolated test
schema. Network or contract failures fail the check; it does not bypass challenges.
