package lokigo

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

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

	c, err := NewClient(Config{Endpoint: srv.URL, BatchMaxEntries: 3, BatchMaxWait: 5 * time.Second})
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
