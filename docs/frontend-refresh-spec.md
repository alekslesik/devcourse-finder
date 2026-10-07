# Frontend refresh specification

## Goal and delivery

Make course discovery reliable, readable, consistent across routes, and usable in
light and dark themes. Preserve filtering, shareable URLs, verified pricing,
comparison, source links, keyboard access, and catalog import boundaries.

Deliver one PR with a separate implementation commit for each numbered task.
Increment the release version once for this PR and summarize it in CHANGELOG.
Deploy only through the existing manual release workflow.

## 1. Isolate analytics failures (P0)

- [x] Complete.
- Search must succeed when `crypto.randomUUID` is unavailable, event requests fail,
  or event serialization/transmission throws synchronously.
- Use a supported cryptographic UUID fallback or skip analytics gracefully.
- Never display an internal browser exception as the search error.
- Browser regression tests cover missing UUID and failing analytics.

## 2. Distinguish catalog states (P1)

- [x] Complete.
- Provide separate loading, service-error, empty-catalog, and filtered-no-match states.
- Keep the user's filters when retrying. Offer reset only when it is meaningful.
- Use plain Russian messages and accessible status/alert semantics.
- An empty production catalog must not advise increasing a budget with no filters.

## 3. Establish visual foundations (P1)

- [x] Complete.
- Introduce semantic CSS tokens for surfaces, text, borders, actions, and statuses.
- Standardize typography, spacing, field/button sizes, radius, and focus treatment.
- Remove hard-coded white surfaces and unreadably small essential labels.
- Use the same controls across catalog, course, comparison, and information pages.

## 4. Add theme preferences (P1)

- [ ] Complete.
- Expose System, Light, and Dark preferences and persist an explicit choice.
- Follow OS changes while System is selected; work when localStorage is unavailable.
- Apply the initial theme before paint without a light flash or hydration warning.
- Theme applies to every route, native form controls, dialogs, and loading states.

## 5. Restructure the catalog workspace (P1)

- [ ] Complete.
- Compact the introductory content and align results/actions in one toolbar.
- Show applied filter chips with individual removal and a clear reset action.
- Keep sorting and link sharing discoverable without dominating the result area.
- Use correct Russian count wording and retain URL/history navigation.

## 6. Improve filters and mobile layout (P1)

- [ ] Complete.
- Preserve unsaved filter edits when changing sort order.
- Keep URL-applied filters separate from draft form values; apply on search.
- Hide filters behind an accessible expandable control on small screens.
- Show active filter count, prevent invalid min/max budgets, and keep numeric fields readable.
- Reset restores defaults; back/forward restores applied filters.

## 7. Improve course cards (P2)

- [ ] Complete.
- Prioritize title, provider, verified full price, practical conditions, and actions.
- Keep demo, enrollment, freshness, and unknown-price information explicit.
- Use stable offer identifiers for card keys and consistent selectable comparison controls.
- Support long titles, summaries, and prices without overflow in either theme.

## 8. Unify routes and comparison (P2)

- [ ] Complete.
- Share navigation, footer, and theme controls across all routes and error pages.
- Style course details, about content, and comparison using the same primitives.
- Reuse one comparison table for the full page and legacy comparison dialog.
- Preserve unavailable/closed offers, limit selection to three, and allow removal.

## 9. Verify visuals and accessibility (P1)

- [ ] Complete.
- Check 360, 768, and 1440 px in both themes; no page-level horizontal overflow.
- Inspect successful results, loading, errors, empty catalog, no matches,
  course details, comparison, and about pages.
- Verify keyboard interaction, visible focus, WCAG AA contrast, and reduced motion.
- Add meaningful browser regressions; retain existing pricing and search-race coverage.
- Run frontend typecheck, production/test build, route tests, and browser tests.

## Scope boundaries

Catalog publication remains a separate reviewed operation. This PR does not
import courses, change ranking/pricing rules, deploy the site, or alter database schema.
