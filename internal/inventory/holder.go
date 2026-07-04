package inventory

import "context"

type Holder interface {
	HoldInventory(ctx context.Context, req CheckoutRequest) error
	ReleaseInventory(bookingID string) error
}

var _ Holder = (*BookingService)(nil)
