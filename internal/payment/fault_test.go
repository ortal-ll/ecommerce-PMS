package payment_test

import (
	"context"
	"errors"
	"testing"

	"github.com/example/booking-engine/internal/payment"
)

func TestFaultGatewayAuthorize(t *testing.T) {
	base := payment.NewGateway()
	fg := payment.NewFaultGateway(base, payment.FaultConfig{FailAuthorize: true})
	_, err := fg.Authorize(context.Background(), "bk", 100)
	if !errors.Is(err, payment.ErrUnavailable) {
		t.Fatalf("err=%v", err)
	}
}

func TestFaultGatewayVoid(t *testing.T) {
	base := payment.NewGateway()
	authID, _ := base.Authorize(context.Background(), "bk", 100)
	fg := payment.NewFaultGateway(base, payment.FaultConfig{FailVoid: true})
	err := fg.Void(context.Background(), authID)
	if !errors.Is(err, payment.ErrUnavailable) {
		t.Fatalf("err=%v", err)
	}
}
