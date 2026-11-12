# Full catalog filter coverage

Single tracking epic: [#170](https://github.com/alekslesik/devcourse-finder/issues/170).
The product goal is evidence-based experience, goal and direction classification
for every course. A missing classification is unfinished work, not a successful
enrichment. Unsupported prices, support and duration remain unknown.

`catalog-audit` now exports `completeness` alongside the existing counter audit:

```sh
cd backend
go run ./cmd/catalog-audit -url https://devcoursefinder.ru > coverage.json
```

For each facet, `confirmed_ids`, `recommended_only_ids` and `unclassified_ids`
partition the visible unique courses. A course matching several options counts
only once. Any confirmed match takes precedence over advisory matches for this
coverage partition; this does not alter the matching rules for individual
filters. Recommendations must pass the same validity, freshness and explicit
incompatibility rules as search. Expired or contradicted advice cannot hide gaps.
The three lengths must sum to `snapshot_courses`. Duplicate/missing identities
fail acceptance instead of silently inflating coverage.

`by_provider` contains the same partition for every school, including providers
without any AI adapter. `backlog` lists each incomplete course once with its ID,
provider, language, official source and missing facets. All-three-field gaps sort
first, then stable course ID. This order describes diagnostic priority; it does
not replace the worker's due times, provider fairness, cache or manual holds.

These are observable search gaps. The public API cannot establish whether a
missing value means never attempted, insufficient curriculum, rejected output,
source outage or a taxonomy gap. Operational state and source inspection must
explain those causes before choosing a retry or changing labels. The report
does not label a gap as successfully handled merely because a reason exists.

The baseline on 12 November 2026, v0.38.1, contains 897 visible courses. With
current recommendations, 444 match an experience, 301 a goal, 361 a direction,
and 222 all three. Thus the initial missing counts are 453, 596 and 536.
Option totals need not equal 897 because categories overlap; the unique-course
partitions must. Unknowns never count as classified. The existing facet rows (95 after adding game development),
four modes, 50 combinations, pagination and stable-snapshot checks still run.

This diagnostic reads official public application endpoints only. It performs
no model requests, catalog edits or budget resets and is safe after the daily
200-request budget is exhausted. Subsequent work in the epic addresses source
quality, taxonomies and field-specific retry selection against this backlog.
