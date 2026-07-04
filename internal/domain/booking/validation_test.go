package booking_test

import (
	"testing"
	"time"

	"github.com/example/booking-engine/internal/domain/booking"
)

func TestValidateCreateRejectsBadInput(t *testing.T) {
	checkIn := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		name  string
		out   time.Time
		total int64
		qty   int
	}{
		{"zero qty", checkIn.Add(24 * time.Hour), 100, 0},
		{"negative total", checkIn.Add(24 * time.Hour), -1, 1},
		{"checkout before checkin", checkIn.Add(-24 * time.Hour), 100, 1},
		{"31 nights", checkIn.Add(31 * 24 * time.Hour), 100, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := booking.ValidateCreate(checkIn, tc.out, tc.total, tc.qty); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestValidateIDs(t *testing.T) {
	if err := booking.ValidateIDs("", "p", "g"); err == nil {
		t.Fatal("empty booking_id")
	}
}
