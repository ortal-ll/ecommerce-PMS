package eventsourcing

import (
	"encoding/json"
	"time"
)

// StreamID is usually aggregate id. kept as string bc some streams are composite keys.
type StreamID string

type EventType string

type Event struct {
	ID        string
	StreamID  StreamID
	Type      EventType
	Payload   json.RawMessage
	Version   int64 // monotonic per stream
	Occurred  time.Time
	Metadata  map[string]string
}

func NewEvent(stream StreamID, typ EventType, version int64, payload any) (Event, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return Event{}, err
	}
	id, err := newEventID()
	if err != nil {
		return Event{}, err
	}
	return Event{
		ID:       id,
		StreamID: stream,
		Type:     typ,
		Payload:  raw,
		Version:  version,
		Occurred: time.Now().UTC(),
	}, nil
}
