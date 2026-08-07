package adapter

import (
	"context"
	"fmt"

	"arb/internal/decimalutil"
	"arb/internal/listing"
	mt5 "arb/proto/gen/mtapi/mt5"
)

// Listing fetches full SymbolParams for one symbol and maps them to a
// listing.Listing (02 §1.2). The Instrument field is left nil — the caller
// (cache/symbol_map layer) resolves canonical symbol and fills it.
func (a *MT5Adapter) Listing(ctx context.Context, brokerSymbol string) (*listing.Listing, error) {
	reply, err := a.SymbolParamsRaw(ctx, brokerSymbol)
	if err != nil {
		return nil, fmt.Errorf("listing %s: %w", brokerSymbol, err)
	}
	if reply.GetResult() == nil || reply.GetResult().GetSymbolInfo() == nil {
		return nil, fmt.Errorf("listing %s: empty SymbolParams", brokerSymbol)
	}
	sp := reply.GetResult()
	si := sp.GetSymbolInfo()
	sg := sp.GetSymbolGroup()

	l := &listing.Listing{
		Broker:       a.brokerName,
		BrokerSymbol: brokerSymbol,
		ContractSize: decimalutil.FromFloat64(si.ContractSize, 8),
		Digits:       si.Digits,
		Points:       decimalutil.FromFloat64(si.Points, 8),
		ProfitCurrency: si.ProfitCurrency,
		MarginCurrency: si.MarginCurrency,
		CalcMode:     mapCalcMode(si.CalcMode),
	}

	if sg != nil {
		l.VolumeMin = decimalutil.FromFloat64(sg.MinLots, 8)
		l.VolumeMax = decimalutil.FromFloat64(sg.MaxLots, 8)
		l.VolumeStep = decimalutil.FromFloat64(sg.LotsStep, 8)
		l.InitMargin = decimalutil.FromFloat64(sg.InitialMargin, 8)
		l.TradeMode = mapTradeMode(sg.TradeMode)
		l.ExecType = mapExecType(sg.TradeType)
		l.FillPolicy = mapFillPolicy(sg.FillPolicy)
		l.TripleSwap = mapTripleSwapDay(sg.ThreeDaysSwap)
		l.Swap = listing.Funding{
			SwapType:       mapSwapType(sg.SwapType),
			SwapLong:       decimalutil.FromFloat64(sg.SwapLong, 8),
			SwapShort:      decimalutil.FromFloat64(sg.SwapShort, 8),
			SettlementFreq: listing.SettleDaily,
			TripleSwapDay:  l.TripleSwap,
		}
	}
	return l, nil
}

// Proto enum values are int32 with identical numeric values to our Go-native
// listing enums, so a direct cast is safe and avoids verbose switch blocks.
func mapSwapType(t mt5.SwapType) listing.SwapType       { return listing.SwapType(t) }
func mapCalcMode(m mt5.CalculationMode) listing.CalcMode { return listing.CalcMode(m) }
func mapTradeMode(m mt5.TradeMode) listing.TradeMode     { return listing.TradeMode(m) }
func mapExecType(t mt5.ExecutionType) listing.ExecutionType { return listing.ExecutionType(t) }
func mapFillPolicy(f mt5.FillingFlags) listing.FillPolicy { return listing.FillPolicy(f) }
func mapTripleSwapDay(d mt5.V3DaysSwap) listing.TripleSwapDay { return listing.TripleSwapDay(d) }
