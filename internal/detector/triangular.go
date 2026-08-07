package detector

import (
	"context"

	"github.com/shopspring/decimal"

	"arb/internal/bus"
	"arb/internal/evaluator"
	"arb/internal/listing"
)

type triangularDetector struct{}

func (d *triangularDetector) Type() evaluator.OppType { return evaluator.OppTriangular }

// triDef defines a triangular arbitrage set: three currencies and the
// three canonical pairs that connect them (13 §5).
// Each pair is listed as "base/quote" (e.g. "EURUSD" = EUR/USD).
// The cycle is: start with ccy1 → ccy2 → ccy3 → ccy1.
//
//	Direction 1 (forward): sell pair1 @ Bid, buy pair2 @ Ask, buy pair3 @ Ask
//	Product1 = Bid_pair1 / (Ask_pair2 × Ask_pair3)
//
//	Direction 2 (reverse): sell pair3 @ Bid, sell pair2 @ Bid, buy pair1 @ Ask
//	Product2 = (Bid_pair3 × Bid_pair2) / Ask_pair1
type triDef struct {
	ccy1  string
	ccy2  string
	ccy3  string
	pair1 string // ccy1/ccy2 (sell to go ccy1→ccy2)
	pair2 string // ccy2/ccy3 (buy to go ccy2→ccy3)
	pair3 string // ccy1/ccy3 (buy to go ccy3→ccy1)
}

var knownTriangles = []triDef{
	{"EUR", "USD", "GBP", "EURUSD", "GBPUSD", "EURGBP"},
	{"EUR", "USD", "JPY", "EURUSD", "USDJPY", "EURJPY"},
	{"EUR", "USD", "CHF", "EURUSD", "USDCHF", "EURCHF"},
	{"EUR", "GBP", "JPY", "EURGBP", "GBPJPY", "EURJPY"},
	{"USD", "GBP", "JPY", "GBPUSD", "USDJPY", "GBPJPY"},
}

// Scan finds triangular arbitrage within a single broker (13 §5).
// For each broker that has all three pairs of a known triangle,
// checks both cycle directions. Product > 1 → Candidate.
func (d *triangularDetector) Scan(
	_ context.Context,
	quotes map[string]bus.Quote,
	listings map[listing.CanonicalKey]*listing.Listing,
) ([]evaluator.Candidate, error) {
	// Group listings by broker → canonical → listing
	brokerMap := make(map[string]map[string]*listing.Listing)
	for key, l := range listings {
		if brokerMap[key.Broker] == nil {
			brokerMap[key.Broker] = make(map[string]*listing.Listing)
		}
		brokerMap[key.Broker][key.Canonical] = l
	}

	var out []evaluator.Candidate
	for broker, canons := range brokerMap {
		for _, tri := range knownTriangles {
			c := checkTriangle(broker, canons, quotes, tri)
			out = append(out, c...)
		}
	}
	return out, nil
}

func checkTriangle(
	broker string,
	canons map[string]*listing.Listing,
	quotes map[string]bus.Quote,
	tri triDef,
) []evaluator.Candidate {
	l1, ok1 := canons[tri.pair1]
	l2, ok2 := canons[tri.pair2]
	l3, ok3 := canons[tri.pair3]
	if !ok1 || !ok2 || !ok3 {
		return nil
	}
	q1, ok1q := quotes[l1.BrokerSymbol]
	q2, ok2q := quotes[l2.BrokerSymbol]
	q3, ok3q := quotes[l3.BrokerSymbol]
	if !ok1q || !ok2q || !ok3q {
		return nil
	}

	var out []evaluator.Candidate

	// Direction 1 (forward): ccy1 → ccy2 → ccy3 → ccy1
	// Sell pair1 @ Bid, Buy pair2 @ Ask, Buy pair3 @ Ask
	// Product = Bid_pair1 / (Ask_pair2 × Ask_pair3)
	bid1 := decimal.NewFromFloat(q1.Bid)
	ask2 := decimal.NewFromFloat(q2.Ask)
	ask3 := decimal.NewFromFloat(q3.Ask)
	denom := ask2.Mul(ask3)
	if !denom.IsZero() {
		product := bid1.Div(denom)
		if product.GreaterThan(decimal.NewFromInt(1)) {
			out = append(out, makeTriCandidate(broker, tri, l1, l2, l3, q1, q2, q3,
				product, true))
		}
	}

	// Direction 2 (reverse): ccy1 → ccy3 → ccy2 → ccy1
	// Sell pair3 @ Bid, Sell pair2 @ Bid, Buy pair1 @ Ask
	// Product = (Bid_pair3 × Bid_pair2) / Ask_pair1
	bid3 := decimal.NewFromFloat(q3.Bid)
	bid2 := decimal.NewFromFloat(q2.Bid)
	ask1 := decimal.NewFromFloat(q1.Ask)
	if !ask1.IsZero() {
		product := bid3.Mul(bid2).Div(ask1)
		if product.GreaterThan(decimal.NewFromInt(1)) {
			out = append(out, makeTriCandidate(broker, tri, l1, l2, l3, q1, q2, q3,
				product, false))
		}
	}

	return out
}

func makeTriCandidate(
	broker string,
	tri triDef,
	l1, l2, l3 *listing.Listing,
	q1, q2, q3 bus.Quote,
	product decimal.Decimal,
	forward bool,
) evaluator.Candidate {
	// Use minimum contract size among the three legs as base
	minCS := l1.ContractSize
	if l2.ContractSize.LessThan(minCS) {
		minCS = l2.ContractSize
	}
	if l3.ContractSize.LessThan(minCS) {
		minCS = l3.ContractSize
	}
	lots := l1.VolumeMin
	grossProfit := product.Sub(decimal.NewFromInt(1)).Mul(minCS).Mul(lots)

	quoteTime := earliestTime(
		earliestTime(q1.Time, q2.Time),
		q3.Time,
	)

	var legs []evaluator.CandidateLeg
	if forward {
		legs = []evaluator.CandidateLeg{
			legDef(broker, tri.pair1, l1, evaluator.Sell, decimal.NewFromFloat(q1.Bid), lots),
			legDef(broker, tri.pair2, l2, evaluator.Buy, decimal.NewFromFloat(q2.Ask), lots),
			legDef(broker, tri.pair3, l3, evaluator.Buy, decimal.NewFromFloat(q3.Ask), lots),
		}
	} else {
		legs = []evaluator.CandidateLeg{
			legDef(broker, tri.pair3, l3, evaluator.Sell, decimal.NewFromFloat(q3.Bid), lots),
			legDef(broker, tri.pair2, l2, evaluator.Sell, decimal.NewFromFloat(q2.Bid), lots),
			legDef(broker, tri.pair1, l1, evaluator.Buy, decimal.NewFromFloat(q1.Ask), lots),
		}
	}

	return evaluator.Candidate{
		Type:        evaluator.OppTriangular,
		GrossProfit: grossProfit,
		QuoteTime:   quoteTime,
		Legs:        legs,
	}
}

func legDef(broker, canonical string, l *listing.Listing,
	dir evaluator.BuySell, price, lots decimal.Decimal) evaluator.CandidateLeg {
	return evaluator.CandidateLeg{
		Broker:       broker,
		BrokerSymbol: l.BrokerSymbol,
		Canonical:    canonical,
		Direction:    dir,
		Lots:         lots,
		EstPrice:     price,
	}
}
