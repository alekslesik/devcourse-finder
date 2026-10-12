# Catalog filter quality roadmap

Epic: [#80](https://github.com/alekslesik/devcourse-finder/issues/80).

## Observed baseline (2026-10-11)

Public API audit fetched all eligible courses, before pagination:

| Language | Courses | Unknown audience | Unknown goals | Unknown direction |
| --- | ---: | ---: | ---: | ---: |
| Python | 89 | 84 | 84 | 84 |
| JavaScript | 47 | 42 | 42 | 37 |
| Java | 23 | 18 | 18 | 18 |
| Go | 14 | 9 | 9 | 9 |

Python experience counts: none 4, basics 0, projects 0, working 1, switch 5.
Counts overlap across five classified courses. Stepik accounts for 75 Python
courses with unknown audience. The normalizer sets audience and goals to unknown;
search uses literal audience membership. These observations describe production
at audit time, not guaranteed future counts.

## Required semantics

Separate minimum prerequisites, intended audience and target outcomes. A user's
current experience is not necessarily an exact course category. `switch` denotes
a background, not a numeric proficiency level. Never blindly expand beginner tags
to all higher levels. Accept only course-bound official evidence, with per-field
URL, locator, digest and date; titles, prices and school reputation cannot prove
suitability. Unknown is neither confirmed suitability nor confirmed incompatibility.
Strict matches and explicitly requested unknown candidates must be distinguishable.

Preserve operator edits. Failed reads retain facts and dates; successful fresh
reads clear disappeared collector-owned facts. Refresh existing identities without
creating duplicates. Legacy tags remain literal until the new profile contract is
implemented. v0.19 learning-condition collection does not cover audience or goals.

## Tasks

- [x] [#81](https://github.com/alekslesik/devcourse-finder/issues/81) — Модель требований к опыту, аудитории и целей курса
- [x] [#82](https://github.com/alekslesik/devcourse-finder/issues/82) — Диагностика и contextual counts для фильтров опыта, целей и направления
- [x] [#83](https://github.com/alekslesik/devcourse-finder/issues/83) — Stepik: подтверждённые требования, аудитория, цели и направление
- [x] [#84](https://github.com/alekslesik/devcourse-finder/issues/84) — Большие школы: сбор профиля курса у Практикума, Нетологии, Skillbox, Skillfactory и Skypro
- [x] [#85](https://github.com/alekslesik/devcourse-finder/issues/85) — Специализированные школы: сбор профиля у OTUS, HTML Academy, PurpleSchool, Hexlet, JavaRush, RS School и CodeBasics
- [x] [#86](https://github.com/alekslesik/devcourse-finder/issues/86) — Публикация и обновление профиля существующего каталога
- [x] [#87](https://github.com/alekslesik/devcourse-finder/issues/87) — UX фильтров: неизвестные требования, счётчики и объяснение потерь
- [x] [#88](https://github.com/alekslesik/devcourse-finder/issues/88) — Нулевой нижний бюджет не должен исключать неизвестную цену
- [x] [#89](https://github.com/alekslesik/devcourse-finder/issues/89) — Приёмка фильтров: живая матрица, комбинации и ресурсные лимиты

## First delivery: diagnostics (#82) and neutral zero budget (#88)

This delivery neither fills missing profiles nor implements the new matching
model. Provider collection, publication/backfill and user-facing controls remain
linked tasks. It does not complete the epic.

`GET /api/v1/courses?language=python&experience=basics&diagnostics=true`
adds `diagnostics.experience`, `diagnostics.goal` and `diagnostics.direction`:

- `eligible`: distinct courses with only this facet's own filter removed;
- `unknown`: courses in that population with no confirmed facet value;
- `counts`: confirmed counts for every option; multi-label counts can overlap;
- `selected`: currently selected option, or an empty string.

Other profile filters, language, budget, support, workload, publication and offer
enrollment remain active. Counts precede pagination and do not include unknown
profiles as confirmed matches. Empty facets may result from another active filter.
Normal requests do not compute diagnostics; no filter layout changes are included.
The baseline Python experience population would yield eligible=89, unknown=84,
none=4, basics=0, projects=0, working=1, switch=5. This is a contract example;
production receives the new endpoint response only after manual deployment.

A lower budget of zero with no maximum is unrestricted, including unknown prices.
Do not claim those unknown prices fit a budget. Positive minimums and any maximum
still require verified complete-course prices. Subscription and include_free
rules remain unchanged.

## Acceptance and operations

Facet eligibility shares the search predicate. Count courses rather than tariffs;
exclude archived/draft courses and unavailable offers by default. Test interacting
filters, pagination, unknown data, price freshness and support/workload restrictions.
Measure per-provider field coverage after bounded backfill; synthetic fixture labels
are not live evidence. Run CI and release checks. Deployment stays manual through
the existing published-release workflow.



## Second delivery: remaining seven tasks in one PR

Each remaining issue (#81, #83–87, #89) has its own commit. #82 and #88 were
already delivered by PR #90. Version 0.21.0 completes the implementation;
production changes only after the maintainer merges and manually deploys its release.

### Matching contract

`course.profile` stores each fact independently with official URL, semantic
locator, content digest and observation time. Requirements use a minimum level
and scope (`programming` or `course_language`); audience and explicit exclusions
use user profiles. `switch` is an independent background, never equivalent to
commercial experience or familiarity with the new language. A verified minimum
alone does not establish suitability. A verified minimum plus target level can
establish an interval; explicit audience tags retain their precise meaning.
Legacy curated tags remain literal. Conflicting audience/exclusions, minimums
above the target level and audience below mandatory prerequisites are invalid.
No admission requirement, audience or career outcome is inferred from a title,
price, technology description, job advertisement or school reputation.

The default search remains strict. `include_unknown_profiles=true` includes only
unknown selected profile fields, never verified incompatibility. These results
have `profile_match: unverified` and `unknown_profile_fields`; confirmed results
sort first under every sort order. All existing offer predicates still apply.
Unknown suitability never receives the “Подходит вашему опыту” reason.

`confirmed_total` and `unverified_total` count distinct courses before pagination.
With diagnostics, `unknown_available` counts additional unverified candidates
under the complete combination of filters, rather than adding overlapping facet
unknown counts. Contextual facet option counts confirm that individual parameter
with its own constraint removed; other selected constraints remain active.
Options with zero confirmed results remain selectable. The frontend explains the
missing coverage, offers the opt-in explicitly, and keeps it in shareable URLs,
chips and browser history. Unknown results have a separate group and do not
inflate the headline's confirmed count.

### Collection and lifecycle

Stepik uses its own `requirements`, `target_audience`, summary and explicit
semantic description sections. HTML providers use independent approved headings
and course-bound blocks. Skillfactory additionally has an explicit course feature
artboard. RS School's “For everyone” is open admission, not proof of beginner
suitability; its mandatory base-knowledge sentence can establish prerequisites.
JavaRush University or other tariffs cannot classify Premium. HTML Academy's
previous-course requirement cannot silently become a commercial-experience tag.
Optional requirements, examples, ambiguous sections, recommended courses and
conditional later stages remain unknown. Explicit “not for beginners” evidence
is retained separately, so opt-in cannot bypass it.

Every queue envelope must attest the exact same profile as its normalized record,
with approved source URL and matching fact observation time. Publication stores
private ownership separately for each field. Successful reads replace/clear only
collector-owned fields; source failure and old workers with no profile attestation
retain existing facts and dates. Operator imports reset profile ownership and
protect explicit operator facts. Profile metadata is excluded from repeated
price/enrollment anomaly fingerprints. Catalog changes, identity bindings and
queue acknowledgement share the publication transaction.

No separate production bulk import is required. Existing identities are refreshed
by the normal twice-daily collection slots and the existing manual collection
trigger. A window selects at most 30 Stepik refreshes and six refreshes per HTML
provider, alongside their separately bounded new-candidate lanes. Larger catalogs
need multiple successful windows; failures retain backoff and do not gain fresh
fact dates. `catalog-update status` adds `profile_coverage` by provider, fact field
and oldest fact date. Do not interpret an overall course-check date as verification
of every field.

### Acceptance and measured limitations

See [the live source report](catalog-profile-live-report-2026-10-12.json) and
[acceptance notes](catalog-profile-acceptance.md). The live sample is one course
per provider in a disposable database, not a production-wide rescrape or a promise
that all 89 Python courses now have audience metadata. Stepik and Netology were
unavailable; Hexlet failed the existing structured identity contract. Verified
sources without a clear statement retain unknown fields. Closed/unavailable
samples stay excluded. The new UI makes those limits visible without claiming
that unknown courses match.
