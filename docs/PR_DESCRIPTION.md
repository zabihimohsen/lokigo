## Summary
Initial `lokigo` v0.1 scaffold for a Go client that pushes logs to Grafana Loki with batching, retries, and configurable backpressure.

## What’s included
- Core client API (`NewClient`, `Send`, `Close`)
- HTTP JSON push transport
- Background batching (entries/bytes/time)
- Retry (exponential backoff + jitter)
- Backpressure modes (`block`, `drop-new`, `drop-oldest`)
- Tests for batching/retry/backpressure behavior
- CI (`go test`, `go vet`, `golangci-lint`)
- Docs: README + architecture notes + changelog

## Notes
- Current scope is intentionally early-stage and marked non-production.
- Queue is in-memory only.
- Payload encoding is JSON (protobuf/snappy planned).

## Validation
- [x] `gofmt -w *.go`
- [x] `go test ./...`
- [x] `go vet ./...`

## Follow-ups
- Add metrics hooks and observability
- Add retry error classification
- Add benchmark and soak-test coverage
