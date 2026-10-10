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
hash. Collection reserves 350 seconds before claiming a candidate with up to ten reads,
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

A canonical landing can publish its base offer and up to two plus offers. The
site-wide `prices-config` map is insufficient: each plus SKU needs one real DOM
`common-flow-card__wrapper` with its ID and exactly one official `/profile/<sku>/`
link inside that wrapper. Escaped framework copies are ignored. Each tariff has
independent profession, RUB full-payment and cohort GETs. Profession landing paths,
when present, must match the parent canonical; language must match the base course.
No synthetic course URL or redirected landing establishes identity.

Normalized queue envelopes retain the parent and individual tariff facts or fixed
failure codes. A failed plus read leaves its previous offer unchanged and clears
only that tariff's anomaly confirmation. Partial tariff failures/protection keep the parent on its 12-hour refresh schedule; they do not impose a seven-day hold on healthy siblings. Base verification failure resets all
parent confirmations. Both cases retain the public catalog and allow later retries.
Offer leases are capped at 26 hours and by actual payment/discount deadlines.
Missing tariffs are retained, rather than silently deleted or refreshed.

Existing base profession/product and operator IDs remain protected. Plus bindings
persist SKU, profession UUID, billable product UUID and offer ID in nullable
`catalog_identities.practicum_tariffs`. Changes to one plus identity protect that
one tariff while verified siblings can refresh. Operator-added offers remain
untouched; an unbound offer-ID collision cannot be adopted. Price and enrollment
anomalies use independent per-SKU confirmation keys. Catalog updates, binding
changes and queue acknowledgements share a transaction; acknowledgement failure
rolls them back together. The older base-only contract remains valid on rollback.

Recorded official Python and Go plus API responses and a reduced displayed-card
fixture exercise shared-page behavior. The Go page identity substitution is a
parser test, not a claim of live Go landing verification.

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
