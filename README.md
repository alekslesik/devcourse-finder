# DevCourse Finder

Find and compare programming courses by language, experience level, learning goals, and budget.

## Features

- Courses for Go, Python, Java, and JavaScript
- Free and paid learning options
- Filters for experience, goals, budget, and study format
- Side-by-side course comparison
- Direct links to course providers

No registration or payment required. Choose a course and enroll on the provider’s website.

## Status

All 20 functional acceptance criteria have passed and are merged. Production publication is pending. The repository contains:

- a Go REST API with course filtering, comparison, catalog import, and PostgreSQL storage;
- a Next.js interface for search, course details, and comparison;
- functional requirements and course-provider research;
- a Docker Compose development stack.

The catalog starts empty. Course records must be reviewed and imported explicitly; research data is not published automatically.

## Architecture

The application runs as four Docker Compose services:

| Service | Technology | Responsibility |
| --- | --- | --- |
| `frontend` | Next.js / React | Catalog browsing, filters, course pages, and comparison |
| `api` | Go | Search, comparison, catalog import, provider redirects, and event recording |
| `db` | PostgreSQL | Courses, offers, import history, and events |
| `migrate` | Go CLI | Creates and updates the database schema before the API starts |

Compose starts the database and waits for its health check, runs the migration
service to completion, starts the API and waits for readiness, then starts the
frontend. A failed migration prevents the API from starting.

On a production VDS, requests follow this path:

```text
Browser → HTTPS reverse proxy → Next.js → Go API → PostgreSQL
```

The frontend proxies `/api/*` and `/out/*` to the API. The production Compose
configuration removes the API's published port; PostgreSQL has no published
port. Both are reachable inside the Compose network. The HTTPS reverse proxy
must be configured separately, for example with Nginx and a TLS certificate.

The database, API, and frontend use `restart: unless-stopped` to recover after
process failures or server reboots when Docker starts. Migrations are a one-shot
service. PostgreSQL data persists in the `postgres-data` Docker volume across
ordinary container updates; deleting that volume deletes the database.

## Project documentation

- [Functional requirements](docs/functional-requirements.md)
- [MVP completion specification](docs/mvp-completion-spec.md)
- [MVP status by requirement ID](docs/mvp-readiness.md)
- [Catalog publication runbook](docs/catalog-operations.md)
- [20 real programs, reviewed sources and explicit publication](docs/real-catalog.md)

## Run with Docker

Requirements: Docker Engine with Docker Compose v2.

```sh
cp .env.example .env
docker compose up --build
```

Open <http://localhost:3000>. The API is exposed at <http://localhost:8080>; its readiness endpoint is `/health/ready`.

Compose starts PostgreSQL first, runs the idempotent migration as a one-shot service, and only then starts the API and frontend. Database data is kept in the `postgres-data` volume. To stop the application without deleting the catalog:

```sh
docker compose down
```

To remove the local database as well, explicitly add `--volumes`.

## Catalog operations

Validate a catalog JSON document without connecting to the database:

```sh
docker compose run --rm \
  --volume "$PWD/catalog.json:/data/catalog.json:ro" \
  api catalog validate /data/catalog.json
```

Replace `catalog.json` with the path to the file being checked. Import supports a dry run before applying changes:

```sh
CATALOG_OPERATOR="$USER" \
DATABASE_URL='postgres://devcourse:devcourse-local@localhost:5432/devcourse?sslmode=disable' \
  go run ./backend catalog import ./catalog.json --dry-run
```

Remove `--dry-run` after reviewing the output. Catalog imports are transactional and are not performed automatically when containers restart.

### Demo catalog

The repository includes 20 explicitly fictional programs for checking every supported language and the main price, support, and experience variants. Validate and preview the demo import with:

```sh
docker compose run --rm \
  --volume "$PWD/data/demo-catalog.json:/data/demo-catalog.json:ro" \
  api catalog validate /data/demo-catalog.json

docker compose run --rm \
  --volume "$PWD/data/demo-catalog.json:/data/demo-catalog.json:ro" \
  api catalog import /data/demo-catalog.json --dry-run
```

After reviewing the dry-run output, import the records explicitly:

```sh
docker compose run --rm \
  --volume "$PWD/data/demo-catalog.json:/data/demo-catalog.json:ro" \
  api catalog import /data/demo-catalog.json
```

All demo records are marked with `demo: true`, use `example.com` URLs, and are never imported automatically during container startup.

Production catalog publication, verification, audit, and recovery steps are defined in the [catalog publication runbook](docs/catalog-operations.md). Production imports must set a stable `CATALOG_OPERATOR` identifier rather than using the local default. Record every production run using the [publication record template](docs/catalog-publication-record.md) and keep the completed record with the release artifacts.

