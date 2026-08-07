package evaluator

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"arb/internal/bus"
	"arb/internal/listing"
)

// --- Test helpers ---

func mustDec(s string) decimal.Decimal {
	d, err := decimal.NewFromString(s)
	if err != nil {
		panic(err)
	}
	return d
}

func testListing(broker, sym, profitCcy string, contractSize, points, volMin, volMax, volStep, swapLong, swapShort string, swapType listing.SwapType) *listing.Listing {
	return &listing.Listing{
		Broker:         broker,
		BrokerSymbol:   sym,
		ContractSize:   mustDec(contractSize),
		Points:         mustDec(points),
		ProfitCurrency: profitCcy,
		VolumeMin:      mustDec(volMin),
		VolumeMax:      mustDec(volMax),
		VolumeStep:     mustDec(volStep),
		Swap: listing.Funding{
			SwapType:  swapType,
			SwapLong:  mustDec(swapLong),
			SwapShort: mustDec(swapShort),
		},
	}
}

// We need to use the real Cache since Normalize/Calculate take *listing.Cache.
// Build a real cache and populate it directly.
func testCache(ls ...*listing.Listing) *listing.Cache {
	c := listing.NewCache()
	for _, l := range ls {
		c.PutForTest(l)
	}
	return c
}

func testQuote(sym, broker string, bid, ask float64, t time.Time) bus.Quote {
	return bus.Quote{Symbol: sym, Broker: broker, Bid: bid, Ask: ask, Time: t}
}

// mockRates returns fixed rates for testing.
type mockRates struct {
	rates map[string]decimal.Decimal // "FROM→TO" → rate
}

func (m *mockRates) Rate(_ context.Context, from, to string) (decimal.Decimal, error) {
	if from == to {
		return decimal.NewFromInt(1), nil
	}
	key := from + "→" + to
	if r, ok := m.rates[key]; ok {
		return r, nil
	}
	return decimal.Zero, fmt.Errorf("no rate for %s", key)
}

// --- swap_test.go ---

func TestDailySwap_InPoints_EURUSD(t *testing.T) {
	l := testListing("ICM", "EURUSD", "USD", "100000", "0.00001", "0.01", "200", "0.01", "-8.287", "1.544", listing.SwapInPoints)
	got, err := DailySwap(l, Buy, decimal.NewFromInt(1), mustDec("1.08"))
	if err != nil {
		t.Fatalf("DailySwap: %v", err)
	}
	want := mustDec("-8.287") // -8.287 × 0.00001 × 100000 × 1 = -8.287
	if !got.Equal(want) {
		t.Errorf("EURUSD Buy daily swap = %s, want %s", got.String(), want.String())
	}
}

func TestDailySwap_InPoints_XAUUSD(t *testing.T) {
	l := testListing("ICM", "XAUUSD", "USD", "100", "0.01", "0.01", "100", "0.01", "-56.766", "38.929", listing.SwapInPoints)
	got, err := DailySwap(l, Buy, decimal.NewFromInt(1), mustDec("2000"))
	if err != nil {
		t.Fatalf("DailySwap: %v", err)
	}
	want := mustDec("-56.766") // -56.766 × 0.01 × 100 × 1 = -56.766
	if !got.Equal(want) {
		t.Errorf("XAUUSD Buy daily swap = %s, want %s", got.String(), want.String())
	}
}

func TestDailySwap_InPoints_GBPJPY(t *testing.T) {
	l := testListing("ICM", "GBPJPY", "JPY", "100000", "0.001", "0.01", "200", "0.01", "11.67", "-23.186", listing.SwapInPoints)
	got, err := DailySwap(l, Sell, decimal.NewFromInt(1), mustDec("190"))
	if err != nil {
		t.Fatalf("DailySwap: %v", err)
	}
	// Sell uses SwapShort = -23.186
	// -23.186 × 0.001 × 100000 × 1 = -2318.6 JPY
	want := mustDec("-2318.6")
	if !got.Equal(want) {
		t.Errorf("GBPJPY Sell daily swap = %s, want %s", got.String(), want.String())
	}
}

