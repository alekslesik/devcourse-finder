# Catalog UX: approved design and acceptance

Version 0.24.11 implements the agreed glass catalog layout. The selected visual
references are recorded in the separate `codex/design-preview-downloads` branch:
`compact-cta-desktop-{light,dark}.png`, `assembled-mobile-{light,dark}.png`,
with the authoritative book/path mark selected from `brand-mobile-dark.png`
and the two-row mobile header from `brand-mobile-light.png`.

## Current behaviour

- The headline is **Найди свой курс программирования**, without punctuation. The desktop language
  tray sits beside it at a slight angle, with matching 80px top/bottom hero
  spacing; mobile uses a straight two-by-two grid.
  Language shortcuts pair the maintainer's glass-style icon artwork with the
  full name: cyan Go, amber Java, gold JS letters and blue/yellow Python. A local
  transparent WebP atlas supplies all four images; per-mark view boxes preserve
  proportions. Intrinsic widths replace the shared square slot; optical heights
  balance Java’s fine outline and Go’s broad lettering with JS and Python
  (desktop 40–48px, mobile 28–34px); Go is enlarged further for readability.
  Text labels remain the accessible button names. The earlier
  Devicon vectors and source/license notices remain available in `public/languages/`.
  Selection still uses a light tint, outline and checkmark.
- The open-book logo has a thin gold path and a lilac `finder` segment. Header,
  footer, downloadable SVGs, browser/Apple icons and the versioned social preview
  use the updated mark. The outlined wordmark needs no font download.
- Mobile places the logo above navigation and theme controls inside one glass
  header. Desktop uses one header row. Both light and dark themes are supported.
- The logo, primary actions, links, focus and generic selection share the purple
  accent tokens (light `#6845a8`, dark `#b8a0eb`); language and status colors keep
  their semantic roles. A static blue/lilac ambient canvas is visible through
  lightly tinted glass panels, with further reduced white opacity in light mode. Raised dropdown/table surfaces stay opaque;
  unsupported backdrop filtering and reduced-transparency preferences use solid
  surfaces, with prefixed backdrop filtering for Safari.
- Clearer control panels use thinner tint, a masked bevel rim, localized inner
  highlights and a smaller saturating backdrop blur; controls inside a panel do
  not add another backdrop blur. Form fields now use a slightly stronger translucent
  fill and accent border; course details buttons use a light accent tint and
  matching text/border to stay identifiable. Reading cards use a slightly denser material.
  Viewport safe areas keep content clear of iPhone cutouts; fixed dialogs apply
  their own inset padding and dynamic-height limit rather than relying on body; page backgrounds and supported browser theme-color metadata follow both
  system and explicitly chosen themes, including before React loads. The ambient
  background fades from the solid browser theme color at the top edge, avoiding
  an abrupt color seam; actual Safari status-bar behavior still needs phone validation. Increased contrast and reduced transparency use solid surfaces.
- All route fields share one aligned, always-visible grid without an extra
  parameters heading, divider or accordion. The desktop search button is 240px wide and
  right aligned; mobile uses full width. Native inputs, labels, visible focus,
  at least 44px touch targets and 16px mobile field text remain in place. Empty
  comparison checkboxes use a transparent fill and clear outline; selected boxes
  have an accent fill and checkmark, with native forced-color rendering retained.
  Active header navigation uses heavier text rather than an underline.
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
- Card source links and title links have at least 44px-high hit areas without
  enlarging their type. Source link focus uses zero outline offset so the card
  does not clip it; footer text stays aligned with a compact padded row.
- Course cards use an accent rim, a translucent light wash and a 4px lift on
  pointer hover, without underlining the title. Keyboard focus highlights the
  card without moving it; touch presses get the light wash without sticky hover.
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
With the unified form and restored programming headline, measured first-card
positions are **1615.64px / 1221.66px**.
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

## Liquid Glass references and limits

Apple's [iOS 27 overview](https://www.apple.com/os/ios/) describes refined
refraction/contrast and a system transparency range. The
[Meet Liquid Glass session](https://developer.apple.com/videos/play/wwdc2025/219/)
explains rim highlights, hierarchy, single material layers and accessibility
modifiers. These inform the web adaptation; CSS blur, saturation and masked
edges do not reproduce Apple's native adaptive lensing or system rendering.
Safari controls its own chrome tint; metadata support differs by browser.
No native-device performance or Safari chrome behavior is claimed from Chromium
emulation. Physical iPhone/Safari acceptance remains part of #98.
