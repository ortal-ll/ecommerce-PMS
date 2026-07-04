package payment

import (
	"context"
	"fmt"
	"sync"
)

// FaultConfig drives injected failures in tests — simulates Stripe 5xx / timeout.
type FaultConfig struct {
	FailAuthorize bool
	FailVoid      bool
}

type FaultGateway struct {
	inner Authorizer
	cfg   FaultConfig
	mu    sync.Mutex
}

func NewFaultGateway(inner Authorizer, cfg FaultConfig) *FaultGateway {
	return &FaultGateway{inner: inner, cfg: cfg}
}

func (g *FaultGateway) Authorize(ctx context.Context, bookingID string, amountCents int64) (string, error) {
	g.mu.Lock()
	fail := g.cfg.FailAuthorize
	g.mu.Unlock()
	if fail {
		return "", fmt.Errorf("%w: authorize rejected", ErrUnavailable)
	}
	return g.inner.Authorize(ctx, bookingID, amountCents)
}

func (g *FaultGateway) Void(ctx context.Context, authID string) error {
	g.mu.Lock()
	fail := g.cfg.FailVoid
	g.mu.Unlock()
	if fail {
		return fmt.Errorf("%w: void rejected", ErrUnavailable)
	}
	return g.inner.Void(ctx, authID)
}

func (g *FaultGateway) SetFailAuthorize(v bool) {
	g.mu.Lock()
	g.cfg.FailAuthorize = v
	g.mu.Unlock()
}
