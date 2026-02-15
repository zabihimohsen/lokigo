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
	"time"
)

type capturedEntry struct {
	labels    map[string]string
	line      string
	timestamp string // nanosecond string from Loki values tuple
}

func TestSlogHandlerConformanceWithSlogtest(t *testing.T) {
	var (
		mu      sync.Mutex
		entries []capturedEntry
		arrived = make(chan struct{}, 256)
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
				entries = append(entries, capturedEntry{
					labels:    s.Stream,
					line:      v[1],
					timestamp: v[0],
				})
			}
		}
		mu.Unlock()
		// Signal that entries have arrived.
		arrived <- struct{}{}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	newHandler := func(*testing.T) slog.Handler {
		mu.Lock()
		entries = nil
		mu.Unlock()
		// Drain any stale signals.
		for {
			select {
			case <-arrived:
			default:
				goto drained
			}
		}
	drained:

		c, err := NewClient(Config{
			Endpoint:        srv.URL,
			Encoding:        EncodingJSON,
			BatchMaxEntries: 1,
			BatchMaxWait:    50 * time.Millisecond, // fast flush for synchronous slogtest assertions
		})
		if err != nil {
			t.Fatalf("new client: %v", err)
		}
		t.Cleanup(func() { _ = c.Close(context.Background()) })
		return NewSlogHandler(c,
			WithSlogLevel(slog.LevelDebug),
			WithLabelAllowList(slog.TimeKey, slog.LevelKey, slog.MessageKey),
		)
	}

	result := func(*testing.T) map[string]any {
		// Wait for the async push to complete.
		select {
		case <-arrived:
		case <-time.After(3 * time.Second):
		}

		mu.Lock()
		defer mu.Unlock()
		if len(entries) == 0 {
			return map[string]any{}
		}
		last := entries[len(entries)-1]
		m := parseSlogLine(last.line)

		// Merge labels into the map (time, level, msg come from labels
		// when the handler promotes them via label allowlist).
		for k, v := range last.labels {
			if _, exists := m[k]; !exists {
				m[k] = v
			}
		}

		// Populate msg — the message is the non-kv prefix of the line.
		// If parseSlogLine already set it, use that. Otherwise fall back to the line.
		if _, has := m[slog.MessageKey]; !has {
			m[slog.MessageKey] = last.line
		}

		return m
	}

	slogtest.Run(t, newHandler, result)
}

func parseSlogLine(line string) map[string]any {
	out := map[string]any{}
	parts := strings.Fields(line)
	var msgParts []string
	for _, p := range parts {
		if !strings.Contains(p, "=") {
			msgParts = append(msgParts, p)
			continue
		}
		kv := strings.SplitN(p, "=", 2)
		key, val := kv[0], kv[1]
		insertNested(out, key, parseScalar(val))
	}
	if len(msgParts) > 0 {
		out[slog.MessageKey] = strings.Join(msgParts, " ")
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
	// Only parse unambiguous bool literals; strconv.ParseBool treats "f" as false
	// which would corrupt single-char string values like slog.String("e", "f").
	if s == "true" || s == "false" {
		return s == "true"
	}
	return s
}
