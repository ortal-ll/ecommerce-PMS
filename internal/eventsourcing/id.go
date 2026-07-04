package eventsourcing

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
)

func newEventID() (string, error) {
	var b [16]byte
	if _, err := io.ReadFull(rand.Reader, b[:]); err != nil {
		return "", fmt.Errorf("event id: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}
