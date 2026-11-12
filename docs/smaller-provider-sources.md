# Smaller providers: verified free programs

This integration adds CodeChick and LearnPython.org alongside the existing
providers. Discovery uses their own public index pages and the standard bounded
provider lanes, durable observations, identity binding and publisher. It does not
import a seed course directly into production or add a school whitelist to the
catalog ranking.

CodeChick's initial verified course is `https://codechick.io/paths/python-s-nulya`.
Its Russian canonical page must agree with its single Course JSON-LD record,
provider, zero-RUB Free Offer, visible free policy, course-specific FAQ promising
that all lessons/exercises/exam are free, curriculum and local course start link.
An introductory free lesson or missing price is insufficient. The explicit
course audience supports beginner suitability with evidence. Automatic exercise
checks do not establish teacher feedback or mentorship. The roughly 14-hour
estimate is not published as an exact total, weekly workload or calendar length.

LearnPython.org's integration is limited to its Russian Python tutorial at
`https://www.learnpython.org/ru/`. The fixed hostname/path, Russian page, unique
introductory course text and explicit free-tutorial metadata must match. Only its
own basics/advanced chapter lists enter the AI document. Foreign chapter links,
missing or duplicate sections and paid/unknown pricing metadata are rejected.
Sponsors, children's courses, external certification and paid DataCamp programs
are excluded. No unsupported teacher support, weekly hours or duration is added.

Both free-price attestations expire after 26 hours without another successful
native verification. Failed reads preserve previous facts and do not extend the
lease. AI documents are course-only, checked against saved course/offer identity
at extraction and publication, and use the existing shared budget and independent
review. No provider-specific spending limit or additional schedule is introduced.
The existing catalog sort and pagination treat these providers like any other
smaller school; applicable language, free-price and suitability filters still
apply to the concrete published offer.

Fixtures are excerpts of official anonymous pages captured on 11 November 2026.
Companion JSON records the original public-response digest. They are not claimed
to be complete runtime responses. Opt-in live acceptance reads the current
source and its own discovery root without a model request or production write:

```sh
SMALL_FREE_SOURCE_CHECK=1 go test ./backend/updater -run TestSmallFreeProvidersLiveOfficialSources -count=1 -v
```

On 11 November 2026 both live source preparations passed: CodeChick 1,366 bytes
of scoped learning text and LearnPython 911 bytes, one course document each.
This source-only check uses a temporary derived binding; production processing
must additionally use the saved published identity. Actual publication and paid
recommendation results are reported separately in issue #189 after deployment.
