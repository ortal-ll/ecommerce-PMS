package booking

import (
	"fmt"
	"strings"
	"time"
)

func validateStayDates(checkIn, checkOut time.Time) error {
	if checkOut.Before(checkIn) || checkOut.Equal(checkIn) {
		return fmt.Errorf("checkout must be after checkin")
	}
	nights := int(checkOut.Sub(checkIn).Hours() / 24)
	if nights > 30 {
		// finance cap — chargeback risk on long unconfirmed holds
		return fmt.Errorf("stay too long: %d nights (max 30)", nights)
	}
	return nil
}

func validateMoney(totalCents int64) error {
	if totalCents <= 0 {
		return fmt.Errorf("total must be positive")
	}
	return nil
}

func ValidateCreate(checkIn, checkOut time.Time, totalCents int64, qty int) error {
	if qty <= 0 {
		return fmt.Errorf("qty must be positive")
	}
	if err := validateStayDates(checkIn, checkOut); err != nil {
		return err
	}
	return validateMoney(totalCents)
}

func ValidateIDs(bookingID, propertyID, guestID string) error {
	if strings.TrimSpace(bookingID) == "" {
		return fmt.Errorf("booking_id required")
	}
	if strings.TrimSpace(propertyID) == "" {
		return fmt.Errorf("property_id required")
	}
	if strings.TrimSpace(guestID) == "" {
		return fmt.Errorf("guest_id required")
	}
	return nil
}
