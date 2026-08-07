package detector

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"arb/internal/bus"
	"arb/internal/evaluator"
	"arb/internal/listing"
)

// --- helpers ---

func mustDec(s string) decimal.Decimal {
	d, err := decimal.NewFromString(s)
	if err != nil {
		panic(err)
	}
	return d
}

func mkListing(broker, sym, canonical string, contractSize, points, volMin string,
	swapLong, swapShort string, swapType listing.SwapType) *listing.Listing {
	return &listing.Listing{
		Broker:         broker,
		BrokerSymbol:   sym,
		ContractSize:   mustDec(contractSize),
		Points:         mustDec(points),
		VolumeMin:      mustDec(volMin),
		VolumeStep:     mustDec("0.01"),
		ProfitCurrency: "USD",
		Swap: listing.Funding{
			SwapType:  swapType,
			SwapLong:  mustDec(swapLong),
			SwapShort: mustDec(swapShort),
		},
		Instrument: &listing.Instrument{Symbol: canonical, AssetClass: "FX", Kind: "SPOT"},
	}
}

func mkQuote(sym, broker string, bid, ask float64, t time.Time) bus.Quote {
	return bus.Quote{Symbol: sym, Broker: broker, Bid: bid, Ask: ask, Time: t}
}

func mkListings(ls ...*listing.Listing) map[listing.CanonicalKey]*listing.Listing {
	m := make(map[listing.CanonicalKey]*listing.Listing)
	for _, l := range ls {
		canonical := l.Instrument.Symbol
		m[listing.CanonicalKey{Broker: l.Broker, Canonical: canonical}] = l
	}
	return m
}

func mkQuotes(qs ...bus.Quote) map[string]bus.Quote {
	m := make(map[string]bus.Quote)
	for _, q := range qs {
		m[q.Symbol] = q
	}
	return m
}

// --- CrossExchange tests ---

func TestCrossExchange_FindsPositive(t *testing.T) {
	now := time.Now()
	lA := mkListing("ICM", "EURUSD", "EURUSD", "100000", "0.00001", "0.01", "-8.287", "1.544", listing.SwapInPoints)
	lB := mkListing("EXN", "EURUSDm", "EURUSD", "100000", "0.00001", "0.01", "-5.0", "2.0", listing.SwapInPoints)

	listings := mkListings(lA, lB)
	quotes := mkQuotes(
		mkQuote("EURUSD", "ICM", 1.0799, 1.0800, now),
		mkQuote("EURUSDm", "EXN", 1.0804, 1.0805, now),
	)

	d := NewCrossExchange()
	cands, err := d.Scan(context.Background(), quotes, listings)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(cands) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(cands))
	}
	c := cands[0]
	if c.Type != evaluator.OppCrossExchange {
		t.Errorf("Type = %d, want %d", c.Type, evaluator.OppCrossExchange)
	}
	// spread = 1.0804 − 1.0800 = 0.0004
	// grossProfit = 0.0004 × 100000 × 0.01 = 0.4
	want := mustDec("0.4")
	if !c.GrossProfit.Equal(want) {
		t.Errorf("GrossProfit = %s, want %s", c.GrossProfit.String(), want.String())
	}
	if c.Legs[0].Direction != evaluator.Buy {
		t.Errorf("leg0 direction = %d, want Buy", c.Legs[0].Direction)
	}
	if c.Legs[1].Direction != evaluator.Sell {
		t.Errorf("leg1 direction = %d, want Sell", c.Legs[1].Direction)
	}
}

func TestCrossExchange_NoSpread(t *testing.T) {
	now := time.Now()
	lA := mkListing("ICM", "EURUSD", "EURUSD", "100000", "0.00001", "0.01", "-8.287", "1.544", listing.SwapInPoints)
	lB := mkListing("EXN", "EURUSDm", "EURUSD", "100000", "0.00001", "0.01", "-5.0", "2.0", listing.SwapInPoints)

	listings := mkListings(lA, lB)
	quotes := mkQuotes(
		mkQuote("EURUSD", "ICM", 1.0799, 1.0805, now),
		mkQuote("EURUSDm", "EXN", 1.0800, 1.0804, now),
	)

	d := NewCrossExchange()
	cands, _ := d.Scan(context.Background(), quotes, listings)
	if len(cands) != 0 {
		t.Errorf("expected 0 candidates, got %d", len(cands))
	}
}

