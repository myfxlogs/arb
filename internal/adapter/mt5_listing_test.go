package adapter

import (
	"testing"

	"arb/internal/listing"
	mt5 "arb/proto/gen/mtapi/mt5"
)

func TestMapSymbolParams_Full(t *testing.T) {
	si := &mt5.SymbolInfo{
		ContractSize:   100000,
		Digits:         5,
		Points:         0.00001,
		ProfitCurrency: "USD",
		MarginCurrency: "EUR",
		CalcMode:       mt5.CalculationMode_CalculationMode_Forex,
	}
	sg := &mt5.SymGroup{
		MinLots:        0.01,
		MaxLots:        200,
		LotsStep:       0.01,
		InitialMargin:  100000,
		TradeMode:      mt5.TradeMode_TradeMode_FullAccess,
		TradeType:      mt5.ExecutionType_ExecutionType_Market,
		FillPolicy:     mt5.FillingFlags_FillingFlags_IOC,
		SwapType:       mt5.SwapType_SwapType_InPoints,
		SwapLong:       -8.287,
		SwapShort:      1.544,
		ThreeDaysSwap:  mt5.V3DaysSwap_V3DaysSwap_Sunday,
	}

	l := mapSymbolParams(si, sg, "ICMarketsSC-Demo", "EURUSD")

	if l.Broker != "ICMarketsSC-Demo" {
		t.Errorf("Broker = %s, want ICMarketsSC-Demo", l.Broker)
	}
	if l.BrokerSymbol != "EURUSD" {
		t.Errorf("BrokerSymbol = %s, want EURUSD", l.BrokerSymbol)
	}
	if l.ContractSize.String() != "100000" {
		t.Errorf("ContractSize = %s, want 100000", l.ContractSize)
	}
	if l.Digits != 5 {
		t.Errorf("Digits = %d, want 5", l.Digits)
	}
	if l.Points.String() != "0.00001" {
		t.Errorf("Points = %s, want 0.00001", l.Points)
	}
	if l.ProfitCurrency != "USD" {
		t.Errorf("ProfitCurrency = %s, want USD", l.ProfitCurrency)
	}
	if l.MarginCurrency != "EUR" {
		t.Errorf("MarginCurrency = %s, want EUR", l.MarginCurrency)
	}
	if l.CalcMode != listing.CalcForex {
		t.Errorf("CalcMode = %d, want %d", l.CalcMode, listing.CalcForex)
	}
	if l.VolumeMin.String() != "0.01" {
		t.Errorf("VolumeMin = %s, want 0.01", l.VolumeMin)
	}
	if l.VolumeMax.String() != "200" {
		t.Errorf("VolumeMax = %s, want 200", l.VolumeMax)
	}
	if l.VolumeStep.String() != "0.01" {
		t.Errorf("VolumeStep = %s, want 0.01", l.VolumeStep)
	}
	if l.TradeMode != listing.TradeFullAccess {
		t.Errorf("TradeMode = %d, want %d", l.TradeMode, listing.TradeFullAccess)
	}
	if l.ExecType != listing.ExecMarket {
		t.Errorf("ExecType = %d, want %d", l.ExecType, listing.ExecMarket)
	}
	if l.FillPolicy != listing.FillIOC {
		t.Errorf("FillPolicy = %d, want %d", l.FillPolicy, listing.FillIOC)
	}
	if l.Swap.SwapType != listing.SwapInPoints {
		t.Errorf("SwapType = %d, want %d", l.Swap.SwapType, listing.SwapInPoints)
	}
	if l.Swap.SwapLong.String() != "-8.287" {
		t.Errorf("SwapLong = %s, want -8.287", l.Swap.SwapLong)
	}
	if l.Swap.SwapShort.String() != "1.544" {
		t.Errorf("SwapShort = %s, want 1.544", l.Swap.SwapShort)
	}
	if l.Swap.TripleSwapDay != listing.TripleSwapSunday {
		t.Errorf("TripleSwapDay = %d, want %d", l.Swap.TripleSwapDay, listing.TripleSwapSunday)
	}
	if l.Swap.SettlementFreq != listing.SettleDaily {
		t.Errorf("SettlementFreq = %d, want %d", l.Swap.SettlementFreq, listing.SettleDaily)
	}
}

func TestMapSymbolParams_NilSymGroup(t *testing.T) {
	si := &mt5.SymbolInfo{
		ContractSize:   100,
		Digits:         2,
		Points:         0.01,
		ProfitCurrency: "USD",
		MarginCurrency: "XAU",
	}
	l := mapSymbolParams(si, nil, "broker", "XAUUSD")
	if l.VolumeMin.String() != "0" {
		t.Errorf("VolumeMin = %s, want 0 (nil SymGroup)", l.VolumeMin)
	}
	if l.Swap.SwapType != listing.SwapNone {
		t.Errorf("SwapType = %d, want 0 (nil SymGroup)", l.Swap.SwapType)
	}
}

func TestMapSymbolParams_XAUUSD(t *testing.T) {
	si := &mt5.SymbolInfo{
		ContractSize:   100,
		Digits:         2,
		Points:         0.01,
		ProfitCurrency: "USD",
		MarginCurrency: "XAU",
	}
	sg := &mt5.SymGroup{
		MinLots:   0.01,
		MaxLots:   100,
		LotsStep:  0.01,
		SwapType:  mt5.SwapType_SwapType_InPoints,
		SwapLong:  -56.766,
		SwapShort: 38.929,
	}
	l := mapSymbolParams(si, sg, "ICMarketsSC-Demo", "XAUUSD")
	if l.ContractSize.String() != "100" {
		t.Errorf("ContractSize = %s, want 100", l.ContractSize)
	}
	if l.Swap.SwapLong.String() != "-56.766" {
		t.Errorf("SwapLong = %s, want -56.766", l.Swap.SwapLong)
	}
	if l.Swap.SwapShort.String() != "38.929" {
		t.Errorf("SwapShort = %s, want 38.929", l.Swap.SwapShort)
	}
}
