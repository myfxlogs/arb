// Command probe connects to every MT5 broker in PG (broker_accounts) and
// dumps key SymbolParams fields for a few symbols, side by side.
//
// Design probing tool (not product code): compares swap / contractSize /
// profitCurrency / symbol-naming across brokers, to validate the Listing
// model and the swap/funding model on real data.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"arb/internal/adapter"
	"arb/internal/store"
)

func main() {
	dsn := flag.String("dsn", "postgres://arb:arb@localhost:5433/arb?sslmode=disable", "PG DSN (broker_accounts)")
	flag.Parse()

	ctx := context.Background()
	st, err := store.New(ctx, *dsn)
	if err != nil {
		die("pg connect", err)
	}
	accounts, err := st.ListBrokerAccounts(ctx)
	if err != nil {
		die("list broker accounts", err)
	}

	n := 0
	for i := range accounts {
		if accounts[i].Platform != 1 { // MT5 only (decided scope)
			continue
		}
		probeBroker(ctx, &accounts[i])
		n++
	}
	if n == 0 {
		fmt.Fprintln(os.Stderr, "no MT5 broker in PG broker_accounts")
		os.Exit(1)
	}
}

func probeBroker(ctx context.Context, acc *store.BrokerAccountRecord) {
	fmt.Printf("\n############ %s (login %d, host %s:%d) ############\n", acc.Name, acc.Login, acc.Host, acc.Port)
	a := adapter.NewMT5Adapter(acc.Name, acc.Host, acc.Server, acc.Port, acc.Login, acc.Password, 5)
	token, err := a.Connect(ctx)
	if err != nil {
		fmt.Printf("  CONNECT ERROR: %v\n", err)
		return
	}
	fmt.Printf("  connected (token %d chars)\n", len(token))
	defer a.Stop()

	// symbol naming check (suffixes / GOLD vs XAUUSD) + find real symbol per base
	syms, _ := a.AllSymbols(ctx)
	has := func(s string) bool {
		for _, x := range syms {
			if x == s {
				return true
			}
		}
		return false
	}
	findSym := func(base string) string {
		for _, v := range []string{base, base + "m", base + "z", base + "pro", base + "."} {
			if has(v) {
				return v
			}
		}
		return base
	}
	fmt.Printf("  symbols=%d  has[EURUSD=%v EURUSDm=%v XAUUSD=%v GOLD=%v]\n",
		len(syms), has("EURUSD"), has("EURUSDm"), has("XAUUSD"), has("GOLD"))

	// compact field dump for cross-broker compare (uses each broker's real symbol)
	for _, base := range []string{"EURUSD", "XAUUSD", "GBPJPY", "USDJPY"} {
		sym := findSym(base)
		reply, err := a.SymbolParamsRaw(ctx, sym)
		if err != nil || reply.GetResult() == nil || reply.GetResult().GetSymbolInfo() == nil {
			fmt.Printf("  %-8s -> %-10s: NOT FOUND\n", base, sym)
			continue
		}
		si := reply.GetResult().GetSymbolInfo()
		sg := reply.GetResult().GetSymbolGroup()
		swapType, swapLong, swapShort, minLots, exec := "?", 0.0, 0.0, 0.0, "?"
		if sg != nil {
			swapType = sg.SwapType.String()
			swapLong = sg.SwapLong
			swapShort = sg.SwapShort
			minLots = sg.MinLots
			exec = sg.TradeType.String()
		}
		fmt.Printf("  %-8s -> %-10s: contract=%-9g profitCcy=%-4s digits=%d  swapType=%-20s swapLong=%-9g swapShort=%-9g minLots=%g exec=%s\n",
			base, sym, si.ContractSize, si.ProfitCurrency, si.Digits, swapType, swapLong, swapShort, minLots, exec)
	}
}

func die(msg string, err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, msg+":", err)
	} else {
		fmt.Fprintln(os.Stderr, msg)
	}
	os.Exit(1)
}
