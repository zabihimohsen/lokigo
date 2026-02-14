package lokigo

import (
	"context"
	"math"
	"math/rand"
	"time"
)

func doRetry(ctx context.Context, cfg RetryConfig, fn func() error) error {
	var lastErr error
	for i := 0; i < cfg.MaxAttempts; i++ {
		if err := fn(); err == nil {
			return nil
		} else {
			lastErr = err
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
