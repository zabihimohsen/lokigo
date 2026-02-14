package lokigo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

var ErrDropped = errors.New("entry dropped due to backpressure")

type Entry struct {
	Timestamp time.Time
	Line      string
	Labels    map[string]string
}

type networkPushError struct {
	err error
}

func (e *networkPushError) Error() string { return e.err.Error() }
func (e *networkPushError) Unwrap() error { return e.err }

type httpStatusPushError struct {
	StatusCode int
	Body       string
}

func (e *httpStatusPushError) Error() string {
	return fmt.Sprintf("loki push failed: %d %s", e.StatusCode, e.Body)
}

type Client struct {
	cfg    Config
	queue  chan Entry
	cancel context.CancelFunc
	wg     sync.WaitGroup

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
	if err := enqueueWithMode(ctx, c.queue, e, c.cfg.BackpressureMode); err != nil {
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

func (c *Client) run(ctx context.Context) {
	defer c.wg.Done()
	ticker := time.NewTicker(c.cfg.BatchMaxWait)
	defer ticker.Stop()

	batch := make([]Entry, 0, c.cfg.BatchMaxEntries)
	batchBytes := 0

	flush := func(flushCtx context.Context) {
		if len(batch) == 0 {
			return
		}
		if err := c.pushWithRetry(flushCtx, batch); err != nil {
			c.setErr(err)
		}
		batch = batch[:0]
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
	payload, err := c.buildPayload(entries)
	if err != nil {
		return err
	}
	return doRetry(ctx, c.cfg.Retry, func() error {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.Endpoint, bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		if c.cfg.TenantID != "" {
			req.Header.Set("X-Scope-OrgID", c.cfg.TenantID)
		}
		resp, err := c.cfg.HTTPClient.Do(req)
		if err != nil {
			return &networkPushError{err: err}
		}
		defer resp.Body.Close()
		if resp.StatusCode/100 != 2 {
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
			return &httpStatusPushError{StatusCode: resp.StatusCode, Body: string(b)}
		}
		return nil
	})
}

func (c *Client) buildPayload(entries []Entry) ([]byte, error) {
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
