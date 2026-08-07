package evaluator

import (
	"context"
	"fmt"

	"github.com/shopspring/decimal"

	"arb/internal/bus"
	"arb/internal/listing"
)

// CarryResult holds the Carry-specific computed fields (12 §4.5).
type CarryResult struct {
	NetSwapPerDay    decimal.Decimal
	AnnualizedNetBps decimal.Decimal
	LegRoles         []LegRole
	LegDailySwapUSD  []decimal.Decimal
	LegAnnualizedBps []decimal.Decimal
	SwapCost         decimal.Decimal // NetSwapPerDay × HoldDays (can be negative = income)
}

// Compute calculates Carry-specific annualized metrics (12 §4.5 / 02 §5.1).
// Only called when Type == OppCarry.
//
// legs: the evaluated legs with normalized lots.
// listings: cache for profit currency + contract size.
// rates: for converting per-leg swap to USD.
// notionalUSD: base notional in USD.
// quotes: latest quotes for price lookup.
func Compute(
	ctx context.Context,
	legs []OppLeg,
	listings *listing.Cache,
	rates RateResolver,
	quotes map[string]bus.Quote,
	cfg Config,
	notionalUSD decimal.Decimal,
) (*CarryResult, error) {
	if len(legs) == 0 {
		return nil, fmt.Errorf("no legs")
	}

	roles := make([]LegRole, len(legs))
	dailySwapUSD := make([]decimal.Decimal, len(legs))
	annualizedBps := make([]decimal.Decimal, len(legs))

	netSwapPerDayUSD := decimal.Zero

	for i, leg := range legs {
		l, ok := listings.Get(leg.Broker, leg.BrokerSymbol)
		if !ok {
			return nil, fmt.Errorf("listing not found: %s/%s", leg.Broker, leg.BrokerSymbol)
		}
		q := quotes[leg.BrokerSymbol]
		price := decimal.NewFromFloat(q.Bid)
		if leg.Direction == Buy {
			price = decimal.NewFromFloat(q.Ask)
		}

		dailyCcy, err := DailySwap(l, leg.Direction, leg.Lots, price)
		if err != nil {
			return nil, fmt.Errorf("swap leg %d: %w", i, err)
		}

		usd, err := ConvertToUSD(ctx, dailyCcy, l.ProfitCurrency, rates)
		if err != nil {
			return nil, fmt.Errorf("convert swap leg %d: %w", i, err)
		}
		dailySwapUSD[i] = usd
		netSwapPerDayUSD = netSwapPerDayUSD.Add(usd)

		// LegRole
		switch {
		case usd.GreaterThan(decimal.Zero):
			roles[i] = LegRoleIncome
		case usd.LessThan(decimal.Zero):
			roles[i] = LegRoleHedge
		default:
			roles[i] = LegRoleOmitted
		}

		// Per-leg annualized bps
		legNotionalCcy := l.ContractSize.Mul(leg.Lots).Mul(price)
		legNotionalUSD, _ := ConvertToUSD(ctx, legNotionalCcy, l.ProfitCurrency, rates)
		if !legNotionalUSD.IsZero() {
			annualizedBps[i] = usd.Mul(decimal.NewFromInt(365)).
				Div(legNotionalUSD).Mul(decimal.NewFromInt(10000))
		}
	}

	// AnnualizedNetBps = NetSwapPerDay × 365 / Notional × 10000
	var annualizedNetBps decimal.Decimal
	if !notionalUSD.IsZero() {
		annualizedNetBps = netSwapPerDayUSD.Mul(decimal.NewFromInt(365)).
			Div(notionalUSD).Mul(decimal.NewFromInt(10000))
	}

	holdDays := decimal.NewFromInt(int64(cfg.CarryDefaultHoldDays))
	swapCost := netSwapPerDayUSD.Mul(holdDays)

	return &CarryResult{
		NetSwapPerDay:    netSwapPerDayUSD,
		AnnualizedNetBps: annualizedNetBps,
		LegRoles:         roles,
		LegDailySwapUSD:  dailySwapUSD,
		LegAnnualizedBps: annualizedBps,
		SwapCost:         swapCost,
	}, nil
}
