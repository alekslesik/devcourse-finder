# JavaRush recurring fees in their original currency

Implements #46 without changing frontend files. The catalog/API/store now support
an optional `offer.billing` object:

```json
{"kind":"subscription","amount_minor":3000,"currency":"USD","interval":"month"}
```

Amounts use original-currency minor units. Allowed currencies are USD, EUR and
RUB; intervals are month/year. This object never defines a complete-course RUB
price: subscription offers must have `price=null`, `price_kind=unknown` and
`free=false`. Budget/free filters and `effective_price` cannot treat a recurring
fee as a course total, even if malformed input tries to supply a numeric total.
Legacy offers omit `billing`; existing API fields retain their meaning.

The verified adapter currently covers the Russian Java Premium self-study card
at `https://javarush.com/prices`. It separately reads the public anonymous GET
`/api/1.0/rest/subscriptions/prices/all?duration=MONTH`, identified by inspecting
the official page's pricing-module scripts as text. The exact
`JR40_PREMIUM_JAVA_SELF` key, MONTH fee, preferred USD currency, visible card
amount/period and course-specific buy link must agree. Missing or contradictory
fees, unverified discounts, changed currency/period and altered product bindings
reject publication. Checkout links are inspected, never followed or submitted.

The older `/subscriptions/sale/prices` API exposes a different pricing format
and is not used for monthly imports. It must not be combined with current card
fees. University and Mentor Pro are distinct products and are excluded; their
cohorts/support cannot establish a self-study subscription's conditions.

On 2026-10-10 the matching public page and monthly API verified 30 USD/month.
The adapter publishes a self-paced, continuously available subscription with
unknown complete price and a 26-hour evidence lease. The existing frontend can
show `Premium — 30 USD в месяц` in the tariff name, while its full-price label
remains unknown. No exchange rate, RUB conversion, fixed duration or free status
is guessed. A dedicated billing-widget redesign remains a separate frontend
concern, not a requirement to safely preserve the original fee.

Billing persists in PostgreSQL and is included in API/catalog snapshots. The
collector queues normalized facts; the publisher revalidates original-currency
billing and expiry. Fee changes greater than 50%, or currency/interval changes,
use the existing separate-window confirmation path. Rolling leases are excluded
from its fingerprint so repeat reads can confirm the same change. Failed reads
interrupt pending confirmations as for other providers.

Fixtures omit request IDs, user state, tracking, contact details and real
credentials. Tests cover original currency/interval, monthly/full-price and
free/budget separation, University confusion, page/API disagreement, API-facing
tariff labels, persistence and confirmation of a doubled fee.
Optional read-only live verification:
`JAVARUSH_LIVE_CHECK=1 go test ./backend/updater -run TestJavaRushLiveVerification -v`.

After merging the combined PR, wait for its versioned Release and manually run
**Deploy release** with that published tag. Neither merge nor release publication
deploys. Collection remains twice daily, with the existing manual refresh trigger
available for an earlier run.
