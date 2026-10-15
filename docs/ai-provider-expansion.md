# AI provider expansion

Epic: [#133](https://github.com/alekslesik/devcourse-finder/issues/133).
Native discovery currently supports 13 platforms. The AI worker originally
supported Stepik and Practicum; expansion must use explicit per-provider identity
and tariff contracts rather than treating every native source as model input.

| Task | Provider / scope | Stage |
| --- | --- | --- |
| #134 | Shared registry, canonical source eligibility and document guards | Implemented in this stage |
| #135 | Netology program-family and bound primary plan | Scoped reader implemented; current live template blocked |
| #136 | Skillbox | Planned |
| #137 | Skillfactory | Planned |
| #138 | PurpleSchool | Planned |
| #139 | HTML Academy | Planned |
| #140 | Skypro | Planned |
| #141 | OTUS | Planned |
| #142 | Hexlet | Planned |
| #143 | CodeBasics | Implemented; current source-only check passed |
| #144 | JavaRush | Planned |
| #145 | RS School | Planned |
| #146 | Whole-epic coverage/resource/operations acceptance | Pending remaining adapters |

This first combined PR contains separate commits for the registry, the scoped
Netology work and CodeBasics. Only #134 and #143 are closed by this stage.
The epic and remaining platform issues stay open. Each platform requires official
source identity checks, isolated course/tariff documents, independent quotation
verification, cache/budget preservation and PostgreSQL publication regression tests.

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
