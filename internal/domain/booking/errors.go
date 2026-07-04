package booking

import "errors"

var (
	ErrEmptyStream       = errors.New("empty event stream")
	ErrInvalidTransition = errors.New("invalid state transition")
	ErrUnknownEvent      = errors.New("unknown event type")
)
