package engine

import (
	"context"

	"arb/internal/store"
)

// StoreSymMap adapts store.Store to SymMapProvider.
type StoreSymMap struct {
	Store *store.Store
}

// SymMap returns the current symbol_map from the database.
func (s *StoreSymMap) SymMap(ctx context.Context) (map[string]map[string]string, error) {
	return s.Store.LoadSymbolMap(ctx)
}
