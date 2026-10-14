# Repository and pull request workflow

- Before each task, fetch the latest repository changes and start new work from
  the latest main. Preserve unrelated local changes.
- After creating a PR, wait at least one minute and wait for review and all
  required CI checks to finish. Read both conversation comments and inline review
  threads, address actionable feedback, and resolve addressed review threads.
- Once review is complete, feedback is addressed and required checks pass, merge
  the PR yourself without asking for another confirmation. Do not bypass failed
  checks or unresolved actionable review feedback.
- After merging, ensure the Release workflow succeeds and verify that the matching
  version tag and published GitHub Release exist. Complete release publication
  yourself without asking for another confirmation; never move an existing tag.
- After release publication, dispatch the Deploy release workflow on main with
  that exact published version tag yourself, without asking for another
  confirmation. Wait for deployment to finish and verify production health and
  the displayed version. Report any failure; do not claim deployment succeeded
  until verified.

# GitHub authentication retries

- The maintainer uses a long-lived GitHub token. Do not conclude that it expired
  or request replacement after a single `401 Bad credentials` / authentication
  failure. Make up to four attempts total, pausing 15, 30 and 60 seconds between
  attempts; refresh managed credential readiness when that tool is available.
- Use the configured `GHTOKEN` binding for CLI requests (`GH_TOKEN="$GHTOKEN"`
  for that command). Preserve the session proxy and credential-injection setup;
  never print credentials, change tokens or start an interactive login yourself.
- For mutations, inspect remote state before retrying so a PR, merge, release or
  workflow dispatch is not duplicated. Retry only if the action was not applied;
  monitor an existing deployment of the same tag instead of dispatching another.
- If authentication still fails after these attempts, report the exact operation
  and error, finish unaffected work using available connector tools, and ask for
  credential repair only when the remaining action is actually blocked. A 401
  alone is not evidence that a long-lived token has expired. Permission denials
  and rate limits need their own diagnosis, rather than repeated blind retries.

# Release workflow

- Every PR must increase the root `VERSION` and add its English release notes to
  `CHANGELOG.md`. Use semantic versions: patch for fixes/docs, minor for features,
  major for incompatible changes. Update against the latest main before merging.
- Use `python3 scripts/release.py bump patch --note "Description"` (or minor/major)
  and add all relevant changes to that version's changelog entry.
- Run `python3 scripts/release.py check --base-ref origin/main` before opening a PR.
- After a PR merges, the Release workflow pushes `v<VERSION>` and creates a
  GitHub Release from that version's changelog. Never move an existing tag.
- Deploy through an explicit dispatch of the Deploy release workflow, selecting
  the published tag. The agent is authorized to perform this dispatch after its
  PR merges and its release is published. Keep the workflow dispatch-only; do not
  add deployment triggers on pushes, merges, tags, or release publication.
- Keep passwords and production `.env` out of Git and logs. Keep the pinned public
  SSH host key in `.github/vds_known_hosts`; change it only after verifying a host
  key change with the user.

# Frontend design skills

- This workspace is for frontend work. Keep backend code outside the scope unless
  the maintainer explicitly requests a backend change.
- Project-local skills are installed under `.agents/skills/`. See
  `.agents/skills/README.md` for the catalog and `.agents/skills/sources.json` for
  pinned upstream revisions, licenses and file checksums.
- For interface design, critique, responsive layout, accessibility and polish,
  read `.agents/skills/impeccable/SKILL.md` and the relevant command reference.
- For requested visual direction or a redesign, read
  `.agents/skills/design-taste-frontend/SKILL.md` or
  `.agents/skills/redesign-existing-projects/SKILL.md` as appropriate. The existing
  liquid glass identity and user-provided designs take precedence over generic
  aesthetic defaults or bans. Routine fixes should preserve the approved design.
- For UI details, read `.agents/skills/emil-design-eng/SKILL.md`; for mobile web
  interaction, read `.agents/skills/mobile-native/SKILL.md`; for motion, use
  `.agents/skills/animate/SKILL.md`, `review-animations/SKILL.md` or
  `animation-performance/SKILL.md` under the same directory as needed.
- Load only skills relevant to the current task. Do not install UI libraries,
  change the stack or apply alternative visual styles solely because a skill
  mentions them. Use the existing project tools and honor the user's scope.
