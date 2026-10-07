# Release workflow

- Every PR must increase the root `VERSION` and add its English release notes to
  `CHANGELOG.md`. Use semantic versions: patch for fixes/docs, minor for features,
  major for incompatible changes. Update against the latest main before merging.
- Use `python3 scripts/release.py bump patch --note "Description"` (or minor/major)
  and add all relevant changes to that version's changelog entry.
- Run `python3 scripts/release.py check --base-ref origin/main` before opening a PR.
- After a PR merges, the Release workflow pushes `v<VERSION>` and creates a
  GitHub Release from that version's changelog. Never move an existing tag.
- Deployment is manual through the Deploy release workflow, selecting a published
  tag. Never enable deployment on pushes, merges, tags, or release publication.
- Keep passwords and production `.env` out of Git and logs. Keep the pinned public
  SSH host key in `.github/vds_known_hosts`; change it only after verifying a host
  key change with the user.
