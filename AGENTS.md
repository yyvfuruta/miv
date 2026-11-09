# Project instructions

All interface text, errors, documentation, comments, and commit messages must
be in English. Keep the editor focused on essential workflows.

## Commits and releases

The user has requested commits during implementation and a release for every
completed change. Keep these instructions in effect for future tasks.

- The initial release is `v0.0.1`. Tags use the `vMAJOR.MINOR.PATCH` format.
- Use a major bump for breaking compatibility, a minor bump for new features,
  and a patch bump for fixes. This rule also applies before version 1.0.0.
- Reset lower components when bumping: `0.0.1` -> `0.1.0` for a feature,
  `0.1.0` -> `0.1.1` for a fix, and `0.1.1` -> `1.0.0` for a breaking change.
- Related changes requested together may share a release; choose the highest
  applicable bump. Documentation/build-only maintenance uses a patch bump.
  An explicit version requested by the user takes precedence.
- Check the remote tags before choosing the next version. Update `VERSION`
  with the release version in the implementation commit.
- Run gofmt, `go test ./...`, `go vet ./...`, and `make build`. Run
  `make smoke` for changes affecting terminal input, rendering, or shell handoff.
  Validate GoReleaser configuration when changing release packaging.
- Commit completed work, create an annotated tag, and push the commits and tag
  to origin. Push main first, then push the tag in a separate command so the
  release workflow is already registered before the tag event. Never
  force-push, move an existing release tag, or replace a
  published release.
- `.github/workflows/release.yml` publishes binaries, checksums, and release
  notes via GoReleaser on tag pushes. Check that the workflow completes and
  the release assets exist, then report the release URL and version.
  A manual Release workflow run accepts an existing tag to recover a missing
  or failed release without modifying that tag.
- If publishing is blocked, report the actual blocker and what is already
  committed/tagged; do not claim that an unpublished release is available.
