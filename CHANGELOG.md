# Changelog

Every pull request increments `VERSION` and adds an entry here before merging.
Merged pull requests receive the matching Git tag and GitHub Release.

## [0.24.0] - 2026-10-13

- Ship the approved glass catalog design with a book-path logo, muted language colors, responsive header and grouped filters; include free courses and incomplete profiles by default.
- Verify the deployed frontend footer version before recording a successful deployment.


## [0.23.3] - 2026-10-13

- Authorize agents to deploy published releases after merging and verify production health and version.

## [0.23.2] - 2026-10-13

- Make the catalog introduction and filters more spacious across screen sizes, remove the decorative sphere and clarify the programming-course headline.

## [0.23.1] - 2026-10-13

- Document automatic PR merging after review and CI, followed by verified release publication.

## [0.23.0] - 2026-10-12

- Add an optional OpenAI collector for verified Stepik and Practicum courses, using source-selected quotations and independent semantic verification.
- Cache unchanged scoped documents, cap daily requests persistently and run metadata enrichment in a separate twice-daily worker with health/status and a manual trigger.
- Publish protected profile and learning facts atomically through the existing publisher without changing identities, prices, enrollment or financial verification dates.
- Match verified experience intervals without blindly expanding beginner labels; preserve literal curated legacy classifications.
- Add PostgreSQL rollback, operator ownership, quota, cache and source-isolation regressions plus an opt-in read-only OpenAI benchmark.

## [0.22.0] - 2026-10-12

- Display the root release version in the site footer and include the selected tag in manual deployment run and job names.

## [0.21.1] - 2026-10-12

- Shorten the catalog path, retain search drafts and return context, and keep mobile comparison headings visible.
- Announce completed searches without duplicate focus feedback, improve touch controls and course summaries, and extend browser UX acceptance coverage.

## [0.21.0] - 2026-10-12

- Model independently verified prerequisites, audience, exclusions, outcomes, direction and target level; preserve literal legacy classifications and reject contradictory source claims.
- Add course-bound profile extraction contracts for all 13 supported providers, with provenance and conservative handling of missing, optional, negated or unrelated statements.
- Backfill existing published identities through normal bounded refreshes; protect operator edits, clear only disappeared collector-owned facts and report per-provider profile coverage.
- Display contextual confirmed filter counts and offer an explicit opt-in group for courses with incomplete profile data, preserving price, support, workload, URL history and mobile accessibility constraints.
- Add PostgreSQL rollback/backfill regressions, filter combination acceptance and a bounded live-source report under the production worker memory limits.

## [0.20.0] - 2026-10-11

- Add opt-in contextual filter diagnostics and treat a zero lower budget as unrestricted.

## [0.19.1] - 2026-10-11

- Install pinned project-local frontend design skills from Impeccable, Taste Skill and Emil Kowalski, including references, licenses, provenance and repository usage guidance.

## [0.19.0] - 2026-10-11

- Collect evidence-bound calendar duration, explicit weekly workload and independent human review, personal mentor and community facts for all 13 supported providers; retain unknown values when official evidence is absent or ambiguous.
- Bind learning conditions to verified course/tariff sections, preserve ranges, ignore billing periods and automated checks, and reject conflicting support claims.
- Recover legacy PurpleSchool support ownership from import history while preserving later operator changes; clear vanished collected facts without refreshing retained facts on failed source reads.
- Display duration/workload ranges and independently confirmed support in existing course and comparison views without changing their layout.
- Report per-provider field coverage and oldest fact dates in catalog status; add opt-in bounded live-source acceptance, PostgreSQL publication regressions and browser coverage.

## [0.18.2] - 2026-10-11

- Simplify catalog controls on mobile and desktop: remove hero catalog links, sharing controls and benefit copy, hide the visible sorting label and place mobile filter chips above sorting.

## [0.18.1] - 2026-10-11

- Keep mobile route filters open, scroll language shortcuts to the route form, hide the mobile hero catalog link and align the language tiles and diagonal scene captions.

## [0.18.0] - 2026-10-11

