# Remaining issues: combined acceptance

PR scope: #123 (verified profile coverage), #124 (HTTPS), #98 (browser and
manual acceptance), and parent #91. Built on v0.25.1; release v0.25.2.

## #123 — observed profile coverage

The production query `language=python&experience=none&goal=try&include_unknown_profiles=true`
returned **3 confirmed / 83 unverified / 86 total** on 2026-10-15.
The two captured API pages are projected into the small profile-only fixture
`backend/updater/testdata/profiles/python-try-snapshot-2026-10-15.json`.
It is a frozen API projection, not an atomic database dump or a catalog import.

Eight anonymous Stepik reads failed with HTTP 502 both in the workspace and
in GitHub Actions [run 38708366529](https://github.com/alekslesik/devcourse-finder/actions/runs/38708366529).
No unverified Stepik claim is promoted to a confirmed fact.

The official PurpleSchool `python-start` page was readable. Its single verified
course object explicitly declares no requirements, starting from zero, and
wanting to get acquainted with Python and try it in practice. The recorded field
excerpt includes its source, capture time and full-page SHA-256. Extraction runs
only after the existing identity/canonical/tariff checks, attaches per-field
locators, response digest and verification date, and retains unknown fields.
Prices, tariff identities and the matching rules remain unchanged.

On that **same frozen 86-course projection**, changing only this course's observed
profile produces **4 confirmed / 82 unverified / 86 total**. This is a reproducible
before/after experiment, not a claim about the mutable production catalog.
Tests reject title-based purpose inference, negated/conditional purpose claims,
qualified requirements, mismatched identity and duplicated publication identities.

After deployment, dispatch `Collect catalog` with `recheck_adapter=purpleschool`
and `recheck_id=python-start`. The new operator command schedules only one existing
published candidate; it respects the worker lock, leaves protected/pending/missing
identities alone, and changes no course facts. Normal fetching, validation,
publication and provenance still apply. Inspect the live course evidence/counts
after publication; source availability and other catalog refreshes can change totals.

## #124 — HTTPS diagnosis and repair

The same independent Actions run returned **connection refused** for HTTPS while
HTTP returned 200 without upgrading. This establishes a real public ingress
problem independently of the workspace proxy's 503/certificate diagnostics.
The sanitized result is committed in `public-endpoint-evidence-2026-10-15.json`.

`setup-https.sh` now resolves the active immutable release and shared production
environment instead of obsolete Compose files in the base directory. It uses the
existing deployment lock and keeps the existing managed-site/conflict protections.
The explicit `Configure production HTTPS` workflow uses the existing pinned SSH
key, refuses unmanaged conflicting sites, installs trusted TLS and tests Certbot
renewal. It does not deploy or change application/catalog secrets.

After merge/release/deployment, dispatch this workflow and verify its result.
`scripts/check-public-https.py` independently requires trusted TLS on home,
catalog, live course details, about, API and sitemap; exact permanent HTTP redirects
preserving query/path; and matching canonical/sitemap/social-image origins.
A failed setup or check must be reported and #124 remains open until it passes.

## #98 / #91 — browser evidence and limits

The existing Chromium suite covers 360/390/768/1440px, both themes, nine page/state
captures and four selected languages per configuration (104 images), overflow
and axe WCAG 2 A/AA and 2.1 AA, keyboard search/comparison, focus restoration,
URL/history, 0/1/3 offers, unavailable conditions, long text, pricing and races.

New touch-context checks exercise actual synthesized taps, editable 16px fields,
search focus and the comparison bar with a shortened viewport in both themes.
The shorter viewport does **not** establish software-keyboard behaviour. CI also
runs UX/navigation/dialog/touch checks in Playwright WebKit; that is a second
browser engine, not physical Safari/iPhone acceptance. Separate WebKit output
preserves the Chromium visual artifacts.

The initial service-worker zoom probe was replaced in the subsequent frontend
follow-up by a temporary extension **page** controller. It verifies actual
`tabs.setZoom(..., 2)`/`getZoom()`, halved CSS width and doubled DPR, then exercises
keyboard search, comparison scrolling and detail return in both themes. See
[frontend-brand-acceptance.md](frontend-brand-acceptance.md) for the method and
the separate user smoke feedback, which explicitly did not use a screen reader.

Keep #98 and #91 open for actual VoiceOver/NVDA speech
and physical iPhone Safari keyboard, safe-area/chrome and device performance
checks. There is no physical device or screen reader available in this runtime.
The implementation work from #92–#97 has already shipped; the epic's unchecked
historical checklist must not be interpreted as all those tasks still being open.

## Validation

Run the production build/typecheck, API contract and route checks, complete
Chromium suite, the selected WebKit suite, full Go tests with PostgreSQL and
`-race`, `go vet`, and Python collection/HTTPS acceptance tests. Run the release
metadata check against the current main before opening/merging the finished PR.
CI artifacts and the deployment/HTTPS/collection workflow results are the durable
execution record; do not replace missing device evidence with test counts.
