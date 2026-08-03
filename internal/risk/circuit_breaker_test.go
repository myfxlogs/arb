package risk

import (
	"testing"
	"time"
)

func TestCircuitBreakerTripsOnConsecutiveLosses(t *testing.T) {
	cb := NewCircuitBreaker(3, 5*time.Second, 5000, 500)
	for i := int32(0); i < 3; i++ {
		cb.RecordLoss(10)
	}
	if cb.State() != CircuitOpen {
		t.Errorf("state = %s, want Open", cb.State())
	}
	if err := cb.Allow(); err != ErrCircuitOpen {
		t.Errorf("Allow() err = %v, want ErrCircuitOpen", err)
	}
}

func TestCircuitBreakerTripsOnWindowLoss(t *testing.T) {
	cb := NewCircuitBreaker(100, 5*time.Second, 5000, 100)
	cb.RecordLoss(60)
	if cb.State() != CircuitClosed {
		t.Errorf("state after 60 loss = %s, want Closed", cb.State())
	}
	cb.RecordLoss(50) // total 110 > 100 window limit
	if cb.State() != CircuitOpen {
		t.Errorf("state after 110 loss = %s, want Open", cb.State())
	}
}

func TestCircuitBreakerHalfOpenAfterCooldown(t *testing.T) {
	cb := NewCircuitBreaker(1, 50*time.Millisecond, 5000, 500)
	cb.RecordLoss(10)
	if cb.State() != CircuitOpen {
		t.Fatalf("state = %s, want Open", cb.State())
	}
	time.Sleep(60 * time.Millisecond)
	if err := cb.Allow(); err != nil {
		t.Errorf("Allow() after cooldown = %v, want nil", err)
	}
	if cb.State() != CircuitHalfOpen {
		t.Errorf("state = %s, want HalfOpen", cb.State())
	}
}

func TestCircuitBreakerClosesOnHalfOpenSuccess(t *testing.T) {
	cb := NewCircuitBreaker(1, 50*time.Millisecond, 5000, 500)
	cb.RecordLoss(10)
	time.Sleep(60 * time.Millisecond)
	_ = cb.Allow() // transitions to HalfOpen
	cb.RecordSuccess()
	if cb.State() != CircuitClosed {
		t.Errorf("state = %s, want Closed", cb.State())
	}
}

func TestCircuitBreakerResetsConsecutiveOnSuccess(t *testing.T) {
	cb := NewCircuitBreaker(3, 5*time.Second, 5000, 500)
	cb.RecordLoss(10)
	cb.RecordLoss(10)
	cb.RecordSuccess() // resets consecutive
	cb.RecordLoss(10)
	if cb.State() != CircuitClosed {
		t.Errorf("state = %s, want Closed (only 1 consecutive after reset)", cb.State())
	}
}

func TestCircuitBreakerResetWindow(t *testing.T) {
	cb := NewCircuitBreaker(100, 5*time.Second, 5000, 100)
	cb.RecordLoss(80)
	cb.ResetWindow()
	cb.RecordLoss(50) // would have tripped without reset
	if cb.State() != CircuitClosed {
		t.Errorf("state = %s, want Closed after window reset", cb.State())
	}
}
