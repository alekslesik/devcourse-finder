# Manual catalog acceptance (#98)

Status on 2026-10-20: **not run**. The maintainer explicitly requested that #98
and parent #91 stay open until their manual verification. Automated Chromium,
WebKit, axe and zoom evidence does not complete these rows.

Record the deployed version, URL, device, OS, browser, screen reader/version,
date and actual result for each applicable scenario. Use a recording or a short
transcript for announcements; attach screenshots for a visual failure. Do not
paste secrets. A single normal browsing session is not screen-reader evidence.

| Scenario | Procedure | Expected result | Actual evidence |
| --- | --- | --- | --- |
| Successful search, VoiceOver/NVDA | Enable the screen reader, choose a language and experience, submit | Controls have understandable names; settled results/count are announced once; focus reaches results after explicit submission | Pending |
| Loading/error/retry, VoiceOver/NVDA | Throttle or temporarily take the network offline, submit and retry | Loading, failure and recovered results are understandable without duplicate announcements or a focus trap | Pending |
| Empty results, VoiceOver/NVDA | Submit conditions with no matches, clear a filter | Empty result and subsequent result count are announced once; reset remains operable | Pending |
| Comparison and return, VoiceOver/NVDA | Select courses, open comparison/detail, return | Dialog/table context is understandable; close is reachable; focus returns to the invoking control | Pending |
| Keyboard, physical iPhone/Safari | Focus price fields, type and submit with the software keyboard; try both orientations | Labels/entered values and submit remain reachable, no unwanted input autozoom or page overflow | Pending |
| Touch and browser chrome, physical iPhone/Safari | Scroll results, compare, open/close dialogs and details; expand/collapse Safari chrome; test both themes | Touch controls respond, safe areas do not cover controls, browser/theme edges are readable, dialogs stay usable | Pending |
| Device responsiveness, physical iPhone/Safari | Repeat scrolling, filtering and comparison on the device | No reproducible stalls or broken interactions; record concrete reproduction if one appears | Pending |

The short-viewport automated test does not open a real software keyboard.
Playwright WebKit is not Safari on a physical iPhone. Profile GPU/motion only
when a reproducible device slowdown warrants it; a passing emulator is not proof
of real-device performance. Record failures rather than checking every row by
assumption. Link follow-up defects and keep affected acceptance rows pending.

Suggested result record:

```text
Version / URL:
Date:
Device / OS / browser:
Screen reader / version (if applicable):
Scenario:
Actual speech / focus / touch behavior:
Pass / fail / not run:
Evidence / defect link:
```

The current approved hero, visible route grid and glass design remain in place.
The historical desktop first-card distance is not universally improved; see
[catalog-ux-validation.md](catalog-ux-validation.md#layout-and-accessibility-evidence).