- Introduce evidence-bearing duration ranges, explicit weekly workload and independent review, mentor and community facts with per-field collector ownership and conservative search filters.
- Add the all-provider learning-conditions epic and linked implementation specification.
- Recover pre-upgrade duration ownership from identity-bound import history, preserve later operator overrides and mark newly added Practicum plus durations as collected.
- Track ownership of collected duration and clear disappeared source evidence while retaining manually curated course conditions.

## [0.17.1] - 2026-10-11

- Simplify course cards by hiding unknown facts and weekly workload; preserve verified Practicum duration in months across collection, storage, API and course views.

## [0.17.0] - 2026-10-10
- Complete the catalog-ingestion roadmap with an adapter acceptance matrix, measured coverage/resource evidence and explicit manual-release operation instructions.
- Derive safe multi-provider lane limits, rotate discovery/detail priority, bound candidate and observation queues, and verify concurrent worker resource usage separately from distinct published coverage.

- Group explicitly displayed Practicum base/plus tariffs under one canonical course, with independent verified prices, protected identities and transactional publication.

## [0.16.0] - 2026-10-10

- Add Skillbox product-bound full-card tariffs and Skillfactory course-specific public pricing verification.
- Add Skypro product-scoped enrollment checks; preserve unknown full prices and reject newly closed courses.
- Preserve JavaRush recurring fees in their original currency with additive API/store billing metadata and unchanged frontend files.
- Protect multi-offer identities, interrupt failed-read confirmations, and confirm recurring fee anomalies across bounded evidence leases.
- Add recorded source fixtures, negative and PostgreSQL tests, provider runbooks, and manual-release deployment instructions.

## [0.15.0] - 2026-10-09

- Add HTML Academy discovery and independent public payment verification to the page collector.
- Keep recurring course totals unknown and exclude embedded profession bundles; reject missing, mismatched or expired evidence.
- Document source coverage, limitations, queue safety and manual release deployment.

## [0.14.0] - 2026-10-09

- Add RS School discovery and independent course/catalog checks for free full education and current Russian cohort enrollment.
- Reject undetermined dates, English-only cohorts, contradictory evidence and archived registration identities; preserve prerequisites and stage-specific support limitations.
- Keep new closed cohorts unpublished and confirm later closures across bounded freshness leases; add public fixtures, PostgreSQL tests and an English runbook. Frontend unchanged and deployment remains manual.

## [0.13.0] - 2026-10-09

- Adopt the supplied liquid glass design with local Golos Text fonts, responsive catalog cards, horizontal filters, accessible styled selects and language shortcuts; preserve live API search, pricing and comparison.

## [0.12.0] - 2026-10-09

- Reset all PurpleSchool tariff confirmations after failed reads in both split-worker and legacy collection modes.

- Add automatic PurpleSchool discovery with separately verified self-study, AI and mentor tariffs.
- Exclude demo modules; verify standalone free curricula and reconcile current tariff prices with rendered cards and purchase identities.
- Publish multiple tariffs atomically with independent price confirmation, failed-read resets, protected identities and bounded freshness leases.
- Add public fixtures, negative and PostgreSQL tests, and an English adapter runbook. Deployment remains manual.

## [0.11.0] - 2026-10-09

- Add automatic Netology catalog discovery and verified full-payment parsing in the page collector.
- Protect Netology program families while allowing cohort rotation; reject conflicting prices, tariffs and start dates.
- Add source fixtures, negative and PostgreSQL publication tests, and an English adapter runbook. Deployment remains manual.

## [0.10.0] - 2026-10-09

- Add a dedicated Practicum collector that reconciles official full-price and cohort APIs with canonical course pages and the robots-declared sitemap.
- Reject monthly installments, free introductions, personalized discounts, expired offers and unbound course/tariff identities; verify cohort deadlines and available seats.
- Persist profession and billable product identities separately, protect later identity changes, reset interrupted anomaly confirmations, and retain atomic observation publication.
- Add recorded public API fixtures, failure cases, PostgreSQL integration tests and an optional bounded live verification; document shared-page tariff and access limitations.

## [0.9.0] - 2026-10-09

