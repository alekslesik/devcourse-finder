# PurpleSchool adapter

Issue #40 adds the official robots-declared sitemap at
https://purpleschool.ru/sitemap.xml to the page collector. Supported programming
language hints select canonical `/course/<slug>` candidates; blog, knowledge-base,
subscription and bundle pages are excluded. Discovery alone never publishes.
The existing twice-daily schedule and bounded queue/publisher pipeline apply.

## Source contract

A Russian page must contain one matching canonical URL and one exact-identity
JSON-LD Course. Its code and name must agree with one bound course object in the
Next.js Flight stream. Flight scripts are decoded as JSON data, never executed.
The course must have matching positive internal IDs, published status, site
visibility and no planned release. Render-time CourseInstance start timestamps
are ignored: they are not enrollment/cohort dates.

Each active tariff requires an exact course ID and tariff ID, independent SOLO
purchase support, and no subscription binding. Its JSON-LD Offer must use the
fixed `https://app.purpleschool.ru/buy?course=<id>&v=3&tariffId=<id>` contract and
match the tariff's name, category, RUB full price and one-price specification.
Purchase URLs are compared, never fetched or stored as arbitrary endpoints.
Billing specifications, unknown price specifications and unverified dated offers
are rejected. Credit/extension/token-package/bundle prices are ignored.

For paid tariffs, a unique rendered tariff card must show the same full price,
original price and enabled purchase action. Current tariffs, schema offers and
rendered cards must agree. Legacy course-level price/discount fields are not used:
the recorded JavaScript page still contains a 2023 promotion, while its current
bound tariff cards show different prices. The current card, not the historical
promotion or monthly installment, establishes the verified amount.

Offers receive a 26-hour verification lease. This is an explicit freshness limit,
not an inferred promotion deadline. A failed check cannot extend the lease.
The next scheduled successful check refreshes it.

## Tariffs, free access and support

One course keeps separate self-study, AI and human-mentor offers. AI help does not
set human review/mentor flags. A mentor/practice tariff must contain explicit
human chat and experienced-review evidence. Tariff names and support semantics
must satisfy the verified contract; unsupported types remain unpublished.

Zero-price `Бесплатные модули` tariffs on paid courses are excluded. A standalone
`Бесплатный курс` needs a single zero tariff, explicit full-course/free page text,
and at least three distinct curriculum lessons, each bound to that free tariff.
This verifies the complete named introductory course, not a free profession.

## Publication and identity

The collector queues one normalized record with all verified tariffs and never
writes the catalog. The publisher revalidates every tariff and applies them in
one catalog transaction. Price anomalies are confirmed independently per tariff
in a later scheduled window; other verified tariffs can refresh in that window.
The rolling freshness lease is excluded from the anomaly fingerprint, allowing
identical prices to be confirmed across runs. Failures reset every tariff's
pending confirmation for that course, in both split-worker and legacy collection
modes.

Provider course IDs and category-to-tariff-ID bindings are persisted. Rebinding
or changing the supported tariff set blocks publication and resets confirmation.
Existing operator-curated or ambiguous records are protected rather than merged
by guesswork. New tariff layouts require a verified adapter extension; old prices
expire through their lease while a page is rejected.

## Verification and release

Reduced public fixtures cover Go and JavaScript with three paid tariffs each
(3,999 / 5,499 / 12,999 RUB), plus the standalone free Python introduction.
Negative tests cover mismatched prices and IDs, unauthorized purchase hosts,
recurring/expired specifications, unsupported markup and incomplete free access.
PostgreSQL tests cover collection/publication separation, multiple offers,
independent confirmation across leases, failed-read reset and identity protection.

Optional bounded live verification:

```sh
PURPLESCHOOL_LIVE_CHECK=1 go test ./backend/updater -run TestPurpleSchoolLiveVerification -v
```

The three official sample pages also passed in a non-root, read-only container
limited to 128 MiB and 0.5 CPU. Default tests use fixtures. Deploy manually through **Deploy release** after merge
and release publication; neither event automatically deploys the VDS.
