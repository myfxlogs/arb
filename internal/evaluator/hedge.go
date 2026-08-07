package evaluator

import (
	"errors"
	"fmt"

	"github.com/shopspring/decimal"

	"arb/internal/bus"
	"arb/internal/listing"
)

// HedgeResult holds the normalized lots and whether the notional
// mismatch is within tolerance.
type HedgeResult struct {
	Lots        []decimal.Decimal
	Notional    decimal.Decimal // base leg notional (in price currency)
	WithinTol   bool
	VolumeOK    bool
}

// Normalize computes hedge lot sizes so that each leg's notional value
// equals the base leg's notional, rounded to each leg's VolumeStep.
// The first leg is the base (12 §4.2 / 02 §3.1).
//
// quotes maps brokerSymbol → Quote (must contain each leg's brokerSymbol).
// listings provides ContractSize, VolumeMin/Max/Step per leg.
func Normalize(
	legs []CandidateLeg,
	listings *listing.Cache,
	quotes map[string]bus.Quote,
	tolerancePct float64,
) (*HedgeResult, error) {
	if len(legs) < 2 {
		return nil, errors.New("need at least 2 legs")
	}

	// Resolve listings and quotes for all legs.
	ls := make([]*listing.Listing, len(legs))
	qs := make([]bus.Quote, len(legs))
	for i, leg := range legs {
		l, ok := listings.Get(leg.Broker, leg.BrokerSymbol)
		if !ok {
			return nil, fmt.Errorf("listing not found: %s/%s", leg.Broker, leg.BrokerSymbol)
		}
		ls[i] = l
		q, ok := quotes[leg.BrokerSymbol]
		if !ok {
			return nil, fmt.Errorf("quote not found: %s", leg.BrokerSymbol)
		}
		qs[i] = q
	}

	// Base leg: lots = max(VolumeMin, givenLots)
	lotsA := legs[0].Lots
	if lotsA.LessThan(ls[0].VolumeMin) {
		lotsA = ls[0].VolumeMin
	}
	priceA := legPrice(qs[0], legs[0].Direction)
	notionalA := ls[0].ContractSize.Mul(lotsA).Mul(priceA)

	out := make([]decimal.Decimal, len(legs))
	out[0] = lotsA

	volumeOK := true
	for i := 1; i < len(legs); i++ {
		priceB := legPrice(qs[i], legs[i].Direction)
		denom := ls[i].ContractSize.Mul(priceB)
		if denom.IsZero() {
			return nil, fmt.Errorf("zero notional denominator for leg %d", i)
		}
		lotsRaw := notionalA.Div(denom)
		lotsB := roundToStep(lotsRaw, ls[i].VolumeStep)

		// Volume bounds check
		if lotsB.LessThan(ls[i].VolumeMin) {
			lotsB = ls[i].VolumeMin
			volumeOK = false
		}
		if lotsB.GreaterThan(ls[i].VolumeMax) {
			lotsB = ls[i].VolumeMax
			volumeOK = false
		}
		out[i] = lotsB
	}

	// Check notional deviation for all non-base legs
	withinTol := true
	tol := decimal.NewFromFloat(tolerancePct).Div(decimal.NewFromInt(100))
	for i := 1; i < len(legs); i++ {
		priceB := legPrice(qs[i], legs[i].Direction)
		notionalB := ls[i].ContractSize.Mul(out[i]).Mul(priceB)
		diff := notionalA.Sub(notionalB).Abs()
		ratio := diff.Div(notionalA)
		if ratio.GreaterThan(tol) {
			withinTol = false
		}
	}

	return &HedgeResult{
		Lots:     out,
		Notional: notionalA,
		WithinTol: withinTol,
		VolumeOK: volumeOK,
	}, nil
}

// legPrice returns the appropriate side of the quote for the leg direction.
// Buy → ask (you pay the ask), Sell → bid (you receive the bid).
func legPrice(q bus.Quote, dir BuySell) decimal.Decimal {
	if dir == Buy {
		return decimal.NewFromFloat(q.Ask)
	}
	return decimal.NewFromFloat(q.Bid)
}

// roundToStep rounds d to the nearest multiple of step.
func roundToStep(d, step decimal.Decimal) decimal.Decimal {
	if step.IsZero() {
		return d
	}
	// d / step → round → × step
	rounded := d.Div(step).Round(0)
	return rounded.Mul(step)
}
