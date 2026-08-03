package execute

import (
	"testing"
	"time"

	"arb/internal/adapter"
	"github.com/shopspring/decimal"
)

func TestDedupSameClientIDReturnsCached(t *testing.T) {
	d := NewDedupCache()
	r1 := &adapter.OrderResult{
		ClientID: "abc-123",
		Ticket:   100,
		State:    adapter.StateFilled,
	}
	d.Set("abc-123", r1)
	got, ok := d.Get("abc-123")
	if !ok {
		t.Fatal("expected cached result")
	}
	if got.Ticket != 100 {
		t.Errorf("ticket = %d, want 100", got.Ticket)
	}
}

func TestDedupSyncFromOrders(t *testing.T) {
	d := NewDedupCache()
	orders := []adapter.Order{
		{Ticket: 200, Comment: "order-200", State: adapter.StateFilled},
		{Ticket: 201, Comment: "order-201", State: adapter.StateFilled},
	}
	d.SyncFromOrders(orders)
	got, ok := d.Get("order-200")
	if !ok {
		t.Fatal("expected cached result from sync")
	}
	if got.Ticket != 200 {
		t.Errorf("ticket = %d, want 200", got.Ticket)
	}
}

func TestDedupTTLExpiry(t *testing.T) {
	d := &DedupCache{
		cache: make(map[string]*dedupEntry),
		ttl:   50 * time.Millisecond,
	}
	d.Set("expiring", &adapter.OrderResult{Ticket: 300})
	time.Sleep(60 * time.Millisecond)
	_, ok := d.Get("expiring")
	if ok {
		t.Fatal("expected expired entry to be absent")
	}
}

func TestDedupNonExistent(t *testing.T) {
	d := NewDedupCache()
	_, ok := d.Get("nonexistent")
	if ok {
		t.Fatal("expected not found for nonexistent ClientID")
	}
}

func TestDedupVolumeCheck(t *testing.T) {
	d := NewDedupCache()
	r := &adapter.OrderResult{
		ClientID:    "vol-test",
		Ticket:      400,
		State:       adapter.StateFilled,
		Volume:      decimal.NewFromFloat(0.1),
		CloseVolume: decimal.NewFromFloat(0.1),
	}
	d.Set("vol-test", r)
	got, ok := d.Get("vol-test")
	if !ok {
		t.Fatal("expected cached result")
	}
	if !got.IsFullFill() {
		t.Error("expected full fill")
	}
}
