package store

import (
	"context"
	"fmt"
)

// SymbolMapEntry maps a broker-specific symbol to a canonical symbol.
type SymbolMapEntry struct {
	Broker          string
	BrokerSymbol    string
	CanonicalSymbol string
}

// LoadSymbolMap returns all symbol_map rows grouped by broker into a nested
// map: broker → brokerSymbol → canonicalSymbol.
func (s *Store) LoadSymbolMap(ctx context.Context) (map[string]map[string]string, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT broker, broker_symbol, canonical_symbol FROM symbol_map`)
	if err != nil {
		return nil, fmt.Errorf("load symbol_map: %w", err)
	}
	defer rows.Close()

	m := make(map[string]map[string]string)
	for rows.Next() {
		var broker, brokerSym, canonical string
		if err := rows.Scan(&broker, &brokerSym, &canonical); err != nil {
			return nil, err
		}
		if m[broker] == nil {
			m[broker] = make(map[string]string)
		}
		m[broker][brokerSym] = canonical
	}
	return m, rows.Err()
}

// SaveSymbolMapEntry inserts or updates a single symbol_map row.
func (s *Store) SaveSymbolMapEntry(ctx context.Context, e SymbolMapEntry) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO symbol_map (broker, broker_symbol, canonical_symbol)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (broker, broker_symbol) DO UPDATE SET
		   canonical_symbol = EXCLUDED.canonical_symbol`,
		e.Broker, e.BrokerSymbol, e.CanonicalSymbol)
	if err != nil {
		return fmt.Errorf("save symbol_map: %w", err)
	}
	return nil
}
