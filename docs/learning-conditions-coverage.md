# Learning conditions: provider coverage and operations

This implements epic #59 and issues #61–74. Contracts cover all 13 providers;
coverage of actual fields depends on what the official course/tariff states.
Unknown, explicitly unavailable and confirmed available are distinct values.

| Provider | Authoritative learning scope | Important exclusions |
| --- | --- | --- |
| Practicum | Profession API calendar months; exact purchase card and explicit base/plus inheritance | Daily schedule examples, unrelated plus offers |
| Netology | Matching rendered tariff title, price and feature block | Old plans, neighboring courses, total practice hours |
| Skillbox | Course program statistics and independently verified tariff modal features | Installments, generic footer claims; verified modal portals remain eligible |
| Skillfactory | Course program header calendar duration | Pricing flags and general consultation lead forms do not prove personal support |
| HTML Academy | Individual intensive section | Group format; weekly mentor appointment is not study workload |
| PurpleSchool | Exact tariff features and unique workload FAQ | AI checks; mentor chat is community, not personal appointments |
| Skypro | Target hero and validated product purchase section | Resume recommendations and closed enrollment cannot establish new publication |
| OTUS | Accepted course instance calendar duration and course description | Video hours, academic lesson counts, generic conflicting FAQ |
| Hexlet | Accepted instance and single exact verified offer with matching tariff section | Aggregate minimum prices cannot identify a named plan |
| Stepik | Official API workload and dedicated course support description | Self-paced flags, automated tests, comment counts |
| JavaRush | Verified Premium purchase card | Subscription period and University/MentorPro conditions |
| RS School | Current course Discord section | Conditional later-stage mentors, stage lengths and alumni history |
| CodeBasics | Verified Inertia landing's own fields | Total video/practice hours, AI support and sibling landings |

Each fact retains its own official URL, DOM/field locator, source digest and
observation date. Successful source reads clear disappeared collector-owned facts;
failed reads preserve old facts and their dates. Operator edits remain protected.
Legacy PurpleSchool support ownership is recovered only from identity-bound audit
history; later operator changes take precedence.

Duration keeps days/weeks/months/years and numeric ranges. Weekly workload requires
explicit hours per week. Approximate claims are omitted because the current model
has no approximation qualifier. Independent support evidence never fills another
support field. Contradictory positive/negative claims remain unknown.

## Bounded live acceptance

The accompanying [JSON report](learning-conditions-live-2026-10-11.json) records
one official candidate per provider, sequentially, with a 45-second provider and
seven-minute outer budget. It writes only to a disposable PostgreSQL database and
exercises queue validation and publication. Counts describe sampled offers, not
production totals or coverage of every course in a school. Closed enrollment can
produce verified conditions with zero newly published courses.

A missing fact in a verified row means the selected authoritative source had no
accepted evidence. `source_unavailable` means a request failed; malformed source
payloads remain rejected by existing identity/price checks. Synthetic contract
mutations in fixtures are regression evidence, not live-source coverage. In
particular, approximate Hexlet workload, JavaRush subscription months, CodeBasics
total hours and conditional RS School stage support are not converted to facts.

To repeat locally, start disposable PostgreSQL, export its connection as
`TEST_DATABASE_URL`, then run from the repository root:

```sh
LEARNING_LIVE_CHECK=1 LEARNING_LIVE_REPORT_PATH=/tmp/learning-coverage.json \
  GOMEMLIMIT=96MiB go test ./backend/updater \
  -run '^TestLearningLiveCoverage$' -v -count=1
```

**The test resets its test database. Never use the production database.** The live
check is opt-in and does not run during ordinary CI. The Docker acceptance run passed under a hard 128 MiB limit (swap disabled),
with `GOMEMLIMIT=96MiB`, no OOM and 48,892 KiB peak process RSS. It completed
in 63.90 seconds; request failures are recorded rather than treated as known
conditions. Existing source byte limits,
request/concurrency budgets and production memory limits remain unchanged.

## After manual release deployment

Run the existing Collect catalog workflow or `scripts/collect-catalog.sh`, then
inspect status with the existing Compose environment arguments:

```sh
docker compose exec -T catalog-updater devcourse-finder catalog-update status
```

`learning_conditions_coverage` rows report per-provider offer and field counts,
plus `oldest_fact_at`. Counted support includes explicit false evidence; absent
fields do not count. A course's newer check date cannot refresh an older fact.
Collection continues twice daily; merging or publishing a release does not deploy.
The existing course views consume typed ranges and partial support without layout
changes; unknown workload remains hidden on cards.
