package audit

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	auditpb "arb/proto/gen/audit"
)

func TestAuditLog_WriteRead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.pb")

	l, err := NewLogger(path)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}

	ev1 := &auditpb.AuditEvent{
		Timestamp:     timestamppb.Now(),
		Type:          auditpb.EventType_EVENT_TYPE_DETECTED,
		OpportunityId: "opp-1",
		NetBpsEst:     "3.5",
		GrossProfitEst: "10.0",
	}
	ev2 := &auditpb.AuditEvent{
		Timestamp:     timestamppb.Now(),
		Type:          auditpb.EventType_EVENT_TYPE_FILLED,
		OpportunityId: "opp-1",
		NetBpsEst:     "3.5",
		OrderResult: &auditpb.OrderResult{
			TotalSwap:       "-0.50",
			TotalCommission: "1.00",
			ActualNetProfit: "8.50",
		},
	}

	if err := l.Log(ev1); err != nil {
		t.Fatalf("Log ev1: %v", err)
	}
	if err := l.Log(ev2); err != nil {
		t.Fatalf("Log ev2: %v", err)
	}
	l.Close()

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open file: %v", err)
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

	if len(events) != 2 {
		t.Fatalf("events = %d, want 2", len(events))
	}
	if events[0].Type != auditpb.EventType_EVENT_TYPE_DETECTED {
		t.Errorf("ev0 Type = %v, want DETECTED", events[0].Type)
	}
	if events[0].OpportunityId != "opp-1" {
		t.Errorf("ev0 OppID = %s, want opp-1", events[0].OpportunityId)
	}
	if events[1].Type != auditpb.EventType_EVENT_TYPE_FILLED {
		t.Errorf("ev1 Type = %v, want FILLED", events[1].Type)
	}
	if events[1].OrderResult == nil {
		t.Fatal("ev1 OrderResult is nil")
	}
	if events[1].OrderResult.ActualNetProfit != "8.50" {
		t.Errorf("ev1 ActualNetProfit = %s, want 8.50", events[1].OrderResult.ActualNetProfit)
	}
}
