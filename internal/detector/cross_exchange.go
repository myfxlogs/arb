package detector

import (
	"context"
	"time"

	"github.com/shopspring/decimal"

	"arb/internal/bus"
	"arb/internal/evaluator"
	"arb/internal/listing"
)

type crossExchangeDetector struct{}

func (d *crossExchangeDetector) Type() evaluator.OppType { return evaluator.OppCrossExchange }

// Scan finds cross-broker spread opportunities (13 §3).
// For each canonical, checks every broker pair: if A.Ask < B.Bid,
// the spread is positive → produce a Candidate.
func (d *crossExchangeDetector) Scan(
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
				c := checkPair(canonical, g[i], g[j])
				if c != nil {
					out = append(out, *c)
				}
				c = checkPair(canonical, g[j], g[i])
				if c != nil {
					out = append(out, *c)
				}
			}
		}
	}
	return out, nil
}

// groupEntry holds resolved data for one broker within a canonical group.
type groupEntry struct {
	broker       string
	brokerSymbol string
	l            *listing.Listing
	q            bus.Quote
}

// checkPair checks if buyA/sellB produces a positive spread.
// A Buy: pay Ask_A. B Sell: receive Bid_B. Spread = Bid_B − Ask_A.
func checkPair(canonical string, a, b groupEntry) *evaluator.Candidate {
	askA := decimal.NewFromFloat(a.q.Ask)
	bidB := decimal.NewFromFloat(b.q.Bid)
	spread := bidB.Sub(askA)
	if spread.LessThanOrEqual(decimal.Zero) {
		return nil
	}

	lotsA := a.l.VolumeMin
	grossProfit := spread.Mul(a.l.ContractSize).Mul(lotsA)

	quoteTime := earliestTime(a.q.Time, b.q.Time)

	return &evaluator.Candidate{
		Type:        evaluator.OppCrossExchange,
		GrossProfit: grossProfit,
		QuoteTime:   quoteTime,
		Legs: []evaluator.CandidateLeg{
			{
				Broker:       a.broker,
				BrokerSymbol: a.brokerSymbol,
				Canonical:    canonical,
				Direction:    evaluator.Buy,
				Lots:         lotsA,
				EstPrice:     askA,
			},
			{
				Broker:       b.broker,
				BrokerSymbol: b.brokerSymbol,
				Canonical:    canonical,
				Direction:    evaluator.Sell,
				Lots:         b.l.VolumeMin,
				EstPrice:     bidB,
			},
		},
	}
}

func earliestTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

// groupByCanonical collects all broker entries per canonical that have
// both a listing and a quote.
func groupByCanonical(
	listings map[listing.CanonicalKey]*listing.Listing,
	quotes map[string]bus.Quote,
) map[string][]groupEntry {
	groups := make(map[string][]groupEntry)
	for key, l := range listings {
		q, ok := quotes[l.BrokerSymbol]
		if !ok {
			continue
		}
		groups[key.Canonical] = append(groups[key.Canonical], groupEntry{
			broker:       key.Broker,
			brokerSymbol: l.BrokerSymbol,
			l:            l,
			q:            q,
		})
	}
	return groups
}
