# Skypro product-scoped enrollment and unknown full prices

Implements #45 in the page collector. The official `/course/programming` catalog
seeds real single-segment programming detail links; guessed `/python` and `/java`
URLs are not used. Each detail needs a matching canonical identity, one heading,
a description and exactly one target pricing/registration block.

The product ID is read from the Tilda form's public static input configuration
inside **Стоимость и варианты оплаты**, with profession type required. Diagnostic
and unrelated course forms elsewhere cannot establish enrollment or closure.
Multiple or malformed product identities reject the observation. Persisted
product IDs protect against URL/product reuse.

Recorded and live Python/Java pages on 2026-10-10 explicitly report **Набор
временно закрыт** and **Сейчас мы не принимаем новые заявки** inside the target
registration block. They produce closed observations with unknown full prices;
new closed courses are not published. Later closure of an existing course still
uses the shared anomaly-confirmation path, without a changing lease preventing
confirmation. An open observation requires the scoped application CTA and
installment disclosure, with no contradictory closure copy. Unrelated closed
blocks cannot close that target offer.

Available numbers are monthly installments, salary examples, placeholder zeros
or generic AggregateOffer low prices. None establishes a complete course price.
The adapter therefore never imports them into numeric price, never calls the
subscription free, and never multiplies a monthly sum by an advertised duration.
A course with unknown full price cannot match a confirmed budget. This change
claims no verified complete-price coverage for currently closed Skypro products;
new full-price markup requires its own verified binding before accepting it.

No arbitrary JavaScript runs. Lead, self-pay, webinar and booking POST endpoints
are not catalog APIs and are never called. The collector uses official anonymous
GETs only and queues normalized facts with a 26-hour lease. Only the publisher
writes catalog data. Tests cover recorded closed pages, target/unrelated closure
separation, changed product/markup, no initial closed publication and budget
exclusion for unknown prices.

Optional read-only verification:
`SKYPRO_LIVE_CHECK=1 go test ./backend/updater -run TestSkyproLiveVerification -v`.
After merging the combined PR, manually run **Deploy release** with its published
version. Merge and release publication never deploy. Regular collection remains
twice daily; the existing manual refresh trigger can request an earlier run.
