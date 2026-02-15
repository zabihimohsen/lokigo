# Announcement draft (Dev.to / Hashnode)

I built **lokigo**: a lightweight Go client for Grafana Loki.

Why I built it:
- In some environments (Railway/Fly/Render), sidecars are not an option.
- I wanted direct in-app Loki push with explicit control over batching/retries/backpressure.
- I also wanted a small dependency footprint and clean `slog` integration.

What it includes:
- Protobuf+snappy push by default (JSON fallback available)
- `log/slog` handler built in
- Cardinality-safe label behavior (allow-list based)
- Retry + backpressure controls
- Optional flush/error callbacks for observability

Quick benchmark signal (500 entries fixture):
- JSON payload: `52,337 bytes/batch`
- Protobuf+snappy payload: `~10,211 bytes/batch`

Repo: https://github.com/zabihimohsen/lokigo

If you run Loki from Go services and can’t use sidecars, feedback is very welcome.
