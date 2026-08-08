package engine

import (
	"context"
	"log/slog"

	"arb/internal/evaluator"
	"arb/internal/store"
)

type OppWriter interface {
	WriteOpportunity(ctx context.Context, r store.OpportunityRecord) error
	UpdateOpportunityStatus(ctx context.Context, id, status string) error
}

func oppTypeString(t evaluator.OppType) string {
	switch t {
	case evaluator.OppCrossExchange:
		return "CROSS_EXCHANGE"
	case evaluator.OppCarry:
		return "CARRY"
	case evaluator.OppTriangular:
		return "TRIANGULAR"
	}
	return "UNKNOWN"
}

func oppStatusString(s evaluator.OppStatus) string {
	switch s {
	case evaluator.OppStatusPushed:
		return "PUSHED"
	case evaluator.OppStatusConfirmed:
		return "CONFIRMED"
	case evaluator.OppStatusExecuting:
		return "EXECUTING"
	case evaluator.OppStatusFilled:
		return "FILLED"
	case evaluator.OppStatusFailed:
		return "FAILED"
	case evaluator.OppStatusExpired:
		return "EXPIRED"
	}
	return "UNKNOWN"
}

func dirString(d evaluator.BuySell) string {
	if d == evaluator.Sell {
		return "SELL"
	}
	return "BUY"
}

func toOppRecord(opp *evaluator.Opportunity) store.OpportunityRecord {
	legs := make([]map[string]any, len(opp.Legs))
	for i, l := range opp.Legs {
		legs[i] = map[string]any{
			"broker":        l.Broker,
			"broker_symbol": l.BrokerSymbol,
			"canonical":     l.Canonical,
			"direction":     dirString(l.Direction),
			"lots":          l.Lots.String(),
			"est_price":     l.EstPrice.String(),
		}
	}
	legsJSON, _ := store.MarshalLegs(legs)
	return store.OpportunityRecord{
		ID:             opp.ID,
		Type:           oppTypeString(opp.Type),
		Status:         oppStatusString(opp.Status),
		Legs:           legsJSON,
		GrossProfit:    opp.GrossProfit.String(),
		SpreadCost:     opp.SpreadCost.String(),
		CommissionCost: opp.CommissionCost.String(),
		SlippageCost:   opp.SlippageCost.String(),
		SwapCost:       opp.SwapCost.String(),
		NetProfit:      opp.NetProfit.String(),
		NetBps:         opp.NetBps.String(),
		QuoteTime:      opp.QuoteTime,
		ExpiresAt:      opp.ExpiresAt,
		Confidence:     float32(opp.Confidence),
	}
}

func (e *Engine) writeOpp(opp *evaluator.Opportunity) {
if e.deps.OppStore == nil {
return
}
r := toOppRecord(opp)
if err := e.deps.OppStore.WriteOpportunity(e.runCtx, r); err != nil {
slog.Warn("engine: write opportunity", "id", opp.ID, "error", err)
}
}

func (e *Engine) updateOppStatus(id, status string) {
if e.deps.OppStore == nil {
return
}
if err := e.deps.OppStore.UpdateOpportunityStatus(e.runCtx, id, status); err != nil {
slog.Warn("engine: update opportunity status", "id", id, "error", err)
}
}
