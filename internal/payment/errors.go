package payment

import "errors"

var (
	ErrUnavailable  = errors.New("payment provider unavailable")
	ErrAuthNotFound = errors.New("payment auth not found")
)
