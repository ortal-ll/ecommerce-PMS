package eventsourcing

import (
	"context"
	"sync"
)

// Subscriber receives events after successful append. Wire projector/outbox here.
type Subscriber interface {
	OnAppended(ctx context.Context, stream StreamID, events []Event) error
}

type FanOut struct {
	mu   sync.RWMutex
	subs []Subscriber
}

func NewFanOut() *FanOut {
	return &FanOut{}
}

func (f *FanOut) Register(sub Subscriber) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.subs = append(f.subs, sub)
}

func (f *FanOut) OnAppended(ctx context.Context, stream StreamID, events []Event) error {
	f.mu.RLock()
	subs := append([]Subscriber(nil), f.subs...)
	f.mu.RUnlock()

	for _, sub := range subs {
		if err := sub.OnAppended(ctx, stream, events); err != nil {
			return err
		}
	}
	return nil
}

// NotifyingStore: production swaps fan-out for same-txn outbox insert.
type NotifyingStore struct {
	inner Store
	fan   *FanOut
}

func NewNotifyingStore(inner Store, fan *FanOut) *NotifyingStore {
	return &NotifyingStore{inner: inner, fan: fan}
}

func (s *NotifyingStore) Append(ctx context.Context, stream StreamID, expectedVersion int64, events ...Event) error {
	if err := s.inner.Append(ctx, stream, expectedVersion, events...); err != nil {
		return err
	}
	return s.fan.OnAppended(ctx, stream, events)
}

func (s *NotifyingStore) Load(ctx context.Context, stream StreamID) ([]Event, error) {
	return s.inner.Load(ctx, stream)
}

func (s *NotifyingStore) LoadFrom(ctx context.Context, stream StreamID, fromVersion int64) ([]Event, error) {
	return s.inner.LoadFrom(ctx, stream, fromVersion)
}

// ProjectorSubscriber adapts BookingProjector-like handlers.
type ProjectorSubscriber struct {
	Handle func(ctx context.Context, ev Event) error
}

func (p ProjectorSubscriber) OnAppended(ctx context.Context, stream StreamID, events []Event) error {
	for _, ev := range events {
		if err := p.Handle(ctx, ev); err != nil {
			return err
		}
	}
	return nil
}
