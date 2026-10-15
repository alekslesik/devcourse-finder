# Course labels and actual browser zoom

Follow-up to #131 and #98, 2026-10-15.

## Course label palette

The initial v0.25.4 pastel fills and pronounced contours were softened following
the maintainer's feedback. Course badges use a low-opacity language tint (10%
in light mode, 14% in dark mode) and a 25% tinted contour: Python yellow,
JavaScript gold, Java coral/orange and Go cyan. Text uses the normal theme ink
for readability. These labels are independent of the hero tray's controls and
selected states.

School names use a shared display component in cards and course details. Matching
uses published display names; unknown schools keep neutral text. Display names,
links and catalog facts are unchanged. Forced colors use the system text palette.
Following the same feedback, school accents are blended with muted theme text
(50% brand color in light mode, 35% in dark mode). Their hue remains recognizable
without competing with course titles.

The palette references official school sites and their current visual identities,
with light/dark derivatives for readable small text rather than exact brandbook
hex values:

| School | Reference | Hue |
| --- | --- | --- |
| JavaRush | https://javarush.com | Lime/green |
| Skypro | https://sky.pro | Cyan |
| Skillfactory | https://skillfactory.ru | Green/lime |
| Skillbox | https://skillbox.ru | Violet |
| HTML Academy | https://htmlacademy.ru | Indigo/violet |
| RS School | https://rs.school | Yellow |
| PurpleSchool | https://purpleschool.ru | Purple |
| Нетология | https://netology.ru | Blue |
| Stepik | https://stepik.org | Green |
| OTUS | https://otus.ru | Yellow |
| Яндекс Практикум | https://practicum.yandex.ru | Yandex coral/red |
| Хекслет | https://ru.hexlet.io | Indigo |
| CodeBasics | https://code-basics.com | Teal |

The Stepik homepage rejected the anonymous reference fetch (403); its established
green identity is used without claiming a newly verified brandbook color. Skypro
and CodeBasics hues reference their official SVG logos. The acceptance fixture
covers all thirteen schools, an unknown school and all four language badges at
390/1440px in both themes, including axe text-contrast and overflow checks. It
also exercises forced colors. Synthetic provider substitution tests presentation,
not production catalog coverage.

## Genuine tab zoom

`browser-zoom.spec.ts` loads a temporary test-only Chromium extension page with
the tabs permission and calls `tabs.setZoom(..., 2)`. It verifies `getZoom() === 2`,
halved CSS viewport width and doubled device pixel ratio. The controller does
not depend on a background service worker. No CSS zoom, deviceScaleFactor or
viewport emulation is substituted for actual tab zoom.

Both themes exercise keyboard search, result focus, three selected tariffs,
keyboard horizontal comparison scrolling, course detail/catalog return and page
overflow. Screenshots and JSON measurements are retained as test artifacts.
The extension is temporary test infrastructure and is never shipped to the site.
WebKit does not run the Chromium extension test.

Like the ordinary browser suite, the controller honors `CHROMIUM_PATH`. Without
that override it uses Playwright's full bundled Chromium. Some system builds
disable command-line extension loading: when explicitly selected, those builds
report the two zoom checks as skipped with an explanation, not as passing zoom
evidence. Use an extension-capable build (for example the installed Playwright
Chromium executable) for these checks. CI uses the bundled build and requires
the controller to load; it cannot take this system-browser skip path.

## Manual evidence still pending

The maintainer reported that ordinary browsing seemed fine, then explicitly
clarified that no screen reader was used. This is useful user smoke feedback,
not evidence of VoiceOver/NVDA speech or a recorded physical-device test matrix.
Actual speech and iPhone Safari keyboard/chrome/performance acceptance remain
open in #98; the parent #91 remains open accordingly. Browser automation does
not establish those results.
