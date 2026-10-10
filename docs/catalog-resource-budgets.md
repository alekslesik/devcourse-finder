# Multi-provider worker budgets

The production configuration enables 13 providers: the five original adapters
and Practicum, Netology, PurpleSchool, RS School, HTML Academy, Skillbox,
Skillfactory, Skypro and JavaRush (Practicum overlaps the original five).
The nine expansion adapters use deterministic public fixtures and rejection
cases in the backend suite; live availability is measured separately.

## Limits and fairness

- At most 16 discovery feeds, validated against the fixed adapter/host allowlist.
- Each collection run remains bounded by ten minutes. Discovery gets at most two
  minutes of that deadline, leaving existing detail jobs runnable when discovery
  is slow. Feeds rotate by their last attempted time, recorded before HTTP.
- Non-Stepik providers receive six new/retry jobs and six refresh jobs per round.
  Stepik gets 120 new/retry and 30 refresh jobs. The cap derives from the feed
  ceiling: 330 jobs maximum, with all 13 actual providers needing 294 slots.
  SQL orders by lane rank, oldest attempt and refresh priority. Unclaimed jobs
  retain priority when time truncates a run. Insufficient time for a multi-read
  provider skips that job and still permits cheaper jobs later in the batch.
- Candidates are capped at 50,000 overall, 20,000 for Stepik and 5,000 for any
  other provider. Existing identities remain refreshable at insertion capacity;
  a large provider cannot occupy the entire global candidate table alone.
- The normalized observation queue accepts at most 5,000 pending envelopes,
  each at most 64 KiB. Backpressure rolls back enqueue and candidate status
  together, preserving all pending reads. This bounds raw pending payloads at
  about 313 MiB; PostgreSQL tuples, indexes and history require additional disk.
- The publisher processes at most 500 envelopes per pass and trims at most 1,000
  completed diagnostics per pass. Retention targets 5,000 done rows or seven
  days. Migration from a larger retained backlog is incremental; pending rows
  are never retention candidates. The publisher ticks every ten seconds.
- Workers keep their production 128 MiB limits, 96 MiB Go memory targets,
  read-only filesystems, no capabilities and a non-root user. There is no browser
  process, external broker, or automatic deployment trigger.

`published` in legacy run summaries counts accepted course updates, including
refreshes. Publisher summaries separate `new_courses` from `refreshed_courses`.
`catalog-update status` reports `catalog_totals.distinct_courses` and `offers`
from actual published database rows, plus per-language/provider coverage, queue
age, retained diagnostics, liveness and fixed source failure codes. A discovered
URL, rejected source, fixture or accepted refresh is not another distinct course.

## Reproduce the resource check

Build the backend image, then run against disposable Docker resources:

```bash
docker build -t devcourse-catalog-budget backend
python3 scripts/check-catalog-budgets.py \
  --image devcourse-catalog-budget --output /tmp/catalog-budget-result.json
```

The script starts a temporary PostgreSQL database and isolated Docker network,
then runs the API and page collectors concurrently with a background publisher.
It uses all configured official feeds, production worker memory settings and
samples Docker memory usage. After collection it checks queue drainage, exit/OOM
status and actual database coverage, saves a JSON report and removes only its
own resources. It never reads the production `.env` or connects to the VDS.
Optional `--ca-file` mounts a proxy CA read-only; inherited proxy variables are
preserved. Source outages remain visible and can fail collection; they must not
be hidden to make the check appear green. Sampled peaks are not guaranteed exact
process high-water marks or a guarantee for future source payloads.

Deterministic checks:

```bash
TEST_DATABASE_URL='<disposable database ending in _test>' go test -race ./backend/...
go vet ./backend/...
```

They cover all provider parsers, positive/negative publication, 13-provider lane
fairness, interrupted leases, inexpensive jobs after expensive skips, rotating
feeds, insertion quotas, queue backpressure, bounded retention, separate course
and refresh counts, failed provider isolation and transactional rollback.

After merge, use the published release in **Deploy release** manually. Run
**Collect catalog** for an immediate extra read, then inspect `catalog-update
status`. The twice-daily schedule continues at 09:00/21:00 Europe/Moscow. Merging
and publishing the release do not deploy it or change production coverage.

## Recorded result

On October 10 the concurrent official-source check passed in 200.59 seconds:
28 distinct courses, 46 offers, zero pending observations, no OOM, sampled worker
peaks 13.75/28.61/12.30 MiB (API/pages/publisher). See the
[completion record](catalog-workers-completion.md#measured-anonymous-collection--2026-10-10)
and [JSON evidence](catalog-resource-result-2026-10-10.json). This measured one
bounded run in a disposable database, not maximum catalog capacity or production
coverage. Repeat it after substantial adapter/payload or resource-limit changes.
