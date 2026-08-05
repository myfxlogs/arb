package dashboard

import (
	"sync"

	"arb/internal/bus"
)

// quoteCache stores the latest quote per broker per symbol.
// It is fed by a background goroutine that subscribes to all symbols on QuoteBus.
type quoteCache struct {
	mu      sync.RWMutex
	quotes  map[string]map[string]bus.Quote // broker -> symbol -> Quote
	cancels []func()                    // feeder unsubscribe funcs
}

func newQuoteCache() *quoteCache {
	return &quoteCache{quotes: make(map[string]map[string]bus.Quote)}
}

// update stores the latest quote for a broker+symbol.
func (c *quoteCache) update(q bus.Quote) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.quotes[q.Broker]; !ok {
		c.quotes[q.Broker] = make(map[string]bus.Quote)
	}
	c.quotes[q.Broker][q.Symbol] = q
}

// snapshot returns a copy of all quotes organized by broker -> symbol -> Quote.
func (c *quoteCache) snapshot() map[string]map[string]bus.Quote {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make(map[string]map[string]bus.Quote, len(c.quotes))
	for broker, syms := range c.quotes {
		result[broker] = make(map[string]bus.Quote, len(syms))
		for sym, q := range syms {
			result[broker][sym] = q
		}
	}
	return result
}

// startFeeder starts background goroutines that subscribe to all symbols
// on the QuoteBus and feeds quotes into the cache. Call Stop to clean up.
func (c *quoteCache) startFeeder(bus *bus.QuoteBus, symbols []string) {
	for _, sym := range symbols {
		go c.feedSymbol(bus, sym)
	}
}

func (c *quoteCache) feedSymbol(bus *bus.QuoteBus, symbol string) {
	ch, cancel := bus.Subscribe(symbol)
	c.mu.Lock()
	c.cancels = append(c.cancels, cancel)
	c.mu.Unlock()
	for q := range ch {
		c.update(q)
	}
}

// Stop unsubscribes all feeder goroutines, allowing them to exit.
func (c *quoteCache) Stop() {
	c.mu.Lock()
	for _, cancel := range c.cancels {
		cancel()
	}
	c.cancels = nil
	c.mu.Unlock()
}
