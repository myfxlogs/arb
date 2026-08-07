// Command verify_listing connects to MT5 brokers from PG (broker_accounts),
// fetches Listing structs for EURUSD/XAUUSD, and prints fields side-by-side
// for comparison with the 02-opportunity.md §7 field reference table.
//
// Usage:
//
//	go run ./tools/verify_listing [-dsn ...] [-symbols EURUSD,XAUUSD]
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"arb/internal/adapter"
	"arb/internal/listing"
	"arb/internal/store"
)

func main() {
	dsn := flag.String("dsn", "postgres://arb:arb@localhost:5433/arb?sslmode=disable", "PG DSN")
	symbolsFlag := flag.String("symbols", "EURUSD,XAUUSD", "canonical symbols to verify")
	flag.Parse()

	ctx := context.Background()
	st, err := store.New(ctx, *dsn)
	if err != nil {
		die("pg connect", err)
	}
	defer st.Close()

	if err := st.EnsureMigrations(ctx); err != nil {
		die("ensure migrations", err)
	}
	symMap, err := st.LoadSymbolMap(ctx)
	if err != nil {
		die("load symbol_map", err)
	}

	accounts, err := st.ListBrokerAccounts(ctx)
	if err != nil {
		die("list broker accounts", err)
	}

	canonicalSyms := strings.Split(*symbolsFlag, ",")
	cache := listing.NewCache()
	var fetchers []listing.Fetcher
	brokerSyms := make(map[string][]string)

	for i := range accounts {
		acc := &accounts[i]
		if acc.Platform != 1 {
			continue
		}
		a := adapter.NewMT5Adapter(acc.Name, acc.Host, acc.Server, acc.Port, acc.Login, acc.Password, 5)
		if _, err := a.Connect(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "connect %s: %v\n", acc.Name, err)
			continue
		}
		defer a.Stop()

		allSyms, _ := a.AllSymbols(ctx)
		symSet := make(map[string]bool, len(allSyms))
		for _, s := range allSyms {
			symSet[s] = true
		}

		for _, canonical := range canonicalSyms {
			brokerSym := resolveSymbol(acc.Name, canonical, symMap, symSet)
			if brokerSym == "" {
				fmt.Fprintf(os.Stderr, "  %s: cannot find symbol for %s\n", acc.Name, canonical)
				continue
			}
			brokerSyms[acc.Name] = append(brokerSyms[acc.Name], brokerSym)

			if _, exists := symMap[acc.Name]; !exists || symMap[acc.Name][brokerSym] == "" {
				_ = st.SaveSymbolMapEntry(ctx, store.SymbolMapEntry{
					Broker: acc.Name, BrokerSymbol: brokerSym, CanonicalSymbol: canonical,
				})
				slog.Info("symbol_map: auto-inserted", "broker", acc.Name, "brokerSym", brokerSym, "canonical", canonical)
			}
		}
		fetchers = append(fetchers, a)
	}

	if len(fetchers) == 0 {
		die("no MT5 brokers connected", nil)
	}

	if err := cache.Populate(ctx, fetchers, brokerSyms); err != nil {
		die("populate cache", err)
	}

	for _, l := range cache.All() {
		printListing(l)
	}
}

// resolveSymbol finds the broker-specific symbol for a canonical symbol.
// Tries symbol_map first, then auto-detects via common suffixes.
func resolveSymbol(broker, canonical string, symMap map[string]map[string]string, available map[string]bool) string {
	if m := symMap[broker]; m != nil {
		for brokerSym, canon := range m {
			if canon == canonical && available[brokerSym] {
				return brokerSym
			}
		}
	}
	for _, suffix := range []string{"", "m", "z", "pro", "."} {
		candidate := canonical + suffix
		if available[candidate] {
			return candidate
		}
	}
	return ""
}

func printListing(l *listing.Listing) {
	fmt.Printf("\n══ %s / %s ══\n", l.Broker, l.BrokerSymbol)
	fmt.Printf("  ContractSize:   %s\n", l.ContractSize.String())
	fmt.Printf("  Digits:         %d\n", l.Digits)
	fmt.Printf("  Points:         %s\n", l.Points.String())
	fmt.Printf("  ProfitCurrency: %s\n", l.ProfitCurrency)
	fmt.Printf("  MarginCurrency: %s\n", l.MarginCurrency)
	fmt.Printf("  CalcMode:       %d\n", l.CalcMode)
	fmt.Printf("  VolumeMin:      %s\n", l.VolumeMin.String())
	fmt.Printf("  VolumeMax:      %s\n", l.VolumeMax.String())
	fmt.Printf("  VolumeStep:     %s\n", l.VolumeStep.String())
	fmt.Printf("  InitMargin:     %s\n", l.InitMargin.String())
	fmt.Printf("  TradeMode:      %d\n", l.TradeMode)
	fmt.Printf("  ExecType:       %d\n", l.ExecType)
	fmt.Printf("  FillPolicy:     %d\n", l.FillPolicy)
	fmt.Printf("  TripleSwapDay:  %d\n", l.TripleSwap)
	fmt.Printf("  Swap.SwapType:  %d\n", l.Swap.SwapType)
	fmt.Printf("  Swap.SwapLong:  %s\n", l.Swap.SwapLong.String())
	fmt.Printf("  Swap.SwapShort: %s\n", l.Swap.SwapShort.String())
}

func die(msg string, err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, msg+":", err)
	} else {
		fmt.Fprintln(os.Stderr, msg)
	}
	os.Exit(1)
}
