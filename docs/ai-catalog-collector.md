# AI course metadata collector

Optional separate `catalog-ai` container for verified Stepik, Practicum, CodeBasics and supported-contract Netology courses.
The native workers discover URLs and validate identities, prices and enrollment;
OpenAI (or automatic DeepSeek backup) extracts learning/profile facts from complete official documents. Newly
published courses become eligible automatically. No frontend or deployment trigger
changes are needed.

## Evidence and publication

The extraction model selects source document/segment IDs and structured values.
Our code copies the exact segment text, preventing invented or corrupted Unicode
quotes. An independent model checks whether that statement actually entails the
value in the surrounding context. Who a course suits is not a mandatory admission
requirement; course topics do not establish a graduation level. Recommended
knowledge, conditional later-stage support, automated checks and installment
periods cannot become mandatory prerequisites, mentorship or learning duration.

Default models: `gpt-4.1-mini` for extraction and `gpt-4.1` for verification. Both
are configurable. Verified source quotes, URLs, digests and dates survive in the
durable metadata queue/audit; the public facts expose source/evidence references.
Unknown or rejected claims are omitted. Individually invalid shapes, scopes or
unsupported numeric claims are discarded before independent review of the remaining
claims. Duplicate claims for the same field/tariff are both discarded. Wrong course
identity, incomplete replies and replies with no surviving claims still fail closed.
A partially discarded observation preserves all omitted prior facts, their original
evidence dates and AI/native/operator ownership; it can update only independently
verified surviving facts. Partial results are cached to avoid repeatedly paying for
the same unchanged text. The policy version changes, so old-policy caches are not
reused for new analyses. Invalid replies, refusals, truncation,
wrong identities, oversized inputs and source failures retain previous dates.

The existing publisher merges metadata and acknowledges its queue in one catalog
transaction. It creates no new identities and cannot change price, billing,
enrollment, price verification dates or the overall course-check date. AI-owned
fields have priority over incomplete native metadata reads; explicit operator
imports reset ownership and protect the imported facts. A successful complete
read can clear disappeared AI-owned fields. Prior verified quotes still present
in changed content are proposed again to the independent verifier, so extraction
omissions alone cannot silently remove them.

Experience uses a verified minimum plus explicit target level as a closed interval
when available, even when a primary typed audience mentions novices. Beginner
labels alone never expand to every higher profile. Curated legacy tags retain
literal semantics; switch remains an independent background.

## Cost and resource limits

- AI schedule: **Monday and Thursday at 21:00 Europe/Moscow**. Native API/page
  collectors retain their 09:00/21:00 daily schedule; publication continues normally.
  AI startup outside a scheduled slot waits for the next slot, with no immediate
  paid initial run or replay of missed days. A restart within 30 minutes of a slot
  may recover that slot if no AI run has started since it began. Restarts do not
  repeat a started slot. Manual `catalog-ai once` remains available on any day
  and still respects source due times and the shared daily request budget.
- At most 20 due courses per run (configurable 1–40), serial reads.
- At most **40 reserved model requests per UTC day** by default (configurable
  1–200), shared persistently across manual runs/restarts. A complete two-request analysis is reserved atomically before extraction.
  Failed analyses consume the reserved pair conservatively; one remaining request
  cannot start paid extraction. A limit of 1 permits cache checks only.
- Two requests per changed course: extraction and verification. An unchanged
  document uses cached results with no model request. Therefore 40 requests can
  analyze at most 20 changed courses per day; cached checks do not spend that quota.
- Cache identity includes provider, course, complete normalized text, scopes, model pair and
  policy. Raw transport changes alone do not invalidate unchanged text. Retain
  at most three analyses per course.
- Complete text budget: 96,000 UTF-8 bytes across course/tariff documents. Oversized
  input fails; it is not silently cropped. Maximum 40 claims and 4,096 completion
  tokens per request; 60s model timeout, 300s per course, 30m run limit.
- Container memory 128 MiB; `GOMEMLIMIT=96MiB`. No local model or browser process.
- OpenAI availability, authentication, quota, rate-limit, model or protocol errors
  switch to a configured DeepSeek backup. If the active backup fails, the run stops.
  Without a backup the run stops on those provider errors. `status` reports
  fixed error codes and counters, never provider error bodies or secrets.

