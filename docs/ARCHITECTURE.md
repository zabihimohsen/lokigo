# lokigo v0.1 architecture

`lokigo` is a minimal, learning-focused Grafana Loki client prototype.

## Package layout

- `config.go` - client/retry/batching/backpressure config and defaults
- `client.go` - client API (`NewClient`, `Send`, `Close`), worker loop, Loki push payload building
- `backpressure.go` - enqueue behavior for `block`, `drop-new`, `drop-oldest`
- `retry.go` - exponential backoff with jitter and retry loop
- `*_test.go` - behavioral tests for batching, retry, and backpressure
- `.github/workflows/ci.yml` - CI for test/vet/lint

## Data flow

1. caller invokes `Send(ctx, Entry)`
2. entry is enqueued using configured backpressure mode
3. background worker drains queue into in-memory batch
4. batch flush happens when any trigger is hit:
   - max entries
   - max bytes (line byte size approximation)
   - max wait interval
5. flush posts JSON payload to Loki `/loki/api/v1/push`
6. on transient/HTTP failure, retry with exponential backoff + jitter

## Notes

- This intentionally uses plain JSON push (no protobuf/snappy optimization yet).
- Durability guarantees are minimal (in-memory queue only).
