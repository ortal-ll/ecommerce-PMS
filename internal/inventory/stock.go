package inventory

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

var ErrVersionConflict = errors.New("inventory version conflict")
var ErrInsufficientStock = errors.New("insufficient availability")

// SlotKey identifies a bookable unit: property + room type + night.
type SlotKey struct {
	PropertyID string
	RoomType   string
	Date       time.Time // truncated to date in callers
}

func (k SlotKey) String() string {
	return fmt.Sprintf("%s/%s/%s", k.PropertyID, k.RoomType, k.Date.Format("2006-01-02"))
}

type Slot struct {
	key       SlotKey
	available int
	reserved  int
	version   int64 // bumped on every write — optimistic lock token
}

type Reservation struct {
	SlotKey    SlotKey
	Qty        int
	BookingID  string
	Version    int64 // version after reserve, needed for release
}

// Inventory is a separate consistency boundary from Booking — many guests contend per night.
type Inventory struct {
	mu    sync.Mutex
	slots map[string]*Slot
}

func NewInventory() *Inventory {
	return &Inventory{slots: make(map[string]*Slot)}
}

func (inv *Inventory) Seed(key SlotKey, available int) {
	inv.mu.Lock()
	defer inv.mu.Unlock()
	inv.slots[key.String()] = &Slot{
		key:       key,
		available: available,
		version:   1,
	}
}

// Reserve decrements availability with optimistic locking.
func (inv *Inventory) Reserve(ctx context.Context, key SlotKey, qty int, bookingID string, expectedVersion int64) (*Reservation, error) {
	_ = ctx
	inv.mu.Lock()
	defer inv.mu.Unlock()

	slot, ok := inv.slots[key.String()]
	if !ok {
		return nil, fmt.Errorf("unknown slot %s", key.String())
	}

	if expectedVersion >= 0 && slot.version != expectedVersion {
		return nil, fmt.Errorf("%w: slot %s at v%d, caller had v%d", ErrVersionConflict, key.String(), slot.version, expectedVersion)
	}

	if slot.available < qty {
		return nil, ErrInsufficientStock
	}

	slot.available -= qty
	slot.reserved += qty
	slot.version++

	return &Reservation{
		SlotKey:   key,
		Qty:       qty,
		BookingID: bookingID,
		Version:   slot.version,
	}, nil
}

func (inv *Inventory) Release(key SlotKey, qty int, expectedVersion int64) error {
	inv.mu.Lock()
	defer inv.mu.Unlock()

	slot, ok := inv.slots[key.String()]
	if !ok {
		return fmt.Errorf("unknown slot %s", key.String())
	}

	if slot.version != expectedVersion {
		return fmt.Errorf("%w: release on stale version", ErrVersionConflict)
	}

	slot.available += qty
	slot.reserved -= qty
	if slot.reserved < 0 {
		// shouldn't happen unless compensating saga runs twice
		slot.reserved = 0
	}
	slot.version++
	return nil
}

func (inv *Inventory) Peek(key SlotKey) (available int, version int64, ok bool) {
	inv.mu.Lock()
	defer inv.mu.Unlock()
	s, exists := inv.slots[key.String()]
	if !exists {
		return 0, 0, false
	}
	return s.available, s.version, true
}

// ReserveNights reserves each night in [checkIn, checkOut).
func (inv *Inventory) ReserveNights(ctx context.Context, propertyID, roomType, bookingID string, checkIn, checkOut time.Time, qty int) ([]Reservation, error) {
	var reservations []Reservation

	for d := truncateDay(checkIn); d.Before(checkOut); d = d.Add(24 * time.Hour) {
		key := SlotKey{PropertyID: propertyID, RoomType: roomType, Date: d}

		avail, ver, ok := inv.Peek(key)
		if !ok {
			return nil, fmt.Errorf("slot not seeded: %s", key.String())
		}
		if avail < qty {
			for i := len(reservations) - 1; i >= 0; i-- {
				r := reservations[i]
				_ = inv.Release(r.SlotKey, r.Qty, r.Version)
			}
			return nil, ErrInsufficientStock
		}

		res, err := inv.Reserve(ctx, key, qty, bookingID, ver)
		if err != nil {
			// rollback on version conflict mid-loop
			for i := len(reservations) - 1; i >= 0; i-- {
				r := reservations[i]
				_ = inv.Release(r.SlotKey, r.Qty, r.Version)
			}
			return nil, err
		}
		reservations = append(reservations, *res)
	}
	return reservations, nil
}

func truncateDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}
