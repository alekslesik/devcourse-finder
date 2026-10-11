# Verified learning conditions for every provider

Epic: [#59](https://github.com/alekslesik/devcourse-finder/issues/59).

## Goal

Collect stated course duration, weekly workload and support for every supported
school. The catalog must distinguish missing parser coverage from absent source
data, and must never describe unknown support as confirmed lack of support.

## Collection contract

1. Bind facts to the verified course and exact tariff. Course-level facts may be
   shared only when the source clearly applies them to every tariff.
2. Preserve duration units and numeric ranges. Billing/access/introductory-lesson
   periods and total recorded video hours are not course duration.
3. Accept workload only when explicitly stated in hours per week. Do not derive
   it from total hours, lesson counts, duration or payment schedules.
4. Distinguish assignment/code review, individual mentor sessions and community
   support. Each is independently true, explicitly false, or unknown.
5. Record the official URL, field/DOM locator, source-body digest and observation
   time with every fact. Ignore recommendations, other courses and global claims.
6. Validate typed values before queueing and again before publication. Publish
   conditions with the verified course transaction; do not expand scraper trust.
7. Successful fresh reads clear vanished collector-owned fields. Preserve curated
   facts and operator-only tariffs. Failed reads retain old facts and their own
   confirmation dates; advancing course checked_at cannot reconfirm those facts.
8. Keep legacy weekly/monthly fields compatible. Workload ranges use their upper
   bound for filtering; partial support evidence cannot imply independent facts.
9. Add sanitized positive/negative fixtures and queue/publication/storage tests.
   Document live coverage and unavailable source data without counting guessed
   facts or fixture-only evidence as live collection.

## Delivery order

One implementation commit per issue. Backend changes are the primary scope;
regenerated API types follow schema changes. No new deployment trigger is added.

- [ ] [#60](https://github.com/alekslesik/devcourse-finder/issues/60) — Learning conditions: evidence model and safe publication lifecycle
- [ ] [#61](https://github.com/alekslesik/devcourse-finder/issues/61) — Practicum: collect verified duration, weekly workload and support
- [ ] [#62](https://github.com/alekslesik/devcourse-finder/issues/62) — Netology: collect verified duration, weekly workload and support
- [ ] [#63](https://github.com/alekslesik/devcourse-finder/issues/63) — Skillbox: collect verified duration, weekly workload and support
- [ ] [#64](https://github.com/alekslesik/devcourse-finder/issues/64) — Skillfactory: collect verified duration, weekly workload and support
- [ ] [#65](https://github.com/alekslesik/devcourse-finder/issues/65) — HTML Academy: collect verified duration, weekly workload and support
- [ ] [#66](https://github.com/alekslesik/devcourse-finder/issues/66) — PurpleSchool: collect verified duration, weekly workload and support
- [ ] [#67](https://github.com/alekslesik/devcourse-finder/issues/67) — Skypro: collect verified duration, weekly workload and support
- [ ] [#68](https://github.com/alekslesik/devcourse-finder/issues/68) — OTUS: collect verified duration, weekly workload and support
- [ ] [#69](https://github.com/alekslesik/devcourse-finder/issues/69) — Hexlet: collect verified duration, weekly workload and support
- [ ] [#70](https://github.com/alekslesik/devcourse-finder/issues/70) — Stepik: collect verified duration, weekly workload and support
- [ ] [#71](https://github.com/alekslesik/devcourse-finder/issues/71) — JavaRush: collect verified duration, weekly workload and support
- [ ] [#72](https://github.com/alekslesik/devcourse-finder/issues/72) — RS School: collect verified duration, weekly workload and support
- [ ] [#73](https://github.com/alekslesik/devcourse-finder/issues/73) — CodeBasics: collect verified duration, weekly workload and support
- [ ] [#74](https://github.com/alekslesik/devcourse-finder/issues/74) — Learning conditions: live coverage and regression acceptance

## Completion evidence

- #60: shared provenance/range/support contract and private ownership lifecycle.
- The provider issues remain open until their actual extraction contracts and
  recorded source fixtures are implemented. A shared data model alone does not
  establish provider coverage.
- #74 requires a bounded disposable live run and per-provider field coverage;
  production rollout remains a separate manual release deployment.
