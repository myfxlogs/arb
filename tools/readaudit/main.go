// Command readaudit reads a protobuf length-delimited audit log file
// and prints each AuditEvent in human-readable format.
package main

import (
"bufio"
"encoding/binary"
"fmt"
"io"
"os"

auditpb "arb/proto/gen/audit"
"google.golang.org/protobuf/proto"
)

func main() {
if len(os.Args) < 2 {
fmt.Fprintln(os.Stderr, "usage: readaudit <audit.pb>")
os.Exit(1)
}
f, err := os.Open(os.Args[1])
if err != nil {
fmt.Fprintf(os.Stderr, "open %s: %v\n", os.Args[1], err)
os.Exit(1)
}
defer f.Close()
r := bufio.NewReader(f)
n := 0
for {
msgLen, err := binary.ReadUvarint(r)
if err == io.EOF {
break
}
if err != nil {
fmt.Fprintf(os.Stderr, "read varint: %v\n", err)
os.Exit(1)
}
buf := make([]byte, msgLen)
if _, err := io.ReadFull(r, buf); err != nil {
fmt.Fprintf(os.Stderr, "read message: %v\n", err)
os.Exit(1)
}
ev := &auditpb.AuditEvent{}
if err := proto.Unmarshal(buf, ev); err != nil {
fmt.Fprintf(os.Stderr, "unmarshal event %d: %v\n", n, err)
continue
}
printEvent(n, ev)
n++
}
fmt.Fprintf(os.Stderr, "--- %d events ---\n", n)
}

func printEvent(idx int, ev *auditpb.AuditEvent) {
ts := ""
if ev.GetTimestamp() != nil {
ts = fmt.Sprintf("%v", ev.GetTimestamp().AsTime())
}
fmt.Printf("[%d] ts=%s type=%s opp=%s", idx, ts, ev.GetType().String(), ev.GetOpportunityId())
if ev.GetGrossProfitEst() != "" {
fmt.Printf(" gross=%s", ev.GetGrossProfitEst())
}
if ev.GetNetProfitEst() != "" {
fmt.Printf(" net=%s", ev.GetNetProfitEst())
}
if ev.GetNetBpsEst() != "" {
fmt.Printf(" bps=%s", ev.GetNetBpsEst())
}
if ev.GetLegCount() > 0 {
fmt.Printf(" legs=%d", ev.GetLegCount())
}
if ev.GetDetail() != "" {
fmt.Printf(" detail=%s", ev.GetDetail())
}
if or := ev.GetOrderResult(); or != nil {
for _, leg := range or.GetLegs() {
fmt.Printf("\n  leg: broker=%s sym=%s dir=%s lots=%s est=%s actual=%s ticket=%d",
leg.GetBroker(), leg.GetBrokerSymbol(), leg.GetDirection(),
leg.GetLots(), leg.GetEstPrice(), leg.GetActualPrice(), leg.GetTicket())
}
}
fmt.Println()
}
