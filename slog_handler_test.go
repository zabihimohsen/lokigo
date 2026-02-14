package lokigo

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSlogHandlerDefaultDoesNotPromoteAttrsToLabels(t *testing.T) {
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
	logger := slog.New(h)
	logger.Warn("login failed", "request_id", "r-123", "trace_id", "t-abc")

	if err := c.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	if got.labels["level"] != "WARN" {
		t.Fatalf("expected level label WARN, got %q", got.labels["level"])
	}
	if _, ok := got.labels["request_id"]; ok {
		t.Fatalf("request_id should not be a label by default: %#v", got.labels)
	}
	if _, ok := got.labels["trace_id"]; ok {
		t.Fatalf("trace_id should not be a label by default: %#v", got.labels)
	}
	if !strings.Contains(got.line, "request_id=r-123") || !strings.Contains(got.line, "trace_id=t-abc") {
		t.Fatalf("expected attrs in line output, got %q", got.line)
	}
}

func TestSlogHandlerLabelAllowListPromotesSelectedAttrsAndGroups(t *testing.T) {
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
		got.labels = payload.Streams[0].Stream
		got.line = payload.Streams[0].Values[0][1]
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c, err := NewClient(Config{Endpoint: srv.URL, BatchMaxEntries: 1})
	if err != nil {
		t.Fatal(err)
	}

	h := NewSlogHandler(c, WithLabelAllowList("request_id", "req.id"))
	logger := slog.New(h)
	logger.Info("request", "request_id", "r-123", slog.Group("req", "id", "42", "trace_id", "t-abc"))

	if err := c.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	if got.labels["level"] != "INFO" {
		t.Fatalf("expected level label INFO, got %q", got.labels["level"])
	}
	if got.labels["request_id"] != "r-123" {
		t.Fatalf("expected request_id label, got %#v", got.labels)
	}
	if got.labels["req.id"] != "42" {
		t.Fatalf("expected req.id grouped label, got %#v", got.labels)
	}
	if _, ok := got.labels["req.trace_id"]; ok {
		t.Fatalf("req.trace_id should not be promoted without allow list entry: %#v", got.labels)
	}
	if !strings.Contains(got.line, "request_id=r-123") || !strings.Contains(got.line, "req.id=42") || !strings.Contains(got.line, "req.trace_id=t-abc") {
		t.Fatalf("expected all attrs in line output, got %q", got.line)
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
