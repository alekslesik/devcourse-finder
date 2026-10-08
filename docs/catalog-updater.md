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

## Initial source coverage

`data/updater-sources.json` enables the two existing curated Stepik courses:
Golang (54403) and Generation Python (58852). Both public API contracts were checked
against official responses on October 8, 2026. This automatically fills an empty
catalog with verified courses; it does **not** import the other 18 templates.

The Stepik adapter verifies identity, expected title, visibility, active/enabled
state, censorship, paid/free flags, exact RUB price and enrollment actions/dates.
A free flag explicitly confirms zero price even when the API's price is null.
Paid prices in another currency, or missing prices, become unknown; there is no
currency conversion or installment-price inference. Public anonymous enrollment
actions and explicit dates determine availability. A false enrollment action alone
never means that enrollment is closed.

A conservative `schema-course` adapter is also available for CodeBasics templates,
but is **not enabled by default**: live HTML could not be verified from the build
environment. It requires a single JSON-LD Course matching the exact official URL,
explicit full-course free access, an exact zero-RUB Offer, and explicit availability.
Missing structure is skipped, not guessed. Add one of `codebasics-go`,
`codebasics-python`, `codebasics-java`, or `codebasics-javascript` with this adapter
to the source configuration only after verifying the official HTML contract.

Yandex Practicum, paid Hexlet and OTUS are not automatically refreshed yet.
Their multiple tariffs and full-price/enrollment contracts need dedicated tested
adapters. Adding a school is a code/configuration change; subsequent data updates
remain automatic. This service does not discover arbitrary new schools or let an
LLM invent prices from marketing text.

## Publication rules

- HTTP errors, login/challenge pages, malformed JSON, changed identities, missing
  required fields and contradictory prices are skipped. Other sources continue.
- HTTPS, fixed adapter host allowlists, no redirects, verified TLS, 15-second
  request limits, 2 MiB bodies, one delayed retry for 429/5xx and a 10-minute run
  deadline bound network work. Direct connections reject private IP destinations.
- Only configured course/offer IDs are updated. Other offers, classification,
  description, curated title and manually drafted/archived courses are preserved.
  An offer moved to another URL is protected rather than overwritten.
- An initial publication requires both live price and open enrollment evidence.
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

Runs are stored in `updater_runs`; per-source status and SHA-256 evidence in
`updater_sources`; pending confirmations in `updater_candidates`; catalog snapshots
in `imports`. A successful source may publish even if another fails (`partial`).
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

`go test -race ./backend/...` passes with an isolated PostgreSQL 17 test database.
Updater tests exercise real transactions and audit/cache invalidation, source
failures, missing/contradictory price evidence, redirect rejection, rollback,
concurrent operator changes, anomaly confirmation, protected tariffs, health,
initial collection and restart deduplication. `go vet ./backend/...`, release
metadata tests and production Compose validation also pass.

An October 8 live collection against the anonymous Stepik API verified and
published both configured courses into a disposable database. An earlier partial
run preserved the unavailable Go source while publishing Python. The production
image was built and its worker tested as the non-root `app` user with a read-only
filesystem, no capabilities, a 128 MiB memory limit and no published ports; health
passed and SIGTERM exited cleanly. Tests did not access the VDS database.
