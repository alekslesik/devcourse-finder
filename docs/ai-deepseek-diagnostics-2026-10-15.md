# DeepSeek diagnostics — 2026-10-15

Production reported 11 rejected Practicum analyses, 111,481 tokens and no queued
observations. OpenAI-to-DeepSeek failover worked; source extraction/validation did
not. These production counters are distinct from the bounded diagnostics below.

## Observed failures

A fresh official Practicum Go course page was retrieved successfully. Five bounded
read-only provider requests were made against its extracted public source text.
No diagnostic output was imported or published. Credentials were injected into an
ephemeral process; none are included in captured prompts, reports or the repository.

| Request | Purpose | Tokens | Result |
| --- | --- | ---: | --- |
| 1 | Original Flash extraction | 11,108 | Two numeric claims cited a price block without the claimed duration/workload digits; the entire analysis previously failed |
| 2 | Pro verification of surviving claims | 9,367 | No claims approved |
| 3 | Compact-contract Flash extraction | 9,334 | Five structurally grounded proposals survived; four invalid/scope-mismatched proposals were isolated |
| 4 | Pro verification of those five proposals | 9,011 | No claims approved |
| 5 | Read-only explanatory Pro review | 9,346 | Exposed excessive literal interpretation of English enum codes; also correctly rejected an unsupported project-level outcome |

Total: 48,166 diagnostic tokens. The final enum-translation instructions were
added after these calls and were not live-benchmarked. A provider returning valid
JSON is not evidence of useful facts or production-wide semantic accuracy. The
conservative empty verification results above are reported as empty, not success
at enrichment. Ordinary CI makes no paid provider requests.

## Fix and regression checks

- A malformed or ungrounded individual claim no longer poisons valid claims from
  the same observation. All-invalid output, wrong identities and incomplete
  responses still fail closed. Conflicting duplicate field/tariff claims are both
  discarded. Remaining indices are independently verified after filtering.
- Partial observations retain omitted old facts, dates and ownership. Verified
  surviving facts can refresh normally. Fully complete observations retain the
  previous disappearance semantics. Prices and enrollment remain native-only.
- Compact DeepSeek instructions list each value contract once, distinguish
  installment terms from study duration and explain internal enum translation.
  No unsupported claim is approved locally to compensate for model abstention.
- Logs expose discarded and independently verified claim counts. Partial cache
  replay preserves the partial flag and spends no further model requests. Policy
  v3 prevents reuse of older prompt/validation cache entries.

Regression coverage includes source-scope numeric failures, duplicates, identity
protection, forged provider partial metadata, partial cache replay and PostgreSQL
publication preserving evidence freshness/ownership while updating verified facts.

## Production acceptance after deployment

Use the normal protected Compose environment and inspect `catalog-ai status`
first. Previous failed courses respect their retry time; deployment does not reset
source backoff or the shared daily request budget. Stepik must become reachable
before its AI reads can work. A manually paused Stepik source remains paused until
its recorded due time.

For a bounded eligible-course test, stop the scheduled AI worker and use a single
one-off container with `CATALOG_AI_BATCH_SIZE=1`; then restart the worker. This
respects the same database lock, due times and daily budget. Inspect the per-course
`code`, `discarded_claims` and `verified_claims`, then publisher acceptance. A
cached result or empty batch does not prove a fresh provider call. Do not reset
budgets or repeat full batches to manufacture a nonempty result.
