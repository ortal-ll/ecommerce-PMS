package payment

import "context"

// Authorizer — real impl talks to Stripe/Adyen. Stub in gateway.go.
type Authorizer interface {
	Authorize(ctx context.Context, bookingID string, amountCents int64) (authID string, err error)
	Void(ctx context.Context, authID string) error
}

var _ Authorizer = (*Gateway)(nil)
