package listing

import (
	"context"
	"errors"
	"testing"

	"github.com/shopspring/decimal"
)

type mockFetcher struct {
	broker string
}

func (m *mockFetcher) BrokerName() string { return m.broker }
func (m *mockFetcher) Listing(_ context.Context, brokerSymbol string) (*Listing, error) {
	return &Listing{
		Broker:       m.broker,
		BrokerSymbol: brokerSymbol,
		Digits:       5,
		Points:       decimal.RequireFromString("0.00001"),
	}, nil
}

func TestCacheGetPut(t *testing.T) {
	c := NewCache()
	l := &Listing{Broker: "ICM", BrokerSymbol: "EURUSD", Digits: 5}
	c.mu.Lock()
	c.items[cacheKey("ICM", "EURUSD")] = l
	c.mu.Unlock()

	got, ok := c.Get("ICM", "EURUSD")
	if !ok || got.Digits != 5 {
		t.Fatalf("Get returned %v, ok=%v", got, ok)
	}
	if _, ok := c.Get("ICM", "GBPJPY"); ok {
		t.Fatal("expected miss for uncached symbol")
	}
}

func TestCachePopulate(t *testing.T) {
	c := NewCache()
	f := &mockFetcher{broker: "ICM"}
	syms := map[string][]string{"ICM": {"EURUSD", "XAUUSD"}}
	if err := c.Populate(context.Background(), []Fetcher{f}, syms); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Get("ICM", "EURUSD"); !ok {
		t.Fatal("EURUSD not cached")
	}
	if _, ok := c.Get("ICM", "XAUUSD"); !ok {
		t.Fatal("XAUUSD not cached")
	}
	all := c.All()
	if len(all) != 2 {
		t.Fatalf("All() returned %d, want 2", len(all))
	}
}

type errFetcher struct{ broker string }

func (e *errFetcher) BrokerName() string { return e.broker }
func (e *errFetcher) Listing(_ context.Context, _ string) (*Listing, error) {
	return nil, errors.New("connection refused")
}

func TestCachePopulateEmptyError(t *testing.T) {
	c := NewCache()
	f := &errFetcher{broker: "DEAD"}
	syms := map[string][]string{"DEAD": {"EURUSD"}}
	err := c.Populate(context.Background(), []Fetcher{f}, syms)
	if err == nil {
		t.Fatal("expected error when 0 listings stored, got nil")
	}
}
