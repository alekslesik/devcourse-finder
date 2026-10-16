# Course-detail passport: concept A

The maintainer selected [concept A](https://github.com/alekslesik/devcourse-finder/blob/ef8a535/docs/design-previews/course-details-2026-10-16/A-passport.png)
after reviewing three course-detail sketches. The previous detail page repeated
price/actions, spread conditions across prose and left an empty curriculum heading.

The implementation uses a two-column desktop layout: course identity and readable
condition rows on the left, a compact tariff/action panel on the right. The panel
sticks while scrolling; short desktop viewports use natural scrolling instead.
Mobile layouts stack introduction, tariff and conditions. A bottom action dock
appears only after the primary action has scrolled above the viewport and uses
the same selected tariff. Safe-area padding and focus scroll margins reserve room
for it. Existing brand/header, glass rims, muted metadata palette and both themes
are retained. Shared panel contrast/forced-color fallbacks remain in effect.

For multiple offers, the native tariff selector updates the displayed price,
duration, workload, support, enrollment, outbound offer ID and comparison action
together. The selected ID is shareable in `?offer=` and is server rendered; unknown
or repeated IDs safely fall back to the first offer. `#course-tariffs` and existing
catalog-return context remain available. Catalog cards include their matched offer ID in both detail links. Without JavaScript, initial course facts,
the selected tariff and external links are still present. Interactive tariff
switching requires JavaScript.

The view consumes normalized effective prices and existing condition helpers.
Audience and verified prerequisites are displayed independently, including the prerequisite language scope. The purchase panel resets inherited aside height/overflow constraints so short mobile screens use document scrolling.
Unknown support remains distinct from confirmed absence. Closed/unknown enrollment
offers do not expose an add-to-comparison action. Missing topics produce a clear
link to the school's program; missing offers produce an explicit state. No prices,
topics, suitability claims or support facts are inferred. Source dates and the
verification disclaimer remain visible. Client props omit repeated Course objects
from each search Result.

## Validation scope

`course-passport.spec.ts` exercises 390/1440px in both themes: geometry, sticky
position, anchor focus/current section, mobile dock visibility, focus clearance,
axe and page overflow. Four frontend-only synthetic scenarios exercise shared
tariff SSR without JS, selection/reload/back/comparison identity, free/from/closed
states, absent offers/topics and a catalog-to-detail transition with a non-first matched offer at 320×568px. Those synthetic scenarios are explicitly skipped
in the real database run; the four layout/action cases run there against the
isolated demo catalog. The WebKit CI subset now includes these new scenarios.

Existing route checks cover server metadata, permanent detail URLs, unpublished
404s and outage handling. Existing browser suites cover catalog return, comparison,
unknown conditions, full budgets, themes and genuine Chromium 200% tab zoom.

Browser screenshots and semantics checks are automated evidence. They do not prove
physical iPhone Safari keyboard/chrome behaviour or actual VoiceOver/NVDA speech.
Those manual acceptance items remain open in #98 and the parent #91.
