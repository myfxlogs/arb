package listing

import "github.com/shopspring/decimal"

// SwapType determines how swap (overnight funding) is calculated.
// Values mirror MT5 ENUM_SYMBOL_SWAP_MODE; Crypto uses PERCENTAGE.
type SwapType int32

const (
	SwapNone          SwapType = 0
	SwapInPoints      SwapType = 1
	SwapSymInfoS408   SwapType = 2
	SwapMarginCcy     SwapType = 3
	SwapCurrency      SwapType = 4
	SwapPctCurPrice   SwapType = 5
	SwapPctOpenPrice  SwapType = 6
	SwapPointClose    SwapType = 7
	SwapPointBid      SwapType = 8
)

// CalcMode is the profit/margin calculation mode (MT5 ENUM_SYMBOL_CALC_MODE).
type CalcMode int32

const (
	CalcForex           CalcMode = 0
	CalcFutures         CalcMode = 1
	CalcCFD             CalcMode = 2
	CalcCFDIndex        CalcMode = 3
	CalcCFDLeverage     CalcMode = 4
	CalcMode5           CalcMode = 5
	CalcExchangeStocks  CalcMode = 32
	CalcExchangeFutures CalcMode = 33
	CalcFORTSFutures    CalcMode = 34
	CalcExchangeOption  CalcMode = 35
	CalcExchangeMargin  CalcMode = 36
	CalcExchangeBounds  CalcMode = 37
	CalcCollateral      CalcMode = 64
)

// TradeMode controls whether a symbol can be traded.
type TradeMode int32

const (
	TradeDisabled  TradeMode = 0
	TradeLongOnly  TradeMode = 1
	TradeShortOnly TradeMode = 2
	TradeCloseOnly TradeMode = 3
	TradeFullAccess TradeMode = 4
)

// ExecutionType is the order execution mode.
type ExecutionType int32

const (
	ExecRequest  ExecutionType = 0
	ExecInstant  ExecutionType = 1
	ExecMarket   ExecutionType = 2
	ExecExchange ExecutionType = 3
)

// FillPolicy is the order filling policy (bit flags).
type FillPolicy int32

const (
	FillNone FillPolicy = 0
	FillFOK  FillPolicy = 1
	FillIOC  FillPolicy = 2
	FillBOC  FillPolicy = 4
)

// TripleSwapDay is the day of week when triple swap is charged (0=Sunday).
type TripleSwapDay int32

const (
	TripleSwapSunday   TripleSwapDay = 0
	TripleSwapMonday   TripleSwapDay = 1
	TripleSwapTuesday  TripleSwapDay = 2
	TripleSwapWednesday TripleSwapDay = 3
	TripleSwapThursday TripleSwapDay = 4
	TripleSwapFriday   TripleSwapDay = 5
	TripleSwapSaturday TripleSwapDay = 6
)

// SettlementFreq is how often funding/swap is charged.
type SettlementFreq int32

const (
	SettleDaily  SettlementFreq = 0 // FX: once per day
	SettleEvery8h SettlementFreq = 1 // Crypto perpetual: every 8 hours
)

// CommissionMode determines how commission is calculated (02 §4.4, 12 §2.1).
type CommissionMode int32

const (
	CommissionPerLot        CommissionMode = 0 // Fixed per lot (FX mainstream)
	CommissionPerNotionalBps CommissionMode = 1 // Basis points of notional
)

// Funding unifies FX swap and Crypto funding rate (02 §4.3).
type Funding struct {
	SwapType       SwapType
	SwapLong       decimal.Decimal
	SwapShort      decimal.Decimal
	SettlementFreq SettlementFreq
	TripleSwapDay  TripleSwapDay
}

// Instrument is the logical, broker-agnostic identity of a tradable
// symbol (02 §1.1). Cross-broker comparison uses this to determine
// that ICMarkets "EURUSD" and Exness "EURUSDm" are the same instrument.
type Instrument struct {
	Symbol     string // canonical, e.g. "EURUSD", "XAUUSD"
	AssetClass string // "FX" | "CRYPTO"
	Base       string // "EUR" / "XAU" / "BTC"
	Quote      string // "USD" / "USDT"
	Kind       string // "SPOT" (FX) / "PERP" (Crypto perpetual)
}

// Listing is a concrete instrument instance on a specific broker (02 §1.2).
// All broker-specific real parameters live here. Dynamic quotes (bid/ask)
// are NOT stored here — they flow through QuoteBus (hot path, float64).
type Listing struct {
	Broker       string
	BrokerSymbol string // raw broker symbol, used for order placement
	Instrument   *Instrument

	// From SymbolInfo
	ContractSize   decimal.Decimal
	Digits         int32
	Points         decimal.Decimal
	ProfitCurrency string
	MarginCurrency string
	CalcMode       CalcMode

	// From SymGroup
	VolumeMin  decimal.Decimal
	VolumeMax  decimal.Decimal
	VolumeStep decimal.Decimal
	Swap       Funding
	InitMargin decimal.Decimal
	TradeMode  TradeMode
	ExecType   ExecutionType
	FillPolicy FillPolicy

	// Commission (02 §4.4, 12 §2.1). MT5 SymbolInfo does not provide this;
	// human-entered, calibrated by actual fills. Default 0 = honest over-estimate.
	CommissionMode  CommissionMode
	CommissionRate  decimal.Decimal
}
