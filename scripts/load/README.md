# Search load test (MVP-NFR-04)

Run from any directory with Docker Compose, Python 3 and curl installed:

```sh
./scripts/load/run.sh
```

The script builds the production API image, creates a disposable PostgreSQL 17
volume, validates and imports exactly 10,000 synthetic published offers (one per
course), then runs 20 concurrent HTTP clients. The default warmup is 60 seconds;
measurement is 300 seconds. It removes only its own Compose project and volume
on exit. Never point this fixture at a production database.

Both API and PostgreSQL share CPU IDs `0,1`: they can use at most two CPUs in
aggregate. Memory limits total 4 GiB (API 1 GiB, PostgreSQL 3 GiB), with no extra
swap. Set `LOAD_CPUSET` to two available CPU IDs on another host. This is a
resource-limited container stand, not a dedicated 2-vCPU VM; unrelated host load
can affect results. The client runs outside these limits. The frontend proxy,
TLS, internet latency and analytics requests are outside this search measurement.

The workload rotates 12 query shapes across clients: broad search, each language,
all sorting modes, free/paid, support/hours, schedule/closed offers, combined
filters, pagination and a narrow budget. Clients use keep-alive and issue the
next request after consuming the previous response. This is closed-loop load;
it establishes performance at 20 concurrent clients, not an arrival-rate SLA.
Latency includes the complete HTTP response body; JSON parsing happens before
the next request. Percentiles use nearest rank over all successful measured
requests, including requests started before the deadline and completed after it.
Any HTTP, network or payload error fails the test; p95 must also be at most 500 ms.
Warmup results are recorded separately and excluded from measured percentiles.

Results are saved under `.load-results/` (gitignored): `report.json`, import log,
SQL offer count, base commit, container metadata, API log and final resource
snapshot. Container metadata can include inherited runtime proxy configuration;
do not publish it without review. Commit only the sanitized report and summary.

For diagnostics, override `LOAD_WARMUP`, `LOAD_DURATION`, `LOAD_PORT` or
`LOAD_REPORT_DIR`. Short runs are not acceptance results. Build steps preserve
Docker proxy configuration; if `CODEX_PROXY_CERT` is set, its CA is passed as a
BuildKit secret to the backend Dockerfile. A nonzero exit code means the latency
or error target failed, or the stand could not be prepared.
