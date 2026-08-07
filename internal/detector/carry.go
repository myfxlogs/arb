package detector

import (
	"context"

	"github.com/shopspring/decimal"

	"arb/internal/bus"
	"arb/internal/evaluator"
	"arb/internal/listing"
)

type carryDetector struct{}

func (d *carryDetector) Type() evaluator.OppType { return evaluator.OppCarry }

// Scan finds broker pairs where hedged net swap is positive (13 §4).
// For each canonical, checks every broker pair in both directions.
// Uses evaluator.DailySwap to compute per-leg daily swap in profit currency.
// Same canonical → same ProfitCurrency → daily swaps can be summed directly.
func (d *carryDetector) Scan(
	_ context.Context,
	quotes map[string]bus.Quote,
	listings map[listing.CanonicalKey]*listing.Listing,
) ([]evaluator.Candidate, error) {
	groups := groupByCanonical(listings, quotes)
	var out []evaluator.Candidate

	for canonical, g := range groups {
		if len(g) < 2 {
			continue
		}
		for i := 0; i < len(g); i++ {
			for j := i + 1; j < len(g); j++ {
				if c := checkCarryPair(canonical, g[i], g[j]); c != nil {
					out = append(out, *c)
				}
				if c := checkCarryPair(canonical, g[j], g[i]); c != nil {
					out = append(out, *c)
				}
			}
		}
	}
	return out, nil
}

// checkCarryPair checks A Buy + B Sell for positive net swap.
// dailyA = DailySwap(A, Buy, VolMin_A, midA)
// dailyB = DailySwap(B, Sell, VolMin_B, midB)
// netSwap = dailyA + dailyB (same canonical → same profit currency)
// If netSwap > 0 → produce Candidate with GrossProfit=0.
func checkCarryPair(canonical string, a, b groupEntry) *evaluator.Candidate {
	midA := decimal.NewFromFloat((a.q.Bid + a.q.Ask) / 2)
	midB := decimal.NewFromFloat((b.q.Bid + b.q.Ask) / 2)

	dailyA, err := evaluator.DailySwap(a.l, evaluator.Buy, a.l.VolumeMin, midA)
	if err != nil {
		return nil
	}
	dailyB, err := evaluator.DailySwap(b.l, evaluator.Sell, b.l.VolumeMin, midB)
	if err != nil {
		return nil
	}

	netSwap := dailyA.Add(dailyB)
	if netSwap.LessThanOrEqual(decimal.Zero) {
		return nil
	}

	quoteTime := earliestTime(a.q.Time, b.q.Time)

	return &evaluator.Candidate{
		Type:        evaluator.OppCarry,
		GrossProfit: decimal.Zero,
		QuoteTime:   quoteTime,
		Legs: []evaluator.CandidateLeg{
			{
				Broker:       a.broker,
				BrokerSymbol: a.brokerSymbol,
				Canonical:    canonical,
				Direction:    evaluator.Buy,
				Lots:         a.l.VolumeMin,
				EstPrice:     midA,
			},
			{
				Broker:       b.broker,
				BrokerSymbol: b.brokerSymbol,
				Canonical:    canonical,
				Direction:    evaluator.Sell,
				Lots:         b.l.VolumeMin,
				EstPrice:     midB,
			},
		},
	}
}
