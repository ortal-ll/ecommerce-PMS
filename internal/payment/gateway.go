package payment

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

type Gateway struct {
	mu    sync.Mutex
	auths map[string]AuthRecord
}

type AuthRecord struct {
	AuthID      string
	BookingID   string
	AmountCents int64
	Status      string
}

func NewGateway() *Gateway {
	return &Gateway{auths: make(map[string]AuthRecord)}
}

func (g *Gateway) Authorize(ctx context.Context, bookingID string, amountCents int64) (string, error) {
	_ = ctx
	if amountCents <= 0 {
		return "", fmt.Errorf("invalid amount: %d", amountCents)
	}
	g.mu.Lock()
	defer g.mu.Unlock()

	authID := fmt.Sprintf("auth_%s_%d", bookingID, amountCents)
	g.auths[authID] = AuthRecord{
		AuthID:      authID,
		BookingID:   bookingID,
		AmountCents: amountCents,
		Status:      "authorized",
	}
	return authID, nil
}

func (g *Gateway) Void(ctx context.Context, authID string) error {
	_ = ctx
	g.mu.Lock()
	defer g.mu.Unlock()

	rec, ok := g.auths[authID]
	if !ok {
		return fmt.Errorf("%w: %s", ErrAuthNotFound, authID)
	}
	if rec.Status == "voided" {
		return nil
	}
	rec.Status = "voided"
	g.auths[authID] = rec
	return nil
}

func (g *Gateway) Status(authID string) (string, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	r, ok := g.auths[authID]
	if !ok {
		return "", false
	}
	return r.Status, true
}

// старая версия, оставил на всякий случай
// func (g *Gateway) VoidSync(authID string) error {
// 	return g.Void(context.Background(), authID)
// }

var _ Authorizer = (*Gateway)(nil)

// IsUnavailable helps callers decide retry vs fail checkout.
func IsUnavailable(err error) bool {
	return errors.Is(err, ErrUnavailable)
}
