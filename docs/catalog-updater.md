# Automatic catalog updater

Production uses three isolated roles from one Go image:

| Service | Role | Sources | Schedule |
| --- | --- | --- | --- |
| `catalog-updater` | `api` | Stepik and configured Yandex sources | 09:00 and 21:00 Europe/Moscow |
| `catalog-pages` | `pages` | OTUS, Hexlet and CodeBasics | 09:00 and 21:00 Europe/Moscow |
| `catalog-publisher` | `publisher` | PostgreSQL observations; no HTTP fetching | Poll every 10 seconds |

Each collector has independent persisted run times, advisory lock and heartbeat.
A missed schedule runs once on startup; historical windows are not replayed.
Collectors retain separate ten-minute deadlines and sequential HTTP requests.
A failing or busy API collector does not hold the pages collector's lock.
SIGTERM stops each scheduler gracefully. Empty `CATALOG_WORKER` retains the
legacy single-worker behavior for older operational commands and local tests;
legacy collection cannot overlap split collection/publication.

Collectors enqueue versioned, identity-bound observations, including fixed failure
codes, without changing the public catalog. Candidate/source retry state and the
observation enqueue commit together. The publisher validates envelopes again,
processes at most 500 events per poll, and commits the catalog change, identity,
audit snapshot, watermark and acknowledgement in one transaction. Duplicate,
invalid and stale events are acknowledged without publication. FIFO processing
preserves intervening failure events and original observation timestamps for
6–26-hour anomaly confirmation. No data PR, approval or manual import is required.
The API invalidates its cache using the existing import revision.

Pending observations are capped at 50,000; reaching capacity fails enqueue rather
than dropping observations. Processed observations are incrementally pruned after
seven days or beyond the newest 25,000 rows. Events older than 48 hours, expired
offers, future timestamps and events older than an accepted source observation
cannot refresh the catalog. A prolonged publisher outage therefore requires fresh
collection after recovery, rather than replaying obsolete price evidence.

These roles do not add new school adapters. Practicum's dedicated price contract
and eight further providers are tracked in the [worker roadmap](catalog-workers-roadmap.md).

## Discovery and source coverage

`data/updater-sources.json` enables five official discovery feeds (Stepik, OTUS,
Yandex Practicum, Hexlet and CodeBasics) and the two curated Stepik refreshes.
Sitemap/catalog URLs identify candidates, not published records. CodeBasics uses
its verified Russian catalog at `/ru` (`kind: catalog`), rather than a nonexistent
`sitemap.xml`. Hexlet uses its robots-declared gzipped sitemap and only the
`programs.xml.gz` child, excluding blogs/Q&A/subscription pages. Provider identity,
canonical URL and provenance are persisted before individual detail verification.
Stepik uses course-promo sitemap shards and anonymous detail APIs; general school
sitemaps use course-path and programming-language hints to exclude unrelated URLs.
The detail adapter independently verifies the language and course identity.

The Stepik adapter requires a Russian-language public program with at least three
lessons and units, verified visibility/active flags, enrollment actions/dates and
explicit free/paid evidence. Free flags confirm zero even if price is null. Paid
RUB amounts are exact; missing or foreign-currency prices remain unknown.

CodeBasics uses its anonymous `web/languages/show` Inertia curriculum contract.
Canonical URL, locale, course/landing identity, published/listed/main flags,
successful built curriculum, at least three distinct lessons and an accessible
first-lesson link must all match. A fresh independent `/ru` fetch must still contain
the recorded platform-wide free-pricing FAQ answer; missing, changed or
contradictory pricing text prevents publication/refresh. Both evidence digests
contribute to the stored hash. Pricing is not inferred from an absent Offer or an
exercise's text. This verifies Go, Python, Java, JavaScript and TypeScript programs;
TypeScript uses the existing JavaScript language family. The independent pricing
read happens once per batch and reserves an additional request budget.

