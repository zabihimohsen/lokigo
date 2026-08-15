# Railway production quickstart (no sidecar)

This guide is for teams running Go services on Railway where Loki sidecars/agents are not available.

## 1) Environment variables

Set these in Railway service variables:

```bash
LOKI_ENDPOINT=https://logs-prod-XXX.grafana.net/loki/api/v1/push
LOKI_AUTH_BASIC=<base64(instance_id:api_token)>
APP_NAME=my-service
APP_ENV=prod
```

## 2) Minimal production config

```go
package logging

import (
	"os"
	"time"

	"github.com/zabihimohsen/lokigo"
)

func NewLokiClient() (*lokigo.Client, error) {
	return lokigo.NewClient(lokigo.Config{
		Endpoint: os.Getenv("LOKI_ENDPOINT"),
		Headers: map[string]string{
			"Authorization": "Basic " + os.Getenv("LOKI_AUTH_BASIC"),
		},
		StaticLabels: map[string]string{
			"service": os.Getenv("APP_NAME"),
			"env":     os.Getenv("APP_ENV"),
			"runtime": "railway",
		},
		Encoding:         lokigo.EncodingProtobufSnappy,
		QueueSize:        2048,
		BatchMaxEntries:  500,
		BatchMaxBytes:    1 << 20,
		BatchMaxWait:     1 * time.Second,
		BackpressureMode: lokigo.BackpressureDropOldest,
		Retry: lokigo.RetryConfig{
			MaxAttempts: 6,
			InitialBackoff: 200 * time.Millisecond,
			MaxBackoff: 5 * time.Second,
		},
	})
}
```

## 3) slog wiring with cardinality-safe labels

```go
handler := lokigo.NewSlogHandler(
	client,
	lokigo.WithLabelAllowList("service", "env", "http.method", "http.status"),
)
logger := slog.New(handler).With("service", os.Getenv("APP_NAME"), "env", os.Getenv("APP_ENV"))
```

Avoid using `request_id`, `trace_id`, user IDs, and raw URL paths as labels. Keep those in the log line.

## 4) Shutdown handling

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
_ = client.Close(ctx)
```

This gives the background worker a chance to drain and flush queued entries.

## 5) Operational checklist

- Use protobuf+snappy unless you need JSON inspection.
- Keep `BatchMaxWait` low (500ms–2s) for app logs.
- Track drop/error counters via callbacks (`OnFlush`, `OnError`).
- Verify labels are bounded to avoid Loki stream explosion.
