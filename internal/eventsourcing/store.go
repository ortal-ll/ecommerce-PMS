package eventsourcing

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

var (
	ErrStreamNotFound    = errors.New("stream not found")
	ErrConcurrency       = errors.New("optimistic concurrency conflict")
	ErrDuplicateEvent    = errors.New("duplicate event id")
)

// Store is append-only. projections read from here (or from a relay).
type Store interface {
	Append(ctx context.Context, stream StreamID, expectedVersion int64, events ...Event) error
	Load(ctx context.Context, stream StreamID) ([]Event, error)
	LoadFrom(ctx context.Context, stream StreamID, fromVersion int64) ([]Event, error)
}

// InMemoryStore enforces stream_id+version uniqueness like migrations/schema_v1.sql.
type InMemoryStore struct {
	mu      sync.RWMutex
	streams map[StreamID][]Event
}

func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{streams: make(map[StreamID][]Event)}
}

func (s *InMemoryStore) Append(ctx context.Context, stream StreamID, expectedVersion int64, events ...Event) error {
	_ = ctx // ctx wired for real impl (timeouts, tracing)

	s.mu.Lock()
	defer s.mu.Unlock()

	existing := s.streams[stream]
	currentVer := int64(0)
	if len(existing) > 0 {
		currentVer = existing[len(existing)-1].Version
	}

	if expectedVersion != currentVer {
		return fmt.Errorf("%w: stream %s at v%d, expected %d", ErrConcurrency, stream, currentVer, expectedVersion)
	}

	for _, ev := range events {
		if ev.Version != currentVer+1 {
			return fmt.Errorf("event version gap: got %d want %d", ev.Version, currentVer+1)
		}
		currentVer = ev.Version
		s.streams[stream] = append(s.streams[stream], ev)
	}
	return nil
}

func (s *InMemoryStore) Load(ctx context.Context, stream StreamID) ([]Event, error) {
	_ = ctx
	s.mu.RLock()
	defer s.mu.RUnlock()

	evs, ok := s.streams[stream]
	if !ok || len(evs) == 0 {
		return nil, ErrStreamNotFound
	}
	out := make([]Event, len(evs))
	copy(out, evs)
	return out, nil
}

func (s *InMemoryStore) LoadFrom(ctx context.Context, stream StreamID, fromVersion int64) ([]Event, error) {
	all, err := s.Load(ctx, stream)
	if err != nil {
		return nil, err
	}
	var out []Event
	for _, ev := range all {
		if ev.Version > fromVersion {
			out = append(out, ev)
		}
	}
	return out, nil
}