OTUS uses the recorded official JSON-LD Course/Offer full-payment contract.
The schema reader also recognizes exact-identity Course or Online Course Product
records from other schools, but positive prices from providers without a verified
full-payment contract remain **unknown**, even if JSON-LD calls them an Offer.
Aggregate starting prices are never full amounts. Expired offers, recurring
specifications, multiple tariffs, missing availability and unbound recommendations
are rejected. Free access requires an explicit full-course free flag and zero RUB.
Unsupported markup stays unpublished; no marketing-text price inference is used.

Discovery is bounded to ten configured feeds, one child sitemap per feed/run,
8 MiB fetched/decompressed documents, 100,000 sitemap locations and a 50,000-row
candidate queue. Persistent cursors advance across child failures. Each run checks
at most 210 queued details: up to 120 new/retry Stepik records and 30 Stepik
refreshes, plus six refreshes and six new/retry records per other provider.
Ranks are interleaved so every provider and refresh lane gets an early turn.
These are ceilings, not guaranteed throughput; slow responses reduce the batch. The two curated refreshes run first. Requests are sequential; a
15-minute persisted claim allows recovery after process termination. Runs retain a
10-minute deadline and stop claiming work when less than 35 seconds remain (70 seconds for the first CodeBasics detail plus
pricing-policy read).

Normal detail refresh/retry is due after 12 hours. Repeated failures back off to
24, 48 and at most 72 hours; protected identities are rechecked after seven days.
No operator approval is needed. Queue progress survives restart. A configured feed
may be unavailable without blocking the remaining feeds or already queued details.

Unknown direction, audience, goals, support, schedule and duration remain unknown in the API
and UI. Specific experience, goal and support filters exclude unconfirmed matches.
A paid course with unknown full price cannot match a confirmed budget or appear
free. Existing curated records retain their identity, classifications and edits.

## Publication rules

- HTTP errors, login/challenge pages, malformed JSON, changed identities, missing
  required fields and contradictory prices are skipped. Other sources continue.
- HTTPS, fixed adapter host allowlists, no redirects, verified TLS, 15-second
  request limits, 8 MiB bodies, one delayed retry for 429/5xx and a 10-minute run
  deadline bound network work. Direct connections reject private IP destinations.
- Configured records and verified discovered identities are updated. Other offers, classification,
  description, curated title and manually drafted/archived courses are preserved.
  An offer moved to another URL is protected rather than overwritten.
- An initial publication requires open/continuous enrollment plus either a
  confirmed full price or explicit paid evidence with an unknown full price.
  No template is published solely because its checked-in status is `published`.
- A free/paid transition, a price jump greater than 50%, or an explicit closure
  needs two matching observations 6–26 hours apart. A failed intervening fetch
  resets confirmation. Repeated restarts cannot provide instant confirmation.
- Missing evidence never refreshes the corresponding field's verification time.
  The existing 30-day price expiry continues to apply. An explicitly paid course
  with an unverified price stops being advertised as free immediately.
- Each accepted source update merges under the same transaction lock as manual
  imports. Current database state, validation, before/after history and publication
  commit together. A failed transaction cannot publish half a course. Imports keep
  the pre-update catalog snapshot and the `automatic-catalog-publisher` operator (legacy runs retain `automatic-catalog-updater`).

## Run an extra collection from GitHub

After the workflow PR merges into `main`, open **Actions → Collect catalog →
Run workflow**, select `main`, and click **Run workflow**. No input or terminal is
required. The workflow uses the existing production `VDS_PASS` secret, VDS host/user
variables and pinned SSH host key. It sends its helper over SSH and executes
`catalog-update once` in each **currently deployed** running role (API, pages, publisher), followed by
`catalog-update status`. Deploying the workflow release to the VDS is not required
for the button itself; deploy a newer application release to use its new adapters.

