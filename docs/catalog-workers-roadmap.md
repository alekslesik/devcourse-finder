# Catalog worker roadmap

Tracking epic: https://github.com/alekslesik/devcourse-finder/issues/34

The first implementation stage separates collection and publication. It does not
claim to connect the researched schools before their adapters pass verification.
Yandex Practicum is mandatory in the first provider expansion.

| Issue | Task | Dependency |
| --- | --- | --- |
| #35 | Durable observation queue and atomic shared publication | Foundation |
| #36 | API/pages roles, independent locks, schedules and heartbeats | #35 |
| #37 | Three production containers, manual collection and checks | #35, #36 |
| #38 | Practicum official sitemap and course/tariff price API | #35–#37 |
| #39 | Netology embedded program and tariff data | #35–#37 |
| #40 | PurpleSchool tariffs, demo detection and course identity | #35–#37 |
| #41 | RS School free programs and current cohorts | #35–#37 |
| #42 | HTML Academy courses, professions and subscriptions | #35–#37 |
| #43 | Skillbox product/tariff/payment reconciliation | #35–#37 |
| #44 | Skillfactory course-specific tariff extraction | #35–#37 |
| #45 | Skypro complete pricing and enrollment binding | #35–#37 |
| #46 | JavaRush recurring prices and original currency | Catalog model design |
| #47 | Larger provider limits and multi-provider resource verification | Provider expansion |
| #49 | Identity-bound tariffs sharing one Practicum canonical landing | #38; publication contract design |

One repository and backend image provide three process roles: API collector,
page collector and publisher. PostgreSQL stores candidate jobs and normalized
observations. Each provider belongs to exactly one collector; RS School initially
belongs to pages. A browser process is deferred until an adapter proves that
ordinary anonymous HTTP reads cannot provide the required information.

Collectors own discovery, bounded requests, normalization and source-read status.
They enqueue both verified reads and failed reads. Only the publisher changes the
public catalog, binds identities and applies anomaly confirmation. Original
observation timestamps are preserved. Publication and queue acknowledgement commit
atomically with the existing import snapshot and cache revision.

Collectors run at 09:00 and 21:00 Europe/Moscow, with independent role schedules.
The publisher drains observations frequently. All roles have bounded work and
independent liveness. Unknown price/support/enrollment remains unknown; invalid
pages, introductory freebies, installments and subscription fees cannot establish
a verified full-course price. Manual imports and edited identities remain protected.

Each implementation task has its own commit. Reviewable stages use combined PRs
with an incremented VERSION and English changelog. Deployment remains the manual
Deploy release workflow; merges and release publication never deploy production.

## Completion

The v0.17.0 implementation covers all roadmap tasks, including the shared-landing
Practicum follow-up. See [the completion record](catalog-workers-completion.md)
for delivered contracts, evidence, limits and manual release/deployment steps.