- Separate catalog ingestion into independently scheduled API and page collectors with a durable PostgreSQL observation queue and atomic publisher.
- Preserve original verification timestamps, failure ordering and manual edits; acknowledge publication in the same transaction as catalog and audit changes.
- Add independent worker health, queue diagnostics and bounded retention; reject invalid, duplicate and outdated observations.
- Run all three roles in production and support both split and legacy deployments through the manual Collect catalog button, without enabling automatic deployment.
- Track mandatory Practicum and further provider adapters in a GitHub roadmap; no additional school adapter is enabled by this release.

## [0.8.0] - 2026-10-09

- Expand bounded Stepik discovery to up to 120 new candidates and 30 refreshes per run, preserving provider fairness and the collection deadline.
- Report safe verification rejection codes for Stepik and structured course pages without publishing invalid records.

## [0.7.0] - 2026-10-09

- Add a manual Collect catalog GitHub Actions button to run an extra bounded collection with the currently deployed worker and report actual coverage.
- Coordinate manual collection with release deployment using the host lock, preserve failed collection diagnostics, and protect the SSH script input stream.

## [0.6.0] - 2026-10-08

- Replace broken CodeBasics/Hexlet discovery endpoints with the verified official catalog and robots-declared program sitemap.
- Automatically verify and publish complete CodeBasics programs using current curriculum identity and independently refreshed platform-wide free-pricing evidence; include TypeScript in the JavaScript family.
- Record actual source coverage and remaining catalog-growth limits without counting unverified candidates as published courses.

## [0.5.0] - 2026-10-08

- Discover and publish verified catalog candidates automatically from official provider feeds with persistent identity, bounded scheduling and measured coverage.
- Preserve unknown course classifications, support and full-price evidence in filtering and display; reject ambiguous, expired and recurring-price offers.

## [0.4.0] - 2026-10-08

- Remove services absent from the selected release, including when rolling back with an older deployment script.

- Add a separate production catalog updater scheduled at 09:00 and 21:00 Europe/Moscow, with automatic publication and no data-review PRs.
- Enable verified Stepik Go/Python API sources; retain other catalog templates until dedicated adapters can verify their price and enrollment contracts.
- Preserve manual course states and other tariffs, confirm anomalous prices/closures in separate runs, and commit catalog snapshots atomically with API cache invalidation.
- Track collection runs, source failures and evidence hashes; add worker health and deployment checks without changing manual release deployment.

## [0.3.1] - 2026-10-08

- Simplify the catalog introduction to a smaller, plain heading and a single supporting sentence.
- Add restrained, clipped circular background accents that adapt to light and dark themes.
- Remove the decorative catalog route illustration and replace the theme dropdown with an accessible sun/system/moon capsule that preserves saved and system preferences.

## [0.3.0] - 2026-10-08

- Rebrand the shared header, footer, page metadata, messenger cover and browser icons as DevCourseFinder using a responsive, theme-aware SVG identity.
- Add minimal flat catalog and result-state illustrations without crowding mobile search.
- Keep all desktop filter controls reachable when the sticky sidebar is taller than the viewport.

- Add branded 1200×630 Open Graph and large-image Twitter cards for Telegram and other messengers, with route-specific titles and descriptions.
- Pass the public site origin into frontend builds and ship preview assets in the production container.
- Verify Telegram, WhatsApp and Facebook crawler HTML plus the public PNG in production route tests.

## [0.2.0] - 2026-10-07

- Refresh the catalog interface with accessible light, dark, and system themes, responsive filters, readable offer cards, and shared navigation.
- Keep course search working when browser UUID APIs or analytics are unavailable; distinguish loading, service errors, empty catalogs, and filtered results.
- Preserve filter drafts during sorting, validate budget ranges, add removable applied filters, and reuse comparison tables across routes.
- Add browser regressions and a six-configuration visual and WCAG accessibility matrix; upload screenshots from frontend CI.

## [0.1.0] - 2026-10-07

- Introduce semantic versions, per-PR changelog entries, and automatic GitHub Releases after merging into main.
- Add manual deployment of a selected published release through GitHub Actions.
- Run both application code and the deployment helper from the selected release commit for reproducible deployments.
- Verify the VDS SSH host key, preserve production configuration and database storage, and back up the database before deployment.
- Record the deployed version and commit after successful application checks.
