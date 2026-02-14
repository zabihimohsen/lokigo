# lokigo v0.1 architecture

`lokigo` is a minimal, learning-focused Grafana Loki client prototype.

## Package layout

- `config.go` - client/retry/batching/backpressure config and defaults
- `client.go` - client API (`NewClient`, `Send`, `Close`), worker loop, Loki push payload building
- `slog_handler.go` - `log/slog` adapter (`NewSlogHandler`) that maps records to `Entry`
- `backpressure.go` - enqueue behavior for `block`, `drop-new`, `drop-oldest`
- `retry.go` - exponential backoff with jitter and retry classification helpers
- `*_test.go` - behavioral tests for batching, retry, backpressure, and slog mapping
- `.github/workflows/ci.yml` - CI for test/vet/lint

## Data flow

1. caller invokes `Send(ctx, Entry)` directly or through `slog.Handler`
2. entry is enqueued using configured backpressure mode
3. background worker drains queue into in-memory batch
4. batch flush happens when any trigger is hit:
   - max entries
   - max bytes (line byte size approximation)
   - max wait interval
5. flush posts JSON payload to Loki `/loki/api/v1/push`
6. retry logic retries only on transient push errors:
   - network errors
   - HTTP `429`
   - HTTP `5xx`
7. when async flush fails, latest error is stored and optional `Config.OnError` callback is invoked

## slog handler notes

`NewSlogHandler(client, opts...)` provides a lightweight adapter:

- record time is used as entry timestamp (fallback: `time.Now().UTC()`)
- line format is `"<message> key=value ..."`
- attrs are extracted into labels
- group nesting is flattened using dots, e.g. `request.id`
- a level label is included by default (`level`), configurable via options

## Notes

- This intentionally uses plain JSON push (no protobuf/snappy optimization yet).
- Durability guarantees are minimal (in-memory queue only).
