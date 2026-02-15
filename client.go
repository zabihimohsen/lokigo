package lokigo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang/snappy"
	"github.com/zabihimohsen/lokigo/internal/push"
)

var ErrDropped = errors.New("entry dropped due to backpressure")

type Entry struct {
	Timestamp time.Time
	Line      string
	Labels    map[string]string
}

type NetworkPushError struct {
	Err error
}

func (e *NetworkPushError) Error() string { return e.Err.Error() }
func (e *NetworkPushError) Unwrap() error { return e.Err }

type HTTPStatusPushError struct {
	StatusCode int
	Body       string
}

func (e *HTTPStatusPushError) Error() string {
	return fmt.Sprintf("loki push failed: %d %s", e.StatusCode, e.Body)
}

type Client struct {
	cfg    Config
	queue  chan Entry
	cancel context.CancelFunc
	wg     sync.WaitGroup

	dropped    atomic.Uint64
	pushed     atomic.Uint64
	pushErrors atomic.Uint64
	retries    atomic.Uint64

	errMu   sync.Mutex
	lastErr error
}

func NewClient(cfg Config) (*Client, error) {
	cfg.setDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	c := &Client{cfg: cfg, queue: make(chan Entry, cfg.QueueSize), cancel: cancel}
	c.wg.Add(1)
	go c.run(ctx)
	return c, nil
}

func (c *Client) Send(ctx context.Context, e Entry) error {
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now().UTC()
	}
	dropped, err := enqueueWithMode(ctx, c.queue, e, c.cfg.BackpressureMode)
	if dropped > 0 {
		c.dropped.Add(uint64(dropped))
		c.reportFlushMetrics()
	}
	if err != nil {
		if errors.Is(err, errDroppedInternal) {
			return ErrDropped
		}
		return err
	}
	return nil
}

func (c *Client) Close(ctx context.Context) error {
	c.cancel()
	done := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		return ctx.Err()
	}
	c.errMu.Lock()
	defer c.errMu.Unlock()
	return c.lastErr
}

const (
	// If a temporary spike causes the batch backing array to grow far beyond the
	// normal target, shrink it after flush so long-lived clients don't retain
	// oversized memory indefinitely.
	batchReuseShrinkFactor = 4
)

func (c *Client) run(ctx context.Context) {
	defer c.wg.Done()
	ticker := time.NewTicker(c.cfg.BatchMaxWait)
	defer ticker.Stop()

	baselineCap := c.cfg.BatchMaxEntries
	batch := make([]Entry, 0, baselineCap)
	batchBytes := 0

	flush := func(flushCtx context.Context) {
		if len(batch) == 0 {
			return
		}
		if err := c.pushWithRetry(flushCtx, batch); err != nil {
			c.setErr(err)
		}
		if cap(batch) > baselineCap*batchReuseShrinkFactor {
			batch = make([]Entry, 0, baselineCap)
		} else {
			batch = batch[:0]
		}
		batchBytes = 0
	}

	for {
		select {
		case <-ctx.Done():
			// Drain any buffered entries that were accepted before shutdown.
			for {
				select {
				case e := <-c.queue:
					lineSize := len(e.Line)
					if len(batch) >= c.cfg.BatchMaxEntries || (batchBytes+lineSize) > c.cfg.BatchMaxBytes {
						flush(context.Background())
					}
					batch = append(batch, e)
					batchBytes += lineSize
				default:
					flush(context.Background())
					return
				}
			}
		case <-ticker.C:
			flush(context.Background())
		case e := <-c.queue:
			lineSize := len(e.Line)
			if len(batch) >= c.cfg.BatchMaxEntries || (batchBytes+lineSize) > c.cfg.BatchMaxBytes {
				flush(context.Background())
			}
			batch = append(batch, e)
			batchBytes += lineSize
		}
	}
}

