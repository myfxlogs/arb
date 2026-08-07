package dashboard

import (
	"context"
	"io"
	"testing"
	"time"

	"arb/internal/bus"
	"arb/internal/engine"
	"arb/internal/evaluator"
	"arb/internal/risk"

	"github.com/shopspring/decimal"
	"google.golang.org/grpc/metadata"

	dashpb "arb/proto/gen/dashboard"
)

func TestOpportunityStream_E2E(t *testing.T) {
	eng := engine.New(engine.Deps{
		Bus: bus.New([]string{"EURUSD"}),
	})

	srv := NewServer(Deps{
		Bus:        bus.New(nil),
		Engine:     eng,
		KillSwitch: risk.NewKillSwitch("/tmp/arb_kill_test"),
	})

	opp := &evaluator.Opportunity{
		ID:            "test-opp-1",
		Type:          evaluator.OppCrossExchange,
		QuoteTime:     time.Now(),
		GrossProfit:   decimal.NewFromInt(100),
		NetProfit:     decimal.NewFromInt(80),
		NetBps:        decimal.NewFromInt(50),
		ExpiresAt:     time.Now().Add(5 * time.Second),
		Executable:    true,
		Confidence:    0.95,
		Status:        evaluator.OppStatusPushed,
		NotionalUSD:   decimal.NewFromInt(10000),
		Legs: []evaluator.OppLeg{
			{
				Broker:       "BrokerA",
				BrokerSymbol: "EURUSD",
				Canonical:    "EURUSD",
				Direction:    evaluator.Buy,
				Lots:         decimal.NewFromFloat(1.0),
				EstPrice:     decimal.NewFromFloat(1.0850),
			},
			{
				Broker:       "BrokerB",
				BrokerSymbol: "EURUSD",
				Canonical:    "EURUSD",
				Direction:    evaluator.Sell,
				Lots:         decimal.NewFromFloat(1.0),
				EstPrice:     decimal.NewFromFloat(1.0855),
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	stream := &mockOpportunityStreamServer{ctx: ctx, recvCh: make(chan *dashpb.OpportunityEvent, 4)}
	streamReady := make(chan struct{})
	go func() {
		close(streamReady)
		_ = srv.OpportunityStream(&dashpb.OpportunityStreamRequest{}, stream)
	}()
	<-streamReady
	time.Sleep(50 * time.Millisecond) // allow Subscribe() to register

	eng.PushOpportunityForTest(opp)

	select {
	case ev := <-stream.recvCh:
		if ev.GetId() != "test-opp-1" {
			t.Fatalf("expected id test-opp-1, got %s", ev.GetId())
		}
		if ev.GetAction() != dashpb.OpportunityAction_OPP_ACTION_PUSHED {
			t.Fatalf("expected PUSHED action, got %v", ev.GetAction())
		}
		gotOpp := ev.GetOpp()
		if gotOpp.GetType() != dashpb.OppType_OPP_TYPE_CROSS_EXCHANGE {
			t.Fatalf("expected CROSS_EXCHANGE type, got %v", gotOpp.GetType())
		}
		if len(gotOpp.GetLegs()) != 2 {
			t.Fatalf("expected 2 legs, got %d", len(gotOpp.GetLegs()))
		}
		if gotOpp.GetLegs()[0].GetBroker() != "BrokerA" {
			t.Fatalf("expected BrokerA, got %s", gotOpp.GetLegs()[0].GetBroker())
		}
		if gotOpp.GetLegs()[0].GetDirection() != dashpb.BuySell_BUY_SELL_BUY {
			t.Fatalf("expected BUY direction, got %v", gotOpp.GetLegs()[0].GetDirection())
		}
		if gotOpp.GetLegs()[1].GetDirection() != dashpb.BuySell_BUY_SELL_SELL {
			t.Fatalf("expected SELL direction, got %v", gotOpp.GetLegs()[1].GetDirection())
		}
		if gotOpp.GetNetProfit() != "80" {
			t.Fatalf("expected net_profit 80, got %s", gotOpp.GetNetProfit())
		}
		if !gotOpp.GetExecutable() {
			t.Fatal("expected executable=true")
		}
		if gotOpp.GetStatus() != dashpb.OppStatus_OPP_STATUS_PUSHED {
			t.Fatalf("expected PUSHED status, got %v", gotOpp.GetStatus())
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for opportunity event")
	}

	cancel()
}

func TestConfirmOpportunity(t *testing.T) {
	eng := engine.New(engine.Deps{
		Bus: bus.New([]string{"EURUSD"}),
	})

	srv := NewServer(Deps{
		Bus:        bus.New(nil),
		Engine:     eng,
		KillSwitch: risk.NewKillSwitch("/tmp/arb_kill_test2"),
	})

	opp := &evaluator.Opportunity{
		ID:         "test-confirm-1",
		Type:       evaluator.OppCrossExchange,
		Executable: true,
		Status:     evaluator.OppStatusPushed,
		ExpiresAt:  time.Now().Add(10 * time.Second),
	}
	eng.PushOpportunityForTest(opp)

	reply, err := srv.ConfirmOpportunity(context.Background(), &dashpb.ConfirmRequest{
		OpportunityId: "test-confirm-1",
	})
	if err != nil {
		t.Fatalf("ConfirmOpportunity error: %v", err)
	}
	if !reply.GetAccepted() {
		t.Fatalf("expected accepted=true, reason=%s", reply.GetReason())
	}

	reply2, _ := srv.ConfirmOpportunity(context.Background(), &dashpb.ConfirmRequest{
		OpportunityId: "test-confirm-1",
	})
	if reply2.GetAccepted() {
		t.Fatal("expected second confirm to be rejected (already confirmed)")
	}
}

func TestConfirmOpportunity_NotFound(t *testing.T) {
	eng := engine.New(engine.Deps{
		Bus: bus.New(nil),
	})

	srv := NewServer(Deps{
		Bus:    bus.New(nil),
		Engine: eng,
	})

	reply, err := srv.ConfirmOpportunity(context.Background(), &dashpb.ConfirmRequest{
		OpportunityId: "nonexistent",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reply.GetAccepted() {
		t.Fatal("expected accepted=false for nonexistent opportunity")
	}
}

func TestProtoConversions(t *testing.T) {
	t.Run("oppType", func(t *testing.T) {
		cases := []struct {
			in  evaluator.OppType
			out dashpb.OppType
		}{
			{evaluator.OppCrossExchange, dashpb.OppType_OPP_TYPE_CROSS_EXCHANGE},
			{evaluator.OppCarry, dashpb.OppType_OPP_TYPE_CARRY},
			{evaluator.OppTriangular, dashpb.OppType_OPP_TYPE_TRIANGULAR},
			{0, dashpb.OppType_OPP_TYPE_UNSPECIFIED},
		}
		for _, c := range cases {
			if got := toProtoOppType(c.in); got != c.out {
				t.Errorf("toProtoOppType(%d) = %v, want %v", c.in, got, c.out)
			}
		}
	})

	t.Run("buySell", func(t *testing.T) {
		if toProtoBuySell(evaluator.Buy) != dashpb.BuySell_BUY_SELL_BUY {
			t.Error("Buy mismatch")
		}
		if toProtoBuySell(evaluator.Sell) != dashpb.BuySell_BUY_SELL_SELL {
			t.Error("Sell mismatch")
		}
	})

	t.Run("legRole", func(t *testing.T) {
		if toProtoLegRole(evaluator.LegRoleIncome) != dashpb.LegRole_LEG_ROLE_INCOME {
			t.Error("Income mismatch")
		}
		if toProtoLegRole(evaluator.LegRoleHedge) != dashpb.LegRole_LEG_ROLE_HEDGE {
			t.Error("Hedge mismatch")
		}
	})

	t.Run("oppStatus", func(t *testing.T) {
		cases := []struct {
			in  evaluator.OppStatus
			out dashpb.OppStatus
		}{
			{evaluator.OppStatusPushed, dashpb.OppStatus_OPP_STATUS_PUSHED},
			{evaluator.OppStatusConfirmed, dashpb.OppStatus_OPP_STATUS_CONFIRMED},
			{evaluator.OppStatusExpired, dashpb.OppStatus_OPP_STATUS_EXPIRED},
		}
		for _, c := range cases {
			if got := toProtoOppStatus(c.in); got != c.out {
				t.Errorf("toProtoOppStatus(%d) = %v, want %v", c.in, got, c.out)
			}
		}
	})
}

type mockOpportunityStreamServer struct {
	ctx    context.Context
	recvCh chan *dashpb.OpportunityEvent
}

func (m *mockOpportunityStreamServer) Send(ev *dashpb.OpportunityEvent) error {
	select {
	case m.recvCh <- ev:
		return nil
	default:
		return io.EOF
	}
}

func (m *mockOpportunityStreamServer) Context() context.Context { return m.ctx }

func (m *mockOpportunityStreamServer) SetHeader(metadata.MD) error    { return nil }
func (m *mockOpportunityStreamServer) SendHeader(metadata.MD) error  { return nil }
func (m *mockOpportunityStreamServer) SetTrailer(metadata.MD)        {}
func (m *mockOpportunityStreamServer) SendMsg(any) error             { return nil }
func (m *mockOpportunityStreamServer) RecvMsg(any) error             { return io.EOF }
