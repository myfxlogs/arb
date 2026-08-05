package execute

import (
	"context"
	"sync"
	"testing"
	"time"

	"arb/internal/adapter"
	"arb/internal/bus"
	"github.com/shopspring/decimal"
)

// mockAdapter implements adapter.PlatformAdapter for testing.
type mockAdapter struct {
	broker       string
	placeOrderFn func(ctx context.Context, req adapter.OrderRequest) (*adapter.OrderResult, error)
	calls        int
	mu           sync.Mutex
}

func (m *mockAdapter) Connect(context.Context) (string, error) { return "token", nil }
func (m *mockAdapter) Disconnect() error                       { return nil }
func (m *mockAdapter) HealthCheck(context.Context) error       { return nil }
func (m *mockAdapter) Subscribe(context.Context, []string) error { return nil }
func (m *mockAdapter) QuoteStream(context.Context, *bus.QuoteBus) {}
func (m *mockAdapter) AccountSummary(context.Context) (*adapter.Account, error) { return nil, nil }
func (m *mockAdapter) OpenOrders(context.Context) ([]adapter.Order, error)      { return nil, nil }
func (m *mockAdapter) AllSymbols(context.Context) ([]string, error)            { return nil, nil }
func (m *mockAdapter) SymbolDigits(context.Context, []string) (map[string]int32, error) {
	return nil, nil
}
func (m *mockAdapter) PlaceOrder(ctx context.Context, req adapter.OrderRequest) (*adapter.OrderResult, error) {
	m.mu.Lock()
	m.calls++
	m.mu.Unlock()
	if m.placeOrderFn != nil {
		return m.placeOrderFn(ctx, req)
	}
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
func (m *mockAdapter) OrderHistory(context.Context, time.Time, time.Time) ([]adapter.Order, error) {
	return nil, nil
}
func (m *mockAdapter) Platform() bus.PlatformType { return bus.PlatformMT5 }
func (m *mockAdapter) BrokerName() string         { return m.broker }
func (m *mockAdapter) SetOnReconnect(func(context.Context) error) {}
func (m *mockAdapter) Stop()                                       {}

func (m *mockAdapter) getCalls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

func makePipeline(adapters map[string]adapter.PlatformAdapter) *ExecutionPipeline {
	b := bus.New([]string{"EURUSD", "GBPUSD", "EURGBP"})
	dedup := NewDedupCache()
	return NewPipeline(PipelineDeps{
		Bus:      b,
		Dedup:    dedup,
		Adapters: adapters,
	})
}

func TestPipelineTwoLegsAllFilled(t *testing.T) {
	a := &mockAdapter{broker: "BrokerA"}
	p := makePipeline(map[string]adapter.PlatformAdapter{"BrokerA": a})

	opp := ArbitrageOpportunity{
		Legs: []Leg{
			{Broker: "BrokerA", Symbol: "EURUSD", Operation: adapter.OpBuy, Volume: 0.1, ClientID: "c1"},
			{Broker: "BrokerA", Symbol: "GBPUSD", Operation: adapter.OpSell, Volume: 0.1, ClientID: "c2"},
		},
		Params: StrategyParams{OrderTimeout: 5 * time.Second, MaxSlippage: 100},
	}

	// Publish quotes continuously so all legs can receive them
	go func() {
		b := p.deps.Bus
		for i := 0; i < 10; i++ {
			b.Publish(bus.Quote{Symbol: "EURUSD", Bid: 1.05, Ask: 1.06})
			b.Publish(bus.Quote{Symbol: "GBPUSD", Bid: 1.25, Ask: 1.26})
			time.Sleep(20 * time.Millisecond)
		}
	}()

	if err := p.Execute(context.Background(), opp); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if a.getCalls() != 2 {
		t.Errorf("PlaceOrder calls = %d, want 2", a.getCalls())
	}
}

func TestPipelineThreeLegsAllFilled(t *testing.T) {
	a := &mockAdapter{broker: "BrokerA"}
	p := makePipeline(map[string]adapter.PlatformAdapter{"BrokerA": a})

	opp := ArbitrageOpportunity{
		Legs: []Leg{
			{Broker: "BrokerA", Symbol: "EURUSD", Operation: adapter.OpBuy, Volume: 0.1, ClientID: "c1"},
			{Broker: "BrokerA", Symbol: "GBPUSD", Operation: adapter.OpSell, Volume: 0.1, ClientID: "c2"},
			{Broker: "BrokerA", Symbol: "EURGBP", Operation: adapter.OpBuy, Volume: 0.1, ClientID: "c3"},
		},
		Params: StrategyParams{OrderTimeout: 5 * time.Second, MaxSlippage: 100},
	}

	b := p.deps.Bus
	go func() {
		for i := 0; i < 10; i++ {
			b.Publish(bus.Quote{Symbol: "EURUSD", Bid: 1.05, Ask: 1.06})
			b.Publish(bus.Quote{Symbol: "GBPUSD", Bid: 1.25, Ask: 1.26})
			b.Publish(bus.Quote{Symbol: "EURGBP", Bid: 0.83, Ask: 0.84})
			time.Sleep(20 * time.Millisecond)
		}
	}()

	if err := p.Execute(context.Background(), opp); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if a.getCalls() != 3 {
		t.Errorf("PlaceOrder calls = %d, want 3", a.getCalls())
	}
}

func TestPipelineOneFilledOneRejectedHedge(t *testing.T) {
	a := &mockAdapter{
		broker: "BrokerA",
		placeOrderFn: func(ctx context.Context, req adapter.OrderRequest) (*adapter.OrderResult, error) {
			if req.ClientID == "c2" {
				return &adapter.OrderResult{
					ClientID: req.ClientID,
					State:    adapter.StateRejected,
					Error:    nil,
				}, nil
			}
			return &adapter.OrderResult{
				ClientID:    req.ClientID,
				Ticket:      1,
				State:       adapter.StateFilled,
				Volume:      req.Volume,
				CloseVolume: req.Volume,
			}, nil
		},
	}
	p := makePipeline(map[string]adapter.PlatformAdapter{"BrokerA": a})

	opp := ArbitrageOpportunity{
		Legs: []Leg{
			{Broker: "BrokerA", Symbol: "EURUSD", Operation: adapter.OpBuy, Volume: 0.1, ClientID: "c1"},
			{Broker: "BrokerA", Symbol: "GBPUSD", Operation: adapter.OpSell, Volume: 0.1, ClientID: "c2"},
		},
		Params: StrategyParams{OrderTimeout: 5 * time.Second, MaxSlippage: 100},
	}

	b := p.deps.Bus
	go func() {
		for i := 0; i < 10; i++ {
			b.Publish(bus.Quote{Symbol: "EURUSD", Bid: 1.05, Ask: 1.06})
			b.Publish(bus.Quote{Symbol: "GBPUSD", Bid: 1.25, Ask: 1.26})
			time.Sleep(20 * time.Millisecond)
		}
	}()

	err := p.Execute(context.Background(), opp)
	if err == nil {
		t.Fatal("expected error for partial fill")
	}
	// c1 filled → hedge called (1 extra PlaceOrder for hedge)
	// c2 rejected → no hedge
	// Total calls: c1 + c2 + hedge(c1) = 3
	if a.getCalls() != 3 {
		t.Errorf("PlaceOrder calls = %d, want 3 (2 legs + 1 hedge)", a.getCalls())
	}
}

func TestPipelineTimeoutHedgesFilled(t *testing.T) {
	a := &mockAdapter{
		broker: "BrokerA",
		placeOrderFn: func(ctx context.Context, req adapter.OrderRequest) (*adapter.OrderResult, error) {
			// Simulate slow response that exceeds timeout
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	p := makePipeline(map[string]adapter.PlatformAdapter{"BrokerA": a})

	opp := ArbitrageOpportunity{
		Legs: []Leg{
			{Broker: "BrokerA", Symbol: "EURUSD", Operation: adapter.OpBuy, Volume: 0.1, ClientID: "c1"},
		},
		Params: StrategyParams{OrderTimeout: 50 * time.Millisecond, MaxSlippage: 100},
	}

	b := p.deps.Bus
	go func() {
		for i := 0; i < 10; i++ {
			b.Publish(bus.Quote{Symbol: "EURUSD", Bid: 1.05, Ask: 1.06})
			time.Sleep(20 * time.Millisecond)
		}
	}()

	err := p.Execute(context.Background(), opp)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestPipelineRevalidationFailsNoOrders(t *testing.T) {
	a := &mockAdapter{broker: "BrokerA"}
	p := makePipeline(map[string]adapter.PlatformAdapter{"BrokerA": a})

	opp := ArbitrageOpportunity{
		Legs: []Leg{
			{Broker: "BrokerA", Symbol: "EURUSD", Operation: adapter.OpBuy, Volume: 0.1, ClientID: "c1"},
		},
		Params: StrategyParams{OrderTimeout: 5 * time.Second, MaxSlippage: 100},
	}

	// No quotes published → revalidation fails
	err := p.Execute(context.Background(), opp)
	if err == nil {
		t.Fatal("expected revalidation error")
	}
	if a.getCalls() != 0 {
		t.Errorf("PlaceOrder calls = %d, want 0 (revalidation should block)", a.getCalls())
	}
}

func TestPipelineConcurrentLegSubmission(t *testing.T) {
	var maxConcurrent int32
	var current int32
	var mu sync.Mutex

	a := &mockAdapter{
		broker: "BrokerA",
		placeOrderFn: func(ctx context.Context, req adapter.OrderRequest) (*adapter.OrderResult, error) {
			mu.Lock()
			current++
			if current > maxConcurrent {
				maxConcurrent = current
			}
			mu.Unlock()
			time.Sleep(20 * time.Millisecond)
			mu.Lock()
			current--
			mu.Unlock()
			return &adapter.OrderResult{
				ClientID:    req.ClientID,
				Ticket:      1,
				State:       adapter.StateFilled,
				Volume:      req.Volume,
				CloseVolume: req.Volume,
			}, nil
		},
	}
	p := makePipeline(map[string]adapter.PlatformAdapter{"BrokerA": a})

	opp := ArbitrageOpportunity{
		Legs: []Leg{
			{Broker: "BrokerA", Symbol: "EURUSD", Operation: adapter.OpBuy, Volume: 0.1, ClientID: "c1"},
			{Broker: "BrokerA", Symbol: "GBPUSD", Operation: adapter.OpSell, Volume: 0.1, ClientID: "c2"},
			{Broker: "BrokerA", Symbol: "EURGBP", Operation: adapter.OpBuy, Volume: 0.1, ClientID: "c3"},
		},
		Params: StrategyParams{OrderTimeout: 5 * time.Second, MaxSlippage: 100},
	}

	b := p.deps.Bus
	go func() {
		for i := 0; i < 10; i++ {
			b.Publish(bus.Quote{Symbol: "EURUSD", Bid: 1.05, Ask: 1.06})
			b.Publish(bus.Quote{Symbol: "GBPUSD", Bid: 1.25, Ask: 1.26})
			b.Publish(bus.Quote{Symbol: "EURGBP", Bid: 0.83, Ask: 0.84})
			time.Sleep(20 * time.Millisecond)
		}
	}()

	if err := p.Execute(context.Background(), opp); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if maxConcurrent < 2 {
		t.Errorf("max concurrent legs = %d, expected >= 2 (concurrent submission)", maxConcurrent)
	}
}
