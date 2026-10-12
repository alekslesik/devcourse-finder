# Profile filter acceptance — 2026-10-12

Epic #80; implementation tasks #81, #83–87 and #89. #82/#88 previously shipped
in PR #90. One commit per remaining task, one combined PR, release 0.21.0.

## Verification

- Full Go suite with PostgreSQL and `-race`; `go vet ./backend/...`.
- Additive storage migration, existing-identity backfill, per-field ownership,
  operator preservation, failed reads, poisoned queue envelopes and atomic rollback.
- Every visible experience/goal/direction combination across Python, JavaScript,
  Java and Go, including unknown opt-in, incompatible profiles, same-offer price
  constraints, unavailable enrollment and stale prices.
- API schema response checks, contextual facets before pagination and confirmed/
  unknown counts on a page containing only unknown candidates.
- Frontend production build, type checking, generated OpenAPI contract, route tests
  and all 41 browser tests. New 360px keyboard/axe regression covers counts, explicit
  unknown grouping, preserved filters, URL chips and backward/forward navigation.
- Existing light/dark browser matrix at 360, 768 and 1440px. Browser emulation
  verifies layout/accessibility; physical iPhone touch behavior was not measured.

## Bounded live experiment

One anonymous official sample per provider; serial reads, 45s source context,
7m overall context, no sitemap expansion. It uses a disposable database ending
in `_test` and isolated schema. Eligible verified samples are first published
with legacy unknown profile values, then refreshed under the same course/offer
identity. Refreshes assert zero new courses. Unavailable enrollment is recorded
and excluded without inventing historical availability.

Executed in a read-only container with `memory=128m`, `memory-swap=128m` and
`GOMEMLIMIT=96MiB`. Duration 75.96s; process peak RSS **46,664 KiB** (45.57 MiB);
exit code 0, `OOMKilled=false`. Source availability can change between runs.

| Source | Read outcome | Verified profile facts in this sample | Refreshed identities |
| --- | --- | --- | ---: |
| Practicum | Verified | No unambiguous profile fact | 1 |
| Netology | Unavailable | No new facts | 0 |
| Skillbox | Verified | No unambiguous profile fact | 1 |
| Skillfactory | Verified | Beginner audience | 1 |
| HTML Academy | Verified | Prior-course admission is not a proficiency tag | 1 |
| PurpleSchool | Verified | Beginner audience | 1 |
| Skypro | Verified, unavailable enrollment | No new published identity | 0 |
| OTUS | Verified | Beginner / other-language audience | 1 |
| Hexlet | Rejected structured payload | No new facts | 0 |
| Stepik | Unavailable | No new facts | 0 |
| JavaRush | Verified Premium | No unambiguous audience fact | 1 |
| RS School | Verified, unavailable enrollment | Base-knowledge prerequisite; excluded from results | 0 |
| CodeBasics | Verified | Beginner audience | 1 |

Recorded fixtures retain official wording and section boundaries. Synthetic
mutations exercise positive contracts and adversarial wording; they are labeled
explicitly and are not counted as current live coverage. In particular, a saved
Stepik response tests the API contract but cannot substitute for a fresh live read.

## Sample before/after

The initial publications in this experiment explicitly used unknown profile
values. These eight identities are a controlled backfill sample, not production.

| Language | Eligible sample courses | Unknown audience before | Unknown audience after | Unknown goals after | Unknown direction after |
| --- | ---: | ---: | ---: | ---: | ---: |
| Python | 5 | 5 | 2 | 5 | 5 |
| JavaScript | 2 | 2 | 1 | 2 | 2 |
| Java | 1 | 1 | 1 | 1 | 1 |
| Go | 0 | 0 | 0 | 0 | 0 |

Production baseline remains separately recorded in the epic specification: 89
Python courses, 84 unknown audiences at the 2026-10-11 audit. This PR does not
mutate production or claim that the remaining 75 Stepik profiles were verified.
Normal scheduled refreshes populate only facts that the official source actually
confirms. Profile coverage and date measurements must be checked after deployment.

## Repeat the experiment

```sh
PROFILE_LIVE_CHECK=1 \
PROFILE_LIVE_REPORT_PATH=/tmp/profile-report.json \
TEST_DATABASE_URL='postgres://user:password@127.0.0.1:5432/devcourse_test?sslmode=disable' \
GOMEMLIMIT=96MiB go test ./backend/updater -run '^TestProfileLiveCoverage$' -v -count=1 -timeout 8m
```

The test refuses databases without a `_test` suffix. Apply the 128 MiB container
limit separately when measuring deployment memory budgets. It is opt-in and does
not run in ordinary CI, where official endpoint downtime would be nondeterministic.
