# No-sidecar deployments (Railway, Render, Fly.io, etc.)

Many managed platforms do not support running a dedicated log shipper sidecar (Promtail / Alloy / Fluent Bit) next to your app process.

`lokigo` is designed for this model: push to Loki directly from your Go app while still controlling:

- batching (`BatchMaxEntries`, `BatchMaxBytes`, `BatchMaxWait`)
- retry policy (`Retry`)
- backpressure behavior (`block`, `drop-new`, `drop-oldest`)
- auth and tenant headers (`Headers`, `TenantID`)

## Typical hosted Loki config

```go
client, err := lokigo.NewClient(lokigo.Config{
	Endpoint: "https://logs-prod-xxx.grafana.net/loki/api/v1/push",
	Headers: map[string]string{
		"Authorization": "Basic <base64(instance_id:api_token)>",
	},
	StaticLabels: map[string]string{
		"service": "api",
		"env":     "prod",
		"platform": "railway",
	},
})
if err != nil {
	panic(err)
}
defer client.Close(context.Background())
```

## Operational notes

- Keep labels low-cardinality (`service`, `env`, `region`), not per-request IDs.
- For noisy outages, tune queue/backpressure explicitly to avoid unbounded memory use.
- Prefer default `EncodingProtobufSnappy` for lower wire size; switch to JSON only when debugging payloads.
