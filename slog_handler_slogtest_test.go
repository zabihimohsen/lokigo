package lokigo

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/slogtest"
)

func TestSlogHandlerConformanceWithSlogtest(t *testing.T) {
	var (
		mu    sync.Mutex
		lines []string
	)

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
		mu.Lock()
		for _, s := range payload.Streams {
			for _, v := range s.Values {
				lines = append(lines, v[1])
			}
		}
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	newHandler := func(*testing.T) slog.Handler {
		mu.Lock()
		lines = nil
		mu.Unlock()

		c, err := NewClient(Config{Endpoint: srv.URL, Encoding: EncodingJSON, BatchMaxEntries: 1, BatchMaxWait: 10})
		if err != nil {
			t.Fatalf("new client: %v", err)
		}
		t.Cleanup(func() { _ = c.Close(context.Background()) })
		return NewSlogHandler(c, WithSlogLevel(slog.LevelInfo), WithLabelAllowList(slog.TimeKey, slog.LevelKey, slog.MessageKey))
	}

	result := func(*testing.T) map[string]any {
		mu.Lock()
		defer mu.Unlock()
		if len(lines) == 0 {
			return map[string]any{}
		}
		return parseSlogLine(lines[len(lines)-1])
	}

	slogtest.Run(t, newHandler, result)
}

func parseSlogLine(line string) map[string]any {
	out := map[string]any{}
	parts := strings.Fields(line)
	msg := make([]string, 0, len(parts))
	for _, p := range parts {
		if !strings.Contains(p, "=") {
			msg = append(msg, p)
			continue
		}
		kv := strings.SplitN(p, "=", 2)
		key, val := kv[0], kv[1]
		insertNested(out, key, parseScalar(val))
	}
	if len(msg) > 0 {
		out[slog.MessageKey] = strings.Join(msg, " ")
	}
	if _, ok := out[slog.LevelKey]; !ok {
		out[slog.LevelKey] = "INFO"
	}
	return out
}

func insertNested(root map[string]any, key string, v any) {
	parts := strings.Split(key, ".")
	curr := root
	for i := 0; i < len(parts)-1; i++ {
		n, ok := curr[parts[i]].(map[string]any)
		if !ok {
			n = map[string]any{}
			curr[parts[i]] = n
		}
		curr = n
	}
	curr[parts[len(parts)-1]] = v
}

func parseScalar(s string) any {
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		return i
	}
	if u, err := strconv.ParseUint(s, 10, 64); err == nil {
		return u
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}
	if b, err := strconv.ParseBool(s); err == nil {
		return b
	}
	return s
}
