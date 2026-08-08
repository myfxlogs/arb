package integration_test

import (
"context"
"os"
"testing"
"time"

"arb/internal/adapter"
"arb/internal/bus"
"arb/internal/store"
)

func TestMT5Connect_RealBroker(t *testing.T) {
if testing.Short() {
t.Skip("needs real MT5 broker in PG")
}
dsn := os.Getenv("ARB_TEST_DSN")
if dsn == "" {
dsn = "postgres://arb:arb@localhost:5433/arb?sslmode=disable"
}
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()
st, err := store.New(ctx, dsn)
if err != nil {
t.Skipf("pg connect: %v", err)
}
accounts, err := st.ListBrokerAccounts(ctx)
if err != nil {
t.Skipf("list broker accounts: %v", err)
}
var acc *store.BrokerAccountRecord
for i := range accounts {
if accounts[i].Platform == 1 {
acc = &accounts[i]
break
}
}
if acc == nil {
t.Skip("no MT5 broker in PG broker_accounts")
}
a := adapter.NewMT5Adapter(acc.Name, acc.Host, acc.Server, acc.Port, acc.Login, acc.Password, 5)
if _, err := a.Connect(ctx); err != nil {
t.Fatalf("connect %s: %v", acc.Name, err)
}
defer a.Stop()
if err := a.Subscribe(ctx, []string{"EURUSD"}); err != nil {
t.Fatalf("subscribe EURUSD: %v", err)
}
quoteBus := bus.New([]string{"EURUSD"})
go a.QuoteStream(ctx, quoteBus)
ch, chCancel := quoteBus.Subscribe("EURUSD")
defer chCancel()
for i := 0; i < 3; i++ {
select {
case q := <-ch:
if q.Bid <= 0 || q.Ask <= 0 {
t.Errorf("quote %d: bid=%f ask=%f, want >0", i, q.Bid, q.Ask)
}
if age := time.Since(q.Time); age > 10*time.Second {
t.Errorf("quote %d: age=%v, want <10s", i, age)
}
case <-time.After(15 * time.Second):
t.Fatalf("timed out waiting for quote %d", i)
}
}
}
