package adapter

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"arb/internal/decimalutil"
	mt4 "arb/proto/gen/mtapi/mt4"

	"github.com/shopspring/decimal"
)

func (a *MT4Adapter) PlaceOrder(ctx context.Context, req OrderRequest) (*OrderResult, error) {
	if !a.rsm.canPlaceOrder() {
		return nil, ErrNotConnected
	}

	select {
	case a.execSem <- struct{}{}:
		defer func() { <-a.execSem }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	resp, err := a.trading.OrderSend(a.withSessionMD(ctx), &mt4.OrderSendRequest{
		Id:        a.token,
		Symbol:    req.Symbol,
		Operation: toMT4Op(req.Operation),
		Volume:    decimalutil.ToFloat64(req.Volume),
		Price:     req.Price,
		Slippage:  req.Slippage,
		Comment:   req.ClientID,
	})
	if err != nil {
		return nil, fmt.Errorf("mt4 orderSend: %w", err)
	}
	if resp.Error != nil && resp.Error.Code != mt4.ErrorCode_INTERNAL_ERROR {
		return &OrderResult{
			ClientID: req.ClientID,
			Symbol:   req.Symbol,
			State:    StateRejected,
			Error:    fmt.Errorf("mt4: %s", resp.Error.Message),
		}, nil
	}
	return mt4OrderToResult(resp.Result, req), nil
}

func (a *MT4Adapter) CancelOrder(ctx context.Context, ticket int64) error {
	if !a.rsm.canPlaceOrder() {
		return ErrNotConnected
	}
	_, err := a.trading.OrderDelete(a.withSessionMD(ctx), &mt4.OrderDeleteRequest{
		Id:     a.token,
		Ticket: int32(ticket),
	})
	return err
}

func (a *MT4Adapter) CloseOrder(ctx context.Context, ticket int64, lots decimal.Decimal, price float64, slippage int32) (*OrderResult, error) {
	if !a.rsm.canPlaceOrder() {
		return nil, ErrNotConnected
	}
	resp, err := a.trading.OrderClose(a.withSessionMD(ctx), &mt4.OrderCloseRequest{
		Id:       a.token,
		Ticket:   int32(ticket),
		Lots:     decimalutil.ToFloat64(lots),
		Price:    price,
		Slippage: slippage,
	})
	if err != nil {
		return nil, fmt.Errorf("mt4 orderClose: %w", err)
	}
	if resp.Error != nil && resp.Error.Code != mt4.ErrorCode_INTERNAL_ERROR {
		return nil, fmt.Errorf("mt4 close: %s", resp.Error.Message)
	}
	return &OrderResult{
		Ticket:      int64(resp.Result.Ticket),
		Symbol:      resp.Result.Symbol,
		State:       StateFilled,
		Volume:      decimalutil.FromFloat64(resp.Result.Lots, 2),
		CloseVolume: decimalutil.FromFloat64(resp.Result.Lots, 2),
	}, nil
}

func (a *MT4Adapter) AccountSummary(ctx context.Context) (*Account, error) {
	if !a.rsm.canPlaceOrder() {
		return nil, ErrNotConnected
	}
	resp, err := a.mt4.AccountSummary(a.withSessionMD(ctx), &mt4.AccountSummaryRequest{Id: a.token})
	if err != nil {
		return nil, err
	}
	if resp.Error != nil && resp.Error.Code != mt4.ErrorCode_INTERNAL_ERROR {
		return nil, fmt.Errorf("mt4: %s", resp.Error.Message)
	}
	if resp.Result == nil {
		return &Account{}, nil
	}
	s := resp.Result
	return &Account{
		Balance:    decimalutil.FromFloat64(s.Balance, 2),
		Equity:     decimalutil.FromFloat64(s.Equity, 2),
		Margin:     decimalutil.FromFloat64(s.Margin, 2),
		FreeMargin: decimalutil.FromFloat64(s.FreeMargin, 2),
		Currency:   s.Currency,
	}, nil
}

func (a *MT4Adapter) OpenOrders(ctx context.Context) ([]Order, error) {
	if !a.rsm.canPlaceOrder() {
		return nil, ErrNotConnected
	}
	resp, err := a.mt4.OpenedOrders(a.withSessionMD(ctx), &mt4.OpenedOrdersRequest{Id: a.token})
	if err != nil {
		return nil, err
	}
	if resp.Error != nil && resp.Error.Code != mt4.ErrorCode_INTERNAL_ERROR {
		return nil, fmt.Errorf("mt4: %s", resp.Error.Message)
	}
	orders := make([]Order, 0, len(resp.Result))
	for _, o := range resp.Result {
		if classifyMT4OrderType(o.Type) == "BALANCE" || classifyMT4OrderType(o.Type) == "CREDIT" {
			continue
		}
		orders = append(orders, Order{
			Ticket:  int64(o.Ticket),
			Symbol:  o.Symbol,
			Type:    fromMT4Op(o.Type),
			Lots:    decimalutil.FromFloat64(o.Lots, 2),
			Comment: o.Comment,
		})
	}
	return orders, nil
}

func (a *MT4Adapter) OrderHistory(ctx context.Context, from, to time.Time) ([]Order, error) {
	if !a.rsm.canPlaceOrder() {
		return nil, ErrNotConnected
	}
	resp, err := a.mt4.OrderHistory(a.withSessionMD(ctx), &mt4.OrderHistoryRequest{
		Id:   a.token,
		From: from.Format("2006-01-02T15:04:05"),
		To:   to.Format("2006-01-02T15:04:05"),
	})
	if err != nil {
		return nil, err
	}
	if resp.Error != nil && resp.Error.Code != mt4.ErrorCode_INTERNAL_ERROR {
		return nil, fmt.Errorf("mt4: %s", resp.Error.Message)
	}
	orders := make([]Order, 0, len(resp.Result))
	for _, o := range resp.Result {
		if classifyMT4OrderType(o.Type) == "BALANCE" || classifyMT4OrderType(o.Type) == "CREDIT" {
			continue
		}
		orders = append(orders, Order{
			Ticket:     int64(o.Ticket),
			Symbol:     o.Symbol,
			Type:       fromMT4Op(o.Type),
			Lots:       decimalutil.FromFloat64(o.Lots, 2),
			OpenPrice:  o.OpenPrice,
			ClosePrice: o.ClosePrice,
			Profit:     o.Profit,
			Comment:    o.Comment,
		})
	}
	return orders, nil
}

func (a *MT4Adapter) AllSymbols(ctx context.Context) ([]string, error) {
	if !a.rsm.canPlaceOrder() {
		return nil, ErrNotConnected
	}
	resp, err := a.mt4.Symbols(a.withSessionMD(ctx), &mt4.SymbolsRequest{Id: a.token})
	if err != nil {
		return nil, err
	}
	if resp.Error != nil && resp.Error.Code != mt4.ErrorCode_INTERNAL_ERROR {
		return nil, fmt.Errorf("mt4: %s", resp.Error.Message)
	}
	return resp.Result, nil
}

func (a *MT4Adapter) SymbolDigits(ctx context.Context, symbols []string) (map[string]int32, error) {
	if !a.rsm.canPlaceOrder() {
		return nil, ErrNotConnected
	}
	result := make(map[string]int32, len(symbols))
	for _, sym := range symbols {
		resp, err := a.mt4.SymbolParams(a.withSessionMD(ctx), &mt4.SymbolParamsRequest{Id: a.token, Symbol: sym})
		if err != nil {
			slog.Warn("mt4 symbolParams", "symbol", sym, "error", err)
			continue
		}
		if resp.Result != nil && resp.Result.Symbol != nil {
			result[sym] = resp.Result.Symbol.Digits
		}
	}
	return result, nil
}

func toMT4Op(op OrderOperation) mt4.Op {
	switch op {
	case OpBuy:
		return mt4.Op_Op_Buy
	case OpSell:
		return mt4.Op_Op_Sell
	default:
		return mt4.Op_Op_Buy
	}
}

func fromMT4Op(t mt4.Op) OrderOperation {
	switch t {
	case mt4.Op_Op_Sell:
		return OpSell
	default:
		return OpBuy
	}
}

func mt4OrderToResult(o *mt4.Order, req OrderRequest) *OrderResult {
	r := &OrderResult{
		ClientID:  req.ClientID,
		Ticket:    int64(o.Ticket),
		Symbol:    o.Symbol,
		Operation: req.Operation,
		Volume:    decimalutil.FromFloat64(o.Lots, 2),
	}
	if o.Ticket != 0 {
		r.State = StateFilled
		r.CloseVolume = decimalutil.FromFloat64(o.Lots, 2)
	} else {
		r.State = StateRejected
	}
	return r
}

func classifyMT4OrderType(t mt4.Op) string {
	switch t {
	case mt4.Op_Op_Balance:
		return "BALANCE"
	case mt4.Op_Op_Credit:
		return "CREDIT"
	case mt4.Op_Op_Buy:
		return "BUY"
	case mt4.Op_Op_Sell:
		return "SELL"
	default:
		return "UNKNOWN"
	}
}
