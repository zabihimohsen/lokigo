package lokigo

import (
	"context"
	"errors"
	"math"
	"math/rand"
	"time"
)

func doRetry(ctx context.Context, cfg RetryConfig, fn func(attempt int) error) error {
	var lastErr error
	for i := 0; i < cfg.MaxAttempts; i++ {
		if err := fn(i); err == nil {
			return nil
		} else {
			lastErr = err
			if !shouldRetryPushError(err) {
				return err
			}
		}
		if i == cfg.MaxAttempts-1 {
			break
		}
		wait := backoffWithJitter(cfg, i)
		t := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		case <-t.C:
		}
	}
	return lastErr
}

func shouldRetryPushError(err error) bool {
	if err == nil {
		return false
	}
	var netErr *NetworkPushError
	if errors.As(err, &netErr) {
		return true
	}
	var statusErr *HTTPStatusPushError
	if errors.As(err, &statusErr) {
		return statusErr.StatusCode == 429 || statusErr.StatusCode >= 500
	}
	return false
}

func backoffWithJitter(cfg RetryConfig, attempt int) time.Duration {
	base := float64(cfg.MinBackoff) * math.Pow(2, float64(attempt))
	if max := float64(cfg.MaxBackoff); base > max {
		base = max
	}
	jitter := 1 + ((rand.Float64()*2 - 1) * cfg.JitterFrac)
	if jitter < 0 {
		jitter = 0
	}
	return time.Duration(base * jitter)
}
