package booking_test

import (
	"errors"
	"testing"
	"time"

	"github.com/example/booking-engine/internal/domain/booking"
	"github.com/example/booking-engine/internal/eventsourcing"
)

func mustBooking(t *testing.T) *booking.Booking {
	t.Helper()
	checkIn := time.Date(2026, 9, 1, 14, 0, 0, 0, time.UTC)
	checkOut := checkIn.Add(48 * time.Hour)
	b, err := booking.NewBooking("bk-t", "prop-1", "guest-1", checkIn, checkOut, "std", 20000)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestBookingLifecycle(t *testing.T) {
	b := mustBooking(t)
	if b.Status() != booking.StatusDraft {
		t.Fatalf("status=%s", b.Status())
	}
	if b.Version() != 1 {
		t.Fatalf("version=%d", b.Version())
	}

	if err := b.RecordPaymentHold("auth-1", "card", 20000); err != nil {
		t.Fatal(err)
	}
	if err := b.Confirm(); err != nil {
		t.Fatal(err)
	}
	if b.Status() != booking.StatusConfirmed {
		t.Fatalf("status=%s", b.Status())
	}
	if len(b.Uncommitted()) != 3 {
		t.Fatalf("uncommitted=%d want 3", len(b.Uncommitted()))
	}
}

func TestReplayMatchesMutations(t *testing.T) {
	checkIn := time.Date(2026, 9, 1, 14, 0, 0, 0, time.UTC)
	checkOut := checkIn.Add(48 * time.Hour)

	b1, err := booking.NewBooking("bk-r", "prop-1", "guest-1", checkIn, checkOut, "std", 20000)
	if err != nil {
		t.Fatal(err)
	}
	evs := append([]eventsourcing.Event{}, b1.Uncommitted()...)

	if err := b1.RecordPaymentHold("a", "card", 20000); err != nil {
		t.Fatal(err)
	}
	evs = append(evs, b1.Uncommitted()[1:]...)

	if err := b1.Confirm(); err != nil {
		t.Fatal(err)
	}
	evs = append(evs, b1.Uncommitted()[2:]...)

	replayed, err := booking.LoadFromHistory(evs)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.Status() != booking.StatusConfirmed {
		t.Fatalf("status=%s", replayed.Status())
	}
	if replayed.Version() != 3 {
		t.Fatalf("version=%d", replayed.Version())
	}
}

func TestInvalidTransitions(t *testing.T) {
	b := mustBooking(t)
	_ = b.Confirm()

	err := b.Confirm()
	if !errors.Is(err, booking.ErrInvalidTransition) {
		t.Fatalf("err=%v", err)
	}
}

func TestCancelIdempotent(t *testing.T) {
	b := mustBooking(t)
	_ = b.Confirm()
	_ = b.Cancel("a", true)
	vBefore := b.Version()
	_ = b.Cancel("b", false)
	if b.Version() != vBefore {
		t.Fatal("cancel should not emit twice")
	}
}
