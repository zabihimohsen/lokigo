package lokigo

import (
	"fmt"
	"testing"
	"time"
)

func benchmarkEntries(n int) []Entry {
	base := time.Unix(1700000000, 0).UTC()
	entries := make([]Entry, 0, n)
	for i := 0; i < n; i++ {
		entries = append(entries, Entry{
			Timestamp: base.Add(time.Duration(i) * time.Millisecond),
			Line:      fmt.Sprintf("level=info service=api request=%d user=u-%03d path=/v1/items/%d latency_ms=%d", i, i%97, i%10, i%500),
			Labels: map[string]string{
				"service": "api",
				"env":     "bench",
				"stream":  fmt.Sprintf("s%d", i%8),
			},
		})
	}
	return entries
}

func BenchmarkPayloadBuildEncode_JSON_500Entries(b *testing.B) {
	entries := benchmarkEntries(500)
	c, err := NewClient(Config{Endpoint: "http://127.0.0.1:3100/loki/api/v1/push", Encoding: EncodingJSON})
	if err != nil {
		b.Fatal(err)
	}
	defer c.cancel()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		payload, contentType, contentEncoding, err := c.buildPayload(entries)
		if err != nil {
			b.Fatal(err)
		}
		if len(payload) == 0 || contentType != "application/json" || contentEncoding != "" {
			b.Fatal("unexpected json payload metadata")
		}
	}
}

func BenchmarkPayloadBuildEncode_ProtobufSnappy_500Entries(b *testing.B) {
	entries := benchmarkEntries(500)
	c, err := NewClient(Config{Endpoint: "http://127.0.0.1:3100/loki/api/v1/push", Encoding: EncodingProtobufSnappy})
	if err != nil {
		b.Fatal(err)
	}
	defer c.cancel()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		payload, contentType, contentEncoding, err := c.buildPayload(entries)
		if err != nil {
			b.Fatal(err)
		}
		if len(payload) == 0 || contentType != "application/x-protobuf" || contentEncoding != "snappy" {
			b.Fatal("unexpected protobuf payload metadata")
		}
	}
}
