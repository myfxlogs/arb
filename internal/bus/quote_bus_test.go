package bus

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestSinglePubSingleSub(t *testing.T) {
	b := New([]string{"EURUSD"})
	ch, cancel := b.Subscribe("EURUSD")
	defer cancel()
	b.Publish(Quote{Symbol: "EURUSD", Bid: 1.05, Ask: 1.06})
	select {
	case q := <-ch:
		if q.Bid != 1.05 {
			t.Fatalf("got bid %v, want 1.05", q.Bid)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for quote")
	}
}

func TestSinglePubMultiSub(t *testing.T) {
	b := New([]string{"EURUSD"})
	ch1, c1 := b.Subscribe("EURUSD")
	ch2, c2 := b.Subscribe("EURUSD")
	defer c1()
	defer c2()
	b.Publish(Quote{Symbol: "EURUSD", Bid: 1.05})
	for i, ch := range []<-chan Quote{ch1, ch2} {
		select {
		case q := <-ch:
			if q.Bid != 1.05 {
				t.Fatalf("sub %d: got bid %v, want 1.05", i, q.Bid)
			}
		case <-time.After(time.Second):
			t.Fatalf("sub %d: timeout", i)
		}
	}
}

func TestDifferentSymbolsNoInterference(t *testing.T) {
	b := New([]string{"EURUSD", "GBPUSD"})
	chE, cE := b.Subscribe("EURUSD")
	chG, cG := b.Subscribe("GBPUSD")
	defer cE()
	defer cG()
	b.Publish(Quote{Symbol: "EURUSD", Bid: 1.05})
	select {
	case <-chG:
		t.Fatal("GBPUSD received EURUSD quote")
	case <-time.After(50 * time.Millisecond):
	}
	select {
	case q := <-chE:
		if q.Bid != 1.05 {
			t.Fatalf("got bid %v, want 1.05", q.Bid)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

func TestDrainThenReplace(t *testing.T) {
	b := New([]string{"EURUSD"})
	ch, cancel := b.Subscribe("EURUSD")
	defer cancel()
	b.Publish(Quote{Symbol: "EURUSD", Bid: 1.01})
	b.Publish(Quote{Symbol: "EURUSD", Bid: 1.02})
	b.Publish(Quote{Symbol: "EURUSD", Bid: 1.03})
	select {
	case q := <-ch:
		if q.Bid != 1.03 {
			t.Fatalf("got bid %v, want 1.03 (latest)", q.Bid)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

func TestSlowConsumerNoBlock(t *testing.T) {
	b := New([]string{"EURUSD"})
	_, cancel := b.Subscribe("EURUSD")
	defer cancel()
	done := make(chan struct{})
	go func() {
		b.Publish(Quote{Symbol: "EURUSD", Bid: 1.01})
		b.Publish(Quote{Symbol: "EURUSD", Bid: 1.02})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Publish blocked on slow consumer")
	}
}

func TestConcurrentPublishNoRace(t *testing.T) {
	b := New(nil)
	var wg sync.WaitGroup
	symbols := []string{"EURUSD", "GBPUSD", "USDJPY", "AUDUSD", "USDCAD",
		"EURGBP", "EURJPY", "GBPJPY", "AUDJPY", "NZDUSD"}
	for _, s := range symbols {
		ch, cancel := b.Subscribe(s)
		defer cancel()
		_ = ch
	}
	for _, s := range symbols {
		wg.Add(1)
		go func(sym string) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				b.Publish(Quote{Symbol: sym, Bid: float64(i) / 100, Ask: float64(i)/100 + 0.001})
			}
		}(s)
	}
	wg.Wait()
}

func TestUnsubscribeNoMoreMessages(t *testing.T) {
	b := New([]string{"EURUSD"})
	ch, cancel := b.Subscribe("EURUSD")
	cancel()
	b.Publish(Quote{Symbol: "EURUSD", Bid: 1.05})
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("received quote after unsubscribe")
		}
	case <-time.After(50 * time.Millisecond):
	}
}

func TestSnapshotTimeout(t *testing.T) {
	b := New([]string{"EURUSD"})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	result := b.Snapshot(ctx, []string{"EURUSD"})
	if len(result) != 0 {
		t.Fatalf("expected empty result, got %d items", len(result))
	}
}
