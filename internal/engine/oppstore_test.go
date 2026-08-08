package engine

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"arb/internal/adapter"
	"arb/internal/audit"
	"arb/internal/bus"
	"arb/internal/evaluator"
	"arb/internal/execute"
	"arb/internal/store"

	auditpb "arb/proto/gen/audit"
)

type mockOppWriter struct {
mu      sync.Mutex
writes  []string
updates []string
}

func (m *mockOppWriter) WriteOpportunity(ctx context.Context, r store.OpportunityRecord) error {
m.mu.Lock()
defer m.mu.Unlock()
m.writes = append(m.writes, r.ID)
return nil
}

func (m *mockOppWriter) UpdateOpportunityStatus(ctx context.Context, id, status string) error {
m.mu.Lock()
defer m.mu.Unlock()
m.updates = append(m.updates, id+":"+status)
return nil
}

func (m *mockOppWriter) getWrites() []string {
m.mu.Lock()
defer m.mu.Unlock()
return append([]string(nil), m.writes...)
}

func (m *mockOppWriter) getUpdates() []string {
m.mu.Lock()
defer m.mu.Unlock()
return append([]string(nil), m.updates...)
}

func TestAuditLog_DetectedType(t *testing.T) {
dir := t.TempDir()
auditPath := filepath.Join(dir, "audit.pb")
al, err := audit.NewLogger(auditPath)
if err != nil {
t.Fatalf("NewLogger: %v", err)
}
defer al.Close()

b := bus.New([]string{"EURUSD"})
pipeline := execute.NewPipeline(execute.PipelineDeps{
Bus: b, Dedup: execute.NewDedupCache(),
Adapters: map[string]adapter.PlatformAdapter{"BrokerA": &mockPipelineAdapter{broker: "BrokerA"}},
})
eng := New(Deps{Bus: b, Pipeline: pipeline, Audit: al})

opp := makeConfirmedOpp()
opp.Executable = false
eng.auditLog(auditpb.EventType_EVENT_TYPE_DETECTED, opp)

al.Close()
events := readAuditEvents(t, auditPath)
if len(events) != 1 {
t.Fatalf("events = %d, want 1", len(events))
}
if events[0].Type != auditpb.EventType_EVENT_TYPE_DETECTED {
t.Errorf("type = %v, want DETECTED", events[0].Type)
}
if events[0].OpportunityId != "test-opp-1" {
t.Errorf("opp_id = %s, want test-opp-1", events[0].OpportunityId)
}
}

func TestOppStore_MockWrite(t *testing.T) {
a := &mockPipelineAdapter{broker: "BrokerA"}
mockStore := &mockOppWriter{}
b := bus.New([]string{"EURUSD", "GBPUSD"})
pipeline := execute.NewPipeline(execute.PipelineDeps{
Bus: b, Dedup: execute.NewDedupCache(),
Adapters: map[string]adapter.PlatformAdapter{a.broker: a},
})
eng := New(Deps{Bus: b, Pipeline: pipeline, OppStore: mockStore})

done := make(chan struct{})
publishQuotes(b, done)
defer close(done)

evCh, cancelEv := eng.Subscribe()
defer cancelEv()

opp := makeConfirmedOpp()
eng.PushOpportunityForTest(opp)
<-evCh // drain PUSHED

_, msg := eng.ConfirmOpportunity("test-opp-1")
if msg != "" {
t.Fatalf("ConfirmOpportunity: %s", msg)
}

if err := waitForAction(evCh, "FILLED", 10*time.Second); err != nil {
t.Fatal("timed out waiting for FILLED")
}

updates := mockStore.getUpdates()
foundFilled := false
for _, u := range updates {
if u == "test-opp-1:FILLED" {
foundFilled = true
}
}
if !foundFilled {
t.Errorf("expected FILLED status update, got %v", updates)
}
}

func TestOppStore_NilNoPanic(t *testing.T) {
a := &mockPipelineAdapter{broker: "BrokerA"}
b := bus.New([]string{"EURUSD", "GBPUSD"})
pipeline := execute.NewPipeline(execute.PipelineDeps{
Bus: b, Dedup: execute.NewDedupCache(),
Adapters: map[string]adapter.PlatformAdapter{a.broker: a},
})
eng := New(Deps{Bus: b, Pipeline: pipeline})

opp := makeConfirmedOpp()
eng.writeOpp(opp)
eng.updateOppStatus(opp.ID, "FILLED")
}

func TestExpireOld_RaceFree(t *testing.T) {
a := &mockPipelineAdapter{broker: "BrokerA"}
b := bus.New([]string{"EURUSD", "GBPUSD"})
pipeline := execute.NewPipeline(execute.PipelineDeps{
Bus: b, Dedup: execute.NewDedupCache(),
Adapters: map[string]adapter.PlatformAdapter{a.broker: a},
})
eng := New(Deps{Bus: b, Pipeline: pipeline})

opp := makeConfirmedOpp()
opp.ExpiresAt = time.Now().Add(-1 * time.Second)
eng.PushOpportunityForTest(opp)

var wg sync.WaitGroup
for i := 0; i < 10; i++ {
wg.Add(1)
go func() {
defer wg.Done()
eng.expireOld(context.Background())
}()
}
for i := 0; i < 5; i++ {
wg.Add(1)
go func() {
defer wg.Done()
eng.ConfirmOpportunity("test-opp-1")
}()
}
wg.Wait()
}

func TestToOppRecord_Conversion(t *testing.T) {
opp := makeConfirmedOpp()
opp.Type = evaluator.OppCrossExchange
r := toOppRecord(opp)

if r.ID != "test-opp-1" {
t.Errorf("ID = %s, want test-opp-1", r.ID)
}
if r.Type != "CROSS_EXCHANGE" {
t.Errorf("Type = %s, want CROSS_EXCHANGE", r.Type)
}
if r.Status != "PUSHED" {
t.Errorf("Status = %s, want PUSHED", r.Status)
}
if r.Legs == "" || r.Legs == "[]" {
t.Errorf("Legs = %s, want non-empty JSON", r.Legs)
}
}

func TestOppTypeStatusString(t *testing.T) {
if oppTypeString(evaluator.OppCrossExchange) != "CROSS_EXCHANGE" {
t.Error("CrossExchange mismatch")
}
if oppTypeString(evaluator.OppCarry) != "CARRY" {
t.Error("Carry mismatch")
}
if oppTypeString(evaluator.OppTriangular) != "TRIANGULAR" {
t.Error("Triangular mismatch")
}
if oppStatusString(evaluator.OppStatusPushed) != "PUSHED" {
t.Error("Pushed mismatch")
}
if oppStatusString(evaluator.OppStatusFilled) != "FILLED" {
t.Error("Filled mismatch")
}
if oppStatusString(evaluator.OppStatusExpired) != "EXPIRED" {
t.Error("Expired mismatch")
}
if dirString(evaluator.Buy) != "BUY" {
t.Error("Buy mismatch")
}
if dirString(evaluator.Sell) != "SELL" {
t.Error("Sell mismatch")
}
}
