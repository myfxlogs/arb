package adapter

import "errors"

// ErrEmergencyClosed is returned when the reconnect state machine exceeds
// the retry limit and triggers an emergency close.
var ErrEmergencyClosed = errors.New("adapter emergency closed: reconnect retries exceeded")

// ErrNotConnected is returned when an operation is attempted on a disconnected adapter.
var ErrNotConnected = errors.New("adapter not connected")

// ErrReconnecting is returned when an operation is attempted during reconnection.
var ErrReconnecting = errors.New("adapter reconnecting, try again later")
