# awesome-go submission draft

## Title
Add `lokigo` (lightweight Grafana Loki client for Go) to Logging section

## Description
`lokigo` is a lightweight Grafana Loki client for Go focused on sidecar-less deployments and explicit reliability controls. It supports protobuf+snappy push (default), JSON fallback, built-in `log/slog` handler integration, configurable backpressure/retries, and cardinality-safe label promotion.

## Link
https://github.com/zabihimohsen/lokigo

## Trust/quality signals
- CI: test + vet + lint
- Runnable examples on pkg.go.dev (`example_test.go`)
- Go Report Card badge in README
- Benchmark coverage for payload encoding modes
- `testing/slogtest` conformance coverage
- Tagged releases

## Suggested awesome-go line (Logging section, alphabetical)
- [lokigo](https://github.com/zabihimohsen/lokigo) - Lightweight Grafana Loki client with protobuf+snappy transport, built-in slog handler, configurable backpressure, and 2 Go dependencies.
