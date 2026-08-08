package store

import (
	"testing"
	"time"
)

func TestTickRecordConstruction(t *testing.T) {
	now := time.Now()
	tr := TickRecord{
		Time:   now,
		Broker: "BrokerA",
		Symbol: "EURUSD",
		Bid:    1.05,
		Ask:    1.06,
	}
	if tr.Broker != "BrokerA" || tr.Symbol != "EURUSD" {
		t.Errorf("unexpected fields: %+v", tr)
	}
	if tr.Bid >= tr.Ask {
		t.Error("bid should be < ask")
	}
}

func TestSignalRecordConstruction(t *testing.T) {
	sr := SignalRecord{
		ID:        "sig-001",
		Ts:        time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC),
		Strategy:  "triangular",
		Legs:      "leg1,leg2,leg3",
		GrossBps:  5.2,
		NetBps:    3.1,
		Executed:  false,
		Dismissed: false,
	}
	if sr.ID != "sig-001" {
		t.Errorf("ID = %s, want sig-001", sr.ID)
	}
	if sr.NetBps != 3.1 {
		t.Errorf("NetBps = %v, want 3.1", sr.NetBps)
	}
}

func TestOrderRecordConstruction(t *testing.T) {
	ticket := int64(12345)
	errMsg := "requote"
	or := OrderRecord{
		ClientID: "client-001",
		Broker:   "BrokerA",
		Symbol:   "EURUSD",
		Side:     "buy",
		Volume:   0.1,
		Price:    1.05,
		Ticket:   &ticket,
		Status:   "filled",
		Error:    &errMsg,
	}
	if or.ClientID != "client-001" {
		t.Errorf("ClientID = %s", or.ClientID)
	}
	if or.Ticket == nil || *or.Ticket != 12345 {
		t.Error("Ticket mismatch")
	}
}

func TestDailySummaryConstruction(t *testing.T) {
	ds := DailySummary{
		Day:        time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC),
		TotalPnL:   150.50,
		TradeCount: 10,
		WinCount:   7,
	}
	if ds.TradeCount != 10 || ds.WinCount != 7 {
		t.Errorf("counts mismatch: %+v", ds)
	}
}

func TestAuditEntryConstruction(t *testing.T) {
	ae := AuditEntry{
		Timestamp: time.Now(),
		EventType: "order_placed",
		Broker:    "BrokerA",
		Detail:    "client-001 EURUSD buy 0.1 @ 1.05",
	}
	if ae.EventType != "order_placed" {
		t.Errorf("EventType = %s", ae.EventType)
	}
}

func TestOpportunityRecordConstruction(t *testing.T) {
r := OpportunityRecord{
ID:             "opp-001",
Type:           "CROSS_EXCHANGE",
Status:         "PUSHED",
Legs:           `[{"broker":"BrokerA","symbol":"EURUSD"}]`,
GrossProfit:    "10.00000000",
NetProfit:      "8.00000000",
NetBps:         "3.50000000",
QuoteTime:      time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC),
ExpiresAt:      time.Date(2026, 8, 8, 12, 0, 5, 0, time.UTC),
Confidence:     0.95,
}
if r.ID != "opp-001" {
t.Errorf("ID = %s, want opp-001", r.ID)
}
if r.Type != "CROSS_EXCHANGE" {
t.Errorf("Type = %s, want CROSS_EXCHANGE", r.Type)
}
if r.NetProfit != "8.00000000" {
t.Errorf("NetProfit = %s", r.NetProfit)
}
}

func TestMarshalLegs(t *testing.T) {
legs := []map[string]any{
{"broker": "BrokerA", "symbol": "EURUSD", "direction": "Buy"},
{"broker": "BrokerB", "symbol": "EURUSD", "direction": "Sell"},
}
s, err := MarshalLegs(legs)
if err != nil {
t.Fatalf("MarshalLegs: %v", err)
}
if s == "" {
t.Error("expected non-empty JSON")
}
}