func TestDailySwap_Uncalibrated(t *testing.T) {
	l := testListing("ICM", "EURUSD", "USD", "100000", "0.00001", "0.01", "200", "0.01", "-8.287", "1.544", listing.SwapPointClose)
	_, err := DailySwap(l, Buy, decimal.NewFromInt(1), mustDec("1.08"))
	if err != ErrUncalibratedSwap {
		t.Errorf("expected ErrUncalibratedSwap, got %v", err)
	}
}

func TestDailySwap_SwapNone(t *testing.T) {
	l := testListing("ICM", "EURUSD", "USD", "100000", "0.00001", "0.01", "200", "0.01", "-8.287", "1.544", listing.SwapNone)
	got, err := DailySwap(l, Buy, decimal.NewFromInt(1), mustDec("1.08"))
	if err != nil {
		t.Fatalf("DailySwap: %v", err)
	}
	if !got.IsZero() {
		t.Errorf("SwapNone should be zero, got %s", got.String())
	}
}

// --- hedge_test.go ---

func TestHedge_Normalize_SameSize(t *testing.T) {
	lA := testListing("ICM", "EURUSD", "USD", "100000", "0.00001", "0.01", "200", "0.01", "-8.287", "1.544", listing.SwapInPoints)
	lB := testListing("EXN", "EURUSDm", "USD", "100000", "0.00001", "0.01", "200", "0.01", "-5.0", "2.0", listing.SwapInPoints)
	c := testCache(lA, lB)

	now := time.Now()
	legs := []CandidateLeg{
		{Broker: "ICM", BrokerSymbol: "EURUSD", Direction: Buy, Lots: decimal.NewFromInt(1), EstPrice: mustDec("1.08")},
		{Broker: "EXN", BrokerSymbol: "EURUSDm", Direction: Sell, Lots: decimal.NewFromInt(1), EstPrice: mustDec("1.0801")},
	}
	quotes := map[string]bus.Quote{
		"EURUSD":  testQuote("EURUSD", "ICM", 1.0799, 1.0800, now),
		"EURUSDm": testQuote("EURUSDm", "EXN", 1.0800, 1.0801, now),
	}

	hr, err := Normalize(legs, c, quotes, 1.0)
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if !hr.Lots[1].Equal(decimal.NewFromInt(1)) {
		t.Errorf("L_B = %s, want 1", hr.Lots[1].String())
	}
	if !hr.WithinTol {
		t.Errorf("expected within tolerance")
	}
}

func TestHedge_Normalize_DiffSize(t *testing.T) {
	lA := testListing("ICM", "XAUUSD", "USD", "100", "0.01", "0.01", "100", "0.01", "-56.766", "38.929", listing.SwapInPoints)
	lB := testListing("EXN", "XAGUSD", "USD", "1000", "0.01", "0.01", "100", "0.01", "-5.0", "2.0", listing.SwapInPoints)
	c := testCache(lA, lB)

	now := time.Now()
	legs := []CandidateLeg{
		{Broker: "ICM", BrokerSymbol: "XAUUSD", Direction: Buy, Lots: decimal.NewFromInt(10), EstPrice: mustDec("2000")},
		{Broker: "EXN", BrokerSymbol: "XAGUSD", Direction: Sell, Lots: decimal.NewFromInt(1), EstPrice: mustDec("25")},
	}
	quotes := map[string]bus.Quote{
		"XAUUSD": testQuote("XAUUSD", "ICM", 1999, 2000, now),
		"XAGUSD": testQuote("XAGUSD", "EXN", 24.5, 25, now),
	}

	hr, err := Normalize(legs, c, quotes, 1.0)
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	// notional_A = 100 × 10 × 2000 = 2,000,000 (Buy uses Ask=2000)
	// L_B_raw = 2,000,000 / (1000 × 24.5) = 81.6326... (Sell uses Bid=24.5)
	// roundToStep(81.6326, 0.01) = 81.63
	if !hr.Lots[1].Equal(mustDec("81.63")) {
		t.Errorf("L_B = %s, want 81.63", hr.Lots[1].String())
	}
	if !hr.WithinTol {
		t.Errorf("expected within tolerance")
	}
}

