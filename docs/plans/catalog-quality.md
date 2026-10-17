# Catalog enrichment reliability and filter audit

Epic #158; deliver all five tasks in one PR, one commit per task. No frontend
redesign, paid bulk development runs, price inference or schedule/budget changes.

1. #159: Check Stepik once per enrichment batch before selecting candidates.
   Persist an outage cooldown, exclude the provider before LIMIT, probe again on
   the next run after two hours, and expose availability in status. Preserve
   per-course retry dates, existing facts and independent native collectors.
2. #160: Support proven legacy course identities without relaxing host, product,
   tariff or language checks. In particular, accept one cosmetic trailing slash.
3. #161: Separate source, extraction, local grounding, verification and publication
   failures; distinguish observations from useful verified facts and empty output.
4. #162: Audit every published course, not just the first page. Report confirmed,
   unknown and excluded profile matches, and compare counts with the public API.
5. #163: Record acceptance evidence and external blockers, run backend/PostgreSQL
   checks, update version/changelog, and verify release and deployment.

Acceptance uses recorded/mock model responses and an isolated PostgreSQL database.
Source outages are reported as outages; unknown profile values never become
beginner matches merely to increase counts. A queued observation is not proof of
an updated card. Existing HTTPS and manual accessibility issues remain independent.

## Stepik availability

`catalog-ai status` includes `provider_health` with `available`, `checked_at`,
`next_probe_at` and a fixed code. Before a due Stepik batch, an anonymous API
probe has an eight-second outer timeout. An unavailable or non-JSON endpoint
pauses Stepik for two hours and excludes its courses before the batch limit.
The next enrichment run after the cooldown retries automatically. This is not a
new scheduler: Monday/Thursday Moscow scheduling remains in force. The manual
Actions enrichment workflow can also perform that next run without SQL resets.
No model request is made for the probe, and a pause does not mutate course facts
or course retry dates. Per-course readers still validate identity after recovery.
