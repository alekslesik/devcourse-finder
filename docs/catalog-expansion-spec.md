# Automatic catalog discovery and expansion

The v0.4 updater refreshes two configured courses. It does not yet find new ones.
A large catalog requires discovery as well as refresh; adding many unchecked
IDs to the current source file is not sufficient.

## Goal and scope

Build a useful initial catalog of **100 verified programs**, with Go, Python,
Java and JavaScript coverage, different experience levels, and both paid and
free offers. This is a target, not a claim about current coverage: failed or
ambiguous records never count toward it. Add sources incrementally and report
actual published totals by language and provider. Avoid filling the catalog with
near-duplicates or many fragments of the same program to inflate the count.

Data discovery, verification, publication and retries must operate in the
background without data PRs or operator approval. Changes to connector code
follow the normal code PR/release process. Deployment remains manual.

## Implementation tasks, in order

1. **Discover candidates from official public catalogs, feeds and sitemaps.**
   Start with Stepik and the existing CodeBasics/Hexlet, Yandex Practicum and OTUS
   sources. Fetch bounded pages twice daily, extract stable provider IDs and
   canonical course URLs, and persist a candidate queue with provenance. A
   search hit is a candidate, never a published course. Respect source access
   restrictions and rate limits; an unavailable discovery endpoint leaves other
   sources running. Anonymous Stepik single-course reads are already verified;
   its search/list endpoint must be independently verified before implementation.

2. **Create complete course records automatically.**
   Fetch each candidate's official detail/API data. Normalize title, provider,
   language, curriculum summary, price currency, full price and enrollment.
   Represent missing expertise/goal evidence as unknown rather than guessing
   that every course is for beginners or promises employment; update the record
   schema and filter behavior accordingly. Remove scripts/markup from source text,
   bound strings, and validate against the catalog contract before publication.
   An explicitly paid course can have an unknown full price, with a clear unknown
   price label; it must never be classified as free or match a confirmed budget.

3. **Deduplicate by provider identity and canonical URL.**
   Keep a persistent unique mapping from `(adapter, external_id)` to internal
   course/offer IDs. Reuse the existing Go/Python Stepik records when discovery
   encounters them. Title changes must not create another course. Separate a
   program from its tariffs and exclude exercises, previews, closed private
   courses and unrelated recommendations.

4. **Add verified multi-provider price/enrollment adapters.**
   Stepik is the first working adapter. Add CodeBasics and Hexlet next, then
   Yandex Practicum and OTUS after validating their actual public contracts.
   Each adapter must distinguish a full program from a free introductory module,
   and full payment from installment payments, starting prices and promotions.
   Missing prices do not become zero. A disappeared URL does not mean enrollment
   closed. Each enabled adapter needs recorded official response fixtures and
   rejection tests; unsupported markup stays unpublished.

5. **Schedule bounded discovery and refresh work.**
   Retain the 09:00/21:00 Europe/Moscow schedule. Persist work cursors, use small
   per-provider batches and capped concurrency/retries, and resume incomplete
   work after interruptions. Existing records must keep receiving refreshes while
   the discovery queue grows. Raise the current 100-source configuration limit
   only together with this bounded work design; do not exceed the run deadline.

6. **Publish and monitor automatically.**
   Reuse transactional merge/history, cache invalidation and anomaly confirmation.
   Candidates move automatically through pending, verified/published and rejected
   states. Rejected records store a safe reason and retry policy; there is no
   manual approval queue. Preserve curated edits and archived states. Report
   candidate/published/rejected counts, per-language/provider coverage, oldest
   verified price and consecutive source failures. Healthy worker liveness must
   remain distinct from successful collection.

## Acceptance criteria

- A clean production database gains verified courses after initial collection,
  and gains additional distinct courses in later runs without operator imports.
- All four target languages have measurable coverage; paid and free offers are
  counted from confirmed source evidence, not inferred from the absence of price.
- Re-running discovery and changing a course title create no duplicate records.
- Failed/challenge pages, unrelated course recommendations, unknown currencies,
  installment prices and incomplete identities cannot publish misleading offers.
- Unknown classification remains visible without false beginner/employment claims;
  specific filters exclude records without matching confirmed classification.
- One failing provider does not prevent another from discovering or refreshing
  courses. Restarts preserve queue progress and source limits.
- PostgreSQL integration tests cover discovery-to-publication, duplicate IDs,
  restart/resume, source failure isolation, protected records and rollback.
- Coverage numbers are reported from the database after a real collection; no
  placeholder or fictional course is counted as a published program.

## Current investigation

On October 8, the already-enabled Stepik detail endpoints remained the verified
integration contract. Anonymous `/api/courses?search=...` and `/api/search-results`
requests returned HTTP 403 from this environment. Discovery must therefore verify
an official usable catalog/feed/API route rather than assume these requests work.
No unverified bulk candidates were added to production configuration.
