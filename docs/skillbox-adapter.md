# Skillbox verified tariffs

Implements #43 in the existing page worker. Discovery reads only the official
`/course/sitemap.xml` shard of `https://skillbox.ru/sitemap.xml`; editorial,
promotion and live-event maps are excluded. A canonical `/course/<slug>` and
matching heading bind the public page to static `landingInfo`, `autopayment`
and `priceInfo` objects. Their product/landing IDs and currency must agree.
Only literal objects are decoded; page JavaScript is never executed.

Each basic/advanced/vip card provides its own nomenclature ID and direct full
card payment. The default must match the root product and payment configuration;
its additional-payment discount must reconcile with the discounted full price.
Original list price and monthly installments are not payable totals, and no
monthly amount is multiplied by duration. Each tariff is published separately,
with human support classification unknown. Available full-payment options and
an unexpired tariff deadline establish open enrollment; schedule is unknown,
not an invented cohort or a promise of self-paced learning. Evidence expires at
the earlier of the sale deadline and 26 hours after verification.

Product and tariff-set bindings are persisted. A changed product, removed tariff
or reused identity is protected rather than silently replacing existing offers.
The collector queues normalized facts; only the publisher changes the catalog.
Failed reads reset every tariff's pending anomaly confirmation in split-worker
and legacy modes. Price anomalies still require the existing confirmation path.

Recorded public Python fixtures verify three full card totals (133735, 182551,
210672 RUB), without payment keys or personal data. Negative tests cover payment
conflicts, product changes, currency, altered installments and expired deadlines.
Optional live read: `SKILLBOX_LIVE_CHECK=1 go test ./backend/updater -run TestSkillboxLiveVerification -v`.

Merge the combined PR, wait for the versioned Release, then manually run **Deploy
release** with its published tag. No deployment occurs on merge or publication.
Scheduled collection runs twice daily; the existing manual refresh trigger can
request an earlier run. Provider markup and personalized discounts can change;
unsupported or conflicting contracts are rejected, not guessed.
