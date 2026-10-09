# Changelog

Every pull request increments `VERSION` and adds an entry here before merging.
Merged pull requests receive the matching Git tag and GitHub Release.

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
