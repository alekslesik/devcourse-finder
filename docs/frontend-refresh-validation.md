# Frontend refresh validation

This accompanies the [nine-task specification](frontend-refresh-spec.md).
The browser suite uses an optimized Next.js production build and a local catalog
API fixture. Go/PostgreSQL integration remains covered by the existing CI jobs;
these local UI checks do not publish catalog data or deploy production.

## Reproduce

From `frontend/`:

```sh
npm ci
npm run api:check
npm run typecheck
npm run build:test
npm run test:routes
npx playwright install chromium
npm run test:e2e
```

A system Chromium installation can be selected with `CHROMIUM_PATH`.

## Visual matrix

`tests/visual.spec.ts` checks 360, 768, and 1440 px in both light and dark themes.
For each of the six configurations it inspects and captures:

- Catalog results and offer cards.
- Course details.
- Comparison with available, closed, and removed offers.
- About page.
- Legacy comparison dialog.
- Loading, service error, empty catalog, and filtered no-match states.

Each state checks page-level horizontal overflow and runs axe against WCAG 2 A,
AA, and WCAG 2.1 AA rules, including contrast. Reduced-motion mode disables the
loading animation. The comparison region permits intentional internal scrolling.
There are 54 full-page screenshots. CI uploads them and any failure traces in the
`frontend-visual-checks` artifact. Screenshots are inspection artifacts, not pixel
baselines; review them when changing layout. Automated axe checks do not replace
manual accessibility review.

## Interaction coverage

`tests/refresh.spec.ts` adds regressions for unsupported UUID APIs and failing
analytics, distinct catalog states and malformed responses, theme persistence,
OS theme changes and blocked storage, theme initialization before React loads,
hydration errors, visible keyboard focus, filter drafts during sorting, budget
validation, applied chips/history, and clearing saved comparison selection.

Existing acceptance tests retain pricing qualification, latest-search race
guards, keyboard-only mobile search/comparison, unavailable offers, outbound
links, and dialog focus restoration. Mobile search returns focus to its filter
control after collapsing the form. Inline document links use underlines as well
as color; external-link icons use SVG instead of font-dependent glyphs.

## Validation record

Date: 2026-10-07. Local validation passed:

- API schema/type generation and compatibility gate.
- TypeScript typecheck and optimized production build.
- Production route integration test.
- All 35 fixture-backed browser tests, including all six visual configurations.
- Release metadata validation against `origin/main` and four release-helper tests.

The final course-source icon uses the same SVG primitive as offer cards and
comparison actions; the affected visual suite is rerun after that final adjustment.
Manual screenshot inspection includes mobile catalog cards, course details,
service errors, the comparison dialog, and the desktop dark catalog; the automated matrix covers every state listed above.
