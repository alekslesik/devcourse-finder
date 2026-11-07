# Catalog rollout measurements

Issue #175 tracks actual catalog enrichment, not just the availability of AI
classification. A release does not imply complete field coverage.

## Reproducible public filter audit

Run from the repository root (no database credentials or AI keys required):

```sh
go run ./backend/cmd/catalog-audit --url https://your-public-origin > /tmp/catalog-coverage.json
```

The command performs only public API reads. It snapshots all visible courses,
checks a second snapshot at the end and fails if the catalog changed. Reports
include timestamps, a snapshot digest and the visible course count. Retry an
unstable snapshot rather than interpreting it as a filter regression.

`coverage.confirmed` and `coverage.with_recommendations` count unique courses
with at least one usable value for experience, goal and direction, and the
intersection `all_three`. Multiple tags count once per field. Recommendation
expiry and confirmed incompatibility follow the application's matching rules.
These counts measure discoverability, not semantic precision or school promises.

The audit checks 90 individual facet scenarios (all courses and each language)
in strict, unknown-inclusive, AI-inclusive and combined modes. Another 50 checks
cover realistic combinations of experience + goal + direction with AI enabled,
with and without incomplete profiles. API totals and the three result groups
must match the rules applied to the frozen visible snapshot. Any mismatch returns
a nonzero exit status. Per-value counts overlap and must not be summed.

## Pair coverage with collector evidence

Use the deployed Compose configuration and `catalog-ai status` for per-provider
mapped/never-attempted/attempted/ever-verified counts, source diagnostic codes,
pending observations and UTC-day reserved requests/tokens. Those counters are
operational history, not proof of useful recommendations. Preserve the source,
quotes, accepted/rejected/empty preview decisions and before/after coverage for
semantic pilot reviews; do not replace them with queued-event counts.

Production batch size 100 and the shared 200-request daily cap were explicitly
approved by the maintainer. The daily schedule is 21:00 Europe/Moscow. Extraction
and independent review each consume a request; failures also reserve budget, so
100 courses is an upper bound. Never reset the daily budget or remove manual
review holds to accelerate a backfill. No paid model requests are made by this
audit command.

On 2026-11-07 the accepted pilot/backfill used all 200 requests (490733 tokens).
Further paid enrichment must wait for the next UTC budget day and normal schedule.
Actual rollout reports are retained on the authorized new VDS under
`/srv/devcourse-finder/reports/`. Keep #175 open until the never-attempted queue
has been processed and remaining unknown/source-failure cases are reviewed.

## 2026-11-07 public snapshot

Before v0.35.0, the extended audit passed all 90 facet rows (four modes each)
and 50 combined-filter scenarios on a stable 755-course snapshot:

| Unique courses | Confirmed | Including recommendations |
| --- | ---: | ---: |
| Experience | 212 | 239 |
| Goal | 23 | 72 |
| Direction | 67 | 113 |
| All three fields | 21 | 52 |

This is the current snapshot, not a claim that the new audit itself enriches data.
The full report is `profile-coverage-audit-before-0.35.0-2026-11-07.json` in the
production reports directory. Catalog enrichment is incomplete and remains #175.
