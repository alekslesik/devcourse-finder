# AI course metadata collector

Optional separate `catalog-ai` container for verified Stepik and Practicum courses.
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
Unknown or rejected claims are omitted. Invalid replies, refusals, truncation,
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

- Moscow schedule: 09:00 and 21:00; restarts do not repeat the current slot.
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
Course log lines include `model=deepseek:...`, `cached` and `tokens`.
No fallback can repair an unavailable Stepik/Practicum source page.
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

Stepik uses its own public, active, non-censored Russian API course. Practicum
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
   the activation command above again with `--force-recreate` before `catalog-ai`.
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
