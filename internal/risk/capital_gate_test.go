package risk

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestCapitalGateAllowsUnderLimit(t *testing.T) {
	g := NewCapitalGate(100000, 5)
	opp := testOpp{notional: 50000}
	if err := g.Allow(opp); err != nil {
		t.Fatalf("Allow: %v", err)
	}
}

func TestCapitalGateRejectsOverNotional(t *testing.T) {
	g := NewCapitalGate(10000, 5)
	opp := testOpp{notional: 20000}
	if err := g.Allow(opp); err == nil {
		t.Fatal("expected notional limit error")
	}
}

func TestCapitalGateRejectsOverConcurrent(t *testing.T) {
	g := NewCapitalGate(1000000, 2)
	opp := testOpp{notional: 1000}
	_ = g.Allow(opp)
	_ = g.Allow(opp)
	if err := g.Allow(opp); err == nil {
		t.Fatal("expected concurrent limit error")
	}
}

func TestCapitalGateRelease(t *testing.T) {
	g := NewCapitalGate(1000000, 2)
	opp := testOpp{notional: 1000}
	_ = g.Allow(opp)
	_ = g.Allow(opp)
	g.Release()
	if err := g.Allow(opp); err != nil {
		t.Fatalf("Allow after Release: %v", err)
	}
}

type testOpp struct {
	notional float64
}

func (t testOpp) Notional() float64 { return t.notional }

// Ensure decimal import is used (for future expansion)
var _ = decimal.NewFromInt
