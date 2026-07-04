package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/example/booking-engine/internal/app"
	"github.com/example/booking-engine/internal/domain/booking"
	"github.com/example/booking-engine/internal/eventsourcing"
	"github.com/example/booking-engine/internal/inventory"
	"github.com/example/booking-engine/internal/payment"
	"github.com/example/booking-engine/internal/persistence"
	"github.com/example/booking-engine/internal/saga"
)

func main() {
	ctx := context.Background()

	memStore := eventsourcing.NewInMemoryStore()
	fan := eventsourcing.NewFanOut()
	store := eventsourcing.NewNotifyingStore(memStore, fan)

	bookings := persistence.NewBookingRepository(store)
	projector := persistence.NewBookingProjector()
	app.WireProjector(fan, projector)

	inv := inventory.NewInventory()
	resLog := inventory.NewReservationLog()
	invSvc := inventory.NewBookingService(inv, resLog)
	payGW := payment.NewGateway()
	sagaStore := saga.NewInMemorySagaStore()

	seedInventory(inv, "prop-42", "deluxe", 3)

	checkout := app.NewCheckoutOrchestrator(bookings, invSvc, payGW)
	cancelSaga := saga.NewCancelCoordinator(bookings, invSvc, payGW, sagaStore)

	checkIn := time.Date(2026, 8, 1, 15, 0, 0, 0, time.UTC)
	checkOut := checkIn.Add(3 * 24 * time.Hour)

	fmt.Println("=== checkout ===")
	bk, err := checkout.Create(ctx, app.CreateBookingCmd{
		BookingID:  "bk-001",
		PropertyID: "prop-42",
		GuestID:    "guest-7",
		CheckIn:    checkIn,
		CheckOut:   checkOut,
		RoomType:   "deluxe",
		TotalCents: 45000,
		Qty:        1,
	})
	if err != nil {
		log.Fatal(err)
	}
	printBooking(bk)
	printReadModel(projector, bk.ID())

	fmt.Println("\n=== cancel saga ===")
	sg, err := cancelSaga.Start(ctx, saga.CancelRequest{
		BookingID: "bk-001",
		Reason:    "guest changed plans",
		ByGuest:   true,
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("saga %s state=%s steps=%d\n", sg.ID, sg.State, len(sg.Steps))
	for _, st := range sg.Steps {
		fmt.Printf("  - %s: %s\n", st.Name, st.Status)
	}

	key := inventory.SlotKey{PropertyID: "prop-42", RoomType: "deluxe", Date: checkIn}
	avail, ver, _ := inv.Peek(key)
	fmt.Printf("\ninventory %s: available=%d version=%d\n", key.String(), avail, ver)

	fmt.Println("\n=== replay from event store ===")
	replayed, err := bookings.Load(ctx, "bk-001")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("replayed status=%s version=%d auth=%s\n", replayed.Status(), replayed.Version(), replayed.AuthID())
}

func seedInventory(inv *inventory.Inventory, propertyID, roomType string, nightly int) {
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 14; i++ {
		d := start.Add(time.Duration(i) * 24 * time.Hour)
		inv.Seed(inventory.SlotKey{PropertyID: propertyID, RoomType: roomType, Date: d}, nightly)
	}
}

func printBooking(b *booking.Booking) {
	fmt.Printf("booking %s status=%s v%d total=%d cents\n",
		b.ID(), b.Status(), b.Version(), b.TotalCents())
}

func printReadModel(p *persistence.BookingProjector, id string) {
	if vm, ok := p.Get(id); ok {
		fmt.Printf("read model: status=%s guest=%s\n", vm.Status, vm.GuestID)
	}
}