func TestCrossExchange_SingleBroker(t *testing.T) {
	now := time.Now()
	lA := mkListing("ICM", "EURUSD", "EURUSD", "100000", "0.00001", "0.01", "-8.287", "1.544", listing.SwapInPoints)

	listings := mkListings(lA)
	quotes := mkQuotes(mkQuote("EURUSD", "ICM", 1.0799, 1.0800, now))

	d := NewCrossExchange()
	cands, _ := d.Scan(context.Background(), quotes, listings)
	if len(cands) != 0 {
		t.Errorf("expected 0 candidates with single broker, got %d", len(cands))
	}
}

// --- Carry tests ---

func TestCarry_PositiveNetSwap(t *testing.T) {
	now := time.Now()
	// Construct: A SwapLong=+100, B SwapShort=+50, InPoints
	// dailyA = 100 × 0.00001 × 100000 × 0.01 = 1.0
	// dailyB = 50 × 0.00001 × 100000 × 0.01 = 0.5
	// netSwap = 1.0 + 0.5 = 1.5 > 0 → Candidate
	lA := mkListing("ICM", "EURUSD", "EURUSD", "100000", "0.00001", "0.01", "100", "-50", listing.SwapInPoints)
	lB := mkListing("EXN", "EURUSDm", "EURUSD", "100000", "0.00001", "0.01", "-100", "50", listing.SwapInPoints)

	listings := mkListings(lA, lB)
	quotes := mkQuotes(
		mkQuote("EURUSD", "ICM", 1.0799, 1.0800, now),
		mkQuote("EURUSDm", "EXN", 1.0800, 1.0801, now),
	)

	d := NewCarry()
	cands, err := d.Scan(context.Background(), quotes, listings)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(cands) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(cands))
	}
	c := cands[0]
	if c.Type != evaluator.OppCarry {
		t.Errorf("Type = %d, want %d", c.Type, evaluator.OppCarry)
	}
	if !c.GrossProfit.IsZero() {
		t.Errorf("GrossProfit = %s, want 0 (carry profit from swap)", c.GrossProfit.String())
	}
}

func TestCarry_NegativeNetSwap(t *testing.T) {
	now := time.Now()
	// Real ICM+EXN EURUSD values: ICM Buy swap=-8.287, EXN Sell swap=+2.0
	// dailyA = -8.287 × 0.00001 × 100000 × 0.01 = -0.08287
	// dailyB = 2.0 × 0.00001 × 100000 × 0.01 = 0.02
	// netSwap = -0.08287 + 0.02 = -0.06287 < 0 → no candidate
	// Reverse: EXN Buy swap=-5.0, ICM Sell swap=+1.544
	// dailyA = -5.0 × 0.00001 × 100000 × 0.01 = -0.05
	// dailyB = 1.544 × 0.00001 × 100000 × 0.01 = 0.01544
	// netSwap = -0.05 + 0.01544 = -0.03456 < 0 → no candidate
	lA := mkListing("ICM", "EURUSD", "EURUSD", "100000", "0.00001", "0.01", "-8.287", "1.544", listing.SwapInPoints)
	lB := mkListing("EXN", "EURUSDm", "EURUSD", "100000", "0.00001", "0.01", "-5.0", "2.0", listing.SwapInPoints)

	listings := mkListings(lA, lB)
	quotes := mkQuotes(
		mkQuote("EURUSD", "ICM", 1.0799, 1.0800, now),
		mkQuote("EURUSDm", "EXN", 1.0800, 1.0801, now),
	)

	d := NewCarry()
	cands, _ := d.Scan(context.Background(), quotes, listings)
	if len(cands) != 0 {
		t.Errorf("expected 0 candidates (negative net swap), got %d", len(cands))
	}
}

// --- Triangular tests ---

func TestTriangular_FindsDeviation(t *testing.T) {
	now := time.Now()
	// {EUR,USD,GBP}: EURUSD, GBPUSD, EURGBP
	// Construct deviation: Bid_EURUSD = 1.10, Ask_GBPUSD = 1.20, Ask_EURGBP = 0.90
	// Product1 = 1.10 / (1.20 × 0.90) = 1.10 / 1.08 = 1.01852 > 1 → arbitrage
	l1 := mkListing("ICM", "EURUSD", "EURUSD", "100000", "0.00001", "0.01", "0", "0", listing.SwapNone)
	l2 := mkListing("ICM", "GBPUSD", "GBPUSD", "100000", "0.00001", "0.01", "0", "0", listing.SwapNone)
	l3 := mkListing("ICM", "EURGBP", "EURGBP", "100000", "0.00001", "0.01", "0", "0", listing.SwapNone)

	listings := mkListings(l1, l2, l3)
	quotes := mkQuotes(
		mkQuote("EURUSD", "ICM", 1.0999, 1.1000, now),
		mkQuote("GBPUSD", "ICM", 1.1999, 1.2000, now),
		mkQuote("EURGBP", "ICM", 0.8999, 0.9000, now),
	)

	d := NewTriangular()
	cands, err := d.Scan(context.Background(), quotes, listings)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(cands) < 1 {
		t.Fatalf("expected ≥1 candidate, got %d", len(cands))
	}
	c := cands[0]
	if c.Type != evaluator.OppTriangular {
		t.Errorf("Type = %d, want %d", c.Type, evaluator.OppTriangular)
	}
	if len(c.Legs) != 3 {
		t.Errorf("Legs = %d, want 3", len(c.Legs))
	}
	if !c.GrossProfit.GreaterThan(decimal.Zero) {
		t.Errorf("GrossProfit = %s, want > 0", c.GrossProfit.String())
	}
}

