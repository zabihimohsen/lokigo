package lokigo_test

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/zabihimohsen/lokigo"
)

func ExampleNewClient_basic() {
	client, err := lokigo.NewClient(lokigo.Config{
		Endpoint: "http://localhost:3100/loki/api/v1/push",
		StaticLabels: map[string]string{
			"service": "api",
			"env":     "dev",
		},
	})
	if err != nil {
		panic(err)
	}
	defer client.Close(context.Background())

	_ = client.Send(context.Background(), lokigo.Entry{Line: "hello from lokigo"})
}

func ExampleNewSlogHandler() {
	client, err := lokigo.NewClient(lokigo.Config{
		Endpoint:        "http://localhost:3100/loki/api/v1/push",
		Encoding:        lokigo.EncodingJSON,
		BatchMaxEntries: 1,
	})
	if err != nil {
		panic(err)
	}
	defer client.Close(context.Background())

	h := lokigo.NewSlogHandler(
		client,
		lokigo.WithSlogLevel(slog.LevelInfo),
		lokigo.WithLabelAllowList("service", "http.status"),
	)
	logger := slog.New(h).With("service", "api").WithGroup("http")
	logger.Info("request complete", "status", 200, "path", "/health")
}

func ExampleNewClient_hostedAuthHeader() {
	client, err := lokigo.NewClient(lokigo.Config{
		Endpoint: "https://logs-prod-012.grafana.net/loki/api/v1/push",
		Headers: map[string]string{
			"Authorization": "Basic <base64(instance_id:token)>",
		},
		BatchMaxWait: time.Second,
	})
	if err != nil {
		panic(err)
	}
	defer client.Close(context.Background())

	slog.New(slog.NewTextHandler(os.Stdout, nil)).Info("configured client for hosted Loki")
}
