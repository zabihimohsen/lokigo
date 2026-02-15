package lokigo

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

type SlogHandlerOption func(*slogHandlerConfig)

type slogHandlerConfig struct {
	level      slog.Leveler
	levelLabel string
	labelAllow map[string]struct{}
	labelDeny  map[string]struct{}
}

// WithSlogLevel sets the minimum level this handler accepts.
func WithSlogLevel(level slog.Leveler) SlogHandlerOption {
	return func(c *slogHandlerConfig) { c.level = level }
}

// WithSlogLevelLabel sets the label key used to store slog level.
// Set to empty string to disable level labels.
func WithSlogLevelLabel(label string) SlogHandlerOption {
	return func(c *slogHandlerConfig) { c.levelLabel = label }
}

// WithLabelAllowList configures which slog attrs are promoted to Loki labels.
//
// Keys must use flattened dot notation for grouped attrs (for example: "http.status").
// By default, no attrs are promoted to labels.
func WithLabelAllowList(keys ...string) SlogHandlerOption {
	return func(c *slogHandlerConfig) {
		if c.labelAllow == nil {
			c.labelAllow = map[string]struct{}{}
		}
		for _, key := range keys {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			c.labelAllow[key] = struct{}{}
		}
	}
}

// WithLabelDenyList configures slog attrs that should never be promoted to Loki labels.
//
// Deny list has precedence over allow list.
func WithLabelDenyList(keys ...string) SlogHandlerOption {
	return func(c *slogHandlerConfig) {
		if c.labelDeny == nil {
			c.labelDeny = map[string]struct{}{}
		}
		for _, key := range keys {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			c.labelDeny[key] = struct{}{}
		}
	}
}

// NewSlogHandler adapts lokigo.Client to slog.Handler.
//
// It maps slog.Record to lokigo.Entry:
//   - timestamp -> Entry.Timestamp
//   - message + attrs -> Entry.Line
//   - allow-listed attrs/groups (+ optional level) -> Entry.Labels
func NewSlogHandler(client *Client, opts ...SlogHandlerOption) slog.Handler {
	cfg := slogHandlerConfig{level: slog.LevelInfo, levelLabel: "level"}
	for _, opt := range opts {
		opt(&cfg)
	}
	return &slogHandler{client: client, cfg: cfg}
}

type slogHandler struct {
	client *Client
	cfg    slogHandlerConfig
	attrs  []slog.Attr
	group  []string
}

func (h *slogHandler) Enabled(_ context.Context, level slog.Level) bool {
	if h.cfg.level == nil {
		return true
	}
	return level >= h.cfg.level.Level()
}

func (h *slogHandler) Handle(ctx context.Context, r slog.Record) error {
	labels := map[string]string{}
	parts := make([]string, 0, r.NumAttrs()+1)

	if h.cfg.levelLabel != "" {
		labels[h.cfg.levelLabel] = r.Level.String()
	}
	// Promote record time to labels when allow-listed and non-zero.
	if !r.Time.IsZero() && h.shouldPromoteToLabel(slog.TimeKey) {
		labels[slog.TimeKey] = r.Time.Format(time.RFC3339Nano)
	}
	// Promote message to labels when allow-listed and non-empty.
	if r.Message != "" && h.shouldPromoteToLabel(slog.MessageKey) {
		labels[slog.MessageKey] = r.Message
	}
	if r.Message != "" {
		parts = append(parts, r.Message)
	}

	for _, a := range h.attrs {
		h.collectAttr(labels, &parts, nil, a)
	}
	r.Attrs(func(a slog.Attr) bool {
		h.collectAttr(labels, &parts, h.group, a)
		return true
	})

	line := strings.Join(parts, " ")
	if line == "" {
		line = "log entry"
	}
	ts := r.Time
	if ts.IsZero() {
		ts = time.Now().UTC()
	}
	return h.client.Send(ctx, Entry{Timestamp: ts, Line: line, Labels: labels})
}

func (h *slogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := *h
	grouped := prefixAttrsWithGroup(attrs, h.group)
	next.attrs = append(append([]slog.Attr{}, h.attrs...), grouped...)
	return &next
}

func (h *slogHandler) WithGroup(name string) slog.Handler {
	next := *h
	next.group = append(append([]string{}, h.group...), name)
	return &next
}

func (h *slogHandler) collectAttr(labels map[string]string, parts *[]string, group []string, attr slog.Attr) {
	attr.Value = attr.Value.Resolve()
	if attr.Equal(slog.Attr{}) {
		return
	}
	if attr.Value.Kind() == slog.KindGroup {
		nextGroup := group
		if attr.Key != "" {
			nextGroup = append(append([]string{}, group...), attr.Key)
		}
		for _, ga := range attr.Value.Group() {
			h.collectAttr(labels, parts, nextGroup, ga)
		}
		return
	}
	key := attr.Key
	if len(group) > 0 {
		key = strings.Join(append(append([]string{}, group...), attr.Key), ".")
	}
	if key == "" {
		return
	}
	val := valueToString(attr.Value)
	if h.shouldPromoteToLabel(key) {
		labels[key] = val
	}
	*parts = append(*parts, fmt.Sprintf("%s=%s", key, val))
}

func (h *slogHandler) shouldPromoteToLabel(key string) bool {
	if _, denied := h.cfg.labelDeny[key]; denied {
		return false
	}
	if len(h.cfg.labelAllow) == 0 {
		return false
	}
	_, allowed := h.cfg.labelAllow[key]
	return allowed
}

func prefixAttrsWithGroup(attrs []slog.Attr, group []string) []slog.Attr {
	if len(group) == 0 {
		return append([]slog.Attr{}, attrs...)
	}
	out := make([]slog.Attr, 0, len(attrs))
	for _, a := range attrs {
		a.Value = a.Value.Resolve()
		if a.Value.Kind() == slog.KindGroup {
			prefixedGroup := append(append([]string{}, group...), a.Key)
			out = append(out, slog.Attr{Value: slog.GroupValue(prefixAttrsWithGroup(a.Value.Group(), prefixedGroup)...)})
			continue
		}
		if a.Key != "" {
			a.Key = strings.Join(append(append([]string{}, group...), a.Key), ".")
		}
		out = append(out, a)
	}
	return out
}

func valueToString(v slog.Value) string {
	switch v.Kind() {
	case slog.KindString:
		return v.String()
	case slog.KindInt64:
		return fmt.Sprintf("%d", v.Int64())
	case slog.KindUint64:
		return fmt.Sprintf("%d", v.Uint64())
	case slog.KindFloat64:
		return fmt.Sprintf("%g", v.Float64())
	case slog.KindBool:
		return fmt.Sprintf("%t", v.Bool())
	case slog.KindDuration:
		return v.Duration().String()
	case slog.KindTime:
		return v.Time().Format(time.RFC3339Nano)
	default:
		return fmt.Sprint(v.Any())
	}
}
