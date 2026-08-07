package listing

// CanonicalKey identifies a Listing by (broker, canonical symbol).
type CanonicalKey struct {
	Broker    string
	Canonical string
}

// CanonicalIndex builds a canonical view of the cache: key = (broker, canonical),
// value = Listing with Instrument filled from the canonical symbol.
// symMap maps broker → (brokerSymbol → canonical).
// Listings not found in symMap are skipped.
func (c *Cache) CanonicalIndex(symMap map[string]map[string]string) map[CanonicalKey]*Listing {
	c.mu.RLock()
	defer c.mu.RUnlock()

	out := make(map[CanonicalKey]*Listing)
	for broker, brokerSyms := range symMap {
		for brokerSym, canonical := range brokerSyms {
			l, ok := c.items[cacheKey(broker, brokerSym)]
			if !ok {
				continue
			}
			cp := *l
			cp.Instrument = ResolveInstrument(canonical)
			out[CanonicalKey{Broker: broker, Canonical: canonical}] = &cp
		}
	}
	return out
}

// ResolveInstrument derives the logical Instrument from a canonical symbol.
// FX: 6-char → base=first 3, quote=last 3 (EURUSD → EUR/USD).
// Precious metals: XAU/XAG/XPT/XPD prefix → base=prefix, quote=rest (XAUUSD → XAU/USD).
// Crypto (reserved, not implemented in v1): USDT/USD suffix → CRYPTO.
func ResolveInstrument(canonical string) *Instrument {
	if i := tryPreciousMetal(canonical); i != nil {
		return i
	}
	if i := tryStandardFX(canonical); i != nil {
		return i
	}
	if i := tryCrypto(canonical); i != nil {
		return i
	}
	return &Instrument{Symbol: canonical, AssetClass: "FX", Kind: "SPOT"}
}

func tryPreciousMetal(sym string) *Instrument {
	prefixes := []string{"XAU", "XAG", "XPT", "XPD"}
	for _, p := range prefixes {
		if len(sym) > len(p) && sym[:len(p)] == p {
			return &Instrument{
				Symbol:     sym,
				AssetClass: "FX",
				Base:       p,
				Quote:      sym[len(p):],
				Kind:       "SPOT",
			}
		}
	}
	return nil
}

func tryCrypto(sym string) *Instrument {
	suffixes := []string{"USDT", "USD"}
	for _, s := range suffixes {
		if len(sym) > len(s) && sym[len(sym)-len(s):] == s {
			return &Instrument{
				Symbol:     sym,
				AssetClass: "CRYPTO",
				Base:       sym[:len(sym)-len(s)],
				Quote:      s,
				Kind:       "PERP",
			}
		}
	}
	return nil
}

func tryStandardFX(sym string) *Instrument {
	if len(sym) == 6 {
		return &Instrument{
			Symbol:     sym,
			AssetClass: "FX",
			Base:       sym[:3],
			Quote:      sym[3:],
			Kind:       "SPOT",
		}
	}
	return nil
}
