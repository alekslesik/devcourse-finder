# OpenAI catalog pilot

Epic [#102](https://github.com/alekslesik/devcourse-finder/issues/102).
One combined PR, one commit per task (#103–106), version 0.23.0.

1. Strict OpenAI extraction and independent semantic verification. Every field
   carries an exact official quote; source URLs, course IDs and tariff IDs come
   from trusted fetchers. Unknown is omitted; no estimated workload or invented
   support. Structured JSON schema, refusal/truncation handling and bounded output.
2. Stepik API and course/tariff-bound Practicum documents. Complete bounded inputs,
   no silent truncation; official discovery remains the source of URLs. Durable
   cache keyed by course, document contents, model and policy. Persistently bounded
   daily API requests, including failures; unchanged content uses no model calls.
3. A metadata-only durable enrichment queue consumed by the existing publisher.
   No new courses, identity changes, prices, billing, enrollment, price dates or
   whole-course dates from AI. Fresh profile and learning facts update existing
   identities automatically. Per-field AI priority prevents a later native read
   erasing AI facts; operator facts retain priority. Failure retains facts/dates;
   missing successful facts clear only AI ownership. Atomic acknowledgement/rollback.
4. Independent optional background worker, existing Moscow twice-daily slots,
   manual trigger, health/status, Compose integration and explicit secret setup.
   PostgreSQL/HTTP regressions and a bounded reproducible pilot comparison report.

OpenAI is selected by the maintainer. `OPENAI_API_KEY` is supplied only through
secret environment configuration; it must not appear in Git, prompts or logs.
Extraction: `gpt-4.1-mini` (`OPENAI_MODEL`); independent verification: `gpt-4.1` (`OPENAI_VERIFY_MODEL`). Two model requests
per changed course (extraction and independent entailment check); no web-search
billing or autonomous model browsing. Source discovery and financial publication
retain the existing official-feed and identity/price gates.

The pilot enriches verified published Stepik/Practicum courses, including newly
published courses found by existing background discovery. It does not claim to
replace financial contracts or create arbitrary model-suggested courses. JavaScript
content must be supplied by verified server payloads/visible HTML; missing or
oversized complete content is a failed read, never a cropped successful refresh.
