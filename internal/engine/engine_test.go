package engine

import (
	"context"
	"bufio"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"arb/internal/adapter"
	"arb/internal/audit"
	"arb/internal/bus"
	"arb/internal/decimalutil"
	"arb/internal/evaluator"
	"arb/internal/execute"
	"github.com/shopspring/decimal"

	"google.golang.org/protobuf/proto"

	auditpb "arb/proto/gen/audit"
)

type mockPipelineAdapter struct {
broker   string
calls    int
mu       sync.Mutex
failMode bool
}

func (m *mockPipelineAdapter) Connect(context.Context) (string, error) { return "tok", nil }
func (m *mockPipelineAdapter) Disconnect() error                       { return nil }
func (m *mockPipelineAdapter) HealthCheck(context.Context) error       { return nil }
func (m *mockPipelineAdapter) Subscribe(context.Context, []string) error { return nil }
func (m *mockPipelineAdapter) QuoteStream(context.Context, *bus.QuoteBus) {}
func (m *mockPipelineAdapter) AccountSummary(context.Context) (*adapter.Account, error) {
return nil, nil
}
func (m *mockPipelineAdapter) OpenOrders(context.Context) ([]adapter.Order, error) { return nil, nil }
func (m *mockPipelineAdapter) AllSymbols(context.Context) ([]string, error)        { return nil, nil }
func (m *mockPipelineAdapter) SymbolDigits(context.Context, []string) (map[string]int32, error) {
return nil, nil
}
func (m *mockPipelineAdapter) PlaceOrder(ctx context.Context, req adapter.OrderRequest) (*adapter.OrderResult, error) {
m.mu.Lock()
m.calls++
m.mu.Unlock()
if m.failMode {
return &adapter.OrderResult{ClientID: req.ClientID, State: adapter.StateRejected}, nil
}
return &adapter.OrderResult{
ClientID:    req.ClientID,
Ticket:      1,
State:       adapter.StateFilled,
Volume:      req.Volume,
CloseVolume: req.Volume,
}, nil
}
func (m *mockPipelineAdapter) CancelOrder(context.Context, int64) error { return nil }
func (m *mockPipelineAdapter) CloseOrder(context.Context, int64, decimal.Decimal, float64, int32) (*adapter.OrderResult, error) {
return nil, nil
}
func (m *mockPipelineAdapter) OrderHistory(context.Context, time.Time, time.Time) ([]adapter.Order, error) {
return nil, nil
}
func (m *mockPipelineAdapter) Platform() bus.PlatformType            { return bus.PlatformMT5 }
func (m *mockPipelineAdapter) BrokerName() string                    { return m.broker }
func (m *mockPipelineAdapter) SetOnReconnect(func(context.Context) error) {}
func (m *mockPipelineAdapter) Stop()                                      {}

func (m *mockPipelineAdapter) getCalls() int {
m.mu.Lock()
defer m.mu.Unlock()
return m.calls
}

func makeTestEngine(a *mockPipelineAdapter) *Engine {
b := bus.New([]string{"EURUSD", "GBPUSD"})
pipeline := execute.NewPipeline(execute.PipelineDeps{
Bus:      b,
Dedup:    execute.NewDedupCache(),
Adapters: map[string]adapter.PlatformAdapter{a.broker: a},
})
return New(Deps{Bus: b, Pipeline: pipeline})
}

func makeConfirmedOpp() *evaluator.Opportunity {
return &evaluator.Opportunity{
ID:          "test-opp-1",
Status:      evaluator.OppStatusPushed,
Executable:  true,
NotionalUSD: decimal.NewFromFloat(108000),
Legs: []evaluator.OppLeg{
{Broker: "BrokerA", BrokerSymbol: "EURUSD", Direction: evaluator.Buy,
Lots: decimal.NewFromFloat(0.1), EstPrice: decimal.NewFromFloat(1.05)},
{Broker: "BrokerA", BrokerSymbol: "GBPUSD", Direction: evaluator.Sell,
Lots: decimal.NewFromFloat(0.1), EstPrice: decimal.NewFromFloat(1.25)},
},
}
}

