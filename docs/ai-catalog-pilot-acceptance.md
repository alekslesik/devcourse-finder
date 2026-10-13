# OpenAI collector acceptance — 2026-10-12

Epic #102, tasks #103–106, one commit per task. Version 0.23.0.

## Semantic experiment

Two reduced fragments captured from official sources, not current full-course
reads. No database imports or publication. Final configuration: `gpt-4.1-mini`
extraction, independent `gpt-4.1` verification, source-segment quote selection.
Four API requests in the final run; 6,290 provider-reported tokens, 5.23 seconds.
See [the captured-source JSON report](ai-catalog-openai-benchmark-2026-10-12.json).

| Captured source | Native profile facts | Independently verified AI facts | Accepted fields |
| --- | ---: | ---: | --- |
| Stepik Golang API fragment | 1 | 1 | Direction: basics |
| Practicum Python FAQ fragment | 0 | 3 | No mandatory experience, beginner audience, backend direction |

The native baseline uses the same reduced fragment through existing profile
extraction. It is not the number of facts currently published in production.
No promise is made that all 89 Python courses gain complete profiles.

Early trials exposed two problems: generated quote punctuation could differ from
source text, and a same-model reviewer could accept audience descriptions as
mandatory prerequisites or curriculum topics as outcome levels. The final protocol
copies exact quotes from selected source IDs, uses a separate checking model and
adds explicit field-specific entailment rules. In the final run the reviewer
rejected the Stepik mandatory-basics/target-level inference and the Practicum
project-level inference. The collector remains conservative; AI is not a guarantee
that every interpretation is correct.

Fresh official Stepik/Practicum requests during this work failed. Therefore this
experiment measures interpretation of saved public wording, not current price,
enrollment, complete-source coverage or a live production backfill. Only current
identity-checked fetches can enter the production enrichment queue.

## Regression coverage

- Strict OpenAI JSON schema, fixed API endpoint, refusal/truncation/malformed replies,
  exact source-segment grounding, numeric tokens without rounding/conversion and
  independent rejection of unsupported interpretations.
- Public active non-censored Stepik identity checks; Practicum base/Plus source
  isolation; incomplete/oversized source failures without silent truncation.
- PostgreSQL cache reuse with no new requests, persistent reservation limit,
  failure accounting, changed-content invalidation and token counters.
- Existing-identity metadata update, no price/enrollment/date changes, AI/native
  per-field ownership, explicit operator protection, independent watermarks,
  poisoned queue rows, protected changed identities and atomic rollback.
- End-to-end worker to durable queue to existing publisher, unchanged-content
  replay, failed-source date preservation, status/health and demo exclusion.
- Full Go suite with PostgreSQL and race detection; `go vet`; Compose configuration
  with AI disabled/enabled and unchanged private production ports.

The application image built successfully. A separate read-only test container
passed source-processing and PostgreSQL pipeline checks under a 128 MiB hard
limit with `GOMEMLIMIT=96MiB`; exit code 0, no OOM. The HTML-processing test
reported 29,120 KiB peak resident memory while discarding a 3.6 MB script.
This is a bounded fixture measurement, not a production load benchmark.

Ordinary CI never calls OpenAI. Live tests require explicit opt-in and key injection.
The API key used for manual verification was provided only to an ephemeral process;
it is absent from the repository, report, prompts and source files. A key disclosed
in chat should be revoked and replaced before production activation.

## Repeat the read-only benchmark

```sh
OPENAI_LIVE_CHECK=1 \
OPENAI_MODEL=gpt-4.1-mini \
OPENAI_VERIFY_MODEL=gpt-4.1 \
OPENAI_BENCHMARK_REPORT_PATH=/tmp/openai-benchmark.json \
GOMEMLIMIT=96MiB \
go test ./backend/updater -run '^TestOpenAIRecordedSourceBenchmark$' -v -count=1 -timeout 6m
```

Supply `OPENAI_API_KEY` through protected secret environment configuration. This
benchmark makes at most four requests per run and never publishes results. See
[operations and activation](ai-catalog-collector.md) for the background worker.