// --- convert_test.go ---

func TestConvert_JPYtoUSD(t *testing.T) {
	rates := &mockRates{rates: map[string]decimal.Decimal{
		"JPY→USD": mustDec("0.00667"), // 1/150 ≈ 0.00667
	}}
	got, err := ConvertToUSD(context.Background(), mustDec("100000"), "JPY", rates)
	if err != nil {
		t.Fatalf("ConvertToUSD: %v", err)
	}
	want := mustDec("667") // 100000 × 0.00667 = 667
	if !got.Equal(want) {
		t.Errorf("JPY→USD = %s, want %s", got.String(), want.String())
	}
}

func TestConvert_USDToUSD(t *testing.T) {
	rates := &mockRates{}
	got, err := ConvertToUSD(context.Background(), mustDec("500"), "USD", rates)
	if err != nil {
		t.Fatalf("ConvertToUSD: %v", err)
	}
	if !got.Equal(mustDec("500")) {
		t.Errorf("USD→USD = %s, want 500", got.String())
	}
}

// --- evaluator_test.go (end-to-end) ---

func TestEvaluate_CrossExchange(t *testing.T) {
	lA := testListing("ICM", "EURUSD", "USD", "100000", "0.00001", "0.01", "200", "0.01", "-8.287", "1.544", listing.SwapInPoints)
	lB := testListing("EXN", "EURUSDm", "USD", "100000", "0.00001", "0.01", "200", "0.01", "-5.0", "2.0", listing.SwapInPoints)
	c := testCache(lA, lB)

	now := time.Now()
	quotes := map[string]bus.Quote{
		"EURUSD":  testQuote("EURUSD", "ICM", 1.0799, 1.0800, now),
		"EURUSDm": testQuote("EURUSDm", "EXN", 1.0800, 1.0801, now),
	}

	// Build QuoteBus with quotes
	qb := bus.New([]string{"EURUSD", "EURUSDm"})
	qb.Publish(quotes["EURUSD"])
	qb.Publish(quotes["EURUSDm"])

	rates := &mockRates{} // USD→USD = 1

	deps := Deps{
		Listings: c,
		Bus:      qb,
		Rates:    rates,
		Cfg: Config{
			MinNetBps:         3.0,
			SlippageBps:       1.0,
			QuoteFreshness:    2 * time.Second,
			HedgeTolerancePct: 1.0,
			MaxSpreadBps:      5.0,
		},
		Now: func() time.Time { return now },
	}

	eval := New(deps)
	candidate := Candidate{
		Type:        OppCrossExchange,
		QuoteTime:   now,
		GrossProfit: mustDec("10"), // $10 gross spread
		Legs: []CandidateLeg{
			{Broker: "ICM", BrokerSymbol: "EURUSD", Direction: Buy, Lots: decimal.NewFromInt(1), EstPrice: mustDec("1.08")},
			{Broker: "EXN", BrokerSymbol: "EURUSDm", Direction: Sell, Lots: decimal.NewFromInt(1), EstPrice: mustDec("1.0801")},
		},
	}

	opp, err := eval.Evaluate(context.Background(), candidate)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if opp == nil {
		t.Fatal("expected non-nil Opportunity")
	}
	if opp.Type != OppCrossExchange {
		t.Errorf("Type = %d, want %d", opp.Type, OppCrossExchange)
	}
	if len(opp.Legs) != 2 {
		t.Fatalf("Legs = %d, want 2", len(opp.Legs))
	}
	// NetBps should be computed (positive if gross > costs)
	if opp.NetBps.IsZero() {
		t.Error("NetBps should not be zero")
	}
	// Notional = 100000 × 1 × 1.08 = 108000 USD
	wantNotional := mustDec("108000")
	if !opp.NotionalUSD.Equal(wantNotional) {
		t.Errorf("NotionalUSD = %s, want %s", opp.NotionalUSD.String(), wantNotional.String())
	}
}

