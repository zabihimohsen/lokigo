# lokigo

[![Go Reference](https://pkg.go.dev/badge/github.com/zabihimohsen/lokigo.svg)](https://pkg.go.dev/github.com/zabihimohsen/lokigo)
[![Go Report Card](https://goreportcard.com/badge/github.com/zabihimohsen/lokigo)](https://goreportcard.com/report/github.com/zabihimohsen/lokigo)

Lightweight Grafana Loki client for Go with protobuf+snappy transport, built-in `slog` integration, and explicit reliability controls.

> **Stability notice:** `lokigo` follows semantic versioning. Breaking changes are reserved for major releases.

`lokigo` is a Go client for Grafana Loki with:

- background batching (entry count / bytes / max wait)
- retry with exponential backoff + jitter
- configurable backpressure (`block`, `drop-new`, `drop-oldest`)
- `log/slog` handler adapter for direct integration

## Why lokigo / use cases

`lokigo` is most useful when you **cannot rely on sidecars/agents** (for example on platforms like **Railway**) but still want reliable, controlled delivery to Loki from inside your Go service.

Typical use cases:

- Platforms/environments where sidecars are not available
- Lightweight services that want to avoid heavy logging dependency trees
- Teams needing explicit control over retry/backpressure behavior in application code
- `slog`-based apps that want direct Loki integration with cardinality-safe labels

## Docs

- [Architecture](docs/ARCHITECTURE.md)
- [No-sidecar platforms (Railway/Render/Fly)](docs/NO_SIDECAR.md)

## Stability & compatibility

- **Versioning:** semantic versioning (`MAJOR.MINOR.PATCH`).
- **Core surface intended to stay stable:** `Config`, `NewClient`, `Send`, `Close`, and `NewSlogHandler`.
- **Go version:** see `go.mod` for the current minimum supported Go version.

## Install

```bash
go get github.com/zabihimohsen/lokigo
```

## Quickstart

```go
package main

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/zabihimohsen/lokigo"
)

func main() {
	c, err := lokigo.NewClient(lokigo.Config{
		Endpoint:         "http://localhost:3100/loki/api/v1/push",
		StaticLabels:     map[string]string{"app": "demo", "env": "dev"},
		Encoding:         lokigo.EncodingProtobufSnappy, // default
		Headers:          map[string]string{"Authorization": "Bearer <token>"},
		QueueSize:        1024,
		BatchMaxEntries:  500,
		BatchMaxBytes:    1 << 20,
		BatchMaxWait:     time.Second,
		BackpressureMode: lokigo.BackpressureDropOldest,
	})
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := c.Send(ctx, lokigo.Entry{Line: "hello loki"}); err != nil {
		if errors.Is(err, lokigo.ErrDropped) {
			log.Println("log dropped due to backpressure")
		} else {
			log.Fatal(err)
		}
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := c.Close(shutdownCtx); err != nil {
		log.Fatal(err)
	}
}
```

## slog integration

```go
client, _ := lokigo.NewClient(lokigo.Config{
	Endpoint: "http://localhost:3100/loki/api/v1/push",
})

handler := lokigo.NewSlogHandler(
	client,
	lokigo.WithSlogLevel(slog.LevelInfo),
	lokigo.WithLabelAllowList("service", "http.status"),
)
logger := slog.New(handler).With("service", "api").WithGroup("http")

logger.Info("request complete", "status", 200, "path", "/health", "request_id", "r-123")
```

`NewSlogHandler` maps `slog.Record` to `lokigo.Entry`:

- timestamp -> `Entry.Timestamp`
- message plus rendered attrs -> `Entry.Line`
- level -> `level` label by default (configurable/optional with `WithSlogLevelLabel`)
- attrs/groups -> labels only when explicitly allow-listed via `WithLabelAllowList`

### Loki label cardinality guidance

Loki labels define stream cardinality. High-cardinality values (for example `request_id`, `trace_id`, user IDs, session IDs, URLs with unbounded parameters) should usually **stay in the log line**, not labels.

`lokigo` keeps attrs in `Entry.Line` even when they are not labels, so context is preserved without exploding stream count.

Use an allow list to promote only stable, bounded dimensions:

```go
handler := lokigo.NewSlogHandler(
	client,
	lokigo.WithLabelAllowList("service", "http.method", "http.status"),
)
```

Optional hard block for sensitive/high-cardinality fields:

```go
handler := lokigo.NewSlogHandler(
	client,
	lokigo.WithLabelAllowList("service", "trace_id"),
	lokigo.WithLabelDenyList("trace_id"),
)
```

## Transport + headers

`lokigo` now supports two push encodings:

- `EncodingProtobufSnappy` (default): sends `application/x-protobuf` with `Content-Encoding: snappy`
- `EncodingJSON`: sends classic Loki JSON payload (`application/json`)

Example (Grafana Cloud-style basic auth):

```go
client, _ := lokigo.NewClient(lokigo.Config{
	Endpoint: "https://logs-prod-xxx.grafana.net/loki/api/v1/push",
	Headers: map[string]string{
		"Authorization": "Basic <base64(instance_id:api_token)>",
	},
})
```

Custom headers are applied to every push request via `Config.Headers`.

`TenantID` is still mapped to `X-Scope-OrgID` and takes precedence over a same-named key in `Headers`.

## Current behavior

- queue is in-memory only
- retries run per-batch with bounded exponential backoff
- **flush/retry blocking:** each flush attempt (size-triggered, ticker-triggered, or shutdown drain) runs synchronously in the single background worker. while a batch is retrying, that worker is blocked until the batch succeeds or reaches `Retry.MaxAttempts`.
- retry classification for push errors:
  - retries on `*lokigo.NetworkPushError`
  - retries on `*lokigo.HTTPStatusPushError` when status is `429` or `5xx`
  - does not retry other `4xx`
- `Config.OnError` (optional) is called when async flush/push ultimately fails
- `Config.OnFlush` (optional) receives running counters: `Dropped`, `Pushed`, `PushErrors`, `Retries`
  - callback cadence is **per flush attempt/outcome** (including retries), not just per logical batch
  - each retry attempt that errors increments `PushErrors`; successful retry completion increments `Pushed`
  - `Retries` increments on attempts after the first (both failed retry attempts and successful retry completion)
- `Close` drains queued entries, flushes pending data, and returns the last flush error (if any)
- `Close(ctx)` respects caller context: if flush/retry is still in progress and `ctx` expires/cancels first, `Close` returns that context error

## Migration notes

- Default wire format changed from JSON to protobuf+snappy for lower payload size and better Loki-native compatibility.
- If you depend on inspecting raw JSON request bodies (tests/proxies), set `Encoding: lokigo.EncodingJSON`.
- Header injection moved into first-class config (`Config.Headers`) so auth/proxy headers no longer require custom `http.RoundTripper` wrappers.

## Tradeoffs

- Protobuf+snappy reduces wire size, but request payloads are less human-readable while debugging.
- JSON is easier to inspect manually, but tends to be larger over the network.

## Benchmark snapshot

`go test -bench=. -benchmem -count=3 ./...`

| Benchmark (500 entries) | Encode time (ns/op, avg of 3) | Payload size (`bytes/batch`, avg of 3) | allocs/op (avg of 3) |
|---|---:|---:|---:|
| JSON | ~1,255,704 | ~52,337 | ~6,625 |
| Protobuf + Snappy | ~1,527,502 | ~10,211 | ~9,169 |

Notes:
- Results are from this repo's benchmark fixture and are hardware/runtime dependent.
- The key signal is wire size: protobuf+snappy is ~5x smaller payload in this benchmark.

## Development

```bash
go test ./...
go vet ./...
```

## Roadmap

- [x] protobuf + snappy push encoding (with optional JSON mode)
- [x] per-request custom headers (`Config.Headers`)
- [x] metrics callback hook (`Config.OnFlush`) for running push counters
- [ ] graceful shutdown with drain deadline options
- [ ] richer label and stream APIs
- [x] benchmark coverage for payload build/encode modes
