# lokigo (v0.1 scaffold)

> **Non-production warning:** this is an early scaffold for experimentation and API design. It is **not** hardened for production workloads yet.

`lokigo` is a Go client for Grafana Loki with:

- background batching (entry count / bytes / max wait)
- retry with exponential backoff + jitter
- configurable backpressure (`block`, `drop-new`, `drop-oldest`)
- `log/slog` handler adapter for direct integration

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

handler := lokigo.NewSlogHandler(client, lokigo.WithSlogLevel(slog.LevelInfo))
logger := slog.New(handler).With("service", "api").WithGroup("http")

logger.Info("request complete", "status", 200, "path", "/health")
```

`NewSlogHandler` maps `slog.Record` to `lokigo.Entry`:

- timestamp -> `Entry.Timestamp`
- message plus rendered attrs -> `Entry.Line`
- attrs (including groups) -> `Entry.Labels` (grouped keys are flattened with `.`)
- record level -> `level` label (configurable/optional with `WithSlogLevelLabel`)

## v0.1 behavior

- queue is in-memory only
- batches are serialized as Loki JSON push payload
- retries run per-batch with bounded exponential backoff
- retry classification for push errors:
  - retries on network errors
  - retries on HTTP `429` and `5xx`
  - does not retry other `4xx`
- `Config.OnError` (optional) is called when async flush/push fails
- `Close` drains queued entries, flushes pending data, and returns the last flush error (if any)

## Development

```bash
go test ./...
go vet ./...
```

## Roadmap

- [ ] protobuf + snappy push encoding
- [ ] metrics hooks (drop counts, queue depth, retry stats)
- [ ] graceful shutdown with drain deadline options
- [ ] richer label and stream APIs
- [ ] benchmarks and soak tests
