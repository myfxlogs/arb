package risk

import (
	"sync"
	"time"
)

// AdaptiveLimiter implements a token-bucket rate limiter that adapts
// its rate based on success/failure feedback. On consecutive failures,
// the rate halves; on consecutive successes, it recovers toward the initial rate.
type AdaptiveLimiter struct {
	mu               sync.Mutex
	current          float64       // tokens per second
	initial          float64       // initial tokens per second
	min              float64       // minimum tokens per second
	tokens           float64       // current available tokens
	lastRefill       time.Time
	consecutiveFails int
	consecutiveOK    int
	failThreshold    int           // failures before reducing rate
	okThreshold      int           // successes before recovering rate
}

// NewAdaptiveLimiter creates an AdaptiveLimiter with the given initial rate.
func NewAdaptiveLimiter(initialPerSec float64) *AdaptiveLimiter {
	return &AdaptiveLimiter{
		current:       initialPerSec,
		initial:       initialPerSec,
		min:           initialPerSec / 16,
		tokens:        initialPerSec,
		lastRefill:    time.Now(),
		failThreshold: 3,
		okThreshold:   5,
	}
}

// Allow attempts to consume one token. Returns true if allowed, false if rate limited.
func (l *AdaptiveLimiter) Allow() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.refill()
	if l.tokens >= 1 {
		l.tokens--
		return true
	}
	return false
}

// RecordSuccess records a successful operation, potentially increasing the rate.
func (l *AdaptiveLimiter) RecordSuccess() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.consecutiveFails = 0
	l.consecutiveOK++
	if l.consecutiveOK >= l.okThreshold && l.current < l.initial {
		l.current = l.current * 2
		if l.current > l.initial {
			l.current = l.initial
		}
		l.consecutiveOK = 0
	}
}

// RecordFailure records a failed operation, potentially decreasing the rate.
func (l *AdaptiveLimiter) RecordFailure() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.consecutiveOK = 0
	l.consecutiveFails++
	if l.consecutiveFails >= l.failThreshold && l.current > l.min {
		l.current = l.current / 2
		if l.current < l.min {
			l.current = l.min
		}
		l.consecutiveFails = 0
	}
}

// CurrentRate returns the current rate in tokens per second.
func (l *AdaptiveLimiter) CurrentRate() float64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.current
}

// refill adds tokens based on elapsed time since last refill.
func (l *AdaptiveLimiter) refill() {
	now := time.Now()
	elapsed := now.Sub(l.lastRefill).Seconds()
	l.tokens += elapsed * l.current
	if l.tokens > l.current {
		l.tokens = l.current
	}
	l.lastRefill = now
}
