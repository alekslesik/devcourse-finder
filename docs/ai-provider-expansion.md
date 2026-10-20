# AI provider expansion

Epic: [#133](https://github.com/alekslesik/devcourse-finder/issues/133).
Native discovery currently supports 13 platforms. The AI worker originally
supported Stepik and Practicum; expansion must use explicit per-provider identity
and tariff contracts rather than treating every native source as model input.

| Task | Provider / scope | Stage |
| --- | --- | --- |
| #134 | Shared registry, canonical source eligibility and document guards | Shipped in v0.26.0 |
| #135 | Netology program-family and bound primary plan | Scoped reader implemented; current live template blocked |
| #136 | Skillbox | Implemented with scoped fixture and PostgreSQL regressions |
| #137 | Skillfactory | Implemented with scoped fixture and PostgreSQL regressions |
| #138 | PurpleSchool | Implemented with scoped fixture and PostgreSQL regressions |
| #139 | HTML Academy | Implemented with scoped fixture and PostgreSQL regressions |
| #140 | Skypro | Implemented with scoped fixture and PostgreSQL regressions |
| #141 | OTUS | Implemented with scoped fixture and PostgreSQL regressions |
| #142 | Hexlet | Implemented with scoped fixture and PostgreSQL regressions |
| #143 | CodeBasics | Implemented; current source-only check passed |
| #144 | JavaRush | Implemented with scoped fixture and PostgreSQL regressions |
| #145 | RS School | Implemented with scoped fixture and PostgreSQL regressions |
| #146 | Whole-epic coverage/resource/operations acceptance | Automated checks implemented; live-source blockers remain |

The remaining work is delivered in one combined PR, with one commit per issue
(#135–142 and #144–146). #134 and #143 shipped in the prior release. Netology
#135 remains open because its current live template cannot prove product identity;
whole-epic live acceptance #146 remains open for that and other rejected sources.
Implemented readers do not imply that every current page passes the contract.

The native collectors still own discovery, prices, billing and enrollment. AI
only enriches existing published identities. Paid analysis remains scheduled on
Monday and Thursday at 21:00 Europe/Moscow, with the existing shared daily request
cap and unchanged manual-trigger semantics. Adding a provider increases the set
of eligible courses, not the configured run size or request limit.

Netology reuses its native payment/program contract, validates the saved family,
primary offer, title and language, isolates the bound plan and excludes other plan
cards from the course document. The current binding is checked again inside the
publisher transaction. It does not send every embedded JSON field to the model,
bind operator-created extra plans or promise recovery of every missing fact.

The recorded-source and PostgreSQL tests use fake extraction/review outputs;
they prove pipeline and protection behavior, not live semantic accuracy. No paid
API requests or production re-analysis were performed during implementation.

## Live source check — 2026-10-15

Anonymous requests exposed a canonical transport issue: Netology redirects
`/programs/go` to `/programs/go/`. The fixed provider endpoint now includes the
trailing slash directly; redirects remain forbidden and the stored canonical
identity is unchanged.

After that fix, Go, Java and Python landing pages returned HTTP 200 but their
`__NEXT_DATA__` contained only `landingContent` and route query data, with
`programRegistration.isBreedingPrograms=true`. The prior product/family IDs,
active/loaded flags, `plansNew_*` bindings and rendered one-payment cards were
absent. The native identity/payment contract correctly rejected this format as
`invalid_netology_payload`. The AI reader does not guess missing IDs or attach
an overview/multiple-program landing to a specific tariff.

Thus #135 remains open: its recorded-contract reader, cache and publication
protections are implemented, but the current registration-only live template
needs a separately verified official identity/tariff source before live coverage
can be claimed. No paid API request was made by these source checks. A registered
provider in status means a supported source contract exists, not that every
current landing passes it. The remaining-provider issues and whole-epic
acceptance stay open.

Repeat the source-only test with
`AI_NETOLOGY_SOURCE_CHECK=1 go test ./backend/updater -run '^TestAINetologyLiveSource$' -v`.
It makes no model calls and performs no database writes. A source-contract error
is an actual failed acceptance check, not a successful semantic benchmark.

## CodeBasics source and acceptance — 2026-10-15

The current official Python page and independent `/ru` free-policy page returned
HTTP 200. The existing native Inertia contract verified course/landing IDs,
published/built flags, matching Russian curriculum and an accessible first lesson.
The new reader prepared 13,968 bytes of course text (9 source segments) and
229 bytes for the single bound full-course offer (1 segment). This is a source
preparation result, not a paid model accuracy measurement.

Only explicit public landing descriptions, usage/outcome descriptions and course
module/lesson names/descriptions enter the course document. Auth/contact state,
other landings, source/test code and solutions are excluded. Offer input contains
only that landing's header, description and original duration wording; total
curriculum hours are not converted into weeks or hours-per-week. Operator-added
offers are not enriched. A fresh native free-policy check is required before
cache lookup/model reservation; AI never changes the free price or its date.

PostgreSQL tests cover extraction/review → queue → publisher, cached recovery,
queued source-remap rejection, financial freshness and operator-offer protection.
The extraction/review responses in these tests are mocks. No paid API calls were
made. Repeat the optional anonymous source-only check with
`AI_CODEBASICS_SOURCE_CHECK=1 go test ./backend/updater -run '^TestAICodeBasicsLiveSource$' -v`.

### Netology follow-up — 2026-10-16

The saved current program ID is now required as well as its family ID. Both
participate in the cache binding and are rechecked inside publication. Recorded
and PostgreSQL regressions reject a different program in the same family and
the registration-only multi-program template. The anonymous Go source check
still failed with `invalid_netology_payload`; #135 remains blocked, with zero
model requests spent. This does not justify guessing a numeric identity.

## Remaining adapters and source audit — 2026-10-16

The nine additional readers reapply native source contracts, compare the saved
course/offer/product/tier bindings, isolate selected public program sections and
individual tariff features, and recheck the same mapping in the publication
transaction. The binding is part of the cache key. Unbound operator offers are
excluded. A source/identity/scope rejection happens before any model reservation.

| Provider | Source-only result | Input size | Scope / limitation |
| --- | --- | --- | --- |
| Skillbox | Documents ready | 16,857 bytes / 4 documents | Course curriculum/outcomes; each nomenclature's own features and common course statistics |
| Skillfactory | Native contract rejected | 0 | Canonical page and course-specific pricing GET must both pass; lead popups never establish support |
| PurpleSchool | Documents ready | 957 bytes / 2 documents | Typed public Flight description/requirements and each verified tariff's features; exact workload FAQ |
| HTML Academy | Documents ready | 1,865 bytes / 2 documents | Semantic course sections and individual-format section after independent payment verification |
| Skypro | Documents ready | 4,208 bytes / 2 documents | Target-product pricing/support and course hero; school-wide promises excluded |
| OTUS | Documents ready | 2,336 bytes / 2 documents | Unique accepted Course/Offer, explicit course description and learning-format block |
| Hexlet | Native structured payload rejected | 0 | Observed JSON-LD contains an invalid control character in a string; do not repair or guess identity |
| JavaRush | Documents ready | 882 bytes / 2 documents | Java Premium's selected course information; cost/period, University and Mentor Pro excluded |
| RS School | Documents ready | 1,014 bytes / 2 documents | Independently checked Russian course/cohort and explicit communication; conditional later-stage lengths/benefits excluded |

These are nine single-course anonymous source preparation checks. They do not
use the production catalog binding, prove coverage of all course pages, perform
paid semantic evaluation, or publish anything. Seven samples prepared documents;
two failed. Netology's separate source test also failed. All recorded-contract,
cache, queue, publication, shared-budget and failure-isolation tests use mocks.
No paid model requests were made during development. The native parsers continue
to operate independently when AI is disabled, unavailable or out of budget.

Skillbox and Skypro input now preserves subordinate headings inside a uniquely
selected semantic block. A same-or-higher heading ends that block. Recommendations,
forms, portal purchase cards, auth state and executable scripts are removed before
selection. Missing/ambiguous sections are omitted, never replaced by the whole
page. JavaRush's Premium information is scoped to its one verified Java course;
a monthly subscription is not duration. Hexlet requires an exact named single
Offer before DOM tier conditions are sent; aggregate/minimum prices do not bind
subscription benefits. ISO calendar duration is retained verbatim, with no
conversion of video hours, billing intervals or registration dates into study time.

### Repeatable checks

With a disposable PostgreSQL database in `TEST_DATABASE_URL`:

```bash
go test -race ./backend/...
go vet ./backend/...
```

An explicit anonymous diagnostic (no model keys/database required):

```bash
AI_EXPANSION_SOURCE_CHECK=1 go test ./backend/updater -run '^TestAIExpansionLiveSources$' -v
```

The diagnostic deliberately returns failure for a rejected sample; that does not
mean ordinary deterministic CI failed. Optional `AI_SOURCE_PROVIDER=skillbox`
limits it to one provider. `AI_SOURCE_REPORT_PATH` saves a JSON report. Full
acceptance is pending the rejected live sources; do not relabel these as success.

`catalog-ai status` now reports `provider_coverage`: canonical eligible published
courses, existing catalog mappings, attempted courses, courses ever verified, and
current source failures. A mapping count only proves a mapping row exists;
readers still verify its native product/tier contract. `ever_verified_courses`
may include prior successes whose current attempt fails; none of these counters
measures fact completeness or model accuracy. Source rejection details use fixed
`source_*` codes without logging provider bodies, credentials or raw errors.

### Netology public curriculum — 2026-10-20 (#135)

The current registration/curriculum template has no numeric checkout identity.
A separate **partial course-only** reader now checks canonical URL, Russian H1,
language, query slug, first registration slug and uniquely rendered typed module
blocks against the existing catalog binding. Contradictory legacy product state
is rejected. Only public descriptions and module/lesson descriptions are sent;
registration variants, lead forms and payment conditions are excluded. This does
not refresh or assert a current numeric checkout product, price or tariff.

An anonymous live Go check prepared 12,262 bytes of course text on October 20.
Repeat it with `AI_NETOLOGY_PUBLIC_SOURCE_CHECK=1 go test ./backend/updater -run
'^TestAINetologyPublicLiveSource$' -v`. It uses a saved-binding fixture, not a
production database or a paid model. The projected fixture records provenance
alongside its HTML. Its DOM shells/description are synthetic; curriculum fields
are projected from the public response.

PostgreSQL tests exercise a mocked extraction/review, remap rejection, cached
recovery and publication without changing any offer or financial freshness.
Partial course input preserves previously verified facts omitted by the selected
sections. Fully observed documents retain their existing disappearance behavior.
Numeric product and tariff live acceptance in #135 remains outstanding; course
profile readiness must not be reported as tariff verification.

### Skillfactory / Hexlet public profiles — 2026-10-20 (#133)

Current public templates are now handled separately from native price refreshes.
Skillfactory's partial course document requires its canonical URL, matching
published/H1/API title (only controlled course/profession prefixes and the old
"from scratch" suffix differ), language, API product tag, saved product/tier IDs
and all three official purchase-popup hooks. An expired promotion cannot make a
metadata description unreadable, nor can this path renew a price or enrollment.
The existing native financial contract remains unchanged.

Hexlet's Java/frontend landings currently expose FAQ JSON-LD, not Course/Product.
A partial profile can use the matching canonical/H1/language and the three
explicit course-specific payment variants. Links are read, never followed to
purchase. No numeric/tariff identity is invented. A separate digest binds the
published course identity and the presence/value of any native mapping; changing
or adding a mapping before publication rejects the event. Invalid/contradictory
structured product data cannot use this fallback. Free landings without these
independent program links remain rejected, rather than guessing their identity.

Anonymous current-source checks prepared Skillfactory 275 bytes, Hexlet Java
151 bytes and Hexlet frontend 210 bytes. These small samples are scoped public
text, not complete-page coverage or proof of useful AI claims. Repeat with
`AI_SKILLFACTORY_PUBLIC_SOURCE_CHECK=1` / `AI_HEXLET_PUBLIC_SOURCE_CHECK=1` and
`go test ./backend/updater -run 'TestAI(Skillfactory|Hexlet)PublicLive' -v`.
The earlier native audit remains a separate diagnostic and can still reject a
source; these modes must not be reported as native financial verification.

Positive/negative fixtures and PostgreSQL tests cover mocked extraction/review,
cache reuse, mapping changes, publication and unchanged offers/financial dates.
No paid model calls or production catalog writes were made. #133 remains open
for Netology's specific current tariff, Hexlet free/individual tariff scope and
a limited end-to-end paid validation; this PR does not erase
those outstanding acceptance criteria.

### Bounded source checks and free Hexlet profiles — 2026-10-20

The free Java and JavaScript landings now have separate course-only identity
contracts: exact canonical URL, one matching H1, language, and official start
links. JavaScript explicitly uses the captured `js-basics-free/goto` alias;
this is not a general redirect/alias rule. Conflicting start links, payment
variants and contradictory structured product data are rejected. Captured
projections and response digests are retained next to the fixtures. Only the
description and selected course sections are sent to the model. Paid profession
promotions and their duration/career benefits are not free-course evidence.

The anonymous live checks prepared Java 151, frontend 210, free Java 291 and free
JavaScript 286 bytes of partial course text. Netology prepared 12,262 bytes of
curriculum text. Skillfactory prepared 275 bytes on a successful retry after an
initial source-read timeout. These are source-readiness measurements, not verified model
claims, refreshed tariffs or complete-source coverage.

`catalog-ai sources netology-go,hexlet-java-free` reads at most eight explicitly
selected published courses using current database bindings and the production
readers. It reports document counts, scope, partial status and text sizes, with
zero model calls and no database writes. `partial_course_only_tariff_unverified`
means exactly that: a usable course profile does not independently verify a
checkout product or tariff. Invalid selections are rejected before remote reads.

The **Enrich catalog AI** workflow supports `mode=sources` with `course_ids` for
this read-only check. For limited real acceptance, select `mode=enrich`, a short
explicit `course_ids` list and the normal shared daily request cap. The helper
forces one batch for selected runs. Existing cooldowns, cache, UTC budget and
OpenAI → DeepSeek fallback remain active. No force-due updates or financial
publication are introduced. Target selection is rejected in the scheduled
`serve` command, so it cannot silently narrow the permanent worker. Omitting
`course_ids` retains the existing due-course backfill behavior.

Regression coverage includes free/paid promotion isolation, conflicting start
identities, remapping after enqueue, cached publication without offer/financial
changes, exclusion of unselected courses, cooldown and shared-budget retention,
and source-check database immutability. PostgreSQL model tests use mocks.
Local validation: the complete `go test -race ./backend/...` suite against an
isolated PostgreSQL 17 database, `go vet ./backend/...`, 15 manual-helper tests,
four release-helper tests and the release metadata check passed.

Still required to close #135/#133: independent current numeric product and
specific tariff identity for Netology; verified individual tariff conditions
where unavailable; and recorded limited live extraction/review/publication
acceptance. The current Netology registration-only source does not supply the
numeric identity, and code must not manufacture it. Neither issue is closed by
the source checks or the new free-course profile support.
