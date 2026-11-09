# Yandex Metrika

Counter `114139174` is mounted by the shared root layout on every page. Its
initialization preserves the supplied `ssr`, `webvisor`, `clickmap`,
`ecommerce: "dataLayer"`, referrer, URL, accurate bounce and link tracking options.
The standard `/watch/114139174` noscript pixel supports JavaScript-disabled visits.

The SDK loads asynchronously on `devcoursefinder.ru` and its `www` alias. Its
public ID, host and URLs live in `frontend/lib/metrika.ts`; no secret is required.
JavaScript visits on development and preview hosts do not initialize the counter.

Initialization is idempotent. App Router URL changes, including Finder's native
history updates for pagination and filters, send `hit` with the previous URL as
`referer`. An unchanged URL does not add another view. Queuing works before the
SDK arrives, and SDK failures cannot interrupt catalog navigation.

`frontend/tests/metrika.spec.ts` serves the real app at a virtual production
hostname and intercepts Yandex requests. It checks initialization options,
queued views, Back navigation, duplicate prevention, SDK failure, development
hosts and the noscript pixel without adding synthetic traffic to the account.
