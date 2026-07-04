package inventory

import (
	"context"
	"errors"
	"sync"
	"time"
)

// ReservationLog: holds survive process crash so cancel saga can release stock.
type ReservationLog struct {
	mu   sync.RWMutex
	byID map[string][]Reservation // bookingID -> holds
}

func NewReservationLog() *ReservationLog {
	return &ReservationLog{byID: make(map[string][]Reservation)}
}

func (l *ReservationLog) Record(bookingID string, holds []Reservation) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.byID[bookingID] = holds
}

func (l *ReservationLog) Get(bookingID string) ([]Reservation, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	h, ok := l.byID[bookingID]
	if !ok {
		return nil, false
	}
	out := make([]Reservation, len(h))
	copy(out, h)
	return out, true
}

func (l *ReservationLog) Clear(bookingID string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.byID, bookingID)
}

// BookingService wires inventory into the checkout flow.
type BookingService struct {
	inv *Inventory
	log *ReservationLog
}

func NewBookingService(inv *Inventory, log *ReservationLog) *BookingService {
	return &BookingService{inv: inv, log: log}
}

type CheckoutRequest struct {
	BookingID  string
	PropertyID string
	RoomType   string
	CheckIn    time.Time
	CheckOut   time.Time
	Qty        int
}

var ErrCheckoutFailed = errors.New("checkout failed")

func (s *BookingService) HoldInventory(ctx context.Context, req CheckoutRequest) error {
	holds, err := s.inv.ReserveNights(ctx, req.PropertyID, req.RoomType, req.BookingID, req.CheckIn, req.CheckOut, req.Qty)
	if err != nil {
		return err
	}
	s.log.Record(req.BookingID, holds)
	return nil
}

func (s *BookingService) ReleaseInventory(bookingID string) error {
	holds, ok := s.log.Get(bookingID)
	if !ok {
		return nil
	}
	for _, h := range holds {
		// re-peeking caused false conflicts; retry w/ fresh version on compensaton path
		if err := s.inv.Release(h.SlotKey, h.Qty, h.Version); err != nil {
			_, ver, exists := s.inv.Peek(h.SlotKey)
			if !exists {
				continue
			}
			if err2 := s.inv.Release(h.SlotKey, h.Qty, ver); err2 != nil {
				return err
			}
		}
	}
	s.log.Clear(bookingID)
	return nil
}