func TestEvaluate_FreshnessStale(t *testing.T) {
	lA := testListing("ICM", "EURUSD", "USD", "100000", "0.00001", "0.01", "200", "0.01", "-8.287", "1.544", listing.SwapInPoints)
	lB := testListing("EXN", "EURUSDm", "USD", "100000", "0.00001", "0.01", "200", "0.01", "-5.0", "2.0", listing.SwapInPoints)
	c := testCache(lA, lB)

	staleTime := time.Now().Add(-10 * time.Second)
	qb := bus.New([]string{"EURUSD", "EURUSDm"})
	qb.Publish(testQuote("EURUSD", "ICM", 1.0799, 1.0800, staleTime))
	qb.Publish(testQuote("EURUSDm", "EXN", 1.0800, 1.0801, staleTime))

	deps := Deps{
		Listings: c,
		Bus:      qb,
		Rates:    &mockRates{},
		Cfg: Config{
			QuoteFreshness: 2 * time.Second,
			MinNetBps:      3.0,
			MaxSpreadBps:   5.0,
		},
		Now: time.Now,
	}

	eval := New(deps)
	candidate := Candidate{
		Type:        OppCrossExchange,
		QuoteTime:   staleTime,
		GrossProfit: mustDec("10"),
		Legs: []CandidateLeg{
			{Broker: "ICM", BrokerSymbol: "EURUSD", Direction: Buy, Lots: decimal.NewFromInt(1)},
			{Broker: "EXN", BrokerSymbol: "EURUSDm", Direction: Sell, Lots: decimal.NewFromInt(1)},
		},
	}

	opp, err := eval.Evaluate(context.Background(), candidate)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if opp != nil {
		t.Errorf("expected nil (stale discard), got non-nil Opportunity")
	}
}

func TestEvaluate_NotExecutable_NetBpsBelowThreshold(t *testing.T) {
	lA := testListing("ICM", "EURUSD", "USD", "100000", "0.00001", "0.01", "200", "0.01", "-8.287", "1.544", listing.SwapInPoints)
	lB := testListing("EXN", "EURUSDm", "USD", "100000", "0.00001", "0.01", "200", "0.01", "-5.0", "2.0", listing.SwapInPoints)
	c := testCache(lA, lB)

	now := time.Now()
	qb := bus.New([]string{"EURUSD", "EURUSDm"})
	qb.Publish(testQuote("EURUSD", "ICM", 1.0799, 1.0800, now))
	qb.Publish(testQuote("EURUSDm", "EXN", 1.0800, 1.0801, now))

	deps := Deps{
		Listings: c,
		Bus:      qb,
		Rates:    &mockRates{},
		Cfg: Config{
			MinNetBps:      100.0, // impossibly high
			QuoteFreshness: 2 * time.Second,
			MaxSpreadBps:   100.0,
		},
		Now: func() time.Time { return now },
	}

	eval := New(deps)
	candidate := Candidate{
		Type:        OppCrossExchange,
		QuoteTime:   now,
		GrossProfit: mustDec("0.01"), // tiny gross → low NetBps
		Legs: []CandidateLeg{
			{Broker: "ICM", BrokerSymbol: "EURUSD", Direction: Buy, Lots: decimal.NewFromInt(1)},
			{Broker: "EXN", BrokerSymbol: "EURUSDm", Direction: Sell, Lots: decimal.NewFromInt(1)},
		},
	}

	opp, err := eval.Evaluate(context.Background(), candidate)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if opp == nil {
		t.Fatal("expected non-nil Opportunity")
	}
	if opp.Executable {
		t.Error("expected Executable=false (NetBps below threshold)")
	}
	if opp.RejectReason == "" {
		t.Error("expected non-empty RejectReason")
	}
}
