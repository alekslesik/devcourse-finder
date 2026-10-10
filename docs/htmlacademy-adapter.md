# HTML Academy: public course and payment verification

Part of [issue #42](https://github.com/alekslesik/devcourse-finder/issues/42)
and the parser-worker roadmap in #34. HTML Academy runs in the existing `pages`
collector; it needs no separate container, browser, account or credential.

## Discovery and identity

The configured feed is `https://htmlacademy.ru/sitemap.xml`. Only its official
`/sitemap/sitemap_default.xml` child is read. Interactive exercise maps, reviews,
pagination, payment pages and nested profession variants cannot seed identities.
Standalone `/intensive/<slug>` and `/profession/<slug>` paths have separate
identities (`intensive-<slug>` and `profession-<slug>`). Discovery alone never
establishes availability or a price.

The current verified publication contract covers Russian standalone intensive
courses with the individual, self-paced monthly format. A single matching
canonical link, course heading, description, individual-format block and enabled
payment button are required. The payment module must name the exact expected
public data path for that course. Unsupported language/course content is rejected
by the shared normalizer.

## Availability and product boundaries

The collector performs an additional anonymous GET to the identity-derived
`https://htmlacademy.ru/api/payment-data/intensive/<slug>/individual` endpoint.
The response wraps public display data in a JWT-shaped token. This token is
**not used as authentication**, forwarded to a checkout, logged or stored. HTTPS
and the fixed official host establish source provenance; decoding its payload
is not a claim of cryptographic signature verification. Only bounded offer data
is inspected; user identifiers, contact forms, tracking and other fields are
ignored. No payment is initiated, scripts executed or login performed.

The default payment offer must independently match the course title, individual
product subtype, RUB monthly amount and public landing-page rate. It must have
an explicit monthly recurrence, recurrence enabled in order settings, an empty
cohort-date list, unrestricted places (`null`), and a supported direct card/SBP
payment method. A disabled button, unavailable endpoint, exhausted places,
contradictory identity, changed contract or missing evidence rejects publication.
Old marketing copy alone cannot establish enrollment.

Successful reads establish `continuous` enrollment and a `flexible` schedule
for this recurring format, with a 26-hour evidence lease. There is no inferred
cohort start. Group/cohort professions, bundles and unsupported formats remain
unpublished until they have their own verified contract. In particular, a
`profession` lite offer embedded in an intensive page is a different product and
is never presented as the intensive course's tariff.

## Prices and catalog limits

A monthly amount is **not a complete course price**. Suggested durations of two
to four months do not define a mandatory total, so no multiplication is used.
The resulting offer is `price_kind=unknown`, has no numeric price, and is not
free. Its name and description disclose monthly billing and the unknown total.
The adapter does not import discounts, corporate prices, installments, trial
lessons or bundle amounts into the course price. Mentor/review classification
remains unknown rather than inventing a universal support commitment.

Live checks on 2026-10-09 verified JavaScript and React individual-format pages.
These add source coverage, but do not guarantee more results under filters that
require a verified complete price. Existing price/search freshness rules remain
in effect. This change makes no claim to publish all HTML Academy products.

## Queue, failures and operations

The collector queues normalized public facts and evidence hashes. The publisher
revalidates product/offer invariants and leases, and is the only worker that
writes catalog data. Raw HTML, payment tokens and user/form data never enter the
observation queue. Failed reads use the existing backoff and failure path; they
cannot refresh a published course or reuse incomplete anomaly confirmations.
The API collector does not own this provider. The page collector reserves time
for both bounded HTTP reads and respects existing provider/run limits.

Deployment remains manual: merge the PR, wait for its versioned GitHub Release,
then run **Deploy release** with that published tag. Neither merge nor release
publication deploys automatically. After deployment the regular twice-daily
collection schedule discovers and refreshes these candidates. The existing
manual catalog-refresh trigger can request an earlier run; publication still
uses the publisher's safety checks. Inspect worker logs and coverage rather
than assuming every discovered URL became a published course.

## Verification

Reduced anonymous fixtures retain only public course and offer facts. Test
tokens are synthetic; real tokens and user identifiers are absent. Negative
tests cover product/bundle confusion, altered prices/currency, recurrence,
cohort dates, exhausted places, missing payment methods, stale/disabled pages,
external data URLs and malformed payloads. PostgreSQL integration tests cover
sitemap discovery, collector/publisher separation, token-free queue payloads,
failed refresh preservation, invented-total rejection and expired evidence.

Optional live reads (no production writes):

```bash
HTMLACADEMY_LIVE_CHECK=1 go test ./backend/updater -run TestHTMLAcademyLiveVerification -v
```
