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
- attrs are always rendered into the line output
- by default, attrs are **not** promoted to labels (cardinality-safe default)
- group nesting is flattened using dots, e.g. `request.id`
- allow-list based promotion is available via `WithLabelAllowList(...)`
- optional hard exclusions are available via `WithLabelDenyList(...)`
- a level label is included by default (`level`), configurable via options

### Loki cardinality guidance

Loki stream labels should be low-cardinality and bounded (service, environment, region, status class, etc.).

Do not label high-cardinality values such as request IDs, trace IDs, user IDs, or unbounded path/query values unless you explicitly accept higher stream churn and cost. Keep those fields in the log line for search/filter.

Example:

- good labels: `service`, `env`, `http.method`, `http.status`
- keep in line: `request_id`, `trace_id`, `user_id`, `http.path` (if unbounded)

## Notes

- This intentionally uses plain JSON push (no protobuf/snappy optimization yet).
- Durability guarantees are minimal (in-memory queue only).
