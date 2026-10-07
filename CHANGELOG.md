# Changelog

Every pull request increments `VERSION` and adds an entry here before merging.
Merged pull requests receive the matching Git tag and GitHub Release.

## [0.1.0] - 2026-10-07

- Introduce semantic versions, per-PR changelog entries, and automatic GitHub Releases after merging into main.
- Add manual deployment of a selected published release through GitHub Actions.
- Verify the VDS SSH host key, preserve production configuration and database storage, and back up the database before deployment.
- Record the deployed version and commit after successful application checks.

