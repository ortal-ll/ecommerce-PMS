package app_test

import (
	"context"
	"testing"
	"time"

	"github.com/example/booking-engine/internal/app"
	"github.com/example/booking-engine/internal/domain/booking"
	"github.com/example/booking-engine/internal/eventsourcing"
	"github.com/example/booking-engine/internal/inventory"
	"github.com/example/booking-engine/internal/payment"
	"github.com/example/booking-engine/internal/persistence"
	"github.com/example/booking-engine/internal/saga"
)

func TestEndToEndCheckoutCancelReplay(t *testing.T) {
	ctx := context.Background()

	mem := eventsourcing.NewInMemoryStore()
	fan := eventsourcing.NewFanOut()
	store := eventsourcing.NewNotifyingStore(mem, fan)
	bookings := persistence.NewBookingRepository(store)
	projector := persistence.NewBookingProjector()
	app.WireProjector(fan, projector)

	inv := inventory.NewInventory()
	log := inventory.NewReservationLog()
	invSvc := inventory.NewBookingService(inv, log)
	pay := payment.NewGateway()

	checkIn := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	checkOut := checkIn.Add(3 * 24 * time.Hour)
	for d := checkIn; d.Before(checkOut.Add(24 * time.Hour)); d = d.Add(24 * time.Hour) {
		inv.Seed(inventory.SlotKey{PropertyID: "hotel", RoomType: "king", Date: d}, 2)
	}

	checkout := app.NewCheckoutOrchestrator(bookings, invSvc, pay)
	b, err := checkout.Create(ctx, app.CreateBookingCmd{
		BookingID: "bk-e2e", PropertyID: "hotel", GuestID: "g1",
		CheckIn: checkIn, CheckOut: checkOut, RoomType: "king",
		TotalCents: 99000, Qty: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if b.Status() != booking.StatusConfirmed {
		t.Fatalf("status=%s", b.Status())
	}

	vm, ok := projector.Get("bk-e2e")
	if !ok || vm.Status != booking.StatusConfirmed {
		t.Fatalf("read model not projected: %+v", vm)
	}

	coord := saga.NewCancelCoordinator(bookings, invSvc, pay, saga.NewInMemorySagaStore())
	sg, err := coord.Start(ctx, saga.CancelRequest{BookingID: "bk-e2e", Reason: "change", ByGuest: true})
	if err != nil {
		t.Fatal(err)
	}
	if sg.State != saga.SagaCompleted {
		t.Fatalf("saga state=%s", sg.State)
	}

	replayed, err := bookings.Load(ctx, "bk-e2e")
	if err != nil {
		t.Fatal(err)
	}
	if replayed.Status() != booking.StatusCancelled {
		t.Fatalf("status=%s", replayed.Status())
	}
	if replayed.AuthID() != "" {
		t.Fatal("auth should be voided on aggregate")
	}

	streams, _ := mem.Load(ctx, eventsourcing.StreamID("bk-e2e"))
	if len(streams) < 5 {
		t.Fatalf("expected >=5 events, got %d", len(streams))
	}
}
