# Catalog UX epic #91: implementation and acceptance

This change implements the frontend work in #92–#97 and adds acceptance evidence
for #98. It preserves the approved liquid glass identity, Golos Text, both themes,
the existing API and price rules, and always-visible primary route filters.

## Behaviour

- The original epic made the hero more compact; PR #109 subsequently adopts a
  more spacious, full-width introduction at the maintainer's request. The heading
  now specifies programming courses, the decorative sphere is removed, and
  mobile language shortcuts reflow into two rows. A submitted search moves focus to the results
  heading after the response settles, including empty/error outcomes. Sorting or
  editing the form during a pending request cancels the pending focus transfer.
- The latest main profile diagnostics and explicit unknown-profile opt-in are
  retained. Enabling that opt-in preserves unrelated drafts, and status messages
  distinguish confirmed programs from those with incomplete data.
- Passive successful updates use a persistent polite status region. A submitted
  search relies on the focused heading instead, so the count is not announced
  twice. Existing loading/error/empty semantics remain in place.
- Hero and form language choices both edit a draft. Only **Найти обучение** applies
  it. Applied chips still update the current search immediately, while preserving
  unrelated draft fields. The draft notice identifies unapplied edits.
- Course and comparison return links retain the originating applied URL, page,
  sort and scroll position in session storage. Direct entry and blocked/corrupt
  storage fall back to the catalog. Stored return URLs must be same-origin `/`
  or `/courses`; external and other-route targets are rejected.
- Mobile comparison keeps the conditions column and program headings visible in
  a bounded, keyboard-focusable scrolling table. Horizontal overflow inside that
  region is intentional; page-level horizontal overflow remains prohibited.
- Mobile fields use at least 16px text; theme and summary controls have 44px touch
  areas. Hover styles require a fine pointer with hover support, with separate
  press feedback. Course details show a factual first-tariff summary and a tariff
  anchor; external links and current navigation use consistent indicators.

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

An installed Chromium can be selected with `CHROMIUM_PATH=/usr/bin/chromium`.
The route/browser checks start their own fixture API and production frontend and
stop them after completion. They do not deploy or publish catalog data.

The project-local detector can be run from the repository root:

```sh
IMPECCABLE_HOME=/tmp/devcourse-impeccable .agents/skills/impeccable/scripts/impeccable detect --json frontend/app
python3 scripts/check-api-compatibility.py origin/main
python3 scripts/release.py check --base-ref origin/main
```

## Coverage

`frontend/tests/catalog-ux.spec.ts` adds end-to-end scenarios for:

- Draft edits surviving chip removal and sorting; hero selection does not silently
  apply the draft; submit and browser Back agree with the applied URL.
- Focused submitted results and passive live updates without focus theft, including
  a later passive action superseding a pending submitted search.
- Course/comparison return URLs and scroll position, including removal of the last
  tariff, reload, direct entry, unsafe
  saved URLs and unavailable session storage.
- Both axes of mobile comparison staying fixed while the table scrolls.
- Explicit layout budgets on the same demo fixtures and 900px viewport height:
  the first card begins before 1800px at 390px width and before 1250px at 1440px.
  PR #109 increases these limits from 1350px/950px to accommodate the requested
  spacing; the mobile limit still bounds the path to results to two viewport heights.
  The test also checks that language shortcuts and filters share the same width
  and left edge, have at least 24px separation, and that navigation stays clear
  of the theme control.
  This test explicitly supplies the historical response shape without optional
  diagnostics, even in real-stack CI. These are reproducible fixture regression
  limits, not conversion metrics or a cap on extra diagnostic explanations.

The visual suite now covers 360, 390, 768 and 1440px in light/dark themes. Each of
the eight configurations checks catalog, details, comparison, about, legacy
dialog, loading, service error, empty catalog and filtered no-match: **72 captures**.
Each capture checks page overflow and axe WCAG 2 A/AA and WCAG 2.1 AA rules.
CI uploads screenshots and failure traces as `frontend-visual-checks`.

Existing tests retain pricing qualifications, closed/removed offers, native radio
keyboard controls, modal focus, theme persistence and system changes, blocked
storage, long content and protection against stale search responses.

## Evidence

The complete local integration run passed **51 browser tests**, including the
profile-filter regression from current main. After review, **14 relevant browser
tests were repeated and passed**, including return after removing the final tariff
and an explicit fixed-response setup for the layout baseline. The visual matrix produced72
captures with no axe WCAG2A/AA/2.1AA violations or page overflow. Typecheck,
production build, production route checks, release metadata and API compatibility
checks passed.

Baseline audit on `58dc10d` used the same demo catalog: first card y=1647.5px on
390×900, y=1080.98px on 1440×900. The original epic layout measurements were attached as
`catalog-layout.json` by the regression tests: **1301.63px** on 390×900
and **946.19px** on 1440×900. The historical baseline predates the new main
profile opt-in, which is retained in the integrated result. Real-stack CI also
recorded **1360px / 1004.56px** with the additional diagnostic explanation visible;
that content is not hidden to meet the historical fixture budget. Measurement coordinates include
the demo banner; they are not claims about production course count or coverage.

For the spacious layout in PR #109 (version 0.23.2), the same fixed-response
fixture measures **1745.91px** on 390×900 and **1197.25px** on 1440×900.
These measurements replace the original epic values for current regression
limits. The local confirmation run passed all **17** catalog UX and visual
tests, with 72 captures across both themes and four widths, no page overflow
and no axe violations. Typecheck, production build and release metadata passed;
the layout detector returned `[]`. The full PR CI also passed before this
documentation correction. Physical-device acceptance below remains outstanding.

API generation/compatibility checks pass without contract changes. Redocly keeps
its existing advisory about the outbound redirect endpoint having a 302 response
instead of a 2xx response. Impeccable engine 0.1.14 reports `[]` for `frontend/app`;
this detector result alone does not establish accessibility.

## Remaining manual acceptance (#98)

The following require a real device or assistive technology and were not verified
by the available Chromium automation. Keep #98 open until evidence is recorded:

- [ ] VoiceOver/NVDA: announce passive search counts and focused submitted results
  once; confirm loading, error and empty transitions remain understandable.
- [ ] Real Safari/iPhone: budget input without automatic zoom, screen keyboard,
  touch/hover behaviour and safe-area interaction with the comparison bar.
- [ ] Real browser zoom at 200%, including keyboard focus and comparison scrolling.
- [ ] Current live-catalog smoke with real long titles and unknown/stale conditions;
  demo fixtures and existing pricing tests do not prove live provider coverage.

Blur/GPU performance was not profiled on a physical phone. No glass effects were
removed on the assumption of jank. Backend, deployment and release automation are
outside this frontend change; merge and manual deployment remain separate actions.
