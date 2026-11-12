# Advisory review quality: completeness epic #170

A read-only sample on 12 November 2026 inspected five real empty recommendation
analyses after the shared daily budget was exhausted. Extraction proposed useful
claims as well as incorrect or overly broad ones; the saved independent reviews
rejected every claim on four nonempty samples. The fifth sample genuinely lacks
substantive content. This is a diagnostic sample, not an accuracy estimate.

The source-derived text and a manual evaluation plan are in
`backend/updater/testdata/advisory-review/stepik-empty-cases.json`. Examples include
PyGame wrongly proposed as frontend alongside a supported basic-Python audience,
decorator practice requiring Python/OOP, and explicit JavaScript object exercises.
These demonstrate why one rejected direction must not suppress other fields and
why missing categories cannot be fixed by approving an unrelated label.

The existing direction selector gains `games` (Разработка игр), backed by the
same search enum, OpenAPI contract, UI evidence labels and advisory model schema.
It requires substantive game-development content, not merely playful exercises.
The ordinary price/offer/experience constraints and confirmed-versus-advisory
groups still apply. No current course is assigned this new tag by deployment.

Recommendation-only analysis now uses a dedicated independent review rubric with
complete audience, goal and direction semantics. It distinguishes learner entry
experience from course difficulty, web frontend from games, both sides of
fullstack, substantive QA from incidental tests, and practicing prior knowledge
from career promises. It permits grounded advisory interpretations while keeping
literal factual review unchanged. Source quotes, current bindings, independent
review, per-claim rejection, protected fields and manual holds remain mandatory.
The DeepSeek output instructions use the same isolated advisory contract; literal
claims are rejected before an advisory review request. The policy fingerprint
includes both prompts and output instructions, so an older cached rejection is
not silently treated as the new review.

Protocol/race/PostgreSQL tests establish request isolation and protection, not
semantic improvement. No paid replay or production recommendation edits were
performed for this change. After the next normal budget becomes available, run
one bounded sample through source, extraction, independent review and publication,
inspect accepted/rejected fields against full context, and record usage plus
protected-field digests in #170. Do not repeat paid calls until a desired answer
appears; an ambiguous or unsupported match remains unknown. The 200/day cap,
normal due times, provider fairness, caches and manual review holds are unchanged.
