# Website quality acceptance (#213)

Scope: [epic #213](https://github.com/alekslesik/devcourse-finder/issues/213), tasks #214–#217. This is a frontend follow-up to the supplied 26-second video checklist, not a redesign or a claim of improved conversion.

## Baseline

On 18 November 2026 the public site served v0.44.0. Trusted HTTP requests to https://devcoursefinder.ru/ succeeded; title, description, H1 and canonical already existed. `/robots.txt` returned 404. `/courses` was intentionally noindex but appeared in the sitemap. Several offers could repeat a course URL in that sitemap. Yandex Metrika counter 114139174 was already installed, including SPA history tracking; the earlier assessment against an old checkout missed that installation. The first-party API accepted view/compare but the frontend sent only search/empty; outgoing clicks were already counted on the server.

The product already supported experience, goals, budgets, source links and verification dates. The video’s large lead form criticism does not apply: no phone/email application is collected here. There are no invented reviews, social-proof statistics or employment guarantees in this change. Classification coverage remains tracked in #170/#175.

## Implementation

- Keep the interactive `/courses` duplicate and personal comparison URLs noindex. Remove `/courses` from the sitemap rather than opening filtered duplicate pages for indexing. `robots.txt` allows crawling so noindex can be read and links to the configured public sitemap.
- Deduplicate course slugs across offers/pages and exclude demo/unpublished courses from the sitemap. API failure remains an error instead of producing a misleading partial sitemap.
- Record `view` after a successful course page renders, with only course ID and language. Record `compare` after a successfully loaded comparison has at least one available tariff; `total` is the available tariff count, not all requested IDs. Empty, failed and entirely unavailable comparisons do not count. Direct and legacy modal URLs are supported. Replayed effects and aborted modal requests do not count twice. Analytics failures do not block use of the site.
- Explain full tariff prices in the hero and link directly to the existing verification methodology. Keep language/experience/goal/budget visible; put direction/support/schedule/workload in native keyboard-accessible disclosure. Closing it does not unmount or disable inputs. Active URL conditions open it on entry, history navigation and form restoration. Native validation opens a collapsed invalid field.

## Measurement method

For an apples-to-apples layout comparison, build main 131ea67 and the changed frontend against the same fixture API, 18 demo offers, a 900px viewport height, light/dark, reduced motion, and loaded local fonts. These are fixture geometry measurements, not the live catalog or conversion data.

| Width | Baseline form height | Final form height | Baseline first card Y | Final first card Y |
| --- | ---: | ---: | ---: | ---: |
| 390 | 631px | 499px | 1635px | 1614px |
| 1440 | 358px | 261px | 1233px | 1136px |

Both themes had equal geometry and no page overflow before and after. The mobile form is 132px (21%) shorter, the desktop form 97px (27%) shorter. The extra explanatory copy uses some of the mobile space saved, so the first card moves up only 21px there. The approved large hero and language tray remain. The intended improvement is a simpler initial form; no promise of above-the-fold course cards is made.

## Acceptance and limits

Server route regressions cover robots content/hostname, canonical/metadata, noindex, duplicate offers, demo/unpublished sitemap exclusions, 404 and outage handling. Browser regressions cover actual event payloads and blocked analytics, direct/legacy comparisons, disclosure draft/history/reset/validation, keyboard interaction, contrast/axe and overflow in both disclosure states and themes.

The remote browser could not validate the environment proxy certificate. An attempted persistent trust change was rejected by automatic approval review and was not applied. Public HTTP checks keep TLS verification enabled. Browser evidence uses the local production build; it is not represented as a direct production-browser check. Device speech, physical iPhone/Safari keyboard/safe areas and hardware performance remain in #98; automated Chromium/WebKit do not close them. No new external analytics account configuration or advertising changes are included. First-party events measure engagement, not purchases or causal conversion lift.

## Video checklist disposition

| Concern from the video | Disposition |
| --- | --- |
| Value proposition and CTA | Existing clear H1 and search/compare actions; full tariff price explanation and methodology link added |
| JTBD / relevant structure | Existing experience, goal and budget paths; no claim of new user research |
| Weak social proof | Trust comes from source dates and method transparency; testimonials are not fabricated |
| Long lead forms | No lead collection; advanced search conditions are now progressively disclosed |
| Grid / responsive UI | Existing grid and themes retained; browser geometry, keyboard, axe and overflow checks |
| Default metadata / missing H1 / semantics | Existing metadata and H1 retained; server HTML regressions still pass |
| Images / alt | No new images; decorative icons remain hidden from assistive technology with adjacent text labels |
| Missing sitemap / robots / canonical | Sitemap and canonical existed; new robots and consistent sitemap membership/deduplication |
| Missing analytics / goals | Existing Metrika pageviews retained; first-party rendered views and successful comparisons complete the engagement event types. This is not proof of attribution, purchases or configured goals in the external Metrika account |
| CAC / LTV / lost traffic | Cannot infer from UI checks; no acquisition-cost or conversion improvement claimed |

Local validation (before PR): production build, TypeScript, API schema generation/check and server route suite passed. The first browser pass passed 37/38; the one failure was an ambiguous new test locator matching goal option text as well as the direction label, corrected to the named select. The second pass passed 39/39, including 360/390/768/1440 light/dark visual/axe, course pages and dialogs. After the final desktop button alignment fix, all 9 targeted disclosure/analytics checks and the server route suite passed again. The new scenarios are also included in CI’s WebKit invocation.

No persistent TLS trust settings were changed. The environment browser could not trust its network proxy certificate; automatic approval rejected adding that CA to its persistent NSS database. Public production acceptance uses normal trusted HTTPS requests; visual/interaction checks use the local production build, without disabling TLS verification or claiming a real-device test. Physical iPhone/Safari and actual VoiceOver/NVDA speech remain in #98, as before this epic.
