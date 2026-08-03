package adapter

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestReconnectRunningToReconnectingToRunning(t *testing.T) {
	var attempts atomic.Int32
	cfg := reconnectConfig{
		maxRetries:  5,
		retryWindow: time.Minute,
		baseBackoff: 10 * time.Millisecond,
		maxBackoff:  50 * time.Millisecond,
	}
	rsm := newReconnectStateMachine(cfg)
	rsm.setState(stateConnected)

	// Simulate recv error → reconnecting → success
	rsm.setState(stateReconnecting)
	connectFn := func() error {
		attempts.Add(1)
		return nil // succeed immediately
	}
	err := rsm.reconnectLoop(context.Background(), connectFn)
	if err != nil {
		t.Fatalf("reconnectLoop: %v", err)
	}
	if rsm.getState() != stateConnected {
		t.Errorf("state = %s, want Connected", rsm.getState())
	}
	if attempts.Load() != 1 {
		t.Errorf("attempts = %d, want 1", attempts.Load())
	}
}

func TestReconnectBackoffGrowth(t *testing.T) {
	cfg := reconnectConfig{
		maxRetries:  5,
		retryWindow: time.Minute,
		baseBackoff: 10 * time.Millisecond,
		maxBackoff:  100 * time.Millisecond,
	}
	rsm := newReconnectStateMachine(cfg)

	d0 := rsm.backoff()
	rsm.retries.Store(1)
	d1 := rsm.backoff()
	rsm.retries.Store(2)
	d2 := rsm.backoff()

	if d0 != 10*time.Millisecond {
		t.Errorf("backoff(0) = %v, want 10ms", d0)
	}
	if d1 != 20*time.Millisecond {
		t.Errorf("backoff(1) = %v, want 20ms", d1)
	}
	if d2 != 40*time.Millisecond {
		t.Errorf("backoff(2) = %v, want 40ms", d2)
	}
}

func TestReconnectExceedLimit(t *testing.T) {
	called := atomic.Bool{}
	cfg := reconnectConfig{
		maxRetries:     3,
		retryWindow:    time.Minute,
		baseBackoff:    1 * time.Millisecond,
		maxBackoff:     5 * time.Millisecond,
		emergencyClose: func() { called.Store(true) },
	}
	rsm := newReconnectStateMachine(cfg)

	connectFn := func() error { return ErrNotConnected }
	err := rsm.reconnectLoop(context.Background(), connectFn)
	if err != ErrEmergencyClosed {
		t.Errorf("err = %v, want ErrEmergencyClosed", err)
	}
	if !called.Load() {
		t.Error("emergencyClose not called")
	}
	if rsm.getState() != stateEmergencyClosed {
		t.Errorf("state = %s, want EmergencyClosed", rsm.getState())
	}
}

func TestReconnectInvalidTokenFullReconnect(t *testing.T) {
	var attempts atomic.Int32
	cfg := reconnectConfig{
		maxRetries:  5,
		retryWindow: time.Minute,
		baseBackoff: 5 * time.Millisecond,
		maxBackoff:  20 * time.Millisecond,
	}
	rsm := newReconnectStateMachine(cfg)
	rsm.setState(stateReconnecting)

	// First attempt fails (token invalid), second succeeds (full reconnect)
	connectFn := func() error {
		n := attempts.Add(1)
		if n == 1 {
			return ErrNotConnected // simulate invalid token
		}
		return nil
	}
	err := rsm.reconnectLoop(context.Background(), connectFn)
	if err != nil {
		t.Fatalf("reconnectLoop: %v", err)
	}
	if rsm.getState() != stateConnected {
		t.Errorf("state = %s, want Connected", rsm.getState())
	}
}

func TestReconnectRejectsPlaceOrder(t *testing.T) {
	cfg := reconnectConfig{
		maxRetries:  5,
		retryWindow: time.Minute,
		baseBackoff: 10 * time.Millisecond,
		maxBackoff:  50 * time.Millisecond,
	}
	rsm := newReconnectStateMachine(cfg)
	rsm.setState(stateReconnecting)
	if rsm.canPlaceOrder() {
		t.Error("canPlaceOrder should be false during reconnecting")
	}
}