## Real catalog

`data/real-catalog.json` contains 20 real programs (five per language), 24 offers and no demo records. It is prepared for operator review and explicit publication; it is not imported automatically. Individual source checks and limitations are documented in [real-catalog.md](docs/real-catalog.md). Unknown full prices and enrollment remain explicitly unknown; free introductory modules are not labeled as free professions.

Follow the publication runbook with `CATALOG_FILE="$PWD/data/real-catalog.json"`: recheck sources, back up the database, review the dry run, import explicitly and verify the result. Existing demo records are not deleted by this import and require separately reviewed archival. The production host, daily backup schedule and final release verification still need to be configured.

## Local development

The Go workspace points to the backend module, so run repository-root checks with the explicit module path:

```sh
go test -race ./backend/...
go vet ./backend/...
```

Backend HTTP tests use the same handler as the production server and validate JSON responses against `docs/openapi.json`. Set `TEST_DATABASE_URL` to a disposable PostgreSQL database whose name ends in `_test` to run database tests. HTTP integration tests create and drop their own isolated schema; store tests reset tables in the test database. These checks cover imported price updates, event deduplication, and outbound redirects when analytics writes fail.

Run the frontend checks from its directory:

```sh
cd frontend
npm ci
npm run api:check
npm run typecheck
npm run build
npm run test:routes
```

The frontend proxies `/api/*` and `/out/*` to the API. Outside Docker, set `API_URL` when building the frontend if the API is not available at `http://api:8080`.

The CI workflow repeats the backend tests, frontend typecheck and production build, validates the Compose model, and builds both container images on every pull request.

Run the same isolated Compose smoke test locally with:

```sh
./scripts/compose-smoke.sh
```

The test builds and starts the stack under a temporary Compose project, validates and imports the demo catalog, checks health, search, program and comparison endpoints, restarts the stack without deleting its database volume, verifies that the catalog remains available, and removes all temporary resources.

## Public routes and contract

The frontend exposes `/courses`, `/courses/{slug}`, `/compare?offers=...`, and `/about`. Course pages are rendered on the server; draft, archived, and unknown programs return 404. Comparison links restore up to three tariffs independently of local browser storage.

The public contract is `docs/openapi.json` (OpenAPI 3.1). Run `npm run api:generate` from `frontend` after updating it; generated types are committed. CI validates the contract, verifies generated types, and applies a conservative compatibility gate against the target branch. The gate rejects schema and parameter changes requiring compatibility review. Route tests run against a fixture API and do not replace PostgreSQL integration or browser E2E tests.

Set `SITE_URL` to the public origin for canonical links and the sitemap. Set `API_URL` for a backend outside the Compose network.

## Browser verification

From `frontend`, run `npm run build:test`, `npx playwright install --with-deps chromium`, then `npm run test:e2e`. The test build uses a local fixture API on port 8091. Tests cover search, late responses/errors from superseded searches (AC-17), full-budget conversion and unconfirmed price labels, empty results, server errors, permanent program URLs, comparison limits and shared links, external navigation, keyboard controls, 360 px layout, and serious/critical axe accessibility findings. Use `CHROMIUM_PATH` to select an already installed Chromium. A deployment build uses `npm run build` with its intended `API_URL`.

PostgreSQL integration tests require `TEST_DATABASE_URL` pointing to a dedicated database ending in `_test`; tests clear that test database. CI supplies an isolated PostgreSQL service.

## Production configuration and backups

### Prepare an Ubuntu VDS

Copy [`scripts/setup-vds.sh`](scripts/setup-vds.sh) to the server and run:

```sh
sudo bash setup-vds.sh
```

The script installs Git, curl, certificate tools, OpenSSL, jq, cron, and Docker
Engine with Compose when Docker is absent. It preserves an existing Docker
installation and checks that Compose is version 2.24 or newer. It enables cron
at boot and enables Docker when a system-level `docker.service` exists. For
Docker managed by another service manager, it preserves that setup; configure
startup at boot using that installation's service manager. The selected Docker
daemon must be accessible from the root deployment session. The script creates
`/srv/devcourse-finder` as the deployment directory.
Go, Node.js, and PostgreSQL run in the project's containers and do not need host
installations.

This prepares the host only. Repository checkout, production configuration,
HTTPS, application startup, catalog publication, and a daily backup job remain
deployment steps. The script does not install a reverse proxy or change firewall
or SSH settings. If an existing Docker installation lacks a compatible Compose
plugin, install or upgrade the plugin from that installation's package source
and rerun the script.

