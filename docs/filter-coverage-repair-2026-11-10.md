# Filter coverage repair — 2026-11-10

Follow-up to #175/#170; implementation and live acceptance tracked in #194.

The stable public audit contains 844 eligible courses (782 on Stepik). All 90
facet rows in four modes and 50 three-filter scenarios match search rules. The
main gap is classification evidence, rather than truncated result pages.

| Unique courses with a usable classification | Confirmed | With reviewed AI |
| --- | ---: | ---: |
| Experience | 234 | 353 |
| Goal | 23 | 166 |
| Direction | 78 | 232 |
| All three fields | 21 | 123 |

These are unique-course unions, not sums of overlapping option counts. Unknown
courses are not confirmed matches; courses need only match relevant options.

## Repairs

Fill incomplete profiles before refreshing complete ones. Within that lane,
give eligible providers one course per round, starting with the provider least
recently attempted. Remember attempts currently cooling down without selecting
those courses. This also rotates batch-size-one runs. Within a provider retain
never-attempted, missing-field and oldest-attempt priority. Due times, explicit
selection, provider availability, archived/demo exclusions, held courses and the
atomic shared daily reservation cap remain enforced.

Add source-bound program/project sections observed on OTUS and HTML Academy,
and the shared course plan on PurpleSchool. Keep Skillbox's existing program
section. Exact heading scopes, duplicate-root rejection, document size limits,
course/offer identity checks and the separate review model remain in force.
PurpleSchool's project gallery carries AI-tariff badges, so it stays out of the
shared course document. Tariffs, unrelated course cards, navigation and forms
cannot establish shared suitability. No matching predicates or public API change.

Read-only probes of the current official pages:

| Source | Old course-section text, bytes | New course-section text, bytes |
| --- | ---: | ---: |
| otus | 2301 | 10062 |
| htmlacademy | 508 | 3645 |
| purpleschool | 0 | 350 |

Sources: https://otus.ru/lessons/ai-python-devs,
https://htmlacademy.ru/intensive/react,
https://purpleschool.ru/course/python-basics.

The old OTUS input produced no independently approved recommendation in a
bounded DeepSeek preview (2 reserved requests, 4015 tokens, no catalog writes).
The expanded documents change the text-based bundle digest, so old empty cache
entries cannot hide newly supplied curriculum. Unchanged document caches remain
reusable; the model policy and matching safeguards are not weakened.

## Validation and rollout

PostgreSQL/race tests cover a 200-to-3 provider imbalance, complete-vs-incomplete
priority, tiny-batch rotation, cooling-down rows, unavailable providers, explicit
targeting and manual holds. Source tests cover nested curricula, ambiguous roots,
wrong-tariff/private text and optional project exclusion. Full backend suite and
vet pass. The public before/after audit and bounded enrichment acceptance are
recorded with #194 and the release PR after deployment.

Remain in recommendation-only mode and the existing **200 requests/day** cap.
Keep the five manual-review holds and protected fact/offer fields unchanged.
Full-catalog completion remains open in #175; unavailable or unsupported source
claims remain unknown instead of becoming invented matches.
