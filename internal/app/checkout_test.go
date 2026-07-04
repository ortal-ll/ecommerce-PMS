package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/example/booking-engine/internal/app"
	"github.com/example/booking-engine/internal/eventsourcing"
	"github.com/example/booking-engine/internal/inventory"
	"github.com/example/booking-engine/internal/payment"
	"github.com/example/booking-engine/internal/persistence"
)

func testCheckout(t *testing.T) (*app.CheckoutOrchestrator, *inventory.Inventory, payment.Authorizer) {
	t.Helper()
	store := eventsourcing.NewInMemoryStore()
	bookings := persistence.NewBookingRepository(store)
	inv := inventory.NewInventory()
	log := inventory.NewReservationLog()
	invSvc := inventory.NewBookingService(inv, log)
	pay := payment.NewGateway()
	return app.NewCheckoutOrchestrator(bookings, invSvc, pay), inv, pay
}

func seedNight(inv *inventory.Inventory, d time.Time, n int) {
	inv.Seed(inventory.SlotKey{PropertyID: "p", RoomType: "r", Date: d}, n)
}

func TestCheckoutPaymentFailureReleasesInventory(t *testing.T) {
	store := eventsourcing.NewInMemoryStore()
	bookings := persistence.NewBookingRepository(store)
	inv := inventory.NewInventory()
	log := inventory.NewReservationLog()
	invSvc := inventory.NewBookingService(inv, log)
	pay := payment.NewFaultGateway(payment.NewGateway(), payment.FaultConfig{FailAuthorize: true})
	checkout := app.NewCheckoutOrchestrator(bookings, invSvc, pay)

	ctx := context.Background()
	checkIn := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	checkOut := checkIn.Add(48 * time.Hour)
	seedNight(inv, checkIn, 2)
	seedNight(inv, checkIn.Add(24*time.Hour), 2)

	_, err := checkout.Create(ctx, app.CreateBookingCmd{
		BookingID: "bk-fail", PropertyID: "p", GuestID: "g",
		CheckIn: checkIn, CheckOut: checkOut, RoomType: "r",
		TotalCents: 5000, Qty: 1,
	})
	if err == nil {
		t.Fatal("expected payment error")
	}
	if !payment.IsUnavailable(err) && !errors.Is(err, payment.ErrUnavailable) {
		t.Fatalf("err=%v", err)
	}

	avail, _, _ := inv.Peek(inventory.SlotKey{PropertyID: "p", RoomType: "r", Date: checkIn})
	if avail != 2 {
		t.Fatalf("hold not released after payment fail: avail=%d", avail)
	}
}

func TestCheckoutDuplicateBookingID(t *testing.T) {
	checkout, inv, _ := testCheckout(t)
	ctx := context.Background()
	checkIn := time.Date(2026, 12, 5, 0, 0, 0, 0, time.UTC)
	checkOut := checkIn.Add(24 * time.Hour)
	seedNight(inv, checkIn, 1)

	cmd := app.CreateBookingCmd{
		BookingID: "bk-dup", PropertyID: "p", GuestID: "g",
		CheckIn: checkIn, CheckOut: checkOut, RoomType: "r",
		TotalCents: 1000, Qty: 1,
	}
	if _, err := checkout.Create(ctx, cmd); err != nil {
		t.Fatal(err)
	}
	_, err := checkout.Create(ctx, cmd)
	if !errors.Is(err, app.ErrDuplicateBooking) {
		t.Fatalf("err=%v", err)
	}
}

func TestCheckoutRejectsInvalidQty(t *testing.T) {
	checkout, inv, _ := testCheckout(t)
	ctx := context.Background()
	checkIn := time.Date(2026, 12, 6, 0, 0, 0, 0, time.UTC)
	seedNight(inv, checkIn, 1)

	_, err := checkout.Create(ctx, app.CreateBookingCmd{
		BookingID: "bk-bad", PropertyID: "p", GuestID: "g",
		CheckIn: checkIn, CheckOut: checkIn.Add(24 * time.Hour), RoomType: "r",
		TotalCents: 1000, Qty: 0,
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
}
