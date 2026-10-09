# RS School adapter

Issue #41 adds the fixed official https://rs.school/courses catalog to the page
collector. It discovers supported JavaScript/frontend framework course routes,
not blogs, mentorship recruitment pages or generic learning materials. Discovery
never publishes records by itself. The frontend is unchanged.

## Verification contract

Each detail requires one exact canonical URL and one scoped course hero. A fresh
independent official catalog read must contain exactly one matching course card.
Title, declared cohort language, start and enrollment cutoff must agree between
both responses. Date attributes and rendered date text must agree. Unknown/TBD
or contradictory dates cannot establish enrollment. English-only cohorts are
rejected for the current Russian catalog, even if their site navigation mentions
Russian courses. An English interface does not override a Russian cohort label.

An available badge does not override an expired enrollment cutoff. The scoped
registration action must point to the fixed official event host and match the
program and start year/quarter. URLs are compared rather than followed. New
records require open enrollment; verified closed observations can refresh existing
courses, subject to the existing later-window closure confirmation.

The course's own Free education feature must explicitly say the whole training
is completely free. A generic site slogan, donation link, free material or sample
lesson cannot establish zero full price. The linked curriculum must belong to the
exact official rolling-scopes-school repository contract for that course.

Linked repositories are learning-material evidence, not enrollment calendars.
The JavaScript page currently links an English README with 2024 training weeks;
those archived dates do not establish a current cohort. Current enrollment uses
the matching detail and catalog dates, not repository age, a past signup link or
render timestamps. The external event service returned HTTP 403 during research;
its response is not used as evidence or bypassed.

## Prerequisites, workload and support

Course training-program, prerequisite and stage text is normalized into the
summary. Partial stage durations are not summed into a guessed whole-program
workload. Peer cross-checking is not personal mentoring. Later stages describe
mentoring conditional on passing earlier work/interviews; the whole offer does
not promise unconditional review/mentor support. Existing classification and
operator edits retain the normal identity/publication safeguards.

Both response hashes contribute to the evidence digest. The independent catalog
is fetched once per detail batch, reserving a second request budget. A failed
catalog fetch prevents every dependent detail from publishing. Price evidence has
a 26-hour freshness lease. The rolling lease is excluded from closure confirmation
fingerprints, so two matching scheduled reads can confirm closure. Failed reads
still interrupt confirmation in both split-worker and legacy modes.

## Recorded coverage and release

On 2026-10-09 the JavaScript cohort declared Russian/English instruction and free
full training, but enrollment had ended on September 27. It produces a closed
observation and does not become a new published course. Pre-school was TBD and
React was English-only; both are rejected. The adapter therefore does not add
these historical/planned cohorts to the public catalog now. A future verified
open Russian cohort can be published by the scheduled collector automatically.

Tests cover recorded open/closed dates, TBD, English-only cohorts, contradictory
catalog evidence, changed free wording, registration identity, queue/publication
separation and closure confirmation across refreshed leases. Optional live check:

```sh
RSSCHOOL_LIVE_CHECK=1 go test ./backend/updater -run TestRSSchoolLiveVerification -v
```

The live JavaScript/detail-catalog check also passed in a non-root, read-only
container limited to 128 MiB and 0.5 CPU. Default tests use reduced public fixtures.
Deploy manually through **Deploy
release** after merge and release publication; neither event deploys the VDS.
