# Skillfactory course-specific pricing

Implements #44 in the page collector. The configured official HTML catalog is
`https://skillfactory.ru/courses/programmirovanie`. Only single-segment detail
paths are candidates; the independent official pricing response must bind each
canonical URL and course title to its course tag and three tariff popups.

The page's `price-automation.js` was inspected as text, without execution. It
uses the anonymous GET `https://tools.skillfactory.ru/api/get-info-by-url?url=...`.
The adapter constructs that fixed endpoint from a validated canonical identity;
it never follows arbitrary URLs or submits contact/payment forms. This host is
explicitly allowed for source reads, not for feed/course identity.

Basic, personal and personal-plus use their own `price_discount_*` complete
amounts, reconciled against `price_full_*` and the matching discount percentage.
Monthly installment values, salaries, generic Product JSON-LD and additional
conditional payment discounts are not used as full course prices. Static Tilda
installment text is a placeholder replaced by the official GET, so its stale
numbers cannot override current course-specific data. Human support remains
unknown until separately verified.

A dated active campaign and a matching future cohort establish open enrollment.
For a start stated only as day/month, its year is accepted only when the campaign
anchors one upcoming date within 45 days in the same year; ambiguous rollover,
old cohorts, invalid dates and expired campaigns are rejected. Evidence expires
at the earlier of the campaign deadline and 26 hours. Display counters are not
availability proof: the site's script calculates marketing places/viewer counts.

Product/tag and tariff identities are protected by the shared multi-offer
publisher. Changed markup, missing popups, conflicting prices, wrong currencies
or unavailable GETs fail closed. Collector and publisher remain separate.
Fixtures contain reduced public facts only. Recorded Python full prices are
154044, 197640 and 252396 RUB; negative tests cover stale data, mismatched URL/tag,
changed popup/script contracts and monthly/full-price confusion.

The discovery-feed ceiling is raised from 10 to 16 to accommodate the four new
providers. Existing per-provider, queue, body, timeout and run limits are kept;
this does not create unbounded or concurrent crawling.

Optional read-only live check:
`SKILLFACTORY_LIVE_CHECK=1 go test ./backend/updater -run TestSkillfactoryLiveVerification -v`.
After merging the combined PR, deploy its published version manually using
**Deploy release**. Merge/release publication never deploys. The twice-daily
schedule and existing manual refresh trigger collect the provider after deploy.
