package lokigo

import (
	"context"
	"testing"
	"time"
)

func TestBackpressureDropNew(t *testing.T) {
	ch := make(chan Entry, 1)
	ch <- Entry{Line: "old"}
	err := enqueueWithMode(context.Background(), ch, Entry{Line: "new"}, BackpressureDropNew)
	if err != errDroppedInternal {
		t.Fatalf("expected dropped err, got %v", err)
	}
	got := <-ch
	if got.Line != "old" {
		t.Fatalf("expected old entry kept, got %q", got.Line)
	}
}

func TestBackpressureDropOldest(t *testing.T) {
	ch := make(chan Entry, 1)
	ch <- Entry{Line: "old"}
	if err := enqueueWithMode(context.Background(), ch, Entry{Line: "new"}, BackpressureDropOldest); err != nil {
		t.Fatal(err)
	}
	got := <-ch
	if got.Line != "new" {
		t.Fatalf("expected new entry in queue, got %q", got.Line)
	}
}

func TestBackpressureBlockRespectsContext(t *testing.T) {
	ch := make(chan Entry, 1)
	ch <- Entry{Line: "full"}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	err := enqueueWithMode(ctx, ch, Entry{Line: "blocked"}, BackpressureBlock)
	if err == nil {
		t.Fatal("expected context timeout error")
	}
}
