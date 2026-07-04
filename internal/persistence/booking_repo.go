package persistence

import (
	"context"
	"fmt"

	"github.com/example/booking-engine/internal/domain/booking"
	"github.com/example/booking-engine/internal/eventsourcing"
)

type BookingRepository struct {
	store eventsourcing.Store
}

func NewBookingRepository(store eventsourcing.Store) *BookingRepository {
	return &BookingRepository{store: store}
}

func (r *BookingRepository) Load(ctx context.Context, bookingID string) (*booking.Booking, error) {
	events, err := r.store.Load(ctx, eventsourcing.StreamID(bookingID))
	if err != nil {
		return nil, err
	}
	return booking.LoadFromHistory(events)
}

func (r *BookingRepository) Save(ctx context.Context, b *booking.Booking) error {
	pending := b.Uncommitted()
	if len(pending) == 0 {
		return nil
	}
	expected := b.Version() - int64(len(pending))
	if err := r.store.Append(ctx, eventsourcing.StreamID(b.ID()), expected, pending...); err != nil {
		return fmt.Errorf("save booking %s: %w", b.ID(), err)
	}
	b.ClearUncommitted()
	return nil
}

func (r *BookingRepository) Exists(ctx context.Context, bookingID string) bool {
	_, err := r.store.Load(ctx, eventsourcing.StreamID(bookingID))
	return err == nil
}
