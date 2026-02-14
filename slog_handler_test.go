package lokigo

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSlogHandlerMapsRecordToEntry(t *testing.T) {
	type captured struct {
		labels map[string]string
		line   string
	}
	got := captured{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var payload struct {
			Streams []struct {
				Stream map[string]string `json:"stream"`
				Values [][2]string       `json:"values"`
			} `json:"streams"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(payload.Streams) != 1 || len(payload.Streams[0].Values) != 1 {
			t.Fatalf("unexpected payload: %+v", payload)
		}
		got.labels = payload.Streams[0].Stream
		got.line = payload.Streams[0].Values[0][1]
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c, err := NewClient(Config{Endpoint: srv.URL, BatchMaxEntries: 1})
	if err != nil {
		t.Fatal(err)
	}

	h := NewSlogHandler(c)
	logger := slog.New(h).With("app", "demo").WithGroup("req")
	logger.Warn("login failed", "id", "42", "retry", true)

	if err := c.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	if got.labels["level"] != "WARN" {
		t.Fatalf("expected level label WARN, got %q", got.labels["level"])
	}
	if got.labels["app"] != "demo" {
		t.Fatalf("expected app label demo, got %q", got.labels["app"])
	}
	if got.labels["req.id"] != "42" || got.labels["req.retry"] != "true" {
		t.Fatalf("expected grouped labels, got %#v", got.labels)
	}
	if got.line == "" || got.line[:12] != "login failed" {
		t.Fatalf("expected formatted line with message, got %q", got.line)
	}
}

func TestSlogHandlerLevelFilter(t *testing.T) {
	c, err := NewClient(Config{Endpoint: "http://127.0.0.1:1"})
	if err != nil {
		t.Fatal(err)
	}
	h := NewSlogHandler(c, WithSlogLevel(slog.LevelError))
	if h.Enabled(context.Background(), slog.LevelInfo) {
		t.Fatal("expected info to be disabled")
	}
	if !h.Enabled(context.Background(), slog.LevelError) {
		t.Fatal("expected error to be enabled")
	}
}