func publishQuotes(b *bus.QuoteBus, done <-chan struct{}) {
	b.Publish(bus.Quote{Symbol: "EURUSD", Bid: 1.05, Ask: 1.0505})
	b.Publish(bus.Quote{Symbol: "GBPUSD", Bid: 1.25, Ask: 1.2505})
go func() {
ticker := time.NewTicker(10 * time.Millisecond)
defer ticker.Stop()
for {
select {
case <-done:
return
case <-ticker.C:
b.Publish(bus.Quote{Symbol: "EURUSD", Bid: 1.05, Ask: 1.0505})
b.Publish(bus.Quote{Symbol: "GBPUSD", Bid: 1.25, Ask: 1.2505})
}
}
}()
}

// waitForAction drains events until it finds the target action or times out.
func waitForAction(evCh <-chan OpportunityEvent, action string, timeout time.Duration) error {
deadline := time.After(timeout)
for {
select {
case ev := <-evCh:
if ev.Action == action {
return nil
}
case <-deadline:
return context.DeadlineExceeded
}
}
}

func TestConfirm_RunsPipeline(t *testing.T) {
a := &mockPipelineAdapter{broker: "BrokerA"}
eng := makeTestEngine(a)

done := make(chan struct{})
publishQuotes(eng.deps.Bus, done)
defer close(done)

evCh, cancelEv := eng.Subscribe()
defer cancelEv()

opp := makeConfirmedOpp()
eng.PushOpportunityForTest(opp)

// Drain PUSHED event
<-evCh

result, msg := eng.ConfirmOpportunity("test-opp-1")
if result == nil {
t.Fatalf("ConfirmOpportunity returned nil: %s", msg)
}
eng.mu.RLock()
confirmedStatus := result.Status
eng.mu.RUnlock()
if confirmedStatus != evaluator.OppStatusConfirmed {
t.Errorf("Status = %v, want Confirmed", result.Status)
}

if err := waitForAction(evCh, "FILLED", 10*time.Second); err != nil {
t.Fatal("timed out waiting for FILLED event")
}

eng.mu.RLock()
finalStatus := opp.Status
eng.mu.RUnlock()
if finalStatus != evaluator.OppStatusFilled {
t.Errorf("final Status = %v, want Filled", finalStatus)
}
if a.getCalls() != 2 {
t.Errorf("PlaceOrder calls = %d, want 2", a.getCalls())
}
}

func TestConfirm_PipelineError(t *testing.T) {
a := &mockPipelineAdapter{broker: "BrokerA", failMode: true}
eng := makeTestEngine(a)

done := make(chan struct{})
publishQuotes(eng.deps.Bus, done)
defer close(done)

evCh, cancelEv := eng.Subscribe()
defer cancelEv()

opp := makeConfirmedOpp()
eng.PushOpportunityForTest(opp)

// Drain PUSHED event
<-evCh

_, msg := eng.ConfirmOpportunity("test-opp-1")
if msg != "" {
t.Fatalf("unexpected error: %s", msg)
}

if err := waitForAction(evCh, "FAILED", 10*time.Second); err != nil {
t.Fatal("timed out waiting for FAILED event")
}

eng.mu.RLock()
finalStatus := opp.Status
eng.mu.RUnlock()
if finalStatus != evaluator.OppStatusFailed {
t.Errorf("final Status = %v, want Failed", finalStatus)
}
}

func TestConfirm_NotFound(t *testing.T) {
a := &mockPipelineAdapter{broker: "BrokerA"}
eng := makeTestEngine(a)

result, msg := eng.ConfirmOpportunity("nonexistent")
if result != nil {
t.Error("expected nil for nonexistent opportunity")
}
if msg != "opportunity not found" {
t.Errorf("msg = %q, want 'opportunity not found'", msg)
}
}

func TestConfirm_NotPushedState(t *testing.T) {
a := &mockPipelineAdapter{broker: "BrokerA"}
eng := makeTestEngine(a)

opp := makeConfirmedOpp()
opp.Status = evaluator.OppStatusExpired
eng.PushOpportunityForTest(opp)

result, msg := eng.ConfirmOpportunity("test-opp-1")
if result != nil {
t.Error("expected nil for non-Pushed opportunity")
}
if msg != "opportunity not in Pushed state" {
t.Errorf("msg = %q", msg)
}
}

func TestNotional_FromEvaluator(t *testing.T) {
opp := makeConfirmedOpp()
pipeOpp := toPipelineOpp(opp)

if pipeOpp.NotionalUSD != 108000 {
t.Errorf("NotionalUSD = %v, want 108000", pipeOpp.NotionalUSD)
}
if pipeOpp.Notional() != 108000 {
t.Errorf("Notional() = %v, want 108000", pipeOpp.Notional())
}
}

