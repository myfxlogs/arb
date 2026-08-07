package listing

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestResolveInstrument_StandardFX(t *testing.T) {
	got := ResolveInstrument("EURUSD")
	if got.Base != "EUR" || got.Quote != "USD" || got.AssetClass != "FX" || got.Kind != "SPOT" {
		t.Errorf("EURUSD → %+v", got)
	}
}

func TestResolveInstrument_PreciousMetal(t *testing.T) {
	got := ResolveInstrument("XAUUSD")
	if got.Base != "XAU" || got.Quote != "USD" || got.AssetClass != "FX" {
		t.Errorf("XAUUSD → %+v", got)
	}
	got2 := ResolveInstrument("XAGJPY")
	if got2.Base != "XAG" || got2.Quote != "JPY" {
		t.Errorf("XAGJPY → %+v", got2)
	}
}

func TestResolveInstrument_Crypto(t *testing.T) {
	got := ResolveInstrument("BTCUSDT")
	if got.Base != "BTC" || got.Quote != "USDT" || got.AssetClass != "CRYPTO" || got.Kind != "PERP" {
		t.Errorf("BTCUSDT → %+v", got)
	}
}

func TestResolveInstrument_ShortSymbol(t *testing.T) {
	got := ResolveInstrument("ABC")
	if got.Base != "" || got.Quote != "" {
		t.Errorf("ABC → %+v, want empty base/quote", got)
	}
}

func TestCanonicalIndex(t *testing.T) {
	c := NewCache()
	c.mu.Lock()
	c.items[cacheKey("ICM", "EURUSD")] = &Listing{
		Broker: "ICM", BrokerSymbol: "EURUSD",
		ContractSize: decimal.NewFromInt(100000),
	}
	c.items[cacheKey("EXN", "EURUSDm")] = &Listing{
		Broker: "EXN", BrokerSymbol: "EURUSDm",
		ContractSize: decimal.NewFromInt(100000),
	}
	c.mu.Unlock()

	symMap := map[string]map[string]string{
		"ICM": {"EURUSD": "EURUSD"},
		"EXN": {"EURUSDm": "EURUSD"},
	}

	idx := c.CanonicalIndex(symMap)
	if len(idx) != 2 {
		t.Fatalf("CanonicalIndex returned %d entries, want 2", len(idx))
	}

	icm := idx[CanonicalKey{Broker: "ICM", Canonical: "EURUSD"}]
	if icm == nil {
		t.Fatal("ICM/EURUSD missing")
	}
	if icm.Instrument == nil || icm.Instrument.Base != "EUR" {
		t.Errorf("ICM Instrument.Base = %v", icm.Instrument)
	}
	if icm.BrokerSymbol != "EURUSD" {
		t.Errorf("ICM BrokerSymbol = %s, want EURUSD", icm.BrokerSymbol)
	}

	exn := idx[CanonicalKey{Broker: "EXN", Canonical: "EURUSD"}]
	if exn == nil {
		t.Fatal("EXN/EURUSD missing")
	}
	if exn.Instrument == nil || exn.Instrument.Base != "EUR" {
		t.Errorf("EXN Instrument.Base = %v", exn.Instrument)
	}
	if exn.BrokerSymbol != "EURUSDm" {
		t.Errorf("EXN BrokerSymbol = %s, want EURUSDm", exn.BrokerSymbol)
	}
}

func TestCanonicalIndex_MissingInCache(t *testing.T) {
	c := NewCache()
	symMap := map[string]map[string]string{
		"ICM": {"GBPJPY": "GBPJPY"},
	}
	idx := c.CanonicalIndex(symMap)
	if len(idx) != 0 {
		t.Fatalf("expected 0 entries for uncached symbol, got %d", len(idx))
	}
}
