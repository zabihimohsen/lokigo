# Grafana Community Forum draft

## Title
Running Loki from Go services on Railway (no sidecars) with `lokigo`

## Body
I built `lokigo`, a lightweight Go client for Grafana Loki, for environments where sidecars/agents are difficult or unavailable (for example Railway).

What I needed:
- direct in-process push to Loki
- predictable batching/retry/backpressure controls
- low dependency surface
- clean `log/slog` integration

Current highlights:
- protobuf+snappy default transport (JSON fallback)
- `slog` adapter with allow-list label promotion (cardinality-safe)
- retry strategy for transient failures (`429`, `5xx`, network)
- benchmark fixture showing much smaller payload size with protobuf+snappy

Repo: https://github.com/zabihimohsen/lokigo

Would love feedback from teams running Loki in sidecar-less platforms. If you have production constraints I should account for, I’d like to incorporate them into docs and defaults.
