# Catalog UX: approved design and acceptance

Version 0.24.1 implements the agreed glass catalog layout. The selected visual
references are recorded in the separate `codex/design-preview-downloads` branch:
`compact-cta-desktop-{light,dark}.png`, `assembled-mobile-{light,dark}.png`,
with the authoritative book/path mark selected from `brand-mobile-dark.png`
and the two-row mobile header from `brand-mobile-light.png`.

## Current behaviour

- The headline is **Найди свой курс**, without punctuation. The desktop language
  tray sits beside it at a slight angle; mobile uses a straight two-by-two grid.
  Language colors are muted and consistent in shortcuts, selected filters and
  course badges. Selection uses a checkmark as well as color.
- The open-book logo has a thin gold path and a lilac `finder` segment. Header,
  footer, downloadable SVGs, browser/Apple icons and the versioned social preview
  use the updated mark. The outlined wordmark needs no font download.
- Mobile places the logo above navigation and theme controls inside one glass
  header. Desktop uses one header row. Both light and dark themes are supported.
- All route fields share one aligned, always-visible grid without an extra
  parameters heading, divider or accordion. The desktop search button is 240px wide and
  right aligned; mobile uses full width. Native inputs, labels, visible focus,
  at least 44px touch targets and 16px mobile field text remain in place.
- Free and paid courses appear together by default, with price bounds applied
  equally. The frontend requests incomplete profiles by default through the
  existing API; these remain in a separate **Условия нужно уточнить** section,
  with truthful confirmed/unverified counts and explanations. Contradictory known
  conditions are not relabelled as matching.
- Removed budget and opt-in URL parameters (`budget`, `include_free`,
  `include_unknown_profiles`, `include_closed`) are normalized away rather than
  leaving invisible active filters. Closed/unknown enrollment retains the API's
  default exclusion; existing comparison links still retain unavailable tariffs.
  No backend endpoint, data or contract changes are needed.
- Hero and form choices both edit the draft; only search submission applies it.
  There is no redundant submit instruction. Chip removal and sorting preserve
  unrelated draft edits; browser history restores the applied parameters.
- Submitted results receive focus after the response settles. Passive responses
  announce counts without stealing focus. A later passive action cancels an
  earlier pending focus transfer. Late responses cannot overwrite newer searches.
- Course/comparison return links preserve the originating URL and scroll position.
  Direct entry, unavailable storage and untrusted context use a safe fallback.
- Mobile comparison keeps both axes of context in a bounded scroll region;
  horizontal scrolling inside the table is intentional, page overflow is not.
- Hover motion requires a fine pointer with hover capability. Keyboard actions
  and reduced-motion preferences avoid animated transforms. Touch gets short
  press feedback rather than persistent hover.

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

Use `CHROMIUM_PATH=/usr/bin/chromium` for an installed browser. Route and browser
checks own their local fixture API and production frontend; they do not deploy.
Run `./scripts/e2e-real.sh` for the isolated real-stack acceptance suite.

From the repository root:

```sh
IMPECCABLE_HOME=/tmp/devcourse-impeccable .agents/skills/impeccable/scripts/impeccable detect --json --scope layout frontend/app/finder.tsx frontend/app/liquid-glass.css
python3 scripts/check-api-compatibility.py origin/main
python3 scripts/release.py check --base-ref origin/main
```

## Layout and accessibility evidence

The fixed historical response fixture retains the same demo data, first page,
12 offers, banner and 900px viewport height. Regression budgets are **1800px**
at 390px width and **1400px** at 1440px width. The desktop limit increases from
1250px to accommodate all route fields; it is not met by hiding fields.
Removing the accordion reduces the first-card positions from
1669.22px / 1306.66px to **1579.39px / 1221.66px**.
These are fixture coordinates, not conversion metrics or limits on diagnostic text.

The regression checks also verify mobile tray/filter alignment, a minimum 48px
horizontal desktop gap between copy and tilted tray, at least 24px before the
filter panel, navigation separated from theme controls, and the compact desktop
submit button. Measurements are attached as `catalog-layout.json`.

The visual suite covers 360, 390, 768 and 1440px in both themes. It captures nine
routes/states per configuration plus four selected-language states: **104 images**.
Every capture checks page overflow and axe WCAG 2 A/AA and WCAG 2.1 AA rules.
CI uploads screenshots/traces as `frontend-visual-checks`. These checks include
idle and selected color contrast but are not a full manual accessibility audit.

Profile acceptance checks verify default incomplete results, paid/free inclusion,
legacy opt-in normalization, price drafts, reset, reload and history. Existing
suites retain pricing qualifications, unpublished 404s, keyboard radio controls,
modal focus, comparison limits, themes, stale responses and analytics protection.

## Remaining manual acceptance (#98)

Keep #98 open until real-device or assistive-technology evidence is recorded:

- [ ] VoiceOver/NVDA: search counts, focused submitted results and loading/error/empty
  transitions remain understandable without duplicate announcements.
- [ ] Safari/iPhone: input zoom, software keyboard, touch behaviour, safe areas and
  comparison-bar interaction; physical-device glass/GPU performance.
- [ ] Real browser zoom at 200%, including keyboard focus and comparison scrolling.
- [ ] Live catalog with long titles, stale prices and incomplete conditions.

Fixtures and browser automation do not establish provider coverage or physical
phone performance. This change preserves catalog data, pricing rules and backend
contracts. Release and deployment use the established workflows.
