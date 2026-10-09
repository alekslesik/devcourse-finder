# Netology adapter

Issue #39 adds Netology to the existing page collector, using the official
https://netology.ru/development catalog. Only one-segment `/programs/<slug>` URLs
with supported development-language hints become candidates. Discovery does not
publish courses. The existing twice-daily schedule, request budgets, durable
queue, publisher confirmation and stale-offer handling remain in force.

## Verification contract

Each bounded anonymous detail fetch requires a Russian HTML document, one exact
canonical URL and one `__NEXT_DATA__` payload for `/programs/[programName]`.
The route, program name, program family URL, landing template and positive IDs
must agree. Active, loaded, public paid programs are required; fake/sandbox,
deferred payment, additional lessons and personalized promocodes are rejected.

The current and initial full prices must agree, as must their original prices.
A discounted price requires an unexpired explicit date, interpreted through the
end of that day in Moscow. Global payment placeholders and JSON-LD monthly or
starting prices are never used as full prices.

A rendered card must explicitly say `одним платежом` and show the same full RUB
amount. Its section/card index, title and exact program slug must match the
landing's `plansNew_*` metadata. Hidden price/order cards, redirects, ambiguous
cards and conflicting duplicate plans are rejected. CSS module hashes may vary;
missing semantic structure prevents publication rather than selecting another
price. Sibling tariffs do not establish this program's price.

Scheduled programs require agreement between the Russian start date and ISO
date. Enrollment is open only before that date and when the program has not
started; validity is capped by both start and discount deadlines. Explicit async
programs require consistent continuous-start metadata. Contradictory dates are
rejected, rather than choosing whichever date appears later.

Program family and cohort IDs are stored separately. A new cohort in the same
family can refresh the course. A family change blocks publication and clears
pending price confirmation atomically, preventing evidence from a different
program from completing an earlier confirmation.

## Coverage and operation

Recorded official pages on 2026-10-09 verified Java (131,700 RUB) and Go
(99,900 RUB). Python's displayed and structured start dates disagreed, so that
page was rejected. These are verification samples, not a promise of total
provider coverage. Nonempty `resource_packages` and shared canonical tariffs
are not yet verified and are rejected. Free lessons cannot become free full
professions. Unsupported templates remain unpublished automatically.

Positive, negative, duplicate-plan and PostgreSQL integration tests cover the
contract, queue/publisher separation, cohort rotation and family protection.
The Java and Go live checks also passed in a non-root, read-only container with
a 128 MiB memory limit and 0.5 CPU. An optional bounded live check uses current
official responses:

```sh
NETOLOGY_LIVE_CHECK=1 go test ./backend/updater -run TestNetologyLiveVerification -v
```

Default tests use reduced public-page fixtures and do not require the Internet.
Deploy through the existing manual **Deploy release** workflow after merge and
release publication. Merging or publishing a release does not deploy the VDS.
