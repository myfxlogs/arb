package risk

import (
	"sync"
	"time"
)

// CircuitState represents the state of the circuit breaker.
type CircuitState int32

const (
	CircuitClosed   CircuitState = iota // normal operation
	CircuitHalfOpen                     // testing if service recovered
	CircuitOpen                         // blocking all requests
)

func (s CircuitState) String() string {
	switch s {
	case CircuitClosed:
		return "Closed"
	case CircuitHalfOpen:
		return "HalfOpen"
	case CircuitOpen:
		return "Open"
	default:
		return "Unknown"
	}
}

// CircuitBreaker implements a 3-state circuit breaker for order execution.
// After maxConsecutiveLosses failures, it opens and blocks requests.
// After a cooldown period, it enters half-open state, allowing one trial request.
type CircuitBreaker struct {
	mu               sync.Mutex
	state            CircuitState
	maxConsecutive   int32
	consecutiveLoss  int32
	cooldown         time.Duration
	openedAt         time.Time
	dailyLossLimit   float64
	windowLossLimit  float64
	currentWindowLoss float64
	windowStart      time.Time
}

// NewCircuitBreaker creates a CircuitBreaker with the given risk parameters.
func NewCircuitBreaker(maxConsecutiveLosses int32, cooldown time.Duration,
	dailyLossLimit, windowLossLimit float64) *CircuitBreaker {
	return &CircuitBreaker{
		state:           CircuitClosed,
		maxConsecutive:  maxConsecutiveLosses,
		cooldown:        cooldown,
		dailyLossLimit:  dailyLossLimit,
		windowLossLimit: windowLossLimit,
		windowStart:     time.Now(),
	}
}

// Allow checks if a request can proceed. Returns nil if allowed,
// or an error explaining why the request was blocked.
func (c *CircuitBreaker) Allow() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	switch c.state {
	case CircuitOpen:
		if time.Since(c.openedAt) > c.cooldown {
			c.state = CircuitHalfOpen
			return nil
		}
		return ErrCircuitOpen
	case CircuitHalfOpen:
		return nil
	default:
		return nil
	}
}

// RecordSuccess resets the consecutive loss counter.
func (c *CircuitBreaker) RecordSuccess() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.consecutiveLoss = 0
	if c.state == CircuitHalfOpen {
		c.state = CircuitClosed
	}
}

// RecordLoss increments the loss counter and may trip the breaker.
// lossAmount is the monetary loss for this trade (positive = loss).
func (c *CircuitBreaker) RecordLoss(lossAmount float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.consecutiveLoss++
	c.currentWindowLoss += lossAmount

	// Check consecutive losses
	if c.consecutiveLoss >= c.maxConsecutive {
		c.trip()
		return
	}

	// Check window loss limit
	if c.currentWindowLoss >= c.windowLossLimit {
		c.trip()
		return
	}
}

// trip opens the circuit breaker.
func (c *CircuitBreaker) trip() {
	c.state = CircuitOpen
	c.openedAt = time.Now()
}

// State returns the current circuit state.
func (c *CircuitBreaker) State() CircuitState {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state
}

// ResetWindow resets the loss window (called on a new trading day).
func (c *CircuitBreaker) ResetWindow() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.currentWindowLoss = 0
	c.windowStart = time.Now()
}
