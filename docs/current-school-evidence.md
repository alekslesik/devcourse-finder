# Current Hexlet and Skillfactory evidence

Follow-up for #133 and #175. The source excerpts were captured anonymously on
11 November 2026. Their companion JSON files record the complete response digest;
the committed HTML is an excerpt, not a replacement source or production import.

Hexlet's four published Java/Frontend and free Java/JavaScript pages now use
typographic course titles, course-specific FAQ, and a fixed `from=tilda_page`
payment marker. Three also publish Course JSON-LD. The additional reader checks
canonical identity, one H1 and description, language, the exact known suitability
questions, and the three own payment variants or the independently identified
free start link. When Course schema exists, its course/provider/free-or-paid
identity must agree; duplicate/foreign Course, Product and standalone Offer
objects fail closed. Only the selected FAQ answers and own description form the
partial course document. Paid-profession advertising on a free landing is excluded.

The existing independent CTA/popup/payment-chain tariff reader remains responsible
for the three paid offer documents. Unknown query parameters, foreign programs,
duplicate source markers and ambiguous cards do not gain a binding. No native
prices, enrollment conditions or financial dates are read from FAQ or changed by
this reader.

Recommendation-only mode removes offer documents before analysis. Publication now
recognizes the retained course document's independently recomputed tariff-bundle
digest. This exception is limited to recommendation-only events: a literal-fact
event with the same incomplete bundle remains invalid. PostgreSQL acceptance
checks publication and exact preservation of imported facts, offers and dates.

Skillfactory's independently bound Python-developer product renders four current
learning sections inside `tn-molecule` containers also used for pricing. The new
selection preserves those exact semantic sections after removing unsafe,
payment and related-course descendants. It runs only after the existing public
API product tag, saved product/tariff IDs, page identity and purchase hooks agree.
Its result is still a partial course document; no tariff support or workload is
inferred. A product-binding mismatch continues to reject the page.

Opt-in anonymous source checks use no model requests or production writes:

```sh
cd backend
AI_HEXLET_PUBLIC_SOURCE_CHECK=1 go test ./updater -run '^TestAIHexletPublicLiveSources$' -count=1 -v
AI_SKILLFACTORY_PUBLIC_SOURCE_CHECK=1 go test ./updater -run '^TestAISkillfactoryPublicLiveSource$' -count=1 -v
```

These checks use test records, not saved production mappings. After the exact
release is deployed, `catalog-ai sources` must additionally match each published
identity and saved binding. Any bounded paid verification and actual publication
are reported separately in #133; passing parser tests alone does not establish
model accuracy or complete the epic.
