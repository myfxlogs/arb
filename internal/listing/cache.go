package listing

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Fetcher fetches a Listing for one broker symbol. MT5Adapter implements this.
type Fetcher interface {
	BrokerName() string
	Listing(ctx context.Context, brokerSymbol string) (*Listing, error)
}

// Cache stores Listings keyed by broker/brokerSymbol. It is warm-path
// (populated at startup, refreshed daily), so sync.RWMutex is acceptable.
type Cache struct {
	mu    sync.RWMutex
	items map[string]*Listing // key = broker + "/" + brokerSymbol
	srcs  map[string]refreshSrc
}

type refreshSrc struct {
	fetcher Fetcher
	symbols []string
}

// NewCache creates an empty Listing cache.
func NewCache() *Cache {
	return &Cache{
		items: make(map[string]*Listing),
		srcs:  make(map[string]refreshSrc),
	}
}

func cacheKey(broker, brokerSymbol string) string {
	return broker + "/" + brokerSymbol
}

// Get returns a cached Listing, or nil if not present.
func (c *Cache) Get(broker, brokerSymbol string) (*Listing, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	l, ok := c.items[cacheKey(broker, brokerSymbol)]
	return l, ok
}

// All returns all cached Listings (snapshot).
func (c *Cache) All() []*Listing {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]*Listing, 0, len(c.items))
	for _, l := range c.items {
		out = append(out, l)
	}
	return out
}

// Populate fetches Listings for all broker symbols and stores them.
// brokerSymbols maps broker name → list of raw broker symbols to fetch.
// Fetchers must be keyed by broker name.
func (c *Cache) Populate(ctx context.Context, fetchers []Fetcher, brokerSymbols map[string][]string) error {
	for _, f := range fetchers {
		syms := brokerSymbols[f.BrokerName()]
		c.mu.Lock()
		c.srcs[f.BrokerName()] = refreshSrc{fetcher: f, symbols: syms}
		c.mu.Unlock()
		for _, sym := range syms {
			l, err := f.Listing(ctx, sym)
			if err != nil {
				slog.Warn("listing cache: fetch failed", "broker", f.BrokerName(), "symbol", sym, "error", err)
				continue
			}
			c.mu.Lock()
			c.items[cacheKey(l.Broker, l.BrokerSymbol)] = l
			c.mu.Unlock()
			slog.Info("listing cache: stored", "broker", l.Broker, "symbol", l.BrokerSymbol)
		}
	}
	return nil
}

// RunDailyRefresh starts a goroutine that re-fetches all Listings once per
// day to pick up swap rate changes. Blocks until ctx is cancelled.
func (c *Cache) RunDailyRefresh(ctx context.Context) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.refreshAll(ctx)
		}
	}
}

func (c *Cache) refreshAll(ctx context.Context) {
	c.mu.RLock()
	srcs := c.srcs
	c.mu.RUnlock()
	for broker, src := range srcs {
		for _, sym := range src.symbols {
			l, err := src.fetcher.Listing(ctx, sym)
			if err != nil {
				slog.Warn("listing cache: refresh failed", "broker", broker, "symbol", sym, "error", err)
				continue
			}
			c.mu.Lock()
			c.items[cacheKey(l.Broker, l.BrokerSymbol)] = l
			c.mu.Unlock()
		}
	}
	slog.Info("listing cache: daily refresh complete")
}
