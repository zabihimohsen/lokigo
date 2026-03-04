# Release Playbook

Use this flow for clean, intentional releases.

## Pre-release checks

- [ ] `CHANGELOG.md` includes all user-visible changes
- [ ] README/docs match current behavior
- [ ] CI green on `main`
- [ ] No open blocker issues

## Versioning policy

- Patch (`x.y.Z`): fixes/docs/internal improvements without API breaks
- Minor (`x.Y.z`): backward-compatible features
- Major (`X.y.z`): breaking API or behavior changes

## Release steps

1. Merge release-ready PRs into `main`.
2. Trigger **Release** workflow manually.
3. Provide `tag` input (example `v0.2.0`).
4. Verify GitHub Release notes and edit summary.
5. Announce using `.github/marketing-drafts/ANNOUNCEMENT.md` template.

## Post-release checklist

- [ ] Confirm pkg.go.dev indexed new version
- [ ] Post short changelog update on social channels
- [ ] Open follow-up issues for deferred improvements

## Hotfix flow

- Branch from latest release tag
- Apply minimal fix + tests
- Tag patch release (`vX.Y.Z+1`)
- Back-merge fix into `main`
