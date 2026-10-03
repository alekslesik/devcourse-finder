# Browser acceptance with the real stack (MVP-QA-02)

Prerequisites: Docker Compose, Node 22, frontend dependencies (`npm ci` in
`frontend`), Python 3, curl and Playwright Chromium (`npx playwright install
--with-deps chromium` in `frontend`). For a system browser set `CHROMIUM_PATH`.

From the repository root:

```sh
./scripts/e2e-real.sh
```

The script builds the production API and standalone Next.js images, starts a
fresh PostgreSQL 17 database, migrates it, validates/imports temporary demo data
and runs Playwright against the actual frontend proxy. Ports 8091 and 3091 must
be free. `COMPOSE_PROJECT_NAME`, database settings and image names are generated
or overridden by the script to protect existing deployments. On exit it removes
its own containers, database volume, images and temporary catalog, including when
a test fails. Test traces remain under `frontend/test-results/` on failure.

The catalog preserves the demo data and uses current verification dates so
budget tests do not expire over time. It adds closed, draft and archived records
for availability tests. This dataset is synthetic and must never be published.

The five shared test groups cover all eight required scenarios: anonymous home,
search with language/goal filters, empty results/reset, permanent course URL,
three/four tariffs, comparison in a fresh session, provider redirect, and an
understandable API error. Shared tests also check unavailable tariffs, keyboard,
360 px and axe. Two additional real-stack groups stop/restart the actual API to
check retained filters and retry, verify private/missing courses return 404,
and query PostgreSQL to confirm new browser search and outbound events persist.
Provider HTML is intercepted only after the real `/out` redirect. The real-outage
scenario uses no API interception; the shared controlled-error scenario remains
for fixture testing.

`E2E_REAL_API=1` switches Playwright to this already-running stack; the default
fixture mode starts its own servers and excludes real-stack-only tests. The
CI job `real-e2e` runs the script on every PR and saves failure traces.
The existing fixture and Compose smoke jobs continue to run.

Cloud builds preserve Docker proxy settings and pass `CODEX_PROXY_CERT`, when
present, as a temporary BuildKit secret. API outage tests can print expected
connection-refused messages from the frontend proxy; recovery is asserted before
success. No production services are stopped.
