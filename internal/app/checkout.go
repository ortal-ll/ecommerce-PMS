package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/example/booking-engine/internal/domain/booking"
	"github.com/example/booking-engine/internal/eventsourcing"
	"github.com/example/booking-engine/internal/inventory"
	"github.com/example/booking-engine/internal/payment"
	"github.com/example/booking-engine/internal/persistence"
)

var ErrDuplicateBooking = errors.New("booking already exists")

type CheckoutOrchestrator struct {
	bookings *persistence.BookingRepository
	invSvc   inventory.Holder
	payments payment.Authorizer
}

func NewCheckoutOrchestrator(
	bookings *persistence.BookingRepository,
	invSvc inventory.Holder,
	payments payment.Authorizer,
) *CheckoutOrchestrator {
	return &CheckoutOrchestrator{
		bookings: bookings,
		invSvc:   invSvc,
		payments: payments,
	}
}

type CreateBookingCmd struct {
	BookingID  string
	PropertyID string
	GuestID    string
	CheckIn    time.Time
	CheckOut   time.Time
	RoomType   string
	TotalCents int64
	Qty        int
}

func (o *CheckoutOrchestrator) Create(ctx context.Context, cmd CreateBookingCmd) (*booking.Booking, error) {
	if err := booking.ValidateIDs(cmd.BookingID, cmd.PropertyID, cmd.GuestID); err != nil {
		return nil, err
	}
	if err := booking.ValidateCreate(cmd.CheckIn, cmd.CheckOut, cmd.TotalCents, cmd.Qty); err != nil {
		return nil, err
	}
	if o.bookings.Exists(ctx, cmd.BookingID) {
		return nil, fmt.Errorf("%w: %s", ErrDuplicateBooking, cmd.BookingID)
	}

	// inventory before payment: refunding guests is harder than releasing a hold
	comp := &compensation{inv: o.invSvc, pay: o.payments, bookingID: cmd.BookingID}
	defer comp.run(ctx)

	if err := o.invSvc.HoldInventory(ctx, inventory.CheckoutRequest{
		BookingID:  cmd.BookingID,
		PropertyID: cmd.PropertyID,
		RoomType:   cmd.RoomType,
		CheckIn:    cmd.CheckIn,
		CheckOut:   cmd.CheckOut,
		Qty:        cmd.Qty,
	}); err != nil {
		return nil, fmt.Errorf("inventory hold: %w", err)
	}
	comp.holdsDone = true

	b, err := booking.NewBooking(
		cmd.BookingID, cmd.PropertyID, cmd.GuestID,
		cmd.CheckIn, cmd.CheckOut, cmd.RoomType, cmd.TotalCents,
	)
	if err != nil {
		return nil, err
	}

	authID, err := o.payments.Authorize(ctx, cmd.BookingID, cmd.TotalCents)
	if err != nil {
		return nil, fmt.Errorf("payment authorize: %w", err)
	}
	comp.authID = authID

	if err := b.RecordPaymentHold(authID, "card", cmd.TotalCents); err != nil {
		return nil, err
	}
	if err := b.Confirm(); err != nil {
		return nil, err
	}
	if err := o.bookings.Save(ctx, b); err != nil {
		return nil, err
	}

	comp.done = true
	return b, nil
}

type compensation struct {
	inv       inventory.Holder
	pay       payment.Authorizer
	bookingID string
	authID    string
	holdsDone bool
	done      bool
}

func (c *compensation) run(ctx context.Context) {
	if c.done {
		return
	}
	if c.authID != "" {
		_ = c.pay.Void(ctx, c.authID)
	}
	if c.holdsDone {
		_ = c.inv.ReleaseInventory(c.bookingID)
	}
}

func WireProjector(fan *eventsourcing.FanOut, projector *persistence.BookingProjector) {
	fan.Register(eventsourcing.ProjectorSubscriber{
		Handle: projector.Handle,
	})
}