func TestToPipelineOpp_LegMapping(t *testing.T) {
opp := makeConfirmedOpp()
pipeOpp := toPipelineOpp(opp)

if len(pipeOpp.Legs) != 2 {
t.Fatalf("Legs = %d, want 2", len(pipeOpp.Legs))
}
if pipeOpp.Legs[0].Operation != adapter.OpBuy {
t.Errorf("leg0 Operation = %v, want OpBuy", pipeOpp.Legs[0].Operation)
}
if pipeOpp.Legs[1].Operation != adapter.OpSell {
t.Errorf("leg1 Operation = %v, want OpSell", pipeOpp.Legs[1].Operation)
}
if pipeOpp.Legs[0].Symbol != "EURUSD" {
t.Errorf("leg0 Symbol = %q, want EURUSD", pipeOpp.Legs[0].Symbol)
}
if decimalutil.ToFloat64(opp.Legs[0].Lots) != pipeOpp.Legs[0].Volume {
t.Errorf("leg0 Volume mismatch")
}
}

func TestConfirm_NotExecutable(t *testing.T) {
a := &mockPipelineAdapter{broker: "BrokerA"}
eng := makeTestEngine(a)

opp := makeConfirmedOpp()
opp.Executable = false
eng.PushOpportunityForTest(opp)

result, msg := eng.ConfirmOpportunity("test-opp-1")
if result != nil {
t.Error("expected nil for non-executable opportunity")
}
if msg != "opportunity not executable" {
t.Errorf("msg = %q, want 'opportunity not executable'", msg)
}
}

// readAuditEvents reads length-delimited protobuf AuditEvents from a file.
func readAuditEvents(t *testing.T, path string) []*auditpb.AuditEvent {
t.Helper()
f, err := os.Open(path)
if err != nil {
t.Fatalf("open audit: %v", err)
}
defer f.Close()

r := bufio.NewReader(f)
var events []*auditpb.AuditEvent
for {
b, err := r.ReadByte()
if err == io.EOF {
break
}
if err != nil {
t.Fatalf("read byte: %v", err)
}
size := uint64(b)
if size&0x80 != 0 {
size &= 0x7f
for shift := uint(7); ; shift += 7 {
b, err = r.ReadByte()
if err != nil {
t.Fatalf("read varint: %v", err)
}
size |= uint64(b&0x7f) << shift
if b&0x80 == 0 {
break
}
}
}
body := make([]byte, size)
if _, err := io.ReadFull(r, body); err != nil {
t.Fatalf("read body: %v", err)
}
var ev auditpb.AuditEvent
if err := proto.Unmarshal(body, &ev); err != nil {
t.Fatalf("unmarshal: %v", err)
}
events = append(events, &ev)
}
return events
}

func TestAuditLog_Events(t *testing.T) {
a := &mockPipelineAdapter{broker: "BrokerA"}
dir := t.TempDir()
auditPath := filepath.Join(dir, "audit.pb")
al, err := audit.NewLogger(auditPath)
if err != nil {
t.Fatalf("NewLogger: %v", err)
}
defer al.Close()

b := bus.New([]string{"EURUSD", "GBPUSD"})
pipeline := execute.NewPipeline(execute.PipelineDeps{
Bus: b, Dedup: execute.NewDedupCache(),
Adapters: map[string]adapter.PlatformAdapter{a.broker: a},
})
eng := New(Deps{Bus: b, Pipeline: pipeline, Audit: al})

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
t.Fatalf("unexpected error: %s", msg)
}

if err := waitForAction(evCh, "FILLED", 10*time.Second); err != nil {
t.Fatal("timed out waiting for FILLED")
}

// Read back protobuf audit events
events := readAuditEvents(t, auditPath)
want := map[auditpb.EventType]bool{
auditpb.EventType_EVENT_TYPE_CONFIRMED: false,
auditpb.EventType_EVENT_TYPE_FILLED:    false,
}
for _, ev := range events {
if _, ok := want[ev.Type]; ok {
want[ev.Type] = true
}
}
for k, found := range want {
if !found {
t.Errorf("audit log missing event %s", k)
}
}
}

