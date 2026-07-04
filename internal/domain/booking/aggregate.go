package booking

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/example/booking-engine/internal/eventsourcing"
)

type Status string

const (
	StatusDraft     Status = "draft"
	StatusConfirmed Status = "confirmed"
	StatusCancelled Status = "cancelled"
)

// Booking write model — state only via event replay, never mutated directly.
type Booking struct {
	id         string
	propertyID string
	guestID    string
	checkIn    time.Time
	checkOut   time.Time
	roomType   string
	totalCents int64

	status   Status
	authID   string
	version  int64
	uncommitted []eventsourcing.Event
}

func NewBooking(id, propertyID, guestID string, checkIn, checkOut time.Time, roomType string, total int64) (*Booking, error) {
	if err := ValidateIDs(id, propertyID, guestID); err != nil {
		return nil, err
	}
	if err := ValidateCreate(checkIn, checkOut, total, 1); err != nil {
		return nil, err
	}
	b := &Booking{
		id:         id,
		propertyID: propertyID,
		guestID:    guestID,
		checkIn:    checkIn,
		checkOut:   checkOut,
		roomType:   roomType,
		totalCents: total,
		status:     StatusDraft,
	}
	ev, err := eventsourcing.NewEvent(eventsourcing.StreamID(id), eventsourcing.EventType(EvCreated), 1, Created{
		BookingID:  id,
		PropertyID: propertyID,
		GuestID:    guestID,
		CheckIn:    checkIn,
		CheckOut:   checkOut,
		RoomType:   roomType,
		TotalCents: total,
	})
	if err != nil {
		return nil, err
	}
	if err := b.apply(ev); err != nil {
		return nil, err
	}
	b.uncommitted = append(b.uncommitted, ev)
	return b, nil
}

func LoadFromHistory(events []eventsourcing.Event) (*Booking, error) {
	if len(events) == 0 {
		return nil, ErrEmptyStream
	}
	b := &Booking{}
	var prev int64
	for _, ev := range events {
		if ev.Version != prev+1 {
			return nil, fmt.Errorf("version gap at %d (prev %d)", ev.Version, prev)
		}
		if err := b.apply(ev); err != nil {
			return nil, err
		}
		prev = ev.Version
	}
	return b, nil
}

func (b *Booking) ID() string       { return b.id }
func (b *Booking) Version() int64   { return b.version }
func (b *Booking) Status() Status   { return b.status }
func (b *Booking) PropertyID() string { return b.propertyID }
func (b *Booking) RoomType() string { return b.roomType }
func (b *Booking) CheckIn() time.Time { return b.checkIn }
func (b *Booking) CheckOut() time.Time { return b.checkOut }
func (b *Booking) TotalCents() int64 { return b.totalCents }
func (b *Booking) AuthID() string   { return b.authID }

func (b *Booking) Uncommitted() []eventsourcing.Event {
	return b.uncommitted
}

func (b *Booking) ClearUncommitted() {
	b.uncommitted = nil
}

func (b *Booking) Confirm() error {
	if b.status != StatusDraft {
		return fmt.Errorf("%w: can't confirm from %s", ErrInvalidTransition, b.status)
	}
	return b.emit(EvConfirmed, Confirmed{BookingID: b.id, At: time.Now().UTC()})
}

func (b *Booking) RecordPaymentHold(authID, method string, amount int64) error {
	if b.status != StatusDraft && b.status != StatusConfirmed {
		return fmt.Errorf("%w: payment hold not allowed in %s", ErrInvalidTransition, b.status)
	}
	return b.emit(EvPaymentHeld, PaymentHeld{
		BookingID:     b.id,
		AuthID:        authID,
		AmountCents:   amount,
		PaymentMethod: method,
	})
}

func (b *Booking) Cancel(reason string, byGuest bool) error {
	if b.status == StatusCancelled {
		return nil
	}
	return b.emit(EvCancelled, Cancelled{
		BookingID: b.id,
		Reason:    reason,
		ByGuest:   byGuest,
		At:        time.Now().UTC(),
	})
}

func (b *Booking) RecordPaymentVoid(authID, reason string) error {
	return b.emit(EvPaymentVoided, PaymentVoided{
		BookingID: b.id,
		AuthID:    authID,
		Reason:    reason,
	})
}

func (b *Booking) emit(typ EventType, payload any) error {
	ev, err := b.raise(typ, payload)
	if err != nil {
		return err
	}
	b.uncommitted = append(b.uncommitted, ev)
	return nil
}

func (b *Booking) raise(typ EventType, payload any) (eventsourcing.Event, error) {
	ev, err := eventsourcing.NewEvent(eventsourcing.StreamID(b.id), eventsourcing.EventType(typ), b.version+1, payload)
	if err != nil {
		return eventsourcing.Event{}, err
	}
	if err := b.apply(ev); err != nil {
		return eventsourcing.Event{}, err
	}
	return ev, nil
}

func (b *Booking) apply(ev eventsourcing.Event) error {
	b.version = ev.Version

	switch EventType(ev.Type) {
	case EvCreated:
		var p Created
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			return err
		}
		b.id = p.BookingID
		b.propertyID = p.PropertyID
		b.guestID = p.GuestID
		b.checkIn = p.CheckIn
		b.checkOut = p.CheckOut
		b.roomType = p.RoomType
		b.totalCents = p.TotalCents
		b.status = StatusDraft

	case EvConfirmed:
		b.status = StatusConfirmed

	case EvPaymentHeld:
		var p PaymentHeld
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			return err
		}
		b.authID = p.AuthID

	case EvPaymentVoided:
		b.authID = ""

	case EvCancelled:
		b.status = StatusCancelled

	default:
		return fmt.Errorf("%w: %s", ErrUnknownEvent, ev.Type)
	}
	return nil
}
