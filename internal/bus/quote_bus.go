package bus

import (
	"context"
	"sync"
)

// QuoteBus is the pub/sub hub for market quotes.
// Subscribers get a cap=1 channel per symbol; Publish uses drain-then-replace
// to always keep the latest tick, dropping stale data for slow consumers.
type QuoteBus struct {
	mu          sync.RWMutex
	subscribers map[string][]chan Quote
}

// New creates a QuoteBus pre-allocated for the given symbols.
func New(symbols []string) *QuoteBus {
	subs := make(map[string][]chan Quote, len(symbols))
	for _, s := range symbols {
		subs[s] = nil
	}
	return &QuoteBus{subscribers: subs}
}

// Subscribe returns a channel that receives quotes for the given symbol.
// The returned cancel func unsubscribes and closes the channel.
func (b *QuoteBus) Subscribe(symbol string) (<-chan Quote, func()) {
	ch := make(chan Quote, 1)
	b.mu.Lock()
	b.subscribers[symbol] = append(b.subscribers[symbol], ch)
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		subs := b.subscribers[symbol]
		for i, c := range subs {
			if c == ch {
				b.subscribers[symbol] = append(subs[:i], subs[i+1:]...)
				break
			}
		}
		b.mu.Unlock()
	}
}

// Publish sends a quote to all subscribers of the quote's symbol.
// Drain-then-replace: if the channel is full, drop the oldest and write the newest.
func (b *QuoteBus) Publish(q Quote) {
	b.mu.RLock()
	chs := b.subscribers[q.Symbol]
	b.mu.RUnlock()
	for _, ch := range chs {
		select {
		case ch <- q:
		default:
			select { case <-ch: default: }
			select { case ch <- q: default: }
		}
	}
}

// Snapshot returns the latest quote for each requested symbol.
// If a symbol has no pending quote, it is omitted from the result.
// Respects ctx cancellation for timeout.
func (b *QuoteBus) Snapshot(ctx context.Context, symbols []string) map[string]Quote {
	result := make(map[string]Quote, len(symbols))
	for _, sym := range symbols {
		ch, cancel := b.Subscribe(sym)
		select {
		case q := <-ch:
			result[sym] = q
		case <-ctx.Done():
			cancel()
			return result
		}
		cancel()
	}
	return result
}
