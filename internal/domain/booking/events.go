package booking

import "time"

type EventType string

const (
	EvCreated       EventType = "booking.created"
	EvConfirmed     EventType = "booking.confirmed"
	EvCancelled     EventType = "booking.cancelled"
	EvPaymentHeld   EventType = "booking.payment_held"
	EvPaymentVoided EventType = "booking.payment_voided"
)

type Created struct {
	BookingID  string
	PropertyID string
	GuestID    string
	CheckIn    time.Time
	CheckOut   time.Time
	RoomType   string
	TotalCents int64
}

type Confirmed struct {
	BookingID string
	At        time.Time
}

type PaymentHeld struct {
	BookingID     string
	AuthID        string
	AmountCents   int64
	PaymentMethod string
}

type PaymentVoided struct {
	BookingID string
	AuthID    string
	Reason    string
}

type Cancelled struct {
	BookingID string
	Reason    string
	ByGuest   bool
	At        time.Time
}
