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
DATABASE_URL='postgres://devcourse:devcourse-local@localhost:5432/devcourse?sslmode=disable' \
  go run ./backend catalog import ./catalog.json --dry-run
```

Remove `--dry-run` after reviewing the output. Catalog imports are transactional and are not performed automatically when containers restart.

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
npm run typecheck
npm run build
```

The frontend proxies `/api/*` and `/out/*` to the API. Outside Docker, set `API_URL` when building the frontend if the API is not available at `http://api:8080`.

The CI workflow repeats the backend tests, frontend typecheck and production build, validates the Compose model, and builds both container images on every pull request.
