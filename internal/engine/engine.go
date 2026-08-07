package engine

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"arb/internal/bus"
	"arb/internal/detector"
	"arb/internal/evaluator"
	"arb/internal/listing"
)

// SymMapProvider returns the current symbol_map (broker → brokerSymbol → canonical).
// Implemented by store.Store.LoadSymbolMap.
type SymMapProvider interface {
	SymMap(ctx context.Context) (map[string]map[string]string, error)
}

// Deps holds the engine's read-only dependencies.
type Deps struct {
	Bus       *bus.QuoteBus
	Cache     *listing.Cache
	SymMap    SymMapProvider
	Evaluator *evaluator.Evaluator
	Detectors []detector.Detector
	Throttle  time.Duration // min interval between scans (default 100ms)
	Symbols   []string      // all broker symbols to snapshot
}

// Engine runs the scan loop: Snapshot → CanonicalIndex → Detect → Evaluate → push.
type Engine struct {
	deps Deps

	mu   sync.RWMutex
	opp  map[string]*evaluator.Opportunity // active opportunities by ID
	sub  map[chan OpportunityEvent]struct{} // subscribers for opportunity events
}

// OpportunityEvent is pushed to subscribers when an opportunity is created or updated.
type OpportunityEvent struct {
	Opp    *evaluator.Opportunity
	Action string // "PUSHED" | "EXPIRED"
	Reason string
}

// New creates an Engine.
func New(deps Deps) *Engine {
	return &Engine{
		deps: deps,
		opp:  make(map[string]*evaluator.Opportunity),
		sub:  make(map[chan OpportunityEvent]struct{}),
	}
}

// Subscribe returns a channel that receives opportunity events.
// The caller must call cancel to unsubscribe.
func (e *Engine) Subscribe() (<-chan OpportunityEvent, func()) {
	ch := make(chan OpportunityEvent, 16)
	e.mu.Lock()
	e.sub[ch] = struct{}{}
	e.mu.Unlock()
	return ch, func() {
		e.mu.Lock()
		delete(e.sub, ch)
		e.mu.Unlock()
		close(ch)
	}
}

// GetOpportunity returns an active opportunity by ID, or nil if not found.
func (e *Engine) GetOpportunity(id string) *evaluator.Opportunity {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.opp[id]
}

// ConfirmOpportunity transitions an opportunity from Pushed to Confirmed.
// Returns the opportunity if confirmed, nil if not found or not in Pushed state.
func (e *Engine) ConfirmOpportunity(id string) (*evaluator.Opportunity, string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	opp, ok := e.opp[id]
	if !ok {
		return nil, "opportunity not found"
	}
	if opp.Status != evaluator.OppStatusPushed {
		return nil, "opportunity not in Pushed state"
	}
	if !opp.Executable {
		return nil, "opportunity not executable"
	}
	opp.Status = evaluator.OppStatusConfirmed
	return opp, ""
}

// Run starts the event-driven scan loop. Subscribes to QuoteBus for all
// symbols; on any quote arrival triggers a scan, throttled to Throttle
// interval to avoid scanning on every single tick. Blocks until ctx cancelled.
func (e *Engine) Run(ctx context.Context) {
	throttle := e.deps.Throttle
	if throttle <= 0 {
		throttle = 100 * time.Millisecond
	}

	// Subscribe to all symbols — one merged channel.
	quoteCh, cancelSub := e.mergeSubscribe(ctx, e.deps.Symbols)
	defer cancelSub()

	var throttleTimer <-chan time.Time

	for {
		select {
		case <-ctx.Done():
			return
		case <-quoteCh:
			// Quote arrived — if not throttled, scan now; otherwise drain.
			if throttleTimer == nil {
				e.scanOnce(ctx)
				throttleTimer = time.After(throttle)
			}
			// Drain any queued quotes silently (QuoteBus cap=1 already drops stale).
		case <-throttleTimer:
			throttleTimer = nil
			// Throttle window elapsed — next quote triggers immediate scan.
		}
	}
}