### Start the production application

Run all subsequent deployment operations, including repository checkout,
configuration, Compose commands, catalog operations, and backup scheduling,
from a root shell. The setup script does not grant the invoking user Docker
socket access or ownership of the deployment directory. If you used `sudo` for
setup, enter a root shell before proceeding:

```sh
sudo -i
```

If already logged in as root, keep using that session.

For an initial deployment after host preparation, copy
[`scripts/deploy-vds.sh`](scripts/deploy-vds.sh) to `/root/deploy-vds.sh` and run
it as root from outside the deployment directory:

```sh
cd /root
bash deploy-vds.sh
```

Keep the launcher outside `/srv/devcourse-finder`: a fresh checkout requires
that target directory to be absent or empty.

The script clones the public repository into `/srv/devcourse-finder` if it has
not been checked out, creates a protected production `.env` with a random
database password only when no `.env` or existing database volume is present,
builds and starts the stack, and checks the catalog API through the frontend.
It preserves existing configuration and does not update an existing checkout.
The frontend listens on `127.0.0.1:3000`; API and database ports remain private.
The default public origin is `https://alekslesik.fvds.ru`. To use another domain
when creating `.env`, run `SITE_URL=https://your-domain.example bash deploy-vds.sh`.
For an existing `.env`, edit `SITE_URL` there instead. The script validates the
effective frontend `SITE_URL` resolved by Compose before starting containers,
including values from a preserved `.env`. HTTPS configuration, daily
backups, and catalog publication remain separate steps.

For manual deployment, follow the steps below.

Place the repository in `/srv/devcourse-finder`, copy `.env.example` to `.env`,
and set a unique `POSTGRES_PASSWORD`, a stable `CATALOG_OPERATOR`, and the public
HTTPS `SITE_URL`. Protect `.env` with `chmod 600 .env` and keep it out of version
control. Configure the HTTPS reverse proxy to forward requests to the frontend;
for a proxy on the same host, set `WEB_PORT=127.0.0.1:3000` to bind the frontend
to loopback.

From the repository directory, build and start the application:

```sh
docker compose -f compose.yaml -f compose.production.yaml up --build -d
```

This follows the database → migrations → API → frontend startup sequence above.
Use `docker compose -f compose.yaml -f compose.production.yaml ps` to inspect
service status and add `logs --tail=100` instead of `ps` to inspect logs.

The catalog is initially empty. The prepared real catalog contains 20 programs,
but publication requires source review, validation, a backup, a reviewed dry
run, and an explicit import as described in the catalog publication runbook.
Schedule daily database backups separately; installing cron does not create a
backup job.

### Container publishing and backups

GitLab container build and registry publishing are configured in `.gitlab-ci.yml`.
See [GitLab CI setup](docs/gitlab-ci.md) for runner requirements and image tags.

Use `docker compose -f compose.yaml -f compose.production.yaml up --build -d` with an explicit `POSTGRES_PASSWORD`, `CATALOG_OPERATOR`, and HTTPS `SITE_URL`. The API refuses the demo password in production; only the frontend publishes a port. Docker Compose 2.24+ is required. Configure TLS at the deployment ingress.

Run `scripts/backup-database.sh` daily from cron as described in `docs/catalog-operations.md`. The script writes an atomic dump and retains seven days. A successful local restore test does not establish that production scheduling has been installed.

## Search performance

Run `./scripts/load/run.sh` for the MVP-NFR-04 load measurement: 10,000 published synthetic offers, 20 clients, 60-second warmup and 300-second measurement on an API/PostgreSQL stand limited to two shared CPUs and 4 GiB total memory. The script removes its disposable database on exit. See [the load instructions](scripts/load/README.md) and [recorded performance results](docs/search-performance.md). The manual `Search load test` workflow repeats this measurement.

## Browser acceptance with the real API

Run `./scripts/e2e-real.sh` after installing frontend dependencies and Playwright Chromium. It builds production frontend/API images and uses a disposable PostgreSQL database, then checks all eight MVP-QA-02 scenarios, AC-17 search races, pricing UI checks, real API outage/recovery and persisted analytics. See [the real-stack E2E instructions](scripts/e2e/README.md). The `real-e2e` CI job runs this suite for every PR.

## Import diagnostics

CLI failures exit with code 1 and write JSON diagnostics to stderr: command, run ID, stage and code, plus safe validation/SQL metadata. Tests invoke the production entry point in separate processes and confirm rollback and redaction with PostgreSQL. Raw driver/decoder errors and connection strings are omitted. See [the publication runbook](docs/catalog-operations.md) for interpreting the diagnostics.
