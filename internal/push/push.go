// Package push contains the minimal Loki protobuf push schema used by lokigo.
//
// This package intentionally vendors only the types required for /loki/api/v1/push
// protobuf+snappy payloads (logproto.PushRequest, Stream, Entry), to avoid pulling
// the full github.com/grafana/loki module tree.
//
// Schema attribution: compatible with Grafana Loki logproto push schema.
package push

import (
	"fmt"
	"time"

	"google.golang.org/protobuf/encoding/protowire"
)

// PushRequest matches Loki's logproto.PushRequest fields used for ingestion.
type PushRequest struct {
	Streams []Stream
	Format  string
}

// Stream matches Loki's stream payload shape.
type Stream struct {
	Labels  string
	Entries []Entry
}

// Entry matches Loki's log entry shape.
type Entry struct {
	Timestamp time.Time
	Line      string
}

func (m *PushRequest) Marshal() ([]byte, error) {
	var out []byte
	for _, s := range m.Streams {
		b, err := s.marshal()
		if err != nil {
			return nil, err
		}
		out = protowire.AppendTag(out, 1, protowire.BytesType)
		out = protowire.AppendBytes(out, b)
	}
	if m.Format != "" {
		out = protowire.AppendTag(out, 2, protowire.BytesType)
		out = protowire.AppendString(out, m.Format)
	}
	return out, nil
}

func (m *PushRequest) Unmarshal(in []byte) error {
	for len(in) > 0 {
		num, typ, n := protowire.ConsumeTag(in)
		if n < 0 {
			return protowire.ParseError(n)
		}
		in = in[n:]
		switch num {
		case 1:
			if typ != protowire.BytesType {
				return fmt.Errorf("push: bad wire type %v for streams", typ)
			}
			msg, n := protowire.ConsumeBytes(in)
			if n < 0 {
				return protowire.ParseError(n)
			}
			in = in[n:]
			var s Stream
			if err := s.unmarshal(msg); err != nil {
				return err
			}
			m.Streams = append(m.Streams, s)
		case 2:
			if typ != protowire.BytesType {
				return fmt.Errorf("push: bad wire type %v for format", typ)
			}
			v, n := protowire.ConsumeString(in)
			if n < 0 {
				return protowire.ParseError(n)
			}
			in = in[n:]
			m.Format = v
		default:
			n := protowire.ConsumeFieldValue(num, typ, in)
			if n < 0 {
				return protowire.ParseError(n)
			}
			in = in[n:]
		}
	}
	return nil
}

func (m *Stream) marshal() ([]byte, error) {
	var out []byte
	if m.Labels != "" {
		out = protowire.AppendTag(out, 1, protowire.BytesType)
		out = protowire.AppendString(out, m.Labels)
	}
	for _, e := range m.Entries {
		b, err := e.marshal()
		if err != nil {
			return nil, err
		}
		out = protowire.AppendTag(out, 2, protowire.BytesType)
		out = protowire.AppendBytes(out, b)
	}
	return out, nil
}

func (m *Stream) unmarshal(in []byte) error {
	for len(in) > 0 {
		num, typ, n := protowire.ConsumeTag(in)
		if n < 0 {
			return protowire.ParseError(n)
		}
		in = in[n:]
		switch num {
		case 1:
			if typ != protowire.BytesType {
				return fmt.Errorf("push: bad wire type %v for labels", typ)
			}
			v, n := protowire.ConsumeString(in)
			if n < 0 {
				return protowire.ParseError(n)
			}
			in = in[n:]
			m.Labels = v
		case 2:
			if typ != protowire.BytesType {
				return fmt.Errorf("push: bad wire type %v for entries", typ)
			}
			msg, n := protowire.ConsumeBytes(in)
			if n < 0 {
				return protowire.ParseError(n)
			}
			in = in[n:]
			var e Entry
			if err := e.unmarshal(msg); err != nil {
				return err
			}
			m.Entries = append(m.Entries, e)
		default:
			n := protowire.ConsumeFieldValue(num, typ, in)
			if n < 0 {
				return protowire.ParseError(n)
			}
			in = in[n:]
		}
	}
	return nil
}

func (m *Entry) marshal() ([]byte, error) {
	var out []byte
	out = protowire.AppendTag(out, 1, protowire.BytesType)
	out = protowire.AppendBytes(out, marshalTimestamp(m.Timestamp))
	if m.Line != "" {
		out = protowire.AppendTag(out, 2, protowire.BytesType)
		out = protowire.AppendString(out, m.Line)
	}
	return out, nil
}

func (m *Entry) unmarshal(in []byte) error {
	for len(in) > 0 {
		num, typ, n := protowire.ConsumeTag(in)
		if n < 0 {
			return protowire.ParseError(n)
		}
		in = in[n:]
		switch num {
		case 1:
			if typ != protowire.BytesType {
				return fmt.Errorf("push: bad wire type %v for timestamp", typ)
			}
			msg, n := protowire.ConsumeBytes(in)
			if n < 0 {
				return protowire.ParseError(n)
			}
			in = in[n:]
			ts, err := unmarshalTimestamp(msg)
			if err != nil {
				return err
			}
			m.Timestamp = ts
		case 2:
			if typ != protowire.BytesType {
				return fmt.Errorf("push: bad wire type %v for line", typ)
			}
			v, n := protowire.ConsumeString(in)
			if n < 0 {
				return protowire.ParseError(n)
			}
			in = in[n:]
			m.Line = v
		default:
			n := protowire.ConsumeFieldValue(num, typ, in)
			if n < 0 {
				return protowire.ParseError(n)
			}
			in = in[n:]
		}
	}
	return nil
}

func marshalTimestamp(ts time.Time) []byte {
	ts = ts.UTC()
	var out []byte
	out = protowire.AppendTag(out, 1, protowire.VarintType)
	out = protowire.AppendVarint(out, uint64(ts.Unix()))
	out = protowire.AppendTag(out, 2, protowire.VarintType)
	out = protowire.AppendVarint(out, uint64(ts.Nanosecond()))
	return out
}

func unmarshalTimestamp(in []byte) (time.Time, error) {
	var sec int64
	var nanos int32
	for len(in) > 0 {
		num, typ, n := protowire.ConsumeTag(in)
		if n < 0 {
			return time.Time{}, protowire.ParseError(n)
		}
		in = in[n:]
		switch num {
		case 1:
			if typ != protowire.VarintType {
				return time.Time{}, fmt.Errorf("push: bad wire type %v for ts.seconds", typ)
			}
			v, n := protowire.ConsumeVarint(in)
			if n < 0 {
				return time.Time{}, protowire.ParseError(n)
			}
			in = in[n:]
			sec = int64(v)
		case 2:
			if typ != protowire.VarintType {
				return time.Time{}, fmt.Errorf("push: bad wire type %v for ts.nanos", typ)
			}
			v, n := protowire.ConsumeVarint(in)
			if n < 0 {
				return time.Time{}, protowire.ParseError(n)
			}
			in = in[n:]
			nanos = int32(v)
		default:
			n := protowire.ConsumeFieldValue(num, typ, in)
			if n < 0 {
				return time.Time{}, protowire.ParseError(n)
			}
			in = in[n:]
		}
	}
	return time.Unix(sec, int64(nanos)).UTC(), nil
}
