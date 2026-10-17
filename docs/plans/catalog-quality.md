# Catalog enrichment reliability and filter audit

Epic #158; deliver all five tasks in one PR, one commit per task. No frontend
redesign, paid bulk development runs, price inference or schedule/budget changes.

1. #159: Check Stepik once per enrichment batch before selecting candidates.
   Persist an outage cooldown, exclude the provider before LIMIT, probe again on
   the next run after two hours, and expose availability in status. Preserve
   per-course retry dates, existing facts and independent native collectors.
2. #160: Support proven legacy course identities without relaxing host, product,
   tariff or language checks. In particular, accept one cosmetic trailing slash.
3. #161: Separate source, extraction, local grounding, verification and publication
   failures; distinguish observations from useful verified facts and empty output.
4. #162: Audit every published course, not just the first page. Report confirmed,
   unknown and excluded profile matches, and compare counts with the public API.
5. #163: Record acceptance evidence and external blockers, run backend/PostgreSQL
   checks, update version/changelog, and verify release and deployment.

Acceptance uses recorded/mock model responses and an isolated PostgreSQL database.
Source outages are reported as outages; unknown profile values never become
beginner matches merely to increase counts. A queued observation is not proof of
an updated card. Existing HTTPS and manual accessibility issues remain independent.

## Stepik availability

`catalog-ai status` includes `provider_health` with `available`, `checked_at`,
`next_probe_at` and a fixed code. Before a due Stepik batch, an anonymous API
probe has an eight-second outer timeout. An unavailable or non-JSON endpoint
pauses Stepik for two hours and excludes its courses before the batch limit.
The next enrichment run after the cooldown retries automatically. This is not a
new scheduler: Monday/Thursday Moscow scheduling remains in force. The manual
Actions enrichment workflow can also perform that next run without SQL resets.
No model request is made for the probe, and a pause does not mutate course facts
or course retry dates. Per-course readers still validate identity after recovery.

## Legacy identities

Practicum and OTUS accept exactly one historical trailing slash; queries,
fragments, alternative hosts and changed paths remain rejected. Coverage uses the
same equivalence. Stored URLs and identity digests are not rewritten. Practicum
requires the saved external/course ID, and queued observations revalidate the
primary/tariff binding before publication.

CodeBasics has a second proven mismatch: curated free offers are named
`Бесплатная полная программа`, whereas normalization generates `Полная программа`.
Accept the historical name only for the bound `<course-id>-course` offer with
an explicit zero price and free flag. Native page identity, title, language,
publication/build state and current free-policy validation still apply. A
manual extra offer does not inherit learning conditions. Anonymous checks on
2026-10-17 confirmed the Python and Go page headers still match curated titles;
there is no reason to relax title matching.

## AI diagnostics

Each course log and recent status record includes a fixed `stage` and `reason`.
Stages distinguish extraction, grounding, verification, final validation, enqueue
and publication. Raw prompts, model responses, secrets and underlying errors are
not logged. Existing generic error codes remain compatible.

Batch JSON and the Actions summary add `useful_queued`, `empty_queued`,
`verified_claims` and `discarded_claims`. Useful means at least one independently
verified fact; it does not promise a changed field (operator-owned facts can still
win). Empty observations retain existing safe disappearance/publication semantics;
we do not interpret an empty response as a model failure or retry it immediately.
Status adds full `source_codes` counts instead of extrapolating its last 40 lines.
The three historical `invalid_ai_response` events cannot be diagnosed retrospectively:
their raw responses were deliberately not retained. Recorded tests cover extraction,
grounding and invalid verification-index failures without paid replay.

## Repeatable public filter audit

Run from the repository with Go 1.24+ (no credentials or model key required):

```sh
go run ./backend/cmd/catalog-audit --url https://your-domain.example > filter-audit.json
```

The audit fetches **every page** of the public catalog, evaluates the backend
rules over that snapshot, and compares 40 language/experience/goal combinations
against both strict and unknown-inclusive public API searches. Rows split eligible
courses into confirmed, unknown and incompatible; unknown IDs identify records
needing evidence. It fetches all pages again and rejects a changed snapshot,
duplicate IDs, incomplete pages or mismatched counts. `data_as_of` is the API's
request evaluation time, not an immutable catalog revision, so timestamp equality
across pages would be an incorrect consistency check. Exit 0 requires all counts
and the before/after content digest to match. The report describes the visible
catalog; it does not audit unpublished or otherwise ineligible records.

### Initial evidence, 2026-10-17 15:29–15:30 Moscow

174 courses, including 114 Stepik records; stable content digest
`3768e7da727babf189e09319b3318a9df27e174557d09218b786678ff0fd37d5`.
All 40 combinations and both API modes matched backend rules. Python had 89
eligible programs:

| Experience | Confirmed | Unknown | Incompatible |
| --- | ---: | ---: | ---: |
| Starting from zero | 8 | 79 | 2 |
| Know the basics | 0 | 79 | 10 |
| Build projects | 0 | 79 | 10 |
| Working developer | 2 | 79 | 8 |
| Switching languages | 7 | 79 | 3 |

The main observed loss is missing suitability evidence, not a count/pagination
bug. A prerequisite alone does not confirm suitability for everyone above that
level. The audit deliberately does not assign those 79 unknown courses to beginner
or intermediate profiles. Source recovery and safe enrichment must supply that
evidence first. Overlapping audience tags mean these rows cannot be added together.
