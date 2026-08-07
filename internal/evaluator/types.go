package evaluator

import (
	"time"

	"github.com/shopspring/decimal"

	"arb/internal/bus"
	"arb/internal/listing"
	"arb/internal/risk"
)

// OppType identifies the arbitrage strategy (06 §5.2).
type OppType int32

const (
	OppCrossExchange OppType = 1
	OppCarry         OppType = 2
	OppTriangular    OppType = 3
)

// BuySell is the direction of a leg.
type BuySell int32

const (
	Buy  BuySell = 1
	Sell BuySell = 2
)

// LegRole classifies a leg's function in Carry (02 §5).
type LegRole int32

const (
	LegRoleOmitted LegRole = 0
	LegRoleIncome  LegRole = 1
	LegRoleHedge   LegRole = 2
)

// OppStatus is the lifecycle state of an Opportunity (06 §5.2).
type OppStatus int32

const (
	OppStatusPushed     OppStatus = 1
	OppStatusConfirmed  OppStatus = 2
	OppStatusExecuting  OppStatus = 3
	OppStatusFilled     OppStatus = 4
	OppStatusFailed     OppStatus = 5
	OppStatusExpired    OppStatus = 6
)

// Config maps from proto EvaluatorConfig (12 §7). Passed by caller.
type Config struct {
	MinNetBps            float64
	MinAnnualizedNetBps  float64
	SlippageBps          float64
	QuoteFreshness       time.Duration
	HedgeTolerancePct    float64
	CarryDefaultHoldDays int32
	MaxSpreadBps         float64
}

// Candidate is the Detector-produced input (03 §4). Only gross spread;
// Evaluator adds all costs.
type Candidate struct {
	Type        OppType
	Legs        []CandidateLeg
	GrossProfit decimal.Decimal
	QuoteTime   time.Time
}

// CandidateLeg is one leg of a candidate from Detector.
type CandidateLeg struct {
	Broker       string
	BrokerSymbol string
	Canonical    string
	Direction    BuySell
	Lots         decimal.Decimal
	EstPrice     decimal.Decimal
}

// Opportunity is the evaluated output (02 §5 + 06 §5.2, Go-native decimal).
type Opportunity struct {
	ID               string
	Type             OppType
	Legs             []OppLeg
	QuoteTime        time.Time
	GrossProfit      decimal.Decimal
	SpreadCost       decimal.Decimal
	CommissionCost   decimal.Decimal
	SlippageCost     decimal.Decimal
	SwapCost         decimal.Decimal
	NetProfit        decimal.Decimal
	NetBps           decimal.Decimal
	NetSwapPerDay    decimal.Decimal
	HoldDaysHint     int32
	AnnualizedNetBps decimal.Decimal
	ExpiresAt        time.Time
	Executable       bool
	Confidence       float64
	Status           OppStatus
	NotionalUSD      decimal.Decimal
	RejectReason     string
}

// OppLeg is one leg of an evaluated Opportunity.
type OppLeg struct {
	Broker        string
	BrokerSymbol  string
	Canonical     string
	Direction     BuySell
	Lots          decimal.Decimal
	Role          LegRole
	DailySwap     decimal.Decimal
	AnnualizedBps decimal.Decimal
	EstPrice      decimal.Decimal
}

// Deps holds the read-only dependencies injected at construction.
type Deps struct {
	Listings *listing.Cache
	Bus      *bus.QuoteBus
	Rates    RateResolver
	Gate     *risk.CapitalGate
	Cfg      Config
	Now      func() time.Time
}

// Evaluator evaluates Candidates into Opportunities (12 §3).
type Evaluator struct {
	deps Deps
}

// New creates an Evaluator with the given deps.
func New(deps Deps) *Evaluator {
	if deps.Now == nil {
		deps.Now = time.Now
	}
	return &Evaluator{deps: deps}
}
