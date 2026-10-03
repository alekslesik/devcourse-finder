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

MVP implementation is in progress. The repository currently contains:

- a Go REST API with course filtering, comparison, catalog import, and PostgreSQL storage;
- a Next.js interface for search, course details, and comparison;
- functional requirements and course-provider research;
- a Docker Compose development stack.

The catalog starts empty. Course records must be reviewed and imported explicitly; research data is not published automatically.

## Project documentation

- [Functional requirements](docs/functional-requirements.md)
- [MVP completion specification](docs/mvp-completion-spec.md)
- [MVP status by requirement ID](docs/mvp-readiness.md)
- [Catalog publication runbook](docs/catalog-operations.md)

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

## Local development

The Go workspace points to the backend module, so run repository-root checks with the explicit module path:

```sh
go test ./backend/...
go vet ./backend/...
```

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

From `frontend`, run `npm run build:test`, `npx playwright install --with-deps chromium`, then `npm run test:e2e`. The test build uses a local fixture API on port 8091. Tests cover search, empty results, server errors, permanent program URLs, comparison limits and shared links, external navigation, keyboard controls, 360 px layout, and serious/critical axe accessibility findings. Use `CHROMIUM_PATH` to select an already installed Chromium. A deployment build uses `npm run build` with its intended `API_URL`.

PostgreSQL integration tests require `TEST_DATABASE_URL` pointing to a dedicated database ending in `_test`; tests clear that test database. CI supplies an isolated PostgreSQL service.

## Production configuration and backups

Use `docker compose -f compose.yaml -f compose.production.yaml up --build -d` with an explicit `POSTGRES_PASSWORD`, `CATALOG_OPERATOR`, and HTTPS `SITE_URL`. The API refuses the demo password in production; only the frontend publishes a port. Docker Compose 2.24+ is required. Configure TLS at the deployment ingress.

Run `scripts/backup-database.sh` daily from cron as described in `docs/catalog-operations.md`. The script writes an atomic dump and retains seven days. A successful local restore test does not establish that production scheduling has been installed.
