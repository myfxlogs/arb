package integration_test

import (
"context"
"net"
"testing"
"time"

"arb/internal/adapter"
"arb/internal/bus"
"arb/internal/dashboard"
"arb/internal/engine"
"arb/internal/evaluator"
"arb/internal/execute"
"arb/internal/risk"

"github.com/shopspring/decimal"
"google.golang.org/grpc"
"google.golang.org/grpc/credentials/insecure"

dashpb "arb/proto/gen/dashboard"
)

func TestDashboardE2E_OpportunityStream(t *testing.T) {
symbols := []string{"EURUSD", "GBPUSD"}
quoteBus := bus.New(symbols)
mockAd := &mockAdapter{broker: "BrokerA"}
pipeline := execute.NewPipeline(execute.PipelineDeps{
Bus:      quoteBus,
Dedup:    execute.NewDedupCache(),
Adapters: map[string]adapter.PlatformAdapter{"BrokerA": mockAd},
})
eng := engine.New(engine.Deps{
Bus:      quoteBus,
Pipeline: pipeline,
})
dashSrv := dashboard.NewServer(dashboard.Deps{
Bus:        quoteBus,
Adapters:   map[string]adapter.PlatformAdapter{"BrokerA": mockAd},
Engine:     eng,
KillSwitch: risk.NewKillSwitch("/tmp/arb_e2e_kill"),
Symbols:    symbols,
})
lis, err := net.Listen("tcp", "127.0.0.1:0")
if err != nil {
t.Fatalf("listen: %v", err)
}
grpcSrv := grpc.NewServer()
dashpb.RegisterDashboardServiceServer(grpcSrv, dashSrv)
go grpcSrv.Serve(lis)
defer grpcSrv.Stop()
conn, err := grpc.DialContext(context.Background(), lis.Addr().String(),
grpc.WithTransportCredentials(insecure.NewCredentials()),
grpc.WithBlock(),
)
if err != nil {
t.Fatalf("dial: %v", err)
}
defer conn.Close()
client := dashpb.NewDashboardServiceClient(conn)
streamCtx, streamCancel := context.WithTimeout(context.Background(), 5*time.Second)
defer streamCancel()
stream, err := client.OpportunityStream(streamCtx, &dashpb.OpportunityStreamRequest{})
if err != nil {
t.Fatalf("OpportunityStream: %v", err)
}
time.Sleep(100 * time.Millisecond)
opp := &evaluator.Opportunity{
ID:            "e2e-opp-1",
Type:          evaluator.OppCrossExchange,
QuoteTime:     time.Now(),
GrossProfit:   decimal.NewFromInt(100),
NetProfit:     decimal.NewFromInt(80),
NetBps:        decimal.NewFromInt(50),
ExpiresAt:     time.Now().Add(30 * time.Second),
Executable:    true,
Confidence:    0.95,
Status:        evaluator.OppStatusPushed,
NotionalUSD:   decimal.NewFromInt(10000),
Legs: []evaluator.OppLeg{
{Broker: "BrokerA", BrokerSymbol: "EURUSD", Canonical: "EURUSD",
Direction: evaluator.Buy, Lots: decimal.NewFromFloat(0.1), EstPrice: decimal.NewFromFloat(1.05)},
{Broker: "BrokerA", BrokerSymbol: "GBPUSD", Canonical: "GBPUSD",
Direction: evaluator.Sell, Lots: decimal.NewFromFloat(0.1), EstPrice: decimal.NewFromFloat(1.25)},
},
}
eng.PushOpportunityForTest(opp)
ev, err := stream.Recv()
if err != nil {
t.Fatalf("stream.Recv: %v", err)
}
if ev.GetId() != "e2e-opp-1" {
t.Errorf("id = %s, want e2e-opp-1", ev.GetId())
}
if ev.GetOpp() == nil {
t.Fatal("opp is nil")
}
if ev.GetOpp().GetId() == "" {
t.Error("opp.id is empty")
}
if len(ev.GetOpp().GetLegs()) != 2 {
t.Errorf("legs = %d, want 2", len(ev.GetOpp().GetLegs()))
}
if ev.GetAction() != dashpb.OpportunityAction_OPP_ACTION_PUSHED {
t.Errorf("action = %v, want PUSHED", ev.GetAction())
}
}

func TestDashboardE2E_ConfirmOpportunity(t *testing.T) {
symbols := []string{"EURUSD"}
quoteBus := bus.New(symbols)
mockAd := &mockAdapter{broker: "BrokerA"}
pipeline := execute.NewPipeline(execute.PipelineDeps{
Bus:      quoteBus,
Dedup:    execute.NewDedupCache(),
Adapters: map[string]adapter.PlatformAdapter{"BrokerA": mockAd},
})
eng := engine.New(engine.Deps{
Bus:      quoteBus,
Pipeline: pipeline,
})
dashSrv := dashboard.NewServer(dashboard.Deps{
Bus:      quoteBus,
Engine:   eng,
Symbols:  symbols,
})
lis, err := net.Listen("tcp", "127.0.0.1:0")
if err != nil {
t.Fatalf("listen: %v", err)
}
grpcSrv := grpc.NewServer()
dashpb.RegisterDashboardServiceServer(grpcSrv, dashSrv)
go grpcSrv.Serve(lis)
defer grpcSrv.Stop()
conn, err := grpc.DialContext(context.Background(), lis.Addr().String(),
grpc.WithTransportCredentials(insecure.NewCredentials()),
grpc.WithBlock(),
)
if err != nil {
t.Fatalf("dial: %v", err)
}
defer conn.Close()
client := dashpb.NewDashboardServiceClient(conn)
opp := &evaluator.Opportunity{
ID:         "e2e-confirm-1",
Type:       evaluator.OppCrossExchange,
Executable: true,
Status:     evaluator.OppStatusPushed,
ExpiresAt:  time.Now().Add(30 * time.Second),
Legs: []evaluator.OppLeg{
{Broker: "BrokerA", BrokerSymbol: "EURUSD", Direction: evaluator.Buy,
Lots: decimal.NewFromFloat(0.1), EstPrice: decimal.NewFromFloat(1.05)},
{Broker: "BrokerA", BrokerSymbol: "EURUSD", Direction: evaluator.Sell,
Lots: decimal.NewFromFloat(0.1), EstPrice: decimal.NewFromFloat(1.0505)},
},
}
eng.PushOpportunityForTest(opp)
reply, err := client.ConfirmOpportunity(context.Background(), &dashpb.ConfirmRequest{
OpportunityId: "e2e-confirm-1",
})
if err != nil {
t.Fatalf("ConfirmOpportunity: %v", err)
}
if !reply.GetAccepted() {
t.Errorf("accepted=false, reason=%s", reply.GetReason())
}
}