Collection logs show safe source results and actual coverage/queue statistics;
the Actions summary shows success/failure. This starts another bounded discovery
run, but checks only detail records whose persisted retry/refresh times are due.
It does not force all prices to refresh or guarantee a higher visible total.
The regular 09:00/21:00 Europe/Moscow schedule remains active.

Manual workflow runs are serialized. The helper also takes the same host lock as
release deployment before resolving `current`; if that lock is held, collection
fails with a retry message. PostgreSQL separately prevents overlap with scheduled
collection: if it is already running, retry after it finishes. A missing/stopped
worker or an older release without the updater is reported instead of starting
services. Diagnostics still run after a collection failure, and the failure exit
code is preserved.

## Deployment and diagnosis

Release deployment remains manual through **Deploy release**. After deploying
this version, the production Compose overlay automatically builds and starts all three
roles alongside the existing services. No new secret, host port or cron entry is
needed. Deployments remove orphan services, so selecting a pre-worker release
stops and removes the collector. The manual workflow passes this policy to older
release scripts as well; database volumes are preserved. Local development and smoke/e2e fixtures do not start the worker.

From `/srv/devcourse-finder/current`, using the existing production environment:

```bash
docker compose --env-file /srv/devcourse-finder/.env \
  -f compose.yaml -f compose.production.yaml ps catalog-updater catalog-pages catalog-publisher

docker compose --env-file /srv/devcourse-finder/.env \
  -f compose.yaml -f compose.production.yaml logs --tail=100 catalog-updater catalog-pages catalog-publisher

docker compose --env-file /srv/devcourse-finder/.env \
  -f compose.yaml -f compose.production.yaml exec -T \
  catalog-updater devcourse-finder catalog-update status
```

`catalog-update health` checks the selected role's database heartbeat younger than 90 seconds.
Status includes all three heartbeats and publication queue depth, oldest pending
observation and last acknowledgement. A live publisher with a growing queue needs
log/database investigation; health alone does not prove publication progress.
It checks worker liveness, **not** whether every school is available. Source status
includes attempted/verified timestamps, safe error codes and consecutive failures.
The worker emits an error-level event after three consecutive source failures;
connect existing log monitoring to this event if external notifications are needed.
No Telegram/email integration is configured by this change.

Status also reports actual course/offer totals by language/provider, confirmed free
offers, paid offers, unknown/stale prices, oldest price check, queue states and feed
failures. These numbers exclude demo and unpublished records. They are measured
coverage, not a promised count or a worker-health indicator.

Runs are stored in `updater_runs`; per-source status and SHA-256 evidence in
`updater_sources`; pending confirmations in `updater_candidates`; catalog snapshots
in `imports`; feed cursors/status in `discovery_feeds`; durable work in
`catalog_candidates`; unique course/offer mappings in `catalog_identities`. A successful source may publish even if another fails (`partial`).
If all sources fail, no courses are published and the next scheduled run remains
active. A process killed during collection leaves a `running` row with no finish
time, making the interruption visible. `catalog-update once` supports an explicit
extra collection and exits unsuccessfully if every source fails.

The before/after history is a catalog rollback aid, **not a database backup**.
Existing pre-deployment backups remain in place. Daily PostgreSQL backups and
retention/restore checks remain a separate infrastructure task; do not assume this
worker has enabled them. Never prune imports without a separate retention policy.

Official API documentation: <https://github.com/StepicOrg/Stepik-API> and
<https://stepik.org/api/docs/>.

## Verification

PostgreSQL integration tests cover discovery-to-publication, idempotency, existing
curated identity reuse, source isolation, cache invalidation, confirmed closures,
manual URL protection, rollback and recovery. A **synthetic** 100-candidate test
verifies bounded multi-run progress and four-language coverage; it does not count
as 100 real courses. Recorded OTUS, Stepik and expired Yandex fixtures test the
actual contracts. Browser tests verify unknown support is not advertised as absent.

