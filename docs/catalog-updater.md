# Automatic catalog updater

The production stack includes a separate `catalog-updater` process. It collects
configured official sources at **09:00 and 21:00 Europe/Moscow**, then validates
and publishes directly into PostgreSQL. No data PR, approval or manual import is
required. The API invalidates its catalog cache using the existing import revision.

On the first start, collection runs immediately. Restarts use persisted run times
to avoid repeating a schedule window. A missed window is collected once; the
service does not replay a backlog. PostgreSQL prevents overlapping collectors.
An interrupted collection is retried in the next window. SIGTERM stops collection
and the scheduler gracefully.

## Discovery and source coverage

`data/updater-sources.json` enables five official sitemap feeds (Stepik, OTUS,
Yandex Practicum, Hexlet and CodeBasics) and the two curated Stepik refreshes.
Sitemap URLs identify candidates, not published records. Provider identity,
canonical URL and provenance are persisted before individual detail verification.
Stepik uses course-promo sitemap shards and anonymous detail APIs; general school
sitemaps use course-path and programming-language hints to exclude unrelated URLs.
The detail adapter independently verifies the language and course identity.

The Stepik adapter requires a Russian-language public program with at least three
lessons and units, verified visibility/active flags, enrollment actions/dates and
explicit free/paid evidence. Free flags confirm zero even if price is null. Paid
RUB amounts are exact; missing or foreign-currency prices remain unknown.

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
at most 36 queued details, interleaving up to six refreshes and six new/retry records
per provider. The two curated refreshes run first. Requests are sequential; a
15-minute persisted claim allows recovery after process termination. Runs retain a
10-minute deadline and stop claiming work when less than 35 seconds remain.

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
  the pre-update catalog snapshot and the `automatic-catalog-updater` operator.

## Deployment and diagnosis

Release deployment remains manual through **Deploy release**. After deploying
this version, the production Compose overlay automatically builds and starts the
worker alongside the existing services. No new secret, host port or cron entry is
needed. Deployments remove orphan services, so selecting a pre-worker release
stops and removes the collector. The manual workflow passes this policy to older
release scripts as well; database volumes are preserved. Local development and smoke/e2e fixtures do not start the worker.

From `/srv/devcourse-finder/current`, using the existing production environment:

```bash
docker compose --env-file /srv/devcourse-finder/.env \
  -f compose.yaml -f compose.production.yaml ps catalog-updater

docker compose --env-file /srv/devcourse-finder/.env \
  -f compose.yaml -f compose.production.yaml logs --tail=100 catalog-updater

docker compose --env-file /srv/devcourse-finder/.env \
  -f compose.yaml -f compose.production.yaml exec -T \
  catalog-updater devcourse-finder catalog-update status
```

`catalog-update health` checks a database heartbeat younger than 90 seconds.
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
