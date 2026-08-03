package risk

import (
	"testing"
	"time"
)

func TestAdaptiveLimiterAllowsUnderRate(t *testing.T) {
	l := NewAdaptiveLimiter(100)
	for i := 0; i < 10; i++ {
		if !l.Allow() {
			t.Fatalf("Allow() returned false on call %d with rate 100/s", i)
		}
	}
}

func TestAdaptiveLimiterBlocksOverRate(t *testing.T) {
	l := NewAdaptiveLimiter(1)
	// Consume the initial token
	if !l.Allow() {
		t.Fatal("first Allow() should succeed")
	}
	// Immediately try again — should be blocked (no time for refill)
	if l.Allow() {
		t.Fatal("second immediate Allow() should be blocked")
	}
}

func TestAdaptiveLimiterReducesRateOnFailures(t *testing.T) {
	l := NewAdaptiveLimiter(100)
	for i := 0; i < 3; i++ {
		l.RecordFailure()
	}
	if l.CurrentRate() >= 100 {
		t.Errorf("rate = %.1f, expected < 100 after 3 failures", l.CurrentRate())
	}
}

func TestAdaptiveLimiterRecoversOnSuccess(t *testing.T) {
	l := NewAdaptiveLimiter(100)
	// Reduce rate first
	for i := 0; i < 3; i++ {
		l.RecordFailure()
	}
	reduced := l.CurrentRate()
	if reduced >= 100 {
		t.Fatalf("rate not reduced: %.1f", reduced)
	}
	// Now record successes to recover
	for i := 0; i < 5; i++ {
		l.RecordSuccess()
	}
	if l.CurrentRate() <= reduced {
		t.Errorf("rate = %.1f, expected > %.1f after recovery", l.CurrentRate(), reduced)
	}
}

func TestAdaptiveLimiterMinRate(t *testing.T) {
	l := NewAdaptiveLimiter(100)
	// Trigger many failures to hit minimum
	for i := 0; i < 100; i++ {
		l.RecordFailure()
	}
	if l.CurrentRate() < 100.0/16 {
		t.Errorf("rate = %.1f, expected >= %.1f (minimum)", l.CurrentRate(), 100.0/16)
	}
}

func TestAdaptiveLimiterRefillOverTime(t *testing.T) {
	l := NewAdaptiveLimiter(100)
	// Consume all tokens
	for i := 0; i < 100; i++ {
		l.Allow()
	}
	if l.Allow() {
		t.Fatal("should be rate limited after consuming all tokens")
	}
	time.Sleep(20 * time.Millisecond)
	if !l.Allow() {
		t.Fatal("should allow after refill time")
	}
}
