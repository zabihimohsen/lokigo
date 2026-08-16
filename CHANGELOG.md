# Changelog

All notable changes to this project will be documented in this file.

## [0.1.8] - 2026-08-16

### Changed
- Bumped `google.golang.org/protobuf` from 1.36.10 to 1.36.12.
- Migrated CI linting to golangci-lint v2 (`golangci-lint-action` v9); unchecked
  `Close()` returns surfaced by v2 are now handled explicitly.
- Bumped GitHub Actions: `actions/checkout` v7, `actions/setup-go` v7,
  `softprops/action-gh-release` v3.
- Added Dependabot for Go modules and GitHub Actions.

### Fixed
- Release workflow: the `workflow_dispatch` path now publishes the GitHub Release
  directly instead of relying on a `GITHUB_TOKEN`-pushed tag, which cannot
  re-trigger the tag workflow.

## [0.1.7] - 2026-02-15

### Changed
- README positioning and launch narrative were polished.
- Marketing draft docs were moved under `.github/marketing-drafts/`.

## [0.1.6] - 2026-02-15

### Changed
- Iterative docs polish pass for README positioning and launch copy.

## [0.1.5] - 2026-02-15

### Changed
- Iterative docs polish pass for README positioning and launch copy.

## [0.1.4] - 2026-02-15

### Added
- Phase 2 launch-prep docs with benchmark snapshot and distribution draft copy.

### Changed
- Launch messaging refined for practical positioning.

## [0.1.3] - 2026-02-15

### Fixed
- Batch flush now triggers immediately when `BatchMaxEntries` is reached.

### Changed
- CI/docs updates for race checks and clearer `OnFlush` callback cadence.

## [0.1.2] - 2026-02-15

### Added
- Runnable examples and benchmark coverage for payload encoding.
- `slogtest` conformance coverage for the slog handler.
- Exported push error types and flush metrics callback (`Config.OnFlush`).
- No-sidecar deployment guide and doc link improvements.

### Fixed
- Slog handler conformance test improvements.

## [0.1.1] - 2026-02-14

### Added
- Protobuf + snappy transport (`EncodingProtobufSnappy`) for Loki push requests.
- Per-request custom header support via `Config.Headers`.
- Internalized Loki protobuf schema for transport implementation.

### Changed
- CI and lint workflow modernization.

## [0.1.0] - 2026-02-14

### Added
- Initial `lokigo` release with `NewClient`, `Send`, and `Close` APIs.
- Background batching with triggers for max entries, bytes, and wait interval.
- Retry mechanism with exponential backoff + jitter and transient classification.
- Backpressure modes: `block`, `drop-new`, `drop-oldest`.
- `log/slog` integration via `NewSlogHandler`.
- CI baseline for `go test`, `go vet`, and linting.
- Architecture and usage docs for sidecar-less deployments.