On October 8, live collection in a disposable database discovered 20 OTUS candidates
and published verified programs across the target languages (see the implementation
report in the expansion specification). Some Stepik discovery/detail requests and
Yandex, Hexlet and CodeBasics feeds returned access/server errors in this execution
environment. The expired Yandex starting-price offer was rejected. Source access
on the VDS can differ; unavailable feeds keep retrying automatically. The **100
verified-program goal is not yet met**. No placeholders were added to raise totals,
and these checks did not access or deploy to the VDS database.


The production worker image built successfully. Runtime checks passed as non-root
`app`, with a read-only filesystem, all capabilities dropped, no published ports
and the existing 128 MiB limit (`GOMEMLIMIT=96MiB`). Idle memory was approximately
3 MiB; this is a liveness measurement, not a peak-collection benchmark. Health and
coverage status succeeded, and SIGTERM exited with code 0. Frontend production
build/routes, six Chromium pricing/support checks, API compatibility/types,
release metadata and production Compose validation passed.


The source-coverage follow-up independently fetched the real CodeBasics catalog
and five complete detail pages, then published all five into a disposable database.
The focused run also verified the two curated Stepik records: seven real
programs at that stage, with demo fixtures excluded from coverage. PostgreSQL race and
failure-isolation tests passed, including refusal to refresh when the current
platform pricing evidence disappears. Hexlet's real program map contains 138 URLs;
the corrected production configuration queued 41 language-hinted candidates,
**not verified/published courses**. Its marketing detail pages
still need a dedicated full-tariff/enrollment contract. Stepik's sitemap shard and
search/list access still returned 403; no bypass or synthetic IDs were used.


The updated collector binary also completed a live CodeBasics refresh in the
production runtime image as `app`, with a read-only filesystem, all capabilities
dropped and a 128 MiB memory limit (`GOMEMLIMIT=96MiB`). It exited 0 without an
OOM kill. This was a disposable-database verification, not a VDS deployment.

### Detail rejection diagnostics

The candidate `code` in collection logs and persisted queue rows identifies the
failed verification step. Stepik reports `invalid_payload`, `identity_mismatch`,
`unsupported_content_language`, `insufficient_curriculum`,
`missing_visibility_or_price_flags`, `private_or_censored_course`,
`inactive_course`, and `invalid_price_evidence`. Normalization reports
`invalid_course_text` or `unsupported_or_ambiguous_language` (the four supported
programming languages are Go, Python, Java and JavaScript/TypeScript).
Structured pages report `invalid_structured_payload`, `missing_course_schema`,
`unverified_course_offer`, or `ambiguous_course_schema`. `unverified_course_offer`
means no uniquely bound offer passed all availability, price and validity checks;
it does not assert which individual field failed. Other adapter failures retain
`invalid_course`; failed network reads retain `source_unavailable`.
Only fixed codes are recorded, never raw source bodies or underlying error text.
Rejection leaves existing publications intact and keeps the bounded retry policy.

### Split-worker deployment and recovery

All three roles run as the existing non-root app, with read-only data mounts,
no published ports, dropped capabilities and 128 MiB limits each
(`GOMEMLIMIT=96MiB`; combined configured ceiling 384 MiB). No new dependency,
secret, message broker or host cron is required. `check-vds.sh` verifies every
configured role, its health and private ports.

The Collect catalog helper detects the deployed Compose services. Older releases
use one worker; split releases require all three running before collection starts.
It runs collectors sequentially, then the publisher, and always attempts status
while preserving the first failure. A manual publisher waits for background lock
contention within its ten-minute deadline. The workflow allows 40 minutes for the
three bounded commands and connection overhead.

On publisher failure, inspect its logs and status, restore it, then run Collect
catalog. Unacknowledged observations survive process/container restart and a
failed publication transaction. Invalid events cannot block later valid events.
Before rolling back to a legacy single-worker version, let publication drain and
record queue status. The older binary ignores remaining observation tables;
rollback does not delete them or replay them, and schema migration is additive.
Deployment remains manual through Deploy release; neither merge nor release
publication deploys the host.
