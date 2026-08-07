package detector

import (
	"context"
	"fmt"
	"strings"

	"arb/internal/bus"
	"arb/internal/evaluator"
	"arb/internal/listing"
)

// Detector scans latest quotes + listings for arbitrage candidates (03 §4).
// Implementations are pure functions — no I/O, no side effects.
type Detector interface {
	Type() evaluator.OppType
	Scan(ctx context.Context, quotes map[string]bus.Quote,
		listings map[listing.CanonicalKey]*listing.Listing) ([]evaluator.Candidate, error)
}

// Scan dispatches to all detectors and merges their candidates.
// De-duplication: same canonical + same broker pair → keep the one with
// larger GrossProfit (13 §6).
func Scan(
	detectors []Detector,
	quotes map[string]bus.Quote,
	listings map[listing.CanonicalKey]*listing.Listing,
) []evaluator.Candidate {
	ctx := context.Background()
	var all []evaluator.Candidate
	for _, d := range detectors {
		cands, err := d.Scan(ctx, quotes, listings)
		if err != nil {
			continue
		}
		all = append(all, cands...)
	}
	return dedup(all)
}

// dedup removes duplicate candidates. For 2-leg candidates (CrossExchange,
// Carry): same type + same canonical + same broker pair → keep larger
// GrossProfit. For 3-leg (Triangular): same type + same canonicals + same
// brokers → keep larger. Different types are never deduped against each other.
func dedup(cands []evaluator.Candidate) []evaluator.Candidate {
	best := make(map[string]evaluator.Candidate)
	for _, c := range cands {
		if len(c.Legs) < 2 {
			continue
		}
		k := dedupKey(c)
		if existing, ok := best[k]; !ok || c.GrossProfit.GreaterThan(existing.GrossProfit) {
			best[k] = c
		}
	}
	out := make([]evaluator.Candidate, 0, len(best))
	for _, c := range best {
		out = append(out, c)
	}
	return out
}

// dedupKey builds a unique key per opportunity identity.
func dedupKey(c evaluator.Candidate) string {
	if len(c.Legs) == 2 {
		return fmt.Sprintf("%d|%s|%s",
			c.Type, c.Legs[0].Canonical,
			brokerPairKey(c.Legs[0].Broker, c.Legs[1].Broker))
	}
	parts := make([]string, 0, len(c.Legs)*2+1)
	parts = append(parts, fmt.Sprintf("%d", c.Type))
	for _, leg := range c.Legs {
		parts = append(parts, leg.Canonical, leg.Broker)
	}
	return strings.Join(parts, "|")
}

func brokerPairKey(a, b string) string {
	if a < b {
		return a + "|" + b
	}
	return b + "|" + a
}

// NewCrossExchange creates a CrossExchange detector.
func NewCrossExchange() Detector { return &crossExchangeDetector{} }

// NewCarry creates a Carry detector.
func NewCarry() Detector { return &carryDetector{} }

// NewTriangular creates a Triangular detector.
func NewTriangular() Detector { return &triangularDetector{} }
