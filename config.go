package lokigo

import (
	"errors"
	"net/http"
	"time"
)

type BackpressureMode string

const (
	BackpressureBlock      BackpressureMode = "block"
	BackpressureDropNew    BackpressureMode = "drop-new"
	BackpressureDropOldest BackpressureMode = "drop-oldest"
)

type RetryConfig struct {
	MaxAttempts int
	MinBackoff  time.Duration
	MaxBackoff  time.Duration
	JitterFrac  float64
}

type Config struct {
	Endpoint         string
	TenantID         string
	StaticLabels     map[string]string
	HTTPClient       *http.Client
	QueueSize        int
	BatchMaxEntries  int
	BatchMaxBytes    int
	BatchMaxWait     time.Duration
	BackpressureMode BackpressureMode
	Retry            RetryConfig
	// OnError is called when async background flush/push fails.
	// It is optional and must be safe for concurrent use.
	OnError func(error)
}

func (c *Config) setDefaults() {
	if c.HTTPClient == nil {
		c.HTTPClient = &http.Client{Timeout: 10 * time.Second}
	}
	if c.QueueSize <= 0 {
		c.QueueSize = 1024
	}
	if c.BatchMaxEntries <= 0 {
		c.BatchMaxEntries = 500
	}
	if c.BatchMaxBytes <= 0 {
		c.BatchMaxBytes = 1 << 20 // 1MB
	}
	if c.BatchMaxWait <= 0 {
		c.BatchMaxWait = 1 * time.Second
	}
	if c.BackpressureMode == "" {
		c.BackpressureMode = BackpressureBlock
	}
	if c.Retry.MaxAttempts <= 0 {
		c.Retry.MaxAttempts = 5
	}
	if c.Retry.MinBackoff <= 0 {
		c.Retry.MinBackoff = 100 * time.Millisecond
	}
	if c.Retry.MaxBackoff <= 0 {
		c.Retry.MaxBackoff = 3 * time.Second
	}
	if c.Retry.JitterFrac <= 0 {
		c.Retry.JitterFrac = 0.2
	}
}

func (c Config) validate() error {
	if c.Endpoint == "" {
		return errors.New("endpoint is required")
	}
	switch c.BackpressureMode {
	case BackpressureBlock, BackpressureDropNew, BackpressureDropOldest:
	default:
		return errors.New("invalid backpressure mode")
	}
	if c.Retry.MaxAttempts < 1 {
		return errors.New("retry.maxAttempts must be >= 1")
	}
	return nil
}
