package saga_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/example/booking-engine/internal/domain/booking"
	"github.com/example/booking-engine/internal/app"
	"github.com/example/booking-engine/internal/eventsourcing"
	"github.com/example/booking-engine/internal/inventory"
	"github.com/example/booking-engine/internal/payment"
	"github.com/example/booking-engine/internal/persistence"
	"github.com/example/booking-engine/internal/saga"
)

func TestCancelSagaFailsWhenPaymentVoidUnavailable(t *testing.T) {
	ctx := context.Background()
	store := eventsourcing.NewInMemoryStore()
	bookings := persistence.NewBookingRepository(store)
	inv := inventory.NewInventory()
	log := inventory.NewReservationLog()
	invSvc := inventory.NewBookingService(inv, log)
	payBase := payment.NewGateway()

	checkIn := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	checkOut := checkIn.Add(24 * time.Hour)
	inv.Seed(inventory.SlotKey{PropertyID: "p", RoomType: "r", Date: checkIn}, 3)

	checkout := app.NewCheckoutOrchestrator(bookings, invSvc, payBase)
	_, err := checkout.Create(ctx, app.CreateBookingCmd{
		BookingID: "bk-void-fail", PropertyID: "p", GuestID: "g",
		CheckIn: checkIn, CheckOut: checkOut, RoomType: "r",
		TotalCents: 8000, Qty: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	pay := payment.NewFaultGateway(payBase, payment.FaultConfig{FailVoid: true})
	coord := saga.NewCancelCoordinator(bookings, invSvc, pay, saga.NewInMemorySagaStore())

	sg, err := coord.Start(ctx, saga.CancelRequest{BookingID: "bk-void-fail", Reason: "x", ByGuest: true})
	if err == nil {
		t.Fatal("expected void failure")
	}
	if !errors.Is(err, payment.ErrUnavailable) {
		t.Fatalf("err=%v", err)
	}
	if sg.State != saga.SagaFailed {
		t.Fatalf("state=%s", sg.State)
	}

	b, _ := bookings.Load(ctx, "bk-void-fail")
	if b.Status() != booking.StatusConfirmed {
		t.Fatalf("booking should stay confirmed on partial saga fail, got %s", b.Status())
	}
}

func setupCancelledBooking(t *testing.T) (*saga.CancelCoordinator, string, *inventory.Inventory) {
	t.Helper()
	ctx := context.Background()

	store := eventsourcing.NewInMemoryStore()
	bookings := persistence.NewBookingRepository(store)
	inv := inventory.NewInventory()
	log := inventory.NewReservationLog()
	invSvc := inventory.NewBookingService(inv, log)
	pay := payment.NewGateway()

	checkIn := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	checkOut := checkIn.Add(2 * 24 * time.Hour)
	for d := checkIn; d.Before(checkOut); d = d.Add(24 * time.Hour) {
		inv.Seed(inventory.SlotKey{PropertyID: "p", RoomType: "r", Date: d}, 5)
	}

	checkout := app.NewCheckoutOrchestrator(bookings, invSvc, pay)
	_, err := checkout.Create(ctx, app.CreateBookingCmd{
		BookingID: "bk-saga", PropertyID: "p", GuestID: "g",
		CheckIn: checkIn, CheckOut: checkOut, RoomType: "r",
		TotalCents: 10000, Qty: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	coord := saga.NewCancelCoordinator(bookings, invSvc, pay, saga.NewInMemorySagaStore())
	return coord, "bk-saga", inv
}

func TestCancelSagaCompletesAndRestoresStock(t *testing.T) {
	coord, bookingID, inv := setupCancelledBooking(t)
	ctx := context.Background()

	checkIn := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	key := inventory.SlotKey{PropertyID: "p", RoomType: "r", Date: checkIn}
	afterCheckout, _, _ := inv.Peek(key)
	if afterCheckout != 4 {
		t.Fatalf("after checkout avail=%d want 4", afterCheckout)
	}

	sg, err := coord.Start(ctx, saga.CancelRequest{BookingID: bookingID, Reason: "no show", ByGuest: false})
	if err != nil {
		t.Fatal(err)
	}
	if sg.State != saga.SagaCompleted {
		t.Fatalf("state=%s", sg.State)
	}

	afterCancel, _, _ := inv.Peek(key)
	if afterCancel != 5 {
		t.Fatalf("stock not restored: got %d want 5", afterCancel)
	}
}

func TestCancelSagaIdempotent(t *testing.T) {
	coord, bookingID, _ := setupCancelledBooking(t)
	ctx := context.Background()

	_, err := coord.Start(ctx, saga.CancelRequest{BookingID: bookingID, Reason: "x", ByGuest: true})
	if err != nil {
		t.Fatal(err)
	}
	sg2, err := coord.Start(ctx, saga.CancelRequest{BookingID: bookingID, Reason: "y", ByGuest: true})
	if err != nil {
		t.Fatal(err)
	}
	if sg2.State != saga.SagaCompleted {
		t.Fatalf("state=%s", sg2.State)
	}
}

func TestCancelSagaResumeAfterCheckpoint(t *testing.T) {
	coord, bookingID, _ := setupCancelledBooking(t)
	ctx := context.Background()

	sg, err := coord.Start(ctx, saga.CancelRequest{BookingID: bookingID, Reason: "resume test", ByGuest: true})
	if err != nil {
		t.Fatal(err)
	}
	resumed, err := coord.Resume(ctx, sg.ID)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.State != saga.SagaCompleted {
		t.Fatalf("state=%s", resumed.State)
	}
}
