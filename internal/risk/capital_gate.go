package risk

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/shopspring/decimal"
)

// CapitalGate checks pre-trade capital constraints.
type CapitalGate struct {
	mu               sync.RWMutex
	maxNotional      decimal.Decimal
	maxConcurrent    int32
	currentConcurrent atomic.Int32
}

// NewCapitalGate creates a CapitalGate with the given limits.
func NewCapitalGate(maxNotional float64, maxConcurrent int32) *CapitalGate {
	return &CapitalGate{
		maxNotional:   decimal.NewFromFloat(maxNotional),
		maxConcurrent: maxConcurrent,
	}
}

// Allow checks if the trade passes capital constraints.
// The opportunity must implement Notional() float64.
func (g *CapitalGate) Allow(opp interface{ Notional() float64 }) error {
	g.mu.RLock()
	notional := decimal.NewFromFloat(opp.Notional())
	if notional.GreaterThan(g.maxNotional) {
		g.mu.RUnlock()
		return fmt.Errorf("notional %s exceeds max %s", notional.String(), g.maxNotional.String())
	}
	g.mu.RUnlock()

	current := g.currentConcurrent.Add(1)
	if current > g.maxConcurrent {
		g.currentConcurrent.Add(-1)
		return fmt.Errorf("concurrent orders %d exceeds max %d", current, g.maxConcurrent)
	}
	return nil
}

// Release decrements the concurrent order counter after a trade completes.
func (g *CapitalGate) Release() {
	g.currentConcurrent.Add(-1)
}
