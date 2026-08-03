package execute

import (
	"sync"
	"time"

	"arb/internal/adapter"
)

// DedupCache prevents duplicate order submissions by ClientID.
// Entries expire after ttl to bound memory usage.
type DedupCache struct {
	cache map[string]*dedupEntry
	mu    sync.RWMutex
	ttl   time.Duration
}

type dedupEntry struct {
	result  *adapter.OrderResult
	expires time.Time
}

// NewDedupCache creates a DedupCache with a background cleanup goroutine.
func NewDedupCache() *DedupCache {
	d := &DedupCache{
		cache: make(map[string]*dedupEntry),
		ttl:   1 * time.Hour,
	}
	go d.cleanupLoop(1 * time.Hour)
	return d
}

// Get returns the cached OrderResult for a ClientID, or nil if not found/expired.
func (d *DedupCache) Get(clientID string) (*adapter.OrderResult, bool) {
	d.mu.RLock()
	e, ok := d.cache[clientID]
	d.mu.RUnlock()
	if !ok || time.Now().After(e.expires) {
		return nil, false
	}
	return e.result, true
}

// Set stores an OrderResult for a ClientID.
func (d *DedupCache) Set(clientID string, r *adapter.OrderResult) {
	d.mu.Lock()
	d.cache[clientID] = &dedupEntry{result: r, expires: time.Now().Add(d.ttl)}
	d.mu.Unlock()
}

// SyncFromOrders rebuilds the cache from open orders (e.g. after reconnect).
func (d *DedupCache) SyncFromOrders(orders []adapter.Order) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, o := range orders {
		if o.Comment != "" {
			d.cache[o.Comment] = &dedupEntry{
				result:  &adapter.OrderResult{Ticket: o.Ticket, State: o.State},
				expires: time.Now().Add(d.ttl),
			}
		}
	}
}

func (d *DedupCache) cleanupLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		d.mu.Lock()
		now := time.Now()
		for k, e := range d.cache {
			if now.After(e.expires) {
				delete(d.cache, k)
			}
		}
		d.mu.Unlock()
	}
}
