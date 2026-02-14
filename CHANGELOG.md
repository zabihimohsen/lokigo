# Changelog

All notable changes to this project will be documented in this file.

## [0.1.0-alpha.1] - 2026-02-14

### Added
- Initial `lokigo` scaffold with `NewClient`, `Send`, and `Close` APIs.
- Background batching with triggers for:
  - max entries
  - max bytes
  - max wait interval
- Loki HTTP JSON push transport.
- Retry mechanism with exponential backoff and jitter.
- Backpressure modes: `block`, `drop-new`, `drop-oldest`.
- Behavioral tests for batching, retry behavior, and backpressure handling.
- CI workflow for `go test`, `go vet`, and `golangci-lint`.
- Architecture notes in `docs/ARCHITECTURE.md`.

### Changed
- `Close` now drains accepted queued entries before final flush.
- Module path updated to `github.com/zabihimohsen/lokigo`.
- README tightened with realistic quickstart and explicit non-production warning.
