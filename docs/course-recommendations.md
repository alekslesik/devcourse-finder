# Advisory course recommendations

Part of epic #170, recommendation classification #173 and decision-oriented
filters #174. Confirmed conditions and AI interpretations are different data.

## User behaviour

Search keeps its strict API default. `include_recommendations=true` adds courses
whose selected experience/goal/direction matches are supported by reviewed AI
interpretations, provided no selected profile field remains unknown. Confirmed
matches precede recommendations, which precede opt-in unknown candidates. All
price, enrollment, weekly workload, human-support and schedule constraints remain
hard requirements. A confirmed incompatibility always wins.

The website always includes reviewed AI recommendations, without a separate
setting or explanatory block in the form. Legacy website links with
`include_recommendations=false` are normalized to the same enabled behaviour.
The API retains its explicit opt-in parameter and strict default. Facets show
confirmed counts plus additional `+ N ИИ` counts. Results distinguish confirmed,
recommended and insufficient-data groups. Cards explain exactly which fields are
unknown and expose the official quote behind an interpretation. Detailed course
pages keep the advisory overview separate from confirmed admission requirements.
No self-reported confidence percentage or employment guarantee is shown.

The direction filter now also includes data/ML, algorithms and software testing.
These directions can be advisory classifications; existing confirmed direction
facts are not rewritten into inferred categories.

## Provenance and publication

Three new AI claim types are course-scoped: `recommended_audience`,
`recommended_goals` and `recommended_directions`. Each includes one or more
allowed labels and selects an exact source-document segment. Extraction can
interpret substantive curriculum, practice and taught skills for these fields
only. Title-only classification, incidental keywords, incompatible requirements,
unsupported audiences and career marketing alone must abstain. An independent
model reviews every label. Numeric and support facts retain strict grounding,
including the explicit weekly-hours unit rule.

The existing two-request analysis, shared daily request budget, due dates, cache,
source identity checks and publisher transaction still apply. Recommendations
are stored in separate columns, with exact quotations and per-field evidence,
and bound to the course/title/language/source/offer identity. Repricing does not
invalidate suitability; identity changes do. Native confirmed facts prune
contradictory or redundant estimates on both publication and reads, including
in detailed pages. Search estimates expire after 90 days. Manual operator imports
clear recommendations rather than accepting unreviewed AI-shaped input.

Policy v5 separates interpretations from factual claims. The new policy prevents
reuse of old analyses that did not request recommendation fields. It does not
increase the daily request cap. Logs and new run records distinguish
`factual_claims` and `recommendation_claims`; historical rows default to zero for
these new breakdown columns and are not retroactively reclassified. CLI status
adds per-provider `recommended_profile` counts and `unclassified_profile_courses`;
confirmed `profile_unknown` counts remain separate.

## Preview before publication

Use the deployed Compose prefix (production env file, both Compose files and AI
profile), then:

```sh
docker compose --env-file /srv/devcourse-finder/.env \
  -f compose.yaml -f compose.production.yaml --profile ai \
  exec -T catalog-ai devcourse-finder catalog-ai preview COURSE_ID
```

Preview fetches and validates the official source, extracts and independently
reviews proposals with the configured models, and reports exact reviewed quotes.
It shares the durable daily reservation cap and cache with ordinary enrichment.
It does not enqueue or publish, advance course attempt dates, change imports or
update public facts. It takes the same enrichment lock to avoid duplicate paid
analyses. A later ordinary verification can reuse the reviewed cache; changed
source text, model pair or policy requires a new bounded analysis.

Before broad rollout, preview a bounded diverse sample, inspect source/quote/
classification consistency and abstention, then publish only passing cases via
the existing workflow/CLI. Measure useful profile coverage, not merely queued
observations. The full 30–50-course semantic pilot and broader rollout remain
tracked in #172 and #175; availability of the feature is not catalog completeness.

Policy v6 adds a conservative local abstention guard: explicit statements that a
course does not start from zero block no-experience prerequisites and beginner
audience claims across the entire course document, even after model approval.
Other independently supported fields remain eligible. This covers known direct
Russian/English counterstatements; it is not a complete semantic contradiction
detector, so manual pilot review remains necessary.

Policy v7 gives every proposed claim an explicit `claim_index` and an
`evaluation_rule`: curriculum interpretation for recommendations, literal school
evidence for facts. Review decisions refer to claim indices, never quote segment
IDs. It clarifies field-specific abstention while retaining all local scope,
identity, contradiction, numeric and support safeguards.

## Recommendation-only enrichment

Production Compose defaults `CATALOG_AI_RECOMMENDATIONS_ONLY=true`. In this mode
only course-scoped curriculum documents and the three recommended_* fields are
sent to extraction. The independent verifier retains per-claim decisions and
all local identity/source/contradiction checks. Provider output, cache reads,
carry-forward and publication each reject literal facts in this lane. Cache model
identities have a `:recommendations` suffix, separate from mixed fact analysis.
Policy v8 includes the focused extraction contract.

Publication preserves every existing profile requirement, audience fact, outcome,
learning condition, support flag, price and timestamp. Unmentioned prior advice
is preserved until its normal expiry rather than erased by a stochastic abstention;
explicit counterevidence blocks retained zero-entry advice. Recommendations still
cannot override confirmed incompatibility. Targeted verification additionally
reports `facts_unchanged` using a digest of all public facts and private collector
ownership flags. Missing or false acceptance assertions stop the operator run.

`catalog-ai preview COURSE_ID` and `catalog-ai verify COURSE_ID` honour this mode;
preview still performs no catalog/queue/attempt writes. Set
`CATALOG_AI_RECOMMENDATIONS_ONLY=false` explicitly for the original strict factual
collector when needed. Mode selection never raises the shared daily cap. Batch
size supports 1–100 courses, and the daily cap remains 1–200 requests (default 40);
200 requests can reserve at most 100 extraction/review pairs, including failures.
