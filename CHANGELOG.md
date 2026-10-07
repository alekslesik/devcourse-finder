# Changelog

Every pull request increments `VERSION` and adds an entry here before merging.
Merged pull requests receive the matching Git tag and GitHub Release.

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
