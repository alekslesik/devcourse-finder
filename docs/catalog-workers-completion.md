# Catalog ingestion completion record — v0.17.0

This closes the implementation roadmap tracked by issue #34. It describes the
actual verification boundaries of this release; deployment remains manual.

| Issue | Delivered contract | Main verification |
| --- | --- | --- |
| #35 | Durable normalized observations; one transactional publication owner | Atomic acknowledgement, rollback, replay, poison/stale envelope and queue tests |
| #36 | Separate API/pages roles, locks and twice-daily schedules | Role ownership, concurrent lock isolation, restart and heartbeat tests |
| #37 | Three processes from one image; manual collection and checks | Production Compose validation and disposable process resource check |
| #38 | Mandatory Practicum official professions, full prices and cohorts | Public recorded fixtures, live opt-in and protected UUID tests |
| #39 | Netology course/family and full-payment binding | Identity/price fixtures, anonymous live opt-in and PostgreSQL tests |
| #40 | PurpleSchool multiple bound tariffs and demo detection | Free/paid/support fixtures, protected bindings and per-tariff confirmation |
| #41 | RS School free programs and current enrollment | Official catalog/detail pairing, unknown/closed rejection and publication tests |
| #42 | HTML Academy course/payment format binding | Public payment-page fixtures; recurring fees never become full prices |
| #43 | Skillbox product and full-card payment tariffs | Payment/product reconciliation, binding protection and per-tariff publication |
| #44 | Skillfactory course-specific tariff price API | URL/title/tag/popups, full-price/discount/cohort reconciliation and publication |
| #45 | Skypro target pricing block and enrollment | Closed/unbound fixtures, protected product and unknown-price safeguards |
| #46 | JavaRush recurring fees in original currency | USD/month card/API pairing, billing persistence and budget/free exclusion |
| #47 | Safe 13-provider queues, fairness and resource budgets | Lane rotation, quotas, backpressure, retention and distinct coverage counters |
| #49 | Practicum base/plus offers sharing one canonical landing | Displayed-card binding, independent source reads, manual tariff preservation and rollback |

## Operating contract

The API collector owns Stepik and Practicum. The page collector owns the other
11 configured providers. Both collect anonymous official evidence and enqueue
normalized facts or fixed failure codes. They do not publish. The publisher
makes no source HTTP requests and applies verified updates transactionally.

No source failure, unknown number or introductory free module becomes an invented
full price. Unsupported markup, changed product identity and ambiguous manually
edited records stay protected. Existing publications are retained on failed reads;
verification deadlines and stale-price filters prevent old evidence from being
represented as a currently verified amount. A closed enrollment observation is
confirmed before replacing an open offer and does not publish a new course.

The roadmap implements automatic publication without review PRs for each source
change. It does not promise that every URL in a school's sitemap is a supported
program, that every provider is reachable from every environment, or that a fixed
number of new courses will be published each day. Coverage comes from distinct
published database rows; source discovery and accepted refreshes are separate.

## Release and operation

Merge the combined PR to create the version tag and GitHub Release automatically.
Choose that published tag in **Deploy release** to deploy. Merge, tag push and
release publication do not deploy production. After deployment, **Collect catalog**
can request an immediate run; otherwise the 09:00/21:00 Europe/Moscow schedule
continues. Inspect `catalog-update status` for distinct course/offer totals,
per-provider coverage, pending queue age, liveness and source failures.

The resource-check procedure and its limitations are documented in
[catalog-resource-budgets.md](catalog-resource-budgets.md). It uses disposable
resources and does not modify production or read the VDS environment.

## Measured anonymous collection — 2026-10-10

The disposable concurrent-process check finished in **200.59 seconds** and
published **28 distinct courses / 46 offers** from real official HTTP responses.
The four languages had 9 Go, 5 Java, 10 JavaScript and 4 Python courses. Six
Practicum courses produced ten offers, demonstrating independently verified
shared-landing tariffs in this live run. All 68 retained observations were
acknowledged; pending publication queue size was zero.

| Worker | Sampled peak memory | Container limit | Outcome |
| --- | --- | --- | --- |
| API collector | 13.75 MiB | 128 MiB | Exit 0, no OOM |
| Page collector | 28.61 MiB | 128 MiB | Exit 0, no OOM |
| Publisher | 12.30 MiB | 128 MiB | Queue drained, no OOM |

The Stepik sitemap failed, while existing curated Stepik details still verified.
Unsupported/protected source contracts remained unpublished. RS School produced
no newly publishable course in this run; verified closed enrollment cannot seed a
new offer. JavaRush and HTML Academy retained unknown complete-course prices,
and Skypro published only a currently eligible JavaScript program with unknown
complete price. These limitations were preserved rather than replaced by guessed
prices, curriculum or availability.

The machine-readable [resource/coverage record](catalog-resource-result-2026-10-10.json)
includes the image digest, fixed source diagnostics, sampled memory and database
coverage. These are **disposable test results, not the VDS catalog**. Source
availability and results will change across runs; sampled peaks are not guaranteed
high-water marks. All temporary worker containers, PostgreSQL and the network
were removed after measurement. The final production image was also rebuilt, and
the backend race suite, focused retention/rollback checks, vet, API compatibility
and release validation passed.
