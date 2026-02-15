package lokigo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestBatchingByMaxEntries(t *testing.T) {
	var mu sync.Mutex
	var batchSizes []int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var payload struct {
			Streams []struct {
				Values [][2]string `json:"values"`
			} `json:"streams"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode: %v", err)
		}
		n := 0
		for _, s := range payload.Streams {
			n += len(s.Values)
		}
		mu.Lock()
		batchSizes = append(batchSizes, n)
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c, err := NewClient(Config{Endpoint: srv.URL, Encoding: EncodingJSON, BatchMaxEntries: 3, BatchMaxWait: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 5; i++ {
		if err := c.Send(context.Background(), Entry{Line: "x"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(batchSizes) != 2 || batchSizes[0] != 3 || batchSizes[1] != 2 {
		t.Fatalf("unexpected batch sizes: %#v", batchSizes)
	}
}


func TestFlushesImmediatelyWhenBatchHitsMaxEntries(t *testing.T) {
	requests := make(chan int, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var payload struct {
			Streams []struct {
				Values [][2]string `json:"values"`
			} `json:"streams"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode: %v", err)
		}
		n := 0
		for _, st := range payload.Streams {
			n += len(st.Values)
		}
		requests <- n
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c, err := NewClient(Config{Endpoint: srv.URL, Encoding: EncodingJSON, BatchMaxEntries: 3, BatchMaxWait: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close(context.Background()) }()

	for i := 0; i < 3; i++ {
		if err := c.Send(context.Background(), Entry{Line: "x"}); err != nil {
			t.Fatal(err)
		}
	}

	select {
	case got := <-requests:
		if got != 3 {
			t.Fatalf("expected immediate flush of 3 entries, got %d", got)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected flush as soon as batch reached max entries")
	}
}

func TestRetryEventuallySucceeds(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&attempts, 1)
		if n < 3 {
			http.Error(w, "nope", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c, err := NewClient(Config{
		Endpoint:        srv.URL,
		Encoding:        EncodingJSON,
		BatchMaxEntries: 1,
		Retry:           RetryConfig{MaxAttempts: 4, MinBackoff: 10 * time.Millisecond, MaxBackoff: 20 * time.Millisecond, JitterFrac: 0},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Send(context.Background(), Entry{Line: "retry"}); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&attempts); got != 3 {
		t.Fatalf("expected 3 attempts, got %d", got)
	}
}

func TestRetryStopsOnHTTP400(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		http.Error(w, "bad request", http.StatusBadRequest)
	}))
	defer srv.Close()

	c, err := NewClient(Config{
		Endpoint:        srv.URL,
		Encoding:        EncodingJSON,
		BatchMaxEntries: 1,
		Retry:           RetryConfig{MaxAttempts: 5, MinBackoff: 5 * time.Millisecond, MaxBackoff: 10 * time.Millisecond, JitterFrac: 0},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Send(context.Background(), Entry{Line: "no retry"}); err != nil {
		t.Fatal(err)
	}
	err = c.Close(context.Background())
	if err == nil {
		t.Fatal("expected close error")
	}
	if got := atomic.LoadInt32(&attempts); got != 1 {
		t.Fatalf("expected 1 attempt for http 400, got %d", got)
	}
}

func TestRetryOnHTTP429(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&attempts, 1)
		if n < 3 {
			http.Error(w, "too many", http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c, err := NewClient(Config{
		Endpoint:        srv.URL,
		Encoding:        EncodingJSON,
		BatchMaxEntries: 1,
		Retry:           RetryConfig{MaxAttempts: 4, MinBackoff: 5 * time.Millisecond, MaxBackoff: 10 * time.Millisecond, JitterFrac: 0},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Send(context.Background(), Entry{Line: "retry 429"}); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&attempts); got != 3 {
		t.Fatalf("expected 3 attempts for http 429, got %d", got)
	}
}

func TestOnErrorCallback(t *testing.T) {
	var callbackCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad request", http.StatusBadRequest)
	}))
	defer srv.Close()

	c, err := NewClient(Config{
		Endpoint:        srv.URL,
		Encoding:        EncodingJSON,
		BatchMaxEntries: 1,
		OnError: func(err error) {
			if err != nil {
				atomic.AddInt32(&callbackCount, 1)
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Send(context.Background(), Entry{Line: "fail"}); err != nil {
		t.Fatal(err)
	}
	_ = c.Close(context.Background())
	if got := atomic.LoadInt32(&callbackCount); got == 0 {
		t.Fatal("expected OnError callback to be invoked")
	}
}

func TestBatchingByMaxBytes(t *testing.T) {
	var mu sync.Mutex
	var batchSizes []int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var payload struct {
			Streams []struct {
				Values [][2]string `json:"values"`
			} `json:"streams"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode: %v", err)
		}
		n := 0
		for _, s := range payload.Streams {
			n += len(s.Values)
		}
		mu.Lock()
		batchSizes = append(batchSizes, n)
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c, err := NewClient(Config{Endpoint: srv.URL, Encoding: EncodingJSON, BatchMaxBytes: 4, BatchMaxEntries: 100, BatchMaxWait: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 3; i++ {
		if err := c.Send(context.Background(), Entry{Line: "abc"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(batchSizes) != 3 || batchSizes[0] != 1 || batchSizes[1] != 1 || batchSizes[2] != 1 {
		t.Fatalf("unexpected batch sizes: %#v", batchSizes)
	}
}

func TestTenantIDHeaderIsSent(t *testing.T) {
	const tenant = "acme-tenant"
	seen := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen <- r.Header.Get("X-Scope-OrgID")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c, err := NewClient(Config{Endpoint: srv.URL, Encoding: EncodingJSON, TenantID: tenant, BatchMaxEntries: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Send(context.Background(), Entry{Line: "tenant header"}); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	select {
	case got := <-seen:
		if got != tenant {
			t.Fatalf("expected tenant header %q, got %q", tenant, got)
		}
	default:
		t.Fatal("expected request to be captured")
	}
}

func TestStaticLabelsMergedWithEntryLabelsEntryWins(t *testing.T) {
	var gotStream map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var payload struct {
			Streams []struct {
				Stream map[string]string `json:"stream"`
			} `json:"streams"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(payload.Streams) != 1 {
			t.Fatalf("expected one stream, got %d", len(payload.Streams))
		}
		gotStream = payload.Streams[0].Stream
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c, err := NewClient(Config{
		Endpoint:        srv.URL,
		Encoding:        EncodingJSON,
		BatchMaxEntries: 1,
		StaticLabels: map[string]string{
			"service": "api",
			"env":     "prod",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Send(context.Background(), Entry{Line: "msg", Labels: map[string]string{"service": "worker", "trace_id": "t-1"}}); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	if gotStream["service"] != "worker" {
		t.Fatalf("expected entry label to override static label, got %#v", gotStream)
	}
	if gotStream["env"] != "prod" || gotStream["trace_id"] != "t-1" {
		t.Fatalf("unexpected merged labels: %#v", gotStream)
	}
}

func TestCloseRespectsDeadlineDuringRetry(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "retry", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c, err := NewClient(Config{
		Endpoint:        srv.URL,
		Encoding:        EncodingJSON,
		BatchMaxEntries: 1,
		Retry: RetryConfig{
			MaxAttempts: 10,
			MinBackoff:  100 * time.Millisecond,
			MaxBackoff:  100 * time.Millisecond,
			JitterFrac:  0,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := c.Send(context.Background(), Entry{Line: "will retry"}); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err = c.Close(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected close deadline exceeded, got %v", err)
	}
}

func TestCloseRespectsCanceledContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "retry", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c, err := NewClient(Config{
		Endpoint:        srv.URL,
		Encoding:        EncodingJSON,
		BatchMaxEntries: 1,
		Retry: RetryConfig{
			MaxAttempts: 10,
			MinBackoff:  100 * time.Millisecond,
			MaxBackoff:  100 * time.Millisecond,
			JitterFrac:  0,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := c.Send(context.Background(), Entry{Line: "will retry"}); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = c.Close(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected close canceled, got %v", err)
	}
	if err != nil && strings.Contains(err.Error(), "deadline") {
		t.Fatalf("expected canceled context, got %v", err)
	}
}

func TestOnFlushCallbackReportsRunningTotals(t *testing.T) {
	var last atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c, err := NewClient(Config{
		Endpoint:         srv.URL,
		Encoding:         EncodingJSON,
		QueueSize:        1,
		BatchMaxEntries:  1,
		BackpressureMode: BackpressureDropNew,
		Retry: RetryConfig{
			MaxAttempts: 2,
			MinBackoff:  1 * time.Millisecond,
			MaxBackoff:  1 * time.Millisecond,
			JitterFrac:  0,
		},
		OnFlush: func(m Metrics) {
			last.Store(m)
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := c.Send(context.Background(), Entry{Line: "first"}); err != nil {
		t.Fatal(err)
	}
	if err := c.Send(context.Background(), Entry{Line: "second"}); !errors.Is(err, ErrDropped) {
		t.Fatalf("expected ErrDropped, got %v", err)
	}
	_ = c.Close(context.Background())

	v := last.Load()
	if v == nil {
		t.Fatal("expected OnFlush to be called")
	}
	m := v.(Metrics)
	if m.Dropped == 0 {
		t.Fatalf("expected dropped > 0, got %+v", m)
	}
	if m.PushErrors == 0 {
		t.Fatalf("expected push errors > 0, got %+v", m)
	}
	if m.Retries == 0 {
		t.Fatalf("expected retries > 0, got %+v", m)
	}
}

func TestPushErrorTaxonomySupportsErrorsAs(t *testing.T) {
	t.Run("http status", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "bad request", http.StatusBadRequest)
		}))
		defer srv.Close()

		c, err := NewClient(Config{Endpoint: srv.URL, Encoding: EncodingJSON, BatchMaxEntries: 1})
		if err != nil {
			t.Fatal(err)
		}
		if err := c.Send(context.Background(), Entry{Line: "x"}); err != nil {
			t.Fatal(err)
		}
		err = c.Close(context.Background())
		var statusErr *HTTPStatusPushError
		if !errors.As(err, &statusErr) {
			t.Fatalf("expected HTTPStatusPushError, got %v", err)
		}
		if statusErr.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", statusErr.StatusCode)
		}
	})

	t.Run("network", func(t *testing.T) {
		hc := &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
			return nil, fmt.Errorf("dial: %w", &net.OpError{Op: "dial", Err: errors.New("boom")})
		})}
		c, err := NewClient(Config{Endpoint: "http://example.invalid", Encoding: EncodingJSON, BatchMaxEntries: 1, HTTPClient: hc})
		if err != nil {
			t.Fatal(err)
		}
		if err := c.Send(context.Background(), Entry{Line: "x"}); err != nil {
			t.Fatal(err)
		}
		err = c.Close(context.Background())
		var netErr *NetworkPushError
		if !errors.As(err, &netErr) {
			t.Fatalf("expected NetworkPushError, got %v", err)
		}
	})
}
