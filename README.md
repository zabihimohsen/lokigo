# lokigo (v0.1 scaffold)

> **Non-production warning:** this is an early scaffold for experimentation and API design. It is not hardened for production workloads.

`lokigo` is a lightweight Go client for Grafana Loki with:

- background batching (size/time based)
- retry with exponential backoff + jitter
- configurable backpressure (`block`, `drop-new`, `drop-oldest`)

## Quickstart

```go
package main

import (
    "context"
    "time"

    "github.com/yourorg/lokigo"
)

func main() {
    c, err := lokigo.NewClient(lokigo.Config{
        Endpoint:         "http://localhost:3100/loki/api/v1/push",
        StaticLabels:     map[string]string{"app": "demo"},
        BatchMaxEntries:  500,
        BatchMaxWait:     time.Second,
        BackpressureMode: lokigo.BackpressureDropOldest,
    })
    if err != nil {
        panic(err)
    }

    _ = c.Send(context.Background(), lokigo.Entry{Line: "hello loki"})
    _ = c.Close(context.Background())
}
```

## v0.1 behavior

- queue is in-memory only
- batches are serialized as Loki JSON push payload
- retries run per-batch with bounded exponential backoff
- `Close` flushes pending entries and returns the last flush error (if any)

## Development

```bash
go test ./...
```

## Roadmap

- [ ] protobuf + snappy push encoding
- [ ] better transient/permanent error classification
- [ ] metrics hooks (drop counts, queue depth, retry stats)
- [ ] graceful shutdown with drain deadline options
- [ ] richer label and stream APIs
- [ ] benchmarks and soak tests