// mergeSubscribe subscribes to all symbols and returns a single merged channel.
// The returned cancel func unsubscribes all.
func (e *Engine) mergeSubscribe(ctx context.Context, symbols []string) (<-chan bus.Quote, func()) {
	merged := make(chan bus.Quote, len(symbols))
	var cancels []func()
	for _, sym := range symbols {
		ch, cancel := e.deps.Bus.Subscribe(sym)
		cancels = append(cancels, cancel)
		go func(c <-chan bus.Quote) {
			for {
				select {
				case <-ctx.Done():
					return
				case q, ok := <-c:
					if !ok {
						return
					}
					select {
					case merged <- q:
					default:
						// merged full, drop
					}
				}
			}
		}(ch)
	}
	return merged, func() {
		for _, c := range cancels {
			c()
		}
	}
}

func (e *Engine) scanOnce(ctx context.Context) {
	// 1. Snapshot quotes
	quotes := e.deps.Bus.Snapshot(ctx, e.deps.Symbols)
	if len(quotes) == 0 {
		return
	}

	// 2. Build canonical index
	symMap, err := e.deps.SymMap.SymMap(ctx)
	if err != nil {
		slog.Warn("engine: symmap load", "error", err)
		return
	}
	listings := e.deps.Cache.CanonicalIndex(symMap)
	if len(listings) == 0 {
		return
	}

	// 3. Detect
	candidates := detector.Scan(e.deps.Detectors, quotes, listings)
	if len(candidates) == 0 {
		return
	}

	// 4. Evaluate each candidate
	for _, c := range candidates {
		opp, err := e.deps.Evaluator.Evaluate(ctx, c)
		if err != nil {
			slog.Debug("engine: evaluate error", "error", err)
			continue
		}
		if opp == nil {
			continue // stale or discarded
		}
		if !opp.Executable {
			continue // not executable, don't push
		}

		// Generate ID if empty
		if opp.ID == "" {
			opp.ID = genOppID(c)
		}

		e.mu.Lock()
		e.opp[opp.ID] = opp
		e.mu.Unlock()

		e.broadcast(OpportunityEvent{
			Opp:    opp,
			Action: "PUSHED",
		})
	}

	// 5. Expire old opportunities
	e.expireOld(ctx)
}

func (e *Engine) expireOld(ctx context.Context) {
	now := time.Now()
	e.mu.Lock()
	var expired []string
	for id, opp := range e.opp {
		if opp.ExpiresAt.Before(now) && opp.Status == evaluator.OppStatusPushed {
			opp.Status = evaluator.OppStatusExpired
			expired = append(expired, id)
		}
	}
	e.mu.Unlock()

	for _, id := range expired {
		opp := e.opp[id]
		e.broadcast(OpportunityEvent{
			Opp:    opp,
			Action: "EXPIRED",
		})
		e.mu.Lock()
		delete(e.opp, id)
		e.mu.Unlock()
	}
}

func (e *Engine) broadcast(ev OpportunityEvent) {
	e.mu.RLock()
	subs := make([]chan OpportunityEvent, 0, len(e.sub))
	for ch := range e.sub {
		subs = append(subs, ch)
	}
	e.mu.RUnlock()

	for _, ch := range subs {
		select {
		case ch <- ev:
		default:
			// subscriber buffer full, drop event
		}
	}
}

// PushOpportunityForTest injects an opportunity directly and broadcasts it.
// For test use only — normally opportunities come from scanOnce.
func (e *Engine) PushOpportunityForTest(opp *evaluator.Opportunity) {
	e.mu.Lock()
	e.opp[opp.ID] = opp
	e.mu.Unlock()
	e.broadcast(OpportunityEvent{Opp: opp, Action: "PUSHED"})
}

// genOppID generates a deterministic ID from candidate legs.
func genOppID(c evaluator.Candidate) string {
	id := ""
	for _, leg := range c.Legs {
		id += leg.Broker + leg.BrokerSymbol
	}
	return id
}
