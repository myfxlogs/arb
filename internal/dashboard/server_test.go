package dashboard

import (
	"context"
	"testing"
	"time"

	"arb/internal/adapter"
	"arb/internal/bus"
	"arb/internal/risk"
	dashpb "arb/proto/gen/dashboard"

	"github.com/shopspring/decimal"
)

type mockAdapter struct {
	name string
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
func (m *mockAdapter) OrderHistory(context.Context, time.Time, time.Time) ([]adapter.Order, error) {
	return nil, nil
}
func (m *mockAdapter) AllSymbols(context.Context) ([]string, error)        { return nil, nil }
func (m *mockAdapter) SymbolDigits(context.Context, []string) (map[string]int32, error) {
	return nil, nil
}
func (m *mockAdapter) PlaceOrder(context.Context, adapter.OrderRequest) (*adapter.OrderResult, error) {
	return &adapter.OrderResult{State: adapter.StateFilled}, nil
}
func (m *mockAdapter) CancelOrder(context.Context, int64) error { return nil }
func (m *mockAdapter) CloseOrder(context.Context, int64, decimal.Decimal, float64, int32) (*adapter.OrderResult, error) {
	return nil, nil
}
func (m *mockAdapter) Platform() bus.PlatformType { return bus.PlatformMT5 }
func (m *mockAdapter) BrokerName() string         { return m.name }

func TestSpreadMatrixBestBidAsk(t *testing.T) {
	b := bus.New([]string{"EURUSD"})
	adapters := map[string]adapter.PlatformAdapter{
		"BrokerA": &mockAdapter{name: "BrokerA"},
		"BrokerB": &mockAdapter{name: "BrokerB"},
	}
	srv := NewServer(Deps{
		Bus:      b,
		Adapters: adapters,
		Symbols:  []string{"EURUSD"},
	})
	srv.StartFeeder()

	// Publish quotes from two brokers continuously
	go func() {
		for i := 0; i < 20; i++ {
			b.Publish(bus.Quote{Symbol: "EURUSD", Bid: 1.05, Ask: 1.06, Broker: "BrokerA"})
			b.Publish(bus.Quote{Symbol: "EURUSD", Bid: 1.04, Ask: 1.07, Broker: "BrokerB"})
			time.Sleep(10 * time.Millisecond)
		}
	}()

	time.Sleep(100 * time.Millisecond)
	reply := srv.buildSpreadMatrix()
	if reply.TotalSymbols != 1 {
		t.Errorf("totalSymbols = %d, want 1", reply.TotalSymbols)
	}
	if reply.BestBidBroker != "BrokerA" {
		t.Errorf("bestBidBroker = %s, want BrokerA", reply.BestBidBroker)
	}
	if reply.BestAskBroker != "BrokerA" {
		t.Errorf("bestAskBroker = %s, want BrokerA", reply.BestAskBroker)
	}
}

func TestSpreadMatrixArbitrageable(t *testing.T) {
	b := bus.New([]string{"EURUSD"})
	adapters := map[string]adapter.PlatformAdapter{
		"BrokerA": &mockAdapter{name: "BrokerA"},
		"BrokerB": &mockAdapter{name: "BrokerB"},
	}
	srv := NewServer(Deps{
		Bus:      b,
		Adapters: adapters,
		Symbols:  []string{"EURUSD"},
	})
	srv.StartFeeder()

	b.Publish(bus.Quote{Symbol: "EURUSD", Bid: 1.10, Ask: 1.11, Broker: "BrokerA"})
	b.Publish(bus.Quote{Symbol: "EURUSD", Bid: 1.04, Ask: 1.05, Broker: "BrokerB"})

	go func() {
		for i := 0; i < 20; i++ {
			b.Publish(bus.Quote{Symbol: "EURUSD", Bid: 1.10, Ask: 1.11, Broker: "BrokerA"})
			b.Publish(bus.Quote{Symbol: "EURUSD", Bid: 1.04, Ask: 1.05, Broker: "BrokerB"})
			time.Sleep(10 * time.Millisecond)
		}
	}()

	time.Sleep(100 * time.Millisecond)
	reply := srv.buildSpreadMatrix()
	var foundArb bool
	for _, row := range reply.Rows {
		for _, cell := range row.Cells {
			if cell.IsArbitrageable {
				foundArb = true
			}
		}
	}
	if !foundArb {
		t.Error("expected at least one arbitrageable cell")
	}
}

func TestListSubscribedSymbols(t *testing.T) {
	srv := NewServer(Deps{
		Symbols: []string{"EURUSD", "GBPUSD", "USDJPY"},
	})
	reply, err := srv.ListSubscribedSymbols(context.Background(), &dashpb.ListSymbolsRequest{})
	if err != nil {
		t.Fatalf("ListSubscribedSymbols: %v", err)
	}
	if len(reply.Symbols) != 3 {
		t.Errorf("symbols count = %d, want 3", len(reply.Symbols))
	}
}

func TestSubscribeAndUnsubscribe(t *testing.T) {
	srv := NewServer(Deps{Symbols: []string{"EURUSD"}})
	_, _ = srv.SubscribeSymbols(context.Background(), &dashpb.SubscribeSymbolsRequest{
		Symbols: []string{"GBPUSD"},
	})
	reply, _ := srv.ListSubscribedSymbols(context.Background(), &dashpb.ListSymbolsRequest{})
	if len(reply.Symbols) != 2 {
		t.Errorf("after subscribe: %d symbols, want 2", len(reply.Symbols))
	}
	_, _ = srv.UnsubscribeSymbols(context.Background(), &dashpb.UnsubscribeSymbolsRequest{
		Symbols: []string{"EURUSD"},
	})
	reply, _ = srv.ListSubscribedSymbols(context.Background(), &dashpb.ListSymbolsRequest{})
	if len(reply.Symbols) != 1 {
		t.Errorf("after unsubscribe: %d symbols, want 1", len(reply.Symbols))
	}
}

func TestKillSwitch(t *testing.T) {
	path := "/tmp/test_kill_switch_dashboard"
	ks := risk.NewKillSwitch(path)
	defer ks.Deactivate()
	srv := NewServer(Deps{
		KillSwitch: ks,
		Adapters:   map[string]adapter.PlatformAdapter{},
	})
	reply, err := srv.Kill(context.Background(), &dashpb.KillRequest{})
	if err != nil {
		t.Fatalf("Kill: %v", err)
	}
	if !reply.Success {
		t.Error("kill should succeed")
	}
	if !ks.IsActive() {
		t.Error("kill switch should be active")
	}
	reply2, _ := srv.Resume(context.Background(), &dashpb.ResumeRequest{})
	if !reply2.Success {
		t.Error("resume should succeed")
	}
}

func TestGetStrategyStatus(t *testing.T) {
	srv := NewServer(Deps{})
	reply, err := srv.GetStrategyStatus(context.Background(), &dashpb.StrategyStatusRequest{})
	if err != nil {
		t.Fatalf("GetStrategyStatus: %v", err)
	}
	if len(reply.Items) == 0 {
		t.Error("expected at least one strategy")
	}
}

func TestToggleStrategy(t *testing.T) {
	srv := NewServer(Deps{})
	_, err := srv.ToggleStrategy(context.Background(), &dashpb.ToggleStrategyRequest{
		Strategy: "triangular",
		Enabled:  false,
	})
	if err != nil {
		t.Fatalf("ToggleStrategy: %v", err)
	}
	reply, _ := srv.GetStrategyStatus(context.Background(), &dashpb.StrategyStatusRequest{Strategy: "triangular"})
	if len(reply.Items) != 1 {
		t.Fatalf("expected 1 strategy, got %d", len(reply.Items))
	}
	if reply.Items[0].Enabled {
		t.Error("triangular should be disabled after toggle")
	}
}
