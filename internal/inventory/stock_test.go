package inventory_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/example/booking-engine/internal/inventory"
)

func TestOptimisticReserveConflict(t *testing.T) {
	inv := inventory.NewInventory()
	key := inventory.SlotKey{PropertyID: "p", RoomType: "d", Date: time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC)}
	inv.Seed(key, 5)

	ctx := context.Background()
	_, v, _ := inv.Peek(key)
	_, err := inv.Reserve(ctx, key, 1, "bk-1", v)
	if err != nil {
		t.Fatal(err)
	}

	// stale version from before first reserve
	err = inv.Release(key, 1, v)
	if !errors.Is(err, inventory.ErrVersionConflict) {
		t.Fatalf("err=%v", err)
	}
}

func TestConcurrentReserveExactlyOneUnit(t *testing.T) {
	inv := inventory.NewInventory()
	key := inventory.SlotKey{PropertyID: "p", RoomType: "d", Date: time.Date(2026, 8, 6, 0, 0, 0, 0, time.UTC)}
	inv.Seed(key, 1)

	ctx := context.Background()
	var wg sync.WaitGroup
	var okCount atomic.Int32

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, ver, ok := inv.Peek(key)
			if !ok {
				return
			}
			_, err := inv.Reserve(ctx, key, 1, "bk-"+string(rune('a'+idx%26)), ver)
			if err == nil {
				okCount.Add(1)
			}
		}(i)
	}
	wg.Wait()

	if okCount.Load() != 1 {
		t.Fatalf("expected 1 successful reserve, got %d", okCount.Load())
	}
	avail, _, _ := inv.Peek(key)
	if avail != 0 {
		t.Fatalf("avail=%d", avail)
	}
}

func TestReserveNightsRollsBackOnOOS(t *testing.T) {
	inv := inventory.NewInventory()
	d1 := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	d2 := d1.Add(24 * time.Hour)
	inv.Seed(inventory.SlotKey{PropertyID: "p", RoomType: "s", Date: d1}, 2)
	inv.Seed(inventory.SlotKey{PropertyID: "p", RoomType: "s", Date: d2}, 0)

	ctx := context.Background()
	_, err := inv.ReserveNights(ctx, "p", "s", "bk-x", d1, d2.Add(24*time.Hour), 1)
	if !errors.Is(err, inventory.ErrInsufficientStock) {
		t.Fatalf("err=%v", err)
	}

	avail, _, _ := inv.Peek(inventory.SlotKey{PropertyID: "p", RoomType: "s", Date: d1})
	if avail != 2 {
		t.Fatalf("rollback failed, avail=%d", avail)
	}
}
