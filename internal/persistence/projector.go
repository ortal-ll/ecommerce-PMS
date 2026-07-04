package persistence

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/example/booking-engine/internal/domain/booking"
	"github.com/example/booking-engine/internal/eventsourcing"
)

type BookingReadModel struct {
	BookingID  string
	PropertyID string
	GuestID    string
	Status     booking.Status
	TotalCents int64
	Version    int64
}

type BookingProjector struct {
	mu    sync.RWMutex
	views map[string]BookingReadModel
}

func NewBookingProjector() *BookingProjector {
	return &BookingProjector{views: make(map[string]BookingReadModel)}
}

func (p *BookingProjector) Handle(ctx context.Context, ev eventsourcing.Event) error {
	_ = ctx
	p.mu.Lock()
	defer p.mu.Unlock()

	vm, ok := p.views[string(ev.StreamID)]
	if !ok {
		vm = BookingReadModel{BookingID: string(ev.StreamID)}
	}

	switch booking.EventType(ev.Type) {
	case booking.EvCreated:
		var payload booking.Created
		if err := json.Unmarshal(ev.Payload, &payload); err != nil {
			return err
		}
		vm.PropertyID = payload.PropertyID
		vm.GuestID = payload.GuestID
		vm.TotalCents = payload.TotalCents
		vm.Status = booking.StatusDraft

	case booking.EvConfirmed:
		vm.Status = booking.StatusConfirmed

	case booking.EvCancelled:
		vm.Status = booking.StatusCancelled
	}

	vm.Version = ev.Version
	p.views[vm.BookingID] = vm
	return nil
}

func (p *BookingProjector) Get(bookingID string) (BookingReadModel, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	v, ok := p.views[bookingID]
	return v, ok
}

func (p *BookingProjector) Rebuild(ctx context.Context, store eventsourcing.Store, streams []eventsourcing.StreamID) error {
	p.mu.Lock()
	p.views = make(map[string]BookingReadModel)
	p.mu.Unlock()

	for _, sid := range streams {
		events, err := store.Load(ctx, sid)
		if err != nil {
			return err
		}
		for _, ev := range events {
			if err := p.Handle(ctx, ev); err != nil {
				return err
			}
		}
	}
	return nil
}