func (c *Client) pushWithRetry(ctx context.Context, entries []Entry) error {
	payload, contentType, contentEncoding, err := c.buildPayload(entries)
	if err != nil {
		return err
	}
	return doRetry(ctx, c.cfg.Retry, func(attempt int) error {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.Endpoint, bytes.NewReader(payload))
		if err != nil {
			c.pushErrors.Add(uint64(len(entries)))
			if attempt > 0 {
				c.retries.Add(1)
			}
			c.reportFlushMetrics()
			return err
		}
		req.Header.Set("Content-Type", contentType)
		if contentEncoding != "" {
			req.Header.Set("Content-Encoding", contentEncoding)
		}
		for k, v := range c.cfg.Headers {
			req.Header.Set(k, v)
		}
		if c.cfg.TenantID != "" {
			req.Header.Set("X-Scope-OrgID", c.cfg.TenantID)
		}
		resp, err := c.cfg.HTTPClient.Do(req)
		if err != nil {
			c.pushErrors.Add(uint64(len(entries)))
			if attempt > 0 {
				c.retries.Add(1)
			}
			c.reportFlushMetrics()
			return &NetworkPushError{Err: err}
		}
		defer resp.Body.Close()
		if resp.StatusCode/100 != 2 {
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
			c.pushErrors.Add(uint64(len(entries)))
			if attempt > 0 {
				c.retries.Add(1)
			}
			c.reportFlushMetrics()
			return &HTTPStatusPushError{StatusCode: resp.StatusCode, Body: string(b)}
		}
		c.pushed.Add(uint64(len(entries)))
		if attempt > 0 {
			c.retries.Add(1)
		}
		c.reportFlushMetrics()
		return nil
	})
}

func (c *Client) reportFlushMetrics() {
	if c.cfg.OnFlush == nil {
		return
	}
	c.cfg.OnFlush(Metrics{
		Dropped:    c.dropped.Load(),
		Pushed:     c.pushed.Load(),
		PushErrors: c.pushErrors.Load(),
		Retries:    c.retries.Load(),
	})
}

func (c *Client) buildPayload(entries []Entry) ([]byte, string, string, error) {
	switch c.cfg.Encoding {
	case EncodingJSON:
		payload, err := c.buildJSONPayload(entries)
		return payload, "application/json", "", err
	case EncodingProtobufSnappy:
		payload, err := c.buildProtobufSnappyPayload(entries)
		return payload, "application/x-protobuf", "snappy", err
	default:
		return nil, "", "", fmt.Errorf("unsupported encoding %q", c.cfg.Encoding)
	}
}

func (c *Client) buildJSONPayload(entries []Entry) ([]byte, error) {
	type stream struct {
		Stream map[string]string `json:"stream"`
		Values [][2]string       `json:"values"`
	}
	groups := map[string]*stream{}
	for _, e := range entries {
		labels := mergeLabels(c.cfg.StaticLabels, e.Labels)
		keyBytes, _ := json.Marshal(labels)
		key := string(keyBytes)
		s, ok := groups[key]
		if !ok {
			s = &stream{Stream: labels}
			groups[key] = s
		}
		ts := fmt.Sprintf("%d", e.Timestamp.UnixNano())
		s.Values = append(s.Values, [2]string{ts, e.Line})
	}
	out := struct {
		Streams []stream `json:"streams"`
	}{Streams: make([]stream, 0, len(groups))}
	for _, s := range groups {
		out.Streams = append(out.Streams, *s)
	}
	return json.Marshal(out)
}

func (c *Client) buildProtobufSnappyPayload(entries []Entry) ([]byte, error) {
	groups := map[string]*push.Stream{}
	for _, e := range entries {
		labels := mergeLabels(c.cfg.StaticLabels, e.Labels)
		labelSet := toLokiLabelSet(labels)
		s, ok := groups[labelSet]
		if !ok {
			s = &push.Stream{Labels: labelSet}
			groups[labelSet] = s
		}
		s.Entries = append(s.Entries, push.Entry{Timestamp: e.Timestamp, Line: e.Line})
	}
	req := push.PushRequest{Streams: make([]push.Stream, 0, len(groups))}
	for _, s := range groups {
		req.Streams = append(req.Streams, *s)
	}
	raw, err := req.Marshal()
	if err != nil {
		return nil, err
	}
	return snappy.Encode(nil, raw), nil
}

func toLokiLabelSet(labels map[string]string) string {
	if len(labels) == 0 {
		return "{}"
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%q", k, labels[k]))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func mergeLabels(a, b map[string]string) map[string]string {
	if len(a) == 0 && len(b) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(a)+len(b))
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}

func (c *Client) setErr(err error) {
	c.errMu.Lock()
	c.lastErr = err
	onError := c.cfg.OnError
	c.errMu.Unlock()
	if onError != nil {
		onError(err)
	}
}
