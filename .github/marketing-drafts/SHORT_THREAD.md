# Short thread draft (X/LinkedIn)

1) I built `lokigo`: a focused Go client for Grafana Loki.

2) Why: on platforms like Railway, sidecars/agents are often not practical.

3) Design goals:
- direct in-app Loki push
- explicit batching/retry/backpressure
- `slog` integration
- low dependency surface

4) Transport defaults to protobuf+snappy (with JSON fallback).
In benchmark fixture payload size dropped from ~52 KB to ~10 KB per batch.

5) Repo: https://github.com/zabihimohsen/lokigo
If you run Loki from Go in sidecar-less environments, feedback is welcome.
