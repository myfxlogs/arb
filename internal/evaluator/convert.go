package evaluator

import (
	"context"
	"fmt"

	"github.com/shopspring/decimal"

	"arb/internal/bus"
	"arb/internal/listing"
)

// RateResolver converts an amount from one currency to another using
// QuoteBus cross rates (12 §4.3). Implementations must be safe for
// concurrent read.
type RateResolver interface {
	Rate(ctx context.Context, from, to string) (decimal.Decimal, error)
}

// BusRateResolver implements RateResolver using QuoteBus snapshots.
// rate(USD→USD) = 1. rate(JPY→USD) = 1/USDJPY_mid.
// rate(X→USD) = 1/USD_X_mid if USD_X is available.
type BusRateResolver struct {
	Bus *bus.QuoteBus
}

// Rate returns the conversion rate from → to.
// Currently supports USD→USD (1:1) and XXX→USD via USDXXX quote.
func (r *BusRateResolver) Rate(ctx context.Context, from, to string) (decimal.Decimal, error) {
	if from == to {
		return decimal.NewFromInt(1), nil
	}

	// Try direct: USDXXX → rate(XXX→USD) = 1/USDXXX_mid
	if from != "USD" && to == "USD" {
		symbol := "USD" + from
		q := r.latestQuote(ctx, symbol)
		if q.Symbol == "" {
			return decimal.Zero, fmt.Errorf("no quote for %s", symbol)
		}
		mid := decimal.NewFromFloat((q.Bid + q.Ask) / 2)
		if mid.IsZero() {
			return decimal.Zero, fmt.Errorf("zero mid for %s", symbol)
		}
		return decimal.NewFromInt(1).Div(mid), nil
	}

	// Try reverse: XXXUSD → rate(XXX→USD) = XXXUSD_mid
	if from != "USD" && to == "USD" {
		symbol := from + "USD"
		q := r.latestQuote(ctx, symbol)
		if q.Symbol == "" {
			return decimal.Zero, fmt.Errorf("no quote for %s", symbol)
		}
		mid := decimal.NewFromFloat((q.Bid + q.Ask) / 2)
		return mid, nil
	}

	return decimal.Zero, fmt.Errorf("unsupported rate pair: %s→%s", from, to)
}

func (r *BusRateResolver) latestQuote(ctx context.Context, symbol string) bus.Quote {
	m := r.Bus.Snapshot(ctx, []string{symbol})
	return m[symbol]
}

// ConvertToUSD converts an amount in the given currency to USD.
func ConvertToUSD(ctx context.Context, amount decimal.Decimal, ccy string, rates RateResolver) (decimal.Decimal, error) {
	if ccy == "USD" {
		return amount, nil
	}
	rate, err := rates.Rate(ctx, ccy, "USD")
	if err != nil {
		return decimal.Zero, err
	}
	return amount.Mul(rate), nil
}

// ToUSD computes NetProfit and NetBps from per-leg profit in leg currency,
// gross profit (already USD), and cost breakdown (already USD).
//
// legProfitLegCcy: profit per leg in the leg's profit currency.
// legs: the evaluated legs (for profit currency lookup).
// listings: cache to resolve profit currency per leg.
// notionalUSD: the base notional in USD (from hedge normalization + rate).
func ToUSD(
	ctx context.Context,
	legProfitLegCcy []decimal.Decimal,
	legs []OppLeg,
	listings *listing.Cache,
	rates RateResolver,
	grossProfitUSD decimal.Decimal,
	costs CostBreakdown,
	notionalUSD decimal.Decimal,
) (netProfit, netBps decimal.Decimal, err error) {
	totalLegProfitUSD := decimal.Zero
	for i, profitCcy := range legProfitLegCcy {
		l, ok := listings.Get(legs[i].Broker, legs[i].BrokerSymbol)
		if !ok {
			return decimal.Zero, decimal.Zero, fmt.Errorf("listing not found: %s/%s", legs[i].Broker, legs[i].BrokerSymbol)
		}
		usd, err := ConvertToUSD(ctx, profitCcy, l.ProfitCurrency, rates)
		if err != nil {
			return decimal.Zero, decimal.Zero, fmt.Errorf("convert leg %d: %w", i, err)
		}
		totalLegProfitUSD = totalLegProfitUSD.Add(usd)
	}

	netProfit = grossProfitUSD.
		Sub(costs.SpreadCost).
		Sub(costs.CommissionCost).
		Sub(costs.SlippageCost).
		Sub(costs.SwapCost)

	if notionalUSD.IsZero() {
		return netProfit, decimal.Zero, nil
	}
	netBps = netProfit.Div(notionalUSD).Mul(decimal.NewFromInt(10000))
	return netProfit, netBps, nil
}