func TestTriangular_NoDeviation(t *testing.T) {
	now := time.Now()
	// Construct consistent quotes: product ≈ 1
	// EURUSD Bid=1.08, GBPUSD Ask=1.20, EURGBP Ask=0.90
	// Product1 = 1.08 / (1.20 × 0.90) = 1.08 / 1.08 = 1.0 → no arbitrage
	// EURGBP Bid=0.90, GBPUSD Bid=1.1999, EURUSD Ask=1.0801
	// Product2 = 0.90 × 1.1999 / 1.0801 = 1.07991 / 1.0801 = 0.9998 < 1 → no
	l1 := mkListing("ICM", "EURUSD", "EURUSD", "100000", "0.00001", "0.01", "0", "0", listing.SwapNone)
	l2 := mkListing("ICM", "GBPUSD", "GBPUSD", "100000", "0.00001", "0.01", "0", "0", listing.SwapNone)
	l3 := mkListing("ICM", "EURGBP", "EURGBP", "100000", "0.00001", "0.01", "0", "0", listing.SwapNone)

	listings := mkListings(l1, l2, l3)
	quotes := mkQuotes(
		mkQuote("EURUSD", "ICM", 1.0799, 1.0800, now),
		mkQuote("GBPUSD", "ICM", 1.1999, 1.2000, now),
		mkQuote("EURGBP", "ICM", 0.8999, 0.9000, now),
	)

	d := NewTriangular()
	cands, _ := d.Scan(context.Background(), quotes, listings)
	if len(cands) != 0 {
		t.Errorf("expected 0 candidates (no deviation), got %d", len(cands))
	}
}

func TestTriangular_MissingPair(t *testing.T) {
	now := time.Now()
	// Only 2 of 3 pairs present
	l1 := mkListing("ICM", "EURUSD", "EURUSD", "100000", "0.00001", "0.01", "0", "0", listing.SwapNone)
	l2 := mkListing("ICM", "GBPUSD", "GBPUSD", "100000", "0.00001", "0.01", "0", "0", listing.SwapNone)

	listings := mkListings(l1, l2)
	quotes := mkQuotes(
		mkQuote("EURUSD", "ICM", 1.0799, 1.0800, now),
		mkQuote("GBPUSD", "ICM", 1.1999, 1.2000, now),
	)

	d := NewTriangular()
	cands, _ := d.Scan(context.Background(), quotes, listings)
	if len(cands) != 0 {
		t.Errorf("expected 0 candidates (missing pair), got %d", len(cands))
	}
}

// --- Scan dispatch test ---

func TestScan_MultipleDetectors(t *testing.T) {
	now := time.Now()
	lA := mkListing("ICM", "EURUSD", "EURUSD", "100000", "0.00001", "0.01", "-8.287", "1.544", listing.SwapInPoints)
	lB := mkListing("EXN", "EURUSDm", "EURUSD", "100000", "0.00001", "0.01", "-5.0", "2.0", listing.SwapInPoints)

	listings := mkListings(lA, lB)
	quotes := mkQuotes(
		mkQuote("EURUSD", "ICM", 1.0799, 1.0800, now),
		mkQuote("EURUSDm", "EXN", 1.0804, 1.0805, now),
	)

	detectors := []Detector{NewCrossExchange(), NewCarry()}
	cands := Scan(detectors, quotes, listings)

	// CrossExchange should find 1 (positive spread), Carry should find 0 (negative swap)
	// Total after dedup: 1
	if len(cands) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(cands))
	}
	if cands[0].Type != evaluator.OppCrossExchange {
		t.Errorf("Type = %d, want OppCrossExchange", cands[0].Type)
	}
}
