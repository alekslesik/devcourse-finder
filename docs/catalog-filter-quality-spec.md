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

- [ ] [#81](https://github.com/alekslesik/devcourse-finder/issues/81) — Модель требований к опыту, аудитории и целей курса
- [ ] [#82](https://github.com/alekslesik/devcourse-finder/issues/82) — Диагностика и contextual counts для фильтров опыта, целей и направления
- [ ] [#83](https://github.com/alekslesik/devcourse-finder/issues/83) — Stepik: подтверждённые требования, аудитория, цели и направление
- [ ] [#84](https://github.com/alekslesik/devcourse-finder/issues/84) — Большие школы: сбор профиля курса у Практикума, Нетологии, Skillbox, Skillfactory и Skypro
- [ ] [#85](https://github.com/alekslesik/devcourse-finder/issues/85) — Специализированные школы: сбор профиля у OTUS, HTML Academy, PurpleSchool, Hexlet, JavaRush, RS School и CodeBasics
- [ ] [#86](https://github.com/alekslesik/devcourse-finder/issues/86) — Публикация и обновление профиля существующего каталога
- [ ] [#87](https://github.com/alekslesik/devcourse-finder/issues/87) — UX фильтров: неизвестные требования, счётчики и объяснение потерь
- [ ] [#88](https://github.com/alekslesik/devcourse-finder/issues/88) — Нулевой нижний бюджет не должен исключать неизвестную цену
- [ ] [#89](https://github.com/alekslesik/devcourse-finder/issues/89) — Приёмка фильтров: живая матрица, комбинации и ресурсные лимиты

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

