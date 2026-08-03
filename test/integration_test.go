package integration_test

import (
	"context"
	"testing"
	"time"

	"arb/internal/adapter"
	"arb/internal/bus"
	"arb/internal/dashboard"
	"arb/internal/execute"
	"arb/internal/risk"
	"github.com/shopspring/decimal"
)

// mockAdapter implements adapter.PlatformAdapter for integration testing.
type mockAdapter struct {
	broker string
}

func (m *mockAdapter) Connect(context.Context) (string, error) { return "token", nil }
func (m *mockAdapter) Disconnect() error                       { return nil }
func (m *mockAdapter) HealthCheck(context.Context) error       { return nil }
func (m *mockAdapter) Subscribe(context.Context, []string) error { return nil }
func (m *mockAdapter) QuoteStream(context.Context, *bus.QuoteBus) {}
func (m *mockAdapter) AccountSummary(context.Context) (*adapter.Account, error) {
	return &adapter.Account{Currency: "USD"}, nil
}
func (m *mockAdapter) OpenOrders(context.Context) ([]adapter.Order, error) { return nil, nil }
func (m *mockAdapter) AllSymbols(context.Context) ([]string, error)        { return nil, nil }
func (m *mockAdapter) SymbolDigits(context.Context, []string) (map[string]int32, error) {
	return nil, nil
}
func (m *mockAdapter) PlaceOrder(ctx context.Context, req adapter.OrderRequest) (*adapter.OrderResult, error) {
	return &adapter.OrderResult{
		ClientID:    req.ClientID,
		Ticket:      1,
		State:       adapter.StateFilled,
		Volume:      req.Volume,
		CloseVolume: req.Volume,
	}, nil
}
func (m *mockAdapter) CancelOrder(context.Context, int64) error { return nil }
func (m *mockAdapter) CloseOrder(context.Context, int64, decimal.Decimal, float64, int32) (*adapter.OrderResult, error) {
	return nil, nil
}
func (m *mockAdapter) Platform() bus.PlatformType { return bus.PlatformMT5 }
func (m *mockAdapter) BrokerName() string         { return m.broker }

func TestIntegrationPipelineRiskDashboard(t *testing.T) {
	ctx := context.Background()

	// Setup QuoteBus
	symbols := []string{"EURUSD", "GBPUSD", "EURGBP"}
	quoteBus := bus.New(symbols)

	// Setup risk components
	killSwitch := risk.NewKillSwitch("/tmp/arb_integration_kill")
	defer killSwitch.Deactivate()
	breaker := risk.NewCircuitBreaker(5, 30*time.Second, 5000, 500)

	// Setup adapters
	adapters := map[string]adapter.PlatformAdapter{
		"BrokerA": &mockAdapter{broker: "BrokerA"},
	}

	// Setup execution pipeline
	dedup := execute.NewDedupCache()
	pipeline := execute.NewPipeline(execute.PipelineDeps{
		Bus:      quoteBus,
		Dedup:    dedup,
		Adapters: adapters,
	})

	// Setup dashboard server
	dashServer := dashboard.NewServer(dashboard.Deps{
		Bus:        quoteBus,
		Adapters:   adapters,
		KillSwitch: killSwitch,
		Breaker:    breaker,
		Symbols:    symbols,
	})
	dashServer.StartFeeder()

	// Publish quotes continuously
	go func() {
		for i := 0; i < 50; i++ {
			quoteBus.Publish(bus.Quote{Symbol: "EURUSD", Bid: 1.05, Ask: 1.06, Broker: "BrokerA"})
			quoteBus.Publish(bus.Quote{Symbol: "GBPUSD", Bid: 1.25, Ask: 1.26, Broker: "BrokerA"})
			quoteBus.Publish(bus.Quote{Symbol: "EURGBP", Bid: 0.83, Ask: 0.84, Broker: "BrokerA"})
			time.Sleep(10 * time.Millisecond)
		}
	}()

	// Execute arbitrage through pipeline
	opp := execute.ArbitrageOpportunity{
		Legs: []execute.Leg{
			{Broker: "BrokerA", Symbol: "EURUSD", Operation: adapter.OpBuy, Volume: 0.1, ClientID: "int-c1"},
			{Broker: "BrokerA", Symbol: "GBPUSD", Operation: adapter.OpSell, Volume: 0.1, ClientID: "int-c2"},
		},
		Params: execute.StrategyParams{OrderTimeout: 5 * time.Second, MaxSlippage: 100},
	}

	if err := pipeline.Execute(ctx, opp); err != nil {
		t.Fatalf("pipeline Execute: %v", err)
	}

	// Verify dedup cache has the orders
	if _, ok := dedup.Get("int-c1"); !ok {
		t.Error("dedup cache should have int-c1")
	}
	if _, ok := dedup.Get("int-c2"); !ok {
		t.Error("dedup cache should have int-c2")
	}

	// Verify dashboard can build spread matrix
	time.Sleep(100 * time.Millisecond)
	reply := dashServer.BuildSpreadMatrixForTest()
	if reply.TotalSymbols != 3 {
		t.Errorf("spread matrix totalSymbols = %d, want 3", reply.TotalSymbols)
	}

	// Verify kill switch works
	if err := killSwitch.Check(); err != nil {
		t.Errorf("kill switch should be inactive: %v", err)
	}
}

func TestIntegrationRiskChainBlocksOnCircuitOpen(t *testing.T) {
	breaker := risk.NewCircuitBreaker(2, 30*time.Second, 5000, 500)

	// Record 2 losses to trip the breaker
	breaker.RecordLoss(100)
	breaker.RecordLoss(100)

	if err := breaker.Allow(); err != risk.ErrCircuitOpen {
		t.Errorf("breaker should be open, got err=%v", err)
	}

	// Kill switch should also work independently
	ks := risk.NewKillSwitch("/tmp/arb_integration_ks2")
	defer ks.Deactivate()
	_ = ks.Activate()

	if err := ks.Check(); err != risk.ErrKillSwitch {
		t.Errorf("kill switch should be active")
	}
}
