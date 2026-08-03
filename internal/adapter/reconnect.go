package adapter

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"
)

// connState tracks the adapter connection lifecycle.
type connState int32

const (
	stateDisconnected connState = iota
	stateConnecting
	stateConnected
	stateReconnecting
	stateEmergencyClosed
)

var stateNames = []string{
	"Disconnected", "Connecting", "Connected", "Reconnecting", "EmergencyClosed",
}

func (s connState) String() string {
	if int(s) >= 0 && int(s) < len(stateNames) {
		return stateNames[s]
	}
	return "Unknown"
}

// reconnectConfig controls backoff and limits for the reconnect state machine.
type reconnectConfig struct {
	maxRetries      int32
	retryWindow     time.Duration
	baseBackoff     time.Duration
	maxBackoff      time.Duration
	emergencyClose  func()
}

func defaultReconnectConfig(emergencyClose func()) reconnectConfig {
	return reconnectConfig{
		maxRetries:     10,
		retryWindow:    time.Minute,
		baseBackoff:    time.Second,
		maxBackoff:     30 * time.Second,
		emergencyClose: emergencyClose,
	}
}

// reconnectStateMachine manages reconnection with exponential backoff.
// It is embedded by MT5Adapter and MT4Adapter.
type reconnectStateMachine struct {
	state    atomic.Int32
	retries  atomic.Int32
	lastReset atomic.Int64 // unix nano
	cfg      reconnectConfig
}

func newReconnectStateMachine(cfg reconnectConfig) *reconnectStateMachine {
	r := &reconnectStateMachine{cfg: cfg}
	r.lastReset.Store(time.Now().UnixNano())
	return r
}

func (r *reconnectStateMachine) getState() connState {
	return connState(r.state.Load())
}

func (r *reconnectStateMachine) setState(s connState) {
	r.state.Store(int32(s))
}

func (r *reconnectStateMachine) isConnected() bool {
	return r.getState() == stateConnected
}

// canPlaceOrder returns false if the adapter is not in a connected state.
func (r *reconnectStateMachine) canPlaceOrder() bool {
	return r.getState() == stateConnected
}

// backoff returns the exponential backoff duration for the current retry count.
func (r *reconnectStateMachine) backoff() time.Duration {
	n := r.retries.Load()
	if n == 0 {
		return r.cfg.baseBackoff
	}
	d := r.cfg.baseBackoff << uint(n)
	if d > r.cfg.maxBackoff {
		d = r.cfg.maxBackoff
	}
	return d
}

// recordRetry increments the retry counter, resetting if the window has elapsed.
// Returns true if the retry limit is exceeded (should emergency close).
func (r *reconnectStateMachine) recordRetry() bool {
	now := time.Now().UnixNano()
	last := r.lastReset.Load()
	if time.Duration(now-last) > r.cfg.retryWindow {
		r.retries.Store(0)
		r.lastReset.Store(now)
	}
	count := r.retries.Add(1)
	if count > r.cfg.maxRetries {
		r.setState(stateEmergencyClosed)
		if r.cfg.emergencyClose != nil {
			r.cfg.emergencyClose()
		}
		slog.Error("reconnect retries exceeded, emergency close",
			"retries", count, "window", r.cfg.retryWindow)
		return true
	}
	return false
}

// resetRetries clears the retry counter on successful connection.
func (r *reconnectStateMachine) resetRetries() {
	r.retries.Store(0)
	r.lastReset.Store(time.Now().UnixNano())
}

// reconnectLoop runs the reconnection cycle with backoff.
// The connectFn should return nil on success, error on failure.
// Returns nil when connected, or ctx.Err() when context is cancelled.
func (r *reconnectStateMachine) reconnectLoop(ctx context.Context, connectFn func() error) error {
	r.setState(stateReconnecting)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if exceeded := r.recordRetry(); exceeded {
			return ErrEmergencyClosed
		}
		d := r.backoff()
		slog.Info("reconnecting", "backoff", d, "state", r.getState().String())
		select {
		case <-time.After(d):
		case <-ctx.Done():
			return ctx.Err()
		}
		if err := connectFn(); err != nil {
			slog.Warn("reconnect attempt failed", "error", err)
			continue
		}
		r.setState(stateConnected)
		r.resetRetries()
		return nil
	}
}
