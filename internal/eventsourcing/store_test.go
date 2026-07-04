package eventsourcing_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/example/booking-engine/internal/eventsourcing"
)

func TestAppendEnforcesExpectedVersion(t *testing.T) {
	ctx := context.Background()
	store := eventsourcing.NewInMemoryStore()
	stream := eventsourcing.StreamID("s-1")

	ev1, err := eventsourcing.NewEvent(stream, "test.created", 1, map[string]string{"x": "1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Append(ctx, stream, 0, ev1); err != nil {
		t.Fatal(err)
	}

	ev2, _ := eventsourcing.NewEvent(stream, "test.updated", 2, map[string]string{"x": "2"})
	err = store.Append(ctx, stream, 0, ev2) // stale expected
	if !errors.Is(err, eventsourcing.ErrConcurrency) {
		t.Fatalf("err=%v", err)
	}
}

func TestConcurrentAppendSingleWinner(t *testing.T) {
	ctx := context.Background()
	store := eventsourcing.NewInMemoryStore()
	stream := eventsourcing.StreamID("race-stream")

	ev1, _ := eventsourcing.NewEvent(stream, "e", 1, "a")
	if err := store.Append(ctx, stream, 0, ev1); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	var wins atomic.Int32
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			ev, _ := eventsourcing.NewEvent(stream, "e", 2, n)
			if err := store.Append(ctx, stream, 1, ev); err == nil {
				wins.Add(1)
			}
		}(i)
	}
	wg.Wait()

	if wins.Load() != 1 {
		t.Fatalf("expected exactly 1 append at v2, got %d", wins.Load())
	}
}

func TestNotifyingStoreFansOut(t *testing.T) {
	ctx := context.Background()
	inner := eventsourcing.NewInMemoryStore()
	fan := eventsourcing.NewFanOut()
	store := eventsourcing.NewNotifyingStore(inner, fan)

	var handled atomic.Int32
	fan.Register(eventsourcing.ProjectorSubscriber{
		Handle: func(ctx context.Context, ev eventsourcing.Event) error {
			handled.Add(1)
			return nil
		},
	})

	stream := eventsourcing.StreamID("n-1")
	ev, _ := eventsourcing.NewEvent(stream, "x", 1, "payload")
	if err := store.Append(ctx, stream, 0, ev); err != nil {
		t.Fatal(err)
	}
	if handled.Load() != 1 {
		t.Fatalf("handled=%d", handled.Load())
	}
}
