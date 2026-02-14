package lokigo

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang/snappy"
	"github.com/zabihimohsen/lokigo/internal/push"
)

func TestDefaultEncodingIsProtobufSnappy(t *testing.T) {
	var gotContentType, gotContentEncoding string
	var decoded push.PushRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		gotContentEncoding = r.Header.Get("Content-Encoding")
		defer r.Body.Close()
		compressed, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		raw, err := snappy.Decode(nil, compressed)
		if err != nil {
			t.Fatalf("snappy decode: %v", err)
		}
		if err := decoded.Unmarshal(raw); err != nil {
			t.Fatalf("protobuf unmarshal: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c, err := NewClient(Config{Endpoint: srv.URL, BatchMaxEntries: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Send(context.Background(), Entry{Line: "hello", Labels: map[string]string{"service": "api"}}); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	if gotContentType != "application/x-protobuf" {
		t.Fatalf("expected application/x-protobuf, got %q", gotContentType)
	}
	if gotContentEncoding != "snappy" {
		t.Fatalf("expected snappy content-encoding, got %q", gotContentEncoding)
	}
	if len(decoded.Streams) != 1 || len(decoded.Streams[0].Entries) != 1 {
		t.Fatalf("unexpected decoded protobuf payload: %#v", decoded)
	}
	if decoded.Streams[0].Entries[0].Line != "hello" {
		t.Fatalf("unexpected entry line: %q", decoded.Streams[0].Entries[0].Line)
	}
}

func TestHeadersAppliedToEveryRequestTenantIDPrecedence(t *testing.T) {
	seen := make(chan http.Header, 2)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clone := make(http.Header, len(r.Header))
		for k, v := range r.Header {
			clone[k] = append([]string(nil), v...)
		}
		seen <- clone
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c, err := NewClient(Config{
		Endpoint:        srv.URL,
		Encoding:        EncodingJSON,
		BatchMaxEntries: 1,
		TenantID:        "tenant-from-config",
		Headers: map[string]string{
			"Authorization": "Basic Z3JhZmFuYTpzZWNyZXQ=",
			"X-Scope-OrgID": "tenant-from-headers",
			"X-Custom":      "yes",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Send(context.Background(), Entry{Line: "one"}); err != nil {
		t.Fatal(err)
	}
	if err := c.Send(context.Background(), Entry{Line: "two"}); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 2; i++ {
		h := <-seen
		if got := h.Get("Authorization"); got != "Basic Z3JhZmFuYTpzZWNyZXQ=" {
			t.Fatalf("request %d: missing authorization header, got %q", i+1, got)
		}
		if got := h.Get("X-Custom"); got != "yes" {
			t.Fatalf("request %d: missing custom header, got %q", i+1, got)
		}
		if got := h.Get("X-Scope-OrgID"); got != "tenant-from-config" {
			t.Fatalf("request %d: tenant precedence broken, got %q", i+1, got)
		}
	}
}
