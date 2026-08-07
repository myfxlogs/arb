package evaluator

import (
	"context"
	"fmt"

	"github.com/shopspring/decimal"

	"arb/internal/bus"
	"arb/internal/listing"
)

// CostBreakdown holds the four cost components in USD (12 §4.4).
type CostBreakdown struct {
	SpreadCost     decimal.Decimal
	CommissionCost decimal.Decimal
	SlippageCost   decimal.Decimal
	SwapCost       decimal.Decimal
}

// Calculate computes all four cost components for a candidate.
// Listings and quotes must cover every leg. Rates converts per-leg
// costs to USD. The notional is the hedge-normalized base notional
// (in base leg's price currency; converted to USD by caller).
//
// effectiveHoldDays: 0 for CrossExchange/Triangular (intraday),
// Cfg.CarryDefaultHoldDays for Carry.
func Calculate(
	ctx context.Context,
	c Candidate,
	legs []OppLeg,
	listings *listing.Cache,
	quotes map[string]bus.Quote,
	rates RateResolver,
	cfg Config,
	notionalUSD decimal.Decimal,
	effectiveHoldDays int32,
) (CostBreakdown, error) {
	var cb CostBreakdown

	nLegs := decimal.NewFromInt(int64(len(legs)))

	// 1. SpreadCost: Σ (Ask−Bid) × ContractSize × Lots per leg, in leg ccy → USD
	spreadUSD := decimal.Zero
	for i, leg := range legs {
		l, ok := listings.Get(leg.Broker, leg.BrokerSymbol)
		if !ok {
			return cb, fmt.Errorf("listing not found: %s/%s", leg.Broker, leg.BrokerSymbol)
		}
		q, ok := quotes[leg.BrokerSymbol]
		if !ok {
			return cb, fmt.Errorf("quote not found: %s", leg.BrokerSymbol)
		}
		spread := decimal.NewFromFloat(q.Ask - q.Bid)
		costCcy := spread.Mul(l.ContractSize).Mul(leg.Lots)
		costUSD, err := ConvertToUSD(ctx, costCcy, l.ProfitCurrency, rates)
		if err != nil {
			return cb, fmt.Errorf("spread convert leg %d: %w", i, err)
		}
		spreadUSD = spreadUSD.Add(costUSD)
	}
	cb.SpreadCost = spreadUSD

	// 2. CommissionCost: per Listing.CommissionMode → USD
	commissionUSD := decimal.Zero
	for i, leg := range legs {
		l, ok := listings.Get(leg.Broker, leg.BrokerSymbol)
		if !ok {
			return cb, fmt.Errorf("listing not found: %s/%s", leg.Broker, leg.BrokerSymbol)
		}
		var costCcy decimal.Decimal
		switch l.CommissionMode {
		case listing.CommissionPerLot:
			costCcy = l.CommissionRate.Mul(leg.Lots)
		case listing.CommissionPerNotionalBps:
			q := quotes[leg.BrokerSymbol]
			price := decimal.NewFromFloat(q.Bid)
			if leg.Direction == Buy {
				price = decimal.NewFromFloat(q.Ask)
			}
			notionalCcy := l.ContractSize.Mul(leg.Lots).Mul(price)
			costCcy = notionalCcy.Mul(l.CommissionRate).Div(decimal.NewFromInt(10000))
		}
		costUSD, err := ConvertToUSD(ctx, costCcy, l.ProfitCurrency, rates)
		if err != nil {
			return cb, fmt.Errorf("commission convert leg %d: %w", i, err)
		}
		commissionUSD = commissionUSD.Add(costUSD)
	}
	cb.CommissionCost = commissionUSD

	// 3. SlippageCost: n_legs × slippage_bps × Notional / 10000
	slipBps := decimal.NewFromFloat(cfg.SlippageBps)
	cb.SlippageCost = nLegs.Mul(slipBps).Mul(notionalUSD).Div(decimal.NewFromInt(10000))

	// 4. SwapCost: effective_hold_days × NetSwapPerDay (in USD)
	if effectiveHoldDays > 0 {
		netSwapPerDayCcy := decimal.Zero
		for i, leg := range legs {
			l, ok := listings.Get(leg.Broker, leg.BrokerSymbol)
			if !ok {
				return cb, fmt.Errorf("listing not found: %s/%s", leg.Broker, leg.BrokerSymbol)
			}
			q := quotes[leg.BrokerSymbol]
			price := decimal.NewFromFloat(q.Bid)
			if leg.Direction == Buy {
				price = decimal.NewFromFloat(q.Ask)
			}
			daily, err := DailySwap(l, leg.Direction, leg.Lots, price)
			if err != nil {
				return cb, fmt.Errorf("swap leg %d: %w", i, err)
			}
			netSwapPerDayCcy = netSwapPerDayCcy.Add(daily)
		}
		// Convert net swap per day to USD (approximate: use first leg's ccy
		// as representative — for Carry, both legs are same canonical pair)
		if len(legs) > 0 {
			l, _ := listings.Get(legs[0].Broker, legs[0].BrokerSymbol)
			if l != nil {
				swapUSD, err := ConvertToUSD(ctx, netSwapPerDayCcy, l.ProfitCurrency, rates)
				if err != nil {
					return cb, fmt.Errorf("swap convert: %w", err)
				}
				cb.SwapCost = swapUSD.Mul(decimal.NewFromInt(int64(effectiveHoldDays)))
			}
		}
	}

	return cb, nil
}