The request cap is not a dollar cap. Actual cost depends on model pricing and
provider-reported token usage. Configure a project budget in the OpenAI dashboard.

## Automatic DeepSeek backup

OpenAI is always preferred; no provider selector exists. Add `DEEPSEEK_API_KEY`
to the protected server `.env` to enable the backup. Optional model overrides are
`DEEPSEEK_MODEL=deepseek-flash` and `DEEPSEEK_VERIFY_MODEL=deepseek-v4-pro`.
Defaults follow [DeepSeek Chat Completions](https://api-docs.deepseek.com/api/create-chat-completion/)
and [JSON Output](https://api-docs.deepseek.com/guides/json_mode/) documentation
checked on 2026-10-15. DeepSeek uses JSON mode with the same local fact validation;
it does not receive OpenAI's unsupported strict-schema request fields.

Fallback restarts extraction AND independent verification for the current course,
rather than verifying the failed provider's partial result. It remains active
for the rest of the run; the next scheduled/manual run tries OpenAI again.
Refusals, invalid/ungrounded claims, source-page failures, cancellation and our
own exhausted daily budget never trigger a second-provider attempt. Missing
OpenAI credentials can use the backup directly; at least one key is required.

The daily reservation cap is shared across BOTH providers. Each fresh provider
analysis reserves two requests before extraction. A failed OpenAI analysis plus
a fresh DeepSeek analysis reserves four slots, even if a failed request uses no
tokens. Backup cache hits need no new reservation. Both providers' reported token
usage is counted, including verification failures; tokens are not a dollar budget.
Cache entries and queued audit events identify the actual provider/model pair.
Existing pre-provider cache keys are not reused for new analyses.

No secrets or raw provider error bodies enter logs. A switch logs
`AI provider fallback from=openai to=deepseek code=openai_...`.
Course log lines include `model=deepseek:...`, `cached`, `tokens`,
`discarded_claims` (local validation failures) and `verified_claims` (independently
approved claims). A successful provider reply can still have zero verified claims.
DeepSeek receives compact per-field JSON contracts and explicit enum translations;
Russian source text need not literally contain internal English codes such as `none`.
No fallback can repair an unavailable official source page.
Native discovery, pricing and publication workers remain independent.

## Production activation

Merge and manually deploy the published release through the existing workflow.
The AI profile is disabled by default. Edit the protected server environment:

```sh
sudoedit /srv/devcourse-finder/.env
```

Add a **newly generated** API key and the following values; do not paste the key
into Git, issue/PR comments, command logs or chat replies:

```dotenv
COMPOSE_PROFILES=ai
OPENAI_API_KEY=replace-with-a-new-secret
OPENAI_MODEL=gpt-4.1-mini
OPENAI_VERIFY_MODEL=gpt-4.1
DEEPSEEK_API_KEY=replace-with-a-new-backup-secret
DEEPSEEK_MODEL=deepseek-flash
DEEPSEEK_VERIFY_MODEL=deepseek-v4-pro
CATALOG_AI_BATCH_SIZE=20
CATALOG_AI_DAILY_REQUEST_LIMIT=40
```

Keep `.env` mode 600. `COMPOSE_PROFILES=ai` makes future manual release deployments
retain this worker. To activate immediately after deploying the code:

```sh
cd /srv/devcourse-finder/current
sudo docker compose --project-name devcourse-finder \
  --env-file /srv/devcourse-finder/.env \
  -f compose.yaml -f compose.production.yaml --profile ai \
  up -d catalog-ai
```

Inspect status, trigger an eligible batch or check liveness:

```sh
# Use the same Compose prefix as above for each command.
docker compose --project-name devcourse-finder --env-file /srv/devcourse-finder/.env \
  -f compose.yaml -f compose.production.yaml --profile ai exec -T catalog-ai \
  devcourse-finder catalog-ai status

docker compose --project-name devcourse-finder --env-file /srv/devcourse-finder/.env \
  -f compose.yaml -f compose.production.yaml --profile ai exec -T catalog-ai \
  devcourse-finder catalog-ai once

docker compose --project-name devcourse-finder --env-file /srv/devcourse-finder/.env \
  -f compose.yaml -f compose.production.yaml --profile ai exec -T catalog-ai \
  devcourse-finder catalog-ai health
```

The publisher remains `catalog-publisher`. New queued metadata appears after it
consumes the queue. Manual runs respect existing due times and the persistent daily
budget; they do not force endless paid re-analysis.

## Pilot scope and limitations

Stepik uses its own public, active, non-censored Russian API course. Netology
requires the existing stored program-family/offer binding, fresh native
product/payment validation and matching course title/language. It sends only
the primary bound plan as an offer document; other plan cards are removed from
the course-level document. A binding digest is included in the cache identity
and rechecked transactionally before publication. Missing/changed mappings or
ambiguous card roots fail closed. Manually added offers get no inferred facts.
Netology currently consumes rendered HTML text, not arbitrary embedded app state.
Current registration-only Netology pages without those product/family and
plan bindings are rejected before model calls. Live coverage is not yet confirmed;
see the [source check and blocker](ai-provider-expansion.md#live-source-check--2026-10-15).
Other providers are tracked in [epic #133](https://github.com/alekslesik/devcourse-finder/issues/133).
Practicum
requires the existing canonical/SKU identity proof and isolates verified base/Plus
cards. JavaScript-only text absent from server HTML/API is not rendered in this
pilot. Additional offers without a verified binding receive no inferred support.

Live official source availability and semantic accuracy are separate checks. The
recorded-source OpenAI benchmark uses saved official fragments, at most four
requests and a five-minute deadline. It never imports or publishes its output.
Results are not a production-wide rescrape or a promise that every missing field
can be recovered. See the dated acceptance report for measured results.

## Verify the fallback after deployment

1. Add the backup key to the protected `.env`, retain the OpenAI settings, and run
   the activation command above again with `--no-deps --force-recreate` before
   `catalog-ai`, after the deployed database and migration services are healthy.
   A container restart alone does not reload changed environment variables.
2. Check `catalog-ai health`, then `catalog-ai status` using the Compose prefix
   above. Health proves worker liveness, not provider API access.
3. Run `catalog-ai once` for an eligible batch, or wait for the next schedule.
   It respects source due times and the shared budget. An empty batch is not an
   API test; an unavailable source does not contact either provider.
4. Read logs with the same prefix and `logs --tail=100 -f catalog-ai`.
   `model=openai:...` with nonzero tokens confirms a primary reply;
   `AI provider fallback` followed by `model=deepseek:... and code=queued` confirms
   backup analysis reached the queue. `cached=true` does not prove a fresh API call.
   `queued` can also contain zero accepted claims; it is not a promise that every
   public field changed.
5. Inspect `catalog-ai status` again. Token counters include both providers.
   The publisher's `verified_at` confirms acceptance of metadata into the catalog.
   With both providers unavailable, no new event is published and old facts remain.

Do not revoke working keys to test failover in production. Automated offline
transport/resolver/PostgreSQL tests simulate quota, authentication, rate limits,
network/5xx errors, failure during verification, separate caches, no budget bypass
and both-provider failure. Run `go test -race ./backend/...` with the existing
`TEST_DATABASE_URL` test database to repeat these acceptance scenarios without
paid API calls. No live DeepSeek accuracy or production failover is claimed by CI.

## Diagnosing `invalid_ai_response`

Nonzero tokens with `invalid_ai_response` means the provider was reached but the
response did not pass the pipeline. It does not mean the API key is missing.
For example, a claim of 8 months or 15 hours citing only a 36-payment price block
is not grounded. That claim must not be published. With policy v3 it is isolated
and logged in `discarded_claims`; surviving claims still need independent review.
A valid cached analysis can have `verified_claims=0`, and `queued` alone does not
prove enrichment. Check source availability separately before spending requests.
See the [DeepSeek diagnostic report](ai-deepseek-diagnostics-2026-10-15.md).

## Supported providers and staged expansion

`catalog-ai status` includes `supported_providers` and per-provider
`provider_coverage`. Readers now exist for all 13 native platforms: Stepik,
Practicum, CodeBasics, Netology, Skillbox, Skillfactory, PurpleSchool, HTML Academy,
Skypro, OTUS, Hexlet, JavaRush and RS School. Only independently verified source
contracts are eligible for paid requests; registration does not promise current
source availability, complete data or measured model accuracy. See the exact
[acceptance report and live-source blockers](ai-provider-expansion.md).

The nine new readers use the stored course/primary offer and native product/tier
mapping. They isolate public course sections and confirmed tariff features and
recheck that mapping in the publication transaction. Changed bindings invalidate
cache keys. Foreign/canonical-mismatched URLs, missing mappings, ambiguous scopes
and failed native contracts cause no model requests. Additional manual offers
receive no inferred conditions. No prices, billing or enrollment are published
through this path; failure keeps old facts and evidence dates.

Coverage counters distinguish eligible/mapped/attempted/ever-verified courses
and current source failures. They do not count filled facts or establish accuracy.
A prior verification can remain counted while the current source fails. Fixed
`source_*` details distinguish a native contract rejection from an API/model error.
Source-only diagnostic instructions and measured sizes are in the report.

The Netology pipeline is covered by recorded official plan fixtures with an
explicit synthetic course-profile section and mocked model responses. PostgreSQL
regressions cover independent extraction/review, scoped publication, no financial
freshness changes, cache replay, changed-family rejection before model calls and
changed-binding rejection after enqueue. This does not claim live model accuracy
or complete coverage of Netology's current HTML. No paid provider requests were
made to implement this stage. Budgets and Monday/Thursday scheduling are unchanged.

CodeBasics requires its stored course/primary-offer mapping, fresh anonymous
Inertia identity/curriculum validation and the independent official free-policy
page. It sends explicit public landing fields and curriculum descriptions, not
authentication state or lesson solutions. Its single course offer receives only
landing-level condition text. Total curriculum hours are not calendar duration
or weekly workload. A queued mapping change is rejected transactionally. The
current Python source-only check passed; no live semantic model benchmark was run.

## Manual full-catalog enrichment from GitHub Actions

Open **Actions → Enrich catalog AI → Run workflow**, select `main`, and use:

- `mode=enrich`: process due published courses in batches of 40, using the
  currently deployed image and the model keys already stored on the VDS.
- `daily_request_limit=200`: temporary shared UTC-day cap, including requests
  already reserved by other AI runs today. Choices are 40, 100 and 200. This is
  a request cap, not a dollar/token cap; extraction and independent review normally
  reserve two requests per course, and provider fallback can reserve extra pairs.
- `max_batches=10`: upper bound for this button press, from 1 through 25. The
  run stops earlier at budget exhaustion, no due candidates, provider failure,
  or the UTC budget boundary. A new batch is not started in the last 31 minutes
  before reset, allowing the existing 30-minute worker timeout to finish first.
- `mode=status`: inspect operational coverage and budget without model calls.

A single click is not a promise that all cards become complete. For example,
174 fresh courses may require more than the allowed 200 daily requests. Repeat
after the reset at **03:00 Europe/Moscow** to continue; persisted attempt ordering
prioritizes previously unattempted courses. Due times, failed-source cooldowns,
identity protections and the content/model/policy cache remain active. No SQL
reset, free-source inference or forced publication bypass is performed.

The workflow shares the deployment concurrency group and the host deployment
lock, preventing a release switch while processing. It creates an owned, bounded
one-off Compose container per batch with the deployed catalog image, `--no-deps`
and `--pull never`. Interrupt/timeout cleanup removes only that owned container.
The scheduled AI worker and native collectors keep running, and the native
publisher applies independently verified queued observations. Both AI and
publisher services must be running before a paid run starts. No build, migration,
restart, `.env` edit or permanent budget/schedule change is performed.

The Actions summary distinguishes attempted/queued/cached/failed courses, token
usage, source result codes, the stop reason and the remaining queue. `queued`
means awaiting publication, not necessarily that a card gained fields. Inspect
the fixed `source_*` errors and [current source limitations](ai-provider-expansion.md)
instead of treating a green bounded run as whole-catalog completion. Raw
provider errors, `.env` contents and model keys are not included in the report.

The workflow is dispatch-only. Publishing/merging a release does not trigger
paid enrichment. Operational validation uses `mode=status`; development/CI use
mocked Docker/model outputs and do not spend provider tokens.
