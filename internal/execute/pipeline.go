package execute

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"arb/internal/adapter"
	"arb/internal/bus"
	"arb/internal/risk"
)

// Leg represents one leg of an arbitrage opportunity.
type Leg struct {
	Broker    string
	Symbol    string
	Operation adapter.OrderOperation
	Volume    float64
	Price     float64
	Slippage  int32
	ClientID  string
}

// LegResult is the outcome of executing one leg.
type LegResult struct {
	Leg   Leg
	Order *adapter.OrderResult
	Err   error
}

// ArbitrageOpportunity describes a multi-leg arbitrage trade.
type ArbitrageOpportunity struct {
	Legs    []Leg
	Params  StrategyParams
}

// Notional returns the total notional value of all legs.
func (o ArbitrageOpportunity) Notional() float64 {
	total := 0.0
	for _, leg := range o.Legs {
		total += leg.Price * leg.Volume * 100000 // standard lot size
	}
	return total
}

// StrategyParams holds execution parameters for a strategy.
type StrategyParams struct {
	OrderTimeout time.Duration
	MaxSlippage  float64 // bps
}

// ErrOrderTimeout is returned when a leg times out.
var ErrOrderTimeout = errors.New("order timeout")

// ErrRevalidationFailed is returned when pre-trade price check fails.
var ErrRevalidationFailed = errors.New("pre-trade revalidation failed")

// PipelineDeps holds the dependencies for the execution pipeline.
type PipelineDeps struct {
	Bus       *bus.QuoteBus
	Gate      *risk.CapitalGate
	Dedup     *DedupCache
	Adapters  map[string]adapter.PlatformAdapter
	AuditLog  func(LegResult)
}

// ExecutionPipeline handles 4-phase arbitrage execution.
type ExecutionPipeline struct {
	deps PipelineDeps
}

// NewPipeline creates an ExecutionPipeline with the given dependencies.
func NewPipeline(deps PipelineDeps) *ExecutionPipeline {
	return &ExecutionPipeline{deps: deps}
}

// Execute runs the 4-phase execution pipeline for an arbitrage opportunity.
func (p *ExecutionPipeline) Execute(ctx context.Context, opp ArbitrageOpportunity) error {
	// Phase 1: Pre-trade revalidation
	if err := p.revalidate(ctx, opp); err != nil {
		return fmt.Errorf("%w: %v", ErrRevalidationFailed, err)
	}

	// Phase 1.5: Capital gate
	if p.deps.Gate != nil {
		if err := p.deps.Gate.Allow(opp); err != nil {
			return fmt.Errorf("capital gate: %w", err)
		}
	}

	// Phase 2: Concurrent submit
	ctx, cancel := context.WithTimeout(ctx, opp.Params.OrderTimeout)
	defer cancel()

	results := make(chan LegResult, len(opp.Legs))
	for _, leg := range opp.Legs {
		go func(l Leg) {
			results <- p.executeLeg(ctx, l)
		}(leg)
	}

	// Phase 3: Collect — all or nothing
	var filled, failed []LegResult
	for i := 0; i < len(opp.Legs); i++ {
		select {
		case r := <-results:
			if r.Err == nil && r.Order != nil && r.Order.IsFullFill() {
				filled = append(filled, r)
			} else {
				failed = append(failed, r)
			}
		case <-ctx.Done():
			failed = append(failed, LegResult{Err: ErrOrderTimeout})
		}
	}

	// Phase 4: Hedge on failure
	if len(failed) > 0 {
		for _, leg := range filled {
			p.hedge(ctx, leg)
		}
		slog.Error("arbitrage failed",
			"filled", len(filled), "failed", len(failed), "legs", len(opp.Legs))
		return fmt.Errorf("arb failed: %d/%d filled", len(filled), len(opp.Legs))
	}

	// Audit log filled orders
	if p.deps.AuditLog != nil {
		for _, r := range filled {
			p.deps.AuditLog(r)
		}
	}
	return nil
}

// revalidate checks that current quotes are still valid for all legs.
// It retries subscription with short waits to handle race conditions
// where quotes arrive after the first Subscribe call.
func (p *ExecutionPipeline) revalidate(ctx context.Context, opp ArbitrageOpportunity) error {
	for _, leg := range opp.Legs {
		q, err := p.waitForQuote(ctx, leg.Symbol)
		if err != nil {
			return err
		}
		if leg.Price > 0 {
			spread := absFloat(q.Ask-q.Bid) / q.Ask * 10000
			if spread > opp.Params.MaxSlippage {
				return fmt.Errorf("spread too wide for %s: %.2fbps > %.2fbps",
					leg.Symbol, spread, opp.Params.MaxSlippage)
			}
		}
	}
	return nil
}

// waitForQuote returns the latest quote for a symbol, or waits up to 500ms for the next one.
func (p *ExecutionPipeline) waitForQuote(ctx context.Context, symbol string) (bus.Quote, error) {
	ctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	return p.deps.Bus.LatestOrWait(ctx, symbol)
}

// executeLeg submits a single order with dedup check.
func (p *ExecutionPipeline) executeLeg(ctx context.Context, leg Leg) LegResult {
	if cached, ok := p.deps.Dedup.Get(leg.ClientID); ok {
		return LegResult{Leg: leg, Order: cached}
	}

	a, ok := p.deps.Adapters[leg.Broker]
	if !ok {
		return LegResult{Leg: leg, Err: fmt.Errorf("adapter not found: %s", leg.Broker)}
	}

	result, err := a.PlaceOrder(ctx, adapter.OrderRequest{
		ClientID:  leg.ClientID,
		Symbol:    leg.Symbol,
		Operation: leg.Operation,
		Volume:    decimalFromFloat(leg.Volume),
		Price:     leg.Price,
		Slippage:  leg.Slippage,
	})
	if err != nil {
		return LegResult{Leg: leg, Err: err}
	}
	p.deps.Dedup.Set(leg.ClientID, result)
	return LegResult{Leg: leg, Order: result}
}

// hedge closes a filled leg to unwind exposure.
func (p *ExecutionPipeline) hedge(ctx context.Context, leg LegResult) {
	if leg.Order == nil || leg.Order.Ticket == 0 {
		return
	}
	a, ok := p.deps.Adapters[leg.Leg.Broker]
	if !ok {
		slog.Error("hedge: adapter not found", "broker", leg.Leg.Broker)
		return
	}
	opposite := adapter.OpSell
	if leg.Leg.Operation == adapter.OpSell {
		opposite = adapter.OpBuy
	}
	_, err := a.PlaceOrder(ctx, adapter.OrderRequest{
		ClientID:  leg.Leg.ClientID + "-hedge",
		Symbol:    leg.Leg.Symbol,
		Operation: opposite,
		Volume:    leg.Order.Volume,
		Slippage:  leg.Leg.Slippage,
	})
	if err != nil {
		slog.Error("hedge failed", "broker", leg.Leg.Broker, "ticket", leg.Order.Ticket, "error", err)
	}
}

func absFloat(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}
