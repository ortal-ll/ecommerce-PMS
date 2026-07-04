package saga

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/example/booking-engine/internal/domain/booking"
	"github.com/example/booking-engine/internal/inventory"
	"github.com/example/booking-engine/internal/payment"
	"github.com/example/booking-engine/internal/persistence"
)

type State string

const (
	SagaPending    State = "pending"
	SagaCompleted  State = "completed"
	SagaFailed     State = "failed"
	SagaCompensating State = "compensating"
)

type StepName string

const (
	StepVoidPayment    StepName = "void_payment"
	StepReleaseStock   StepName = "release_stock"
	StepMarkCancelled  StepName = "mark_cancelled"
)

type StepRecord struct {
	Name      StepName
	Status    string // done | failed | compensated
	At        time.Time
	ErrorText string
}

// Cancel saga: payment void before stock release — chargeback window is the real risk.
type CancelSaga struct {
	ID          string
	BookingID   string
	Reason      string
	ByGuest     bool
	State       State
	Steps       []StepRecord
	StartedAt   time.Time
	CompletedAt *time.Time
}

type CancelCoordinator struct {
	bookings  *persistence.BookingRepository
	invSvc    inventory.Holder
	payments  payment.Authorizer
	sagaStore SagaStore
}

func NewCancelCoordinator(
	bookings *persistence.BookingRepository,
	invSvc inventory.Holder,
	payments payment.Authorizer,
	sagaStore SagaStore,
) *CancelCoordinator {
	return &CancelCoordinator{
		bookings:  bookings,
		invSvc:    invSvc,
		payments:  payments,
		sagaStore: sagaStore,
	}
}

type CancelRequest struct {
	BookingID string
	Reason    string
	ByGuest   bool
}

func (c *CancelCoordinator) Start(ctx context.Context, req CancelRequest) (*CancelSaga, error) {
	b, err := c.bookings.Load(ctx, req.BookingID)
	if err != nil {
		return nil, err
	}
	if b.Status() == booking.StatusCancelled {
		// already done — return existing saga if we have one
		if existing, ok := c.sagaStore.GetByBooking(req.BookingID); ok {
			return existing, nil
		}
		return &CancelSaga{BookingID: req.BookingID, State: SagaCompleted}, nil
	}

	saga := &CancelSaga{
		ID:        fmt.Sprintf("cancel-%s-%d", req.BookingID, time.Now().UnixNano()),
		BookingID: req.BookingID,
		Reason:    req.Reason,
		ByGuest:   req.ByGuest,
		State:     SagaPending,
		StartedAt: time.Now().UTC(),
	}
	if err := c.sagaStore.Save(saga); err != nil {
		return nil, err
	}

	return c.run(ctx, saga)
}

func (c *CancelCoordinator) Resume(ctx context.Context, sagaID string) (*CancelSaga, error) {
	saga, ok := c.sagaStore.Get(sagaID)
	if !ok {
		return nil, fmt.Errorf("saga not found: %s", sagaID)
	}
	if saga.State == SagaCompleted || saga.State == SagaFailed {
		return saga, nil
	}
	return c.run(ctx, saga)
}

func (c *CancelCoordinator) run(ctx context.Context, saga *CancelSaga) (*CancelSaga, error) {
	steps := []struct {
		name StepName
		fn   func(context.Context, string) error
	}{
		{StepVoidPayment, c.stepVoidPayment},
		{StepReleaseStock, c.stepReleaseStock},
		{StepMarkCancelled, c.stepMarkCancelled(saga)},
	}

	for _, step := range steps {
		if c.stepDone(saga, step.name) {
			continue
		}

		if err := step.fn(ctx, saga.BookingID); err != nil {
			saga.State = SagaFailed
			c.recordStep(saga, step.name, "failed", err.Error())
			_ = c.sagaStore.Save(saga)
			return saga, err
		}
		c.recordStep(saga, step.name, "done", "")
		_ = c.sagaStore.Save(saga)
	}

	now := time.Now().UTC()
	saga.State = SagaCompleted
	saga.CompletedAt = &now
	_ = c.sagaStore.Save(saga)
	return saga, nil
}

func (c *CancelCoordinator) stepDone(saga *CancelSaga, name StepName) bool {
	for _, s := range saga.Steps {
		if s.Name == name && s.Status == "done" {
			return true
		}
	}
	return false
}

func (c *CancelCoordinator) recordStep(saga *CancelSaga, name StepName, status, errText string) {
	saga.Steps = append(saga.Steps, StepRecord{
		Name:      name,
		Status:    status,
		At:        time.Now().UTC(),
		ErrorText: errText,
	})
}

func (c *CancelCoordinator) stepVoidPayment(ctx context.Context, bookingID string) error {
	b, err := c.bookings.Load(ctx, bookingID)
	if err != nil {
		return err
	}
	authID := b.AuthID()
	if authID == "" {
		return nil
	}
	if err := c.payments.Void(ctx, authID); err != nil {
		return err
	}
	if err := b.RecordPaymentVoid(authID, "cancellation"); err != nil {
		return err
	}
	return c.bookings.Save(ctx, b)
}

func (c *CancelCoordinator) stepReleaseStock(ctx context.Context, bookingID string) error {
	_ = ctx
	return c.invSvc.ReleaseInventory(bookingID)
}

func (c *CancelCoordinator) stepMarkCancelled(saga *CancelSaga) func(context.Context, string) error {
	return func(ctx context.Context, bookingID string) error {
		b, err := c.bookings.Load(ctx, bookingID)
		if err != nil {
			return err
		}
		if err := b.Cancel(saga.Reason, saga.ByGuest); err != nil {
			return err
		}
		return c.bookings.Save(ctx, b)
	}
}

// SagaStore persists saga state for crash recovery.
type SagaStore interface {
	Save(saga *CancelSaga) error
	Get(id string) (*CancelSaga, bool)
	GetByBooking(bookingID string) (*CancelSaga, bool)
}

type InMemorySagaStore struct {
	mu        sync.RWMutex
	byID      map[string]*CancelSaga
	byBooking map[string]string
}

func NewInMemorySagaStore() *InMemorySagaStore {
	return &InMemorySagaStore{
		byID:      make(map[string]*CancelSaga),
		byBooking: make(map[string]string),
	}
}

func (s *InMemorySagaStore) Save(saga *CancelSaga) error {
	raw, err := json.Marshal(saga)
	if err != nil {
		return err
	}
	var copy CancelSaga
	if err := json.Unmarshal(raw, &copy); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.byID[saga.ID] = &copy
	s.byBooking[saga.BookingID] = saga.ID
	return nil
}

func (s *InMemorySagaStore) Get(id string) (*CancelSaga, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sg, ok := s.byID[id]
	return sg, ok
}

func (s *InMemorySagaStore) GetByBooking(bookingID string) (*CancelSaga, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.byBooking[bookingID]
	if !ok {
		return nil, false
	}
	sg, ok := s.byID[id]
	return sg, ok
}
