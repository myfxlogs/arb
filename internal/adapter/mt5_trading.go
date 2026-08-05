package adapter

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"arb/internal/decimalutil"
	mt5 "arb/proto/gen/mtapi/mt5"

	"github.com/shopspring/decimal"
	"google.golang.org/protobuf/proto"
)

// PlaceOrder submits a market order via MT5 OrderSend.
func (a *MT5Adapter) PlaceOrder(ctx context.Context, req OrderRequest) (*OrderResult, error) {
	if !a.rsm.canPlaceOrder() {
		return nil, ErrNotConnected
	}

	select {
	case a.execSem <- struct{}{}:
		defer func() { <-a.execSem }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	r := &mt5.OrderSendRequest{
		Id:        a.token,
		Symbol:    req.Symbol,
		Operation: toMT5Op(req.Operation),
		Volume:    decimalutil.ToFloat64(req.Volume),
		Comment:   proto.String(req.ClientID),
	}
	if req.Price > 0 {
		r.Price = proto.Float64(req.Price)
	}
	if req.Slippage > 0 {
		r.Slippage = proto.Uint64(uint64(req.Slippage))
	}

	resp, err := a.trading.OrderSend(a.withSessionMD(ctx), r)
	if err != nil {
		return nil, fmt.Errorf("mt5 orderSend: %w", err)
	}
	if resp.Error != nil && resp.Error.Code != mt5.ErrorCode_DONE {
		return &OrderResult{
			ClientID: req.ClientID,
			Symbol:   req.Symbol,
			State:    StateRejected,
			Error:    fmt.Errorf("mt5: %s", resp.Error.Message),
		}, nil
	}
	return mt5OrderToResult(resp.Result, req), nil
}

// CancelOrder cancels a pending order by ticket via OrderClose.
// MT5 Trading service has no OrderDelete; pending orders are closed.
func (a *MT5Adapter) CancelOrder(ctx context.Context, ticket int64) error {
	if !a.rsm.canPlaceOrder() {
		return ErrNotConnected
	}
	_, err := a.trading.OrderClose(a.withSessionMD(ctx), &mt5.OrderCloseRequest{
		Id:     a.token,
		Ticket: ticket,
	})
	return err
}

// CloseOrder closes a position by ticket.
func (a *MT5Adapter) CloseOrder(ctx context.Context, ticket int64, lots decimal.Decimal, price float64, slippage int32) (*OrderResult, error) {
	if !a.rsm.canPlaceOrder() {
		return nil, ErrNotConnected
	}
	req := &mt5.OrderCloseRequest{
		Id:     a.token,
		Ticket: ticket,
	}
	if lotsF := decimalutil.ToFloat64(lots); lotsF > 0 {
		req.Lots = proto.Float64(lotsF)
	}
	if price > 0 {
		req.Price = proto.Float64(price)
	}
	if slippage > 0 {
		req.Slippage = proto.Uint64(uint64(slippage))
	}
	resp, err := a.trading.OrderClose(a.withSessionMD(ctx), req)
	if err != nil {
		return nil, fmt.Errorf("mt5 orderClose: %w", err)
	}
	if resp.Error != nil && resp.Error.Code != mt5.ErrorCode_DONE {
		return nil, fmt.Errorf("mt5 close: %s", resp.Error.Message)
	}
	return &OrderResult{
		Ticket:      resp.Result.Ticket,
		Symbol:      resp.Result.Symbol,
		State:       StateFilled,
		Volume:      decimalutil.FromFloat64(resp.Result.Lots, 2),
		CloseVolume: decimalutil.FromFloat64(resp.Result.Lots, 2),
	}, nil
}

func (a *MT5Adapter) AccountSummary(ctx context.Context) (*Account, error) {
	if !a.rsm.canPlaceOrder() {
		return nil, ErrNotConnected
	}
	resp, err := a.mt5.AccountSummary(a.withSessionMD(ctx), &mt5.AccountSummaryRequest{Id: a.token})
	if err != nil {
		return nil, err
	}
	if resp.Error != nil && resp.Error.Code != mt5.ErrorCode_DONE {
		return nil, fmt.Errorf("mt5: %s", resp.Error.Message)
	}
	if resp.Result == nil {
		return &Account{}, nil
	}
	s := resp.Result
	return &Account{
		Balance:     decimalutil.FromFloat64(s.Balance, 2),
		Equity:      decimalutil.FromFloat64(s.Equity, 2),
		Margin:      decimalutil.FromFloat64(s.Margin, 2),
		FreeMargin:  decimalutil.FromFloat64(s.FreeMargin, 2),
		Currency:    s.Currency,
		Credit:      s.Credit,
		Profit:      s.Profit,
		MarginLevel: s.MarginLevel,
		Leverage:    int32(s.Leverage),
		Platform:    "MT5",
		Login:       a.user,
	}, nil
}

// OpenOrders returns all open orders.
func (a *MT5Adapter) OpenOrders(ctx context.Context) ([]Order, error) {
	if !a.rsm.canPlaceOrder() {
		return nil, ErrNotConnected
	}
	resp, err := a.mt5.OpenedOrders(a.withSessionMD(ctx), &mt5.OpenedOrdersRequest{Id: a.token})
	if err != nil {
		return nil, err
	}
	if resp.Error != nil && resp.Error.Code != mt5.ErrorCode_DONE {
		return nil, fmt.Errorf("mt5: %s", resp.Error.Message)
	}
	orders := make([]Order, 0, len(resp.Result))
	for _, o := range resp.Result {
		if classifyMT5OrderType(o.OrderType) == "BALANCE" || classifyMT5OrderType(o.OrderType) == "CREDIT" {
			continue
		}
		orders = append(orders, Order{
			Ticket:     o.Ticket,
			Symbol:     safeSymbol(o.Symbol),
			Type:       fromMT5Op(o.OrderType),
			Lots:       decimalutil.FromFloat64(o.Lots, 2),
			OpenPrice:  o.OpenPrice,
			StopLoss:   o.StopLoss,
			TakeProfit: o.TakeProfit,
			Profit:     o.Profit,
			Swap:       o.Swap,
			Commission: o.Commission,
			Comment:    o.Comment,
			OpenTime:   o.OpenTime.AsTime(),
		})
	}
	return orders, nil
}

func (a *MT5Adapter) OrderHistory(ctx context.Context, from, to time.Time) ([]Order, error) {
	if !a.rsm.canPlaceOrder() {
		return nil, ErrNotConnected
	}
	resp, err := a.mt5.OrderHistory(a.withSessionMD(ctx), &mt5.OrderHistoryRequest{
		Id:   a.token,
		From: from.Format("2006-01-02T15:04:05"),
		To:   to.Format("2006-01-02T15:04:05"),
	})
	if err != nil {
		return nil, err
	}
	if resp.Error != nil && resp.Error.Code != mt5.ErrorCode_DONE {
		return nil, fmt.Errorf("mt5: %s", resp.Error.Message)
	}
	orders := make([]Order, 0, len(resp.Result))
	for _, o := range resp.Result {
		if classifyMT5OrderType(o.OrderType) == "BALANCE" || classifyMT5OrderType(o.OrderType) == "CREDIT" {
			continue
		}
		orders = append(orders, Order{
			Ticket:     o.Ticket,
			Symbol:     safeSymbol(o.Symbol),
			Type:       fromMT5Op(o.OrderType),
			Lots:       decimalutil.FromFloat64(o.Lots, 2),
			OpenPrice:  o.OpenPrice,
			ClosePrice: o.ClosePrice,
			Profit:     o.Profit,
			Comment:    o.Comment,
			OpenTime:   o.OpenTime.AsTime(),
			CloseTime:  o.CloseTime.AsTime(),
		})
	}
	return orders, nil
}

// AllSymbols returns all available symbols.
func (a *MT5Adapter) AllSymbols(ctx context.Context) ([]string, error) {
	if !a.rsm.canPlaceOrder() {
		return nil, ErrNotConnected
	}
	resp, err := a.mt5.SymbolList(a.withSessionMD(ctx), &mt5.SymbolListRequest{Id: a.token})
	if err != nil {
		return nil, err
	}
	if resp.Error != nil && resp.Error.Code != mt5.ErrorCode_DONE {
		return nil, fmt.Errorf("mt5: %s", resp.Error.Message)
	}
	return resp.Result, nil
}

// SymbolDigits returns the digit count for each symbol.
func (a *MT5Adapter) SymbolDigits(ctx context.Context, symbols []string) (map[string]int32, error) {
	if !a.rsm.canPlaceOrder() {
		return nil, ErrNotConnected
	}
	result := make(map[string]int32, len(symbols))
	for _, sym := range symbols {
		resp, err := a.mt5.SymbolParams(a.withSessionMD(ctx), &mt5.SymbolParamsRequest{Id: a.token, Symbol: sym})
		if err != nil {
			slog.Warn("mt5 symbolParams", "symbol", sym, "error", err)
			continue
		}
		if resp.Result != nil && resp.Result.SymbolInfo != nil {
			result[sym] = resp.Result.SymbolInfo.Digits
		}
	}
	return result, nil
}

func toMT5Op(op OrderOperation) mt5.OrderType {
	switch op {
	case OpBuy:
		return mt5.OrderType_OrderType_Buy
	case OpSell:
		return mt5.OrderType_OrderType_Sell
	default:
		return mt5.OrderType_OrderType_Buy
	}
}

func fromMT5Op(t mt5.OrderType) OrderOperation {
	switch t {
	case mt5.OrderType_OrderType_Sell:
		return OpSell
	default:
		return OpBuy
	}
}

func mt5OrderToResult(o *mt5.Order, req OrderRequest) *OrderResult {
	r := &OrderResult{
		ClientID:  req.ClientID,
		Ticket:    o.Ticket,
		Symbol:    o.Symbol,
		Operation: req.Operation,
		Volume:    decimalutil.FromFloat64(o.Lots, 2),
	}
	if o.CloseVolume >= o.Volume && o.Volume > 0 {
		r.State = StateFilled
		r.CloseVolume = decimalutil.FromFloat64(float64(o.CloseVolume), 2)
	} else if o.CloseVolume > 0 {
		r.State = StatePartial
		r.CloseVolume = decimalutil.FromFloat64(float64(o.CloseVolume), 2)
	} else {
		r.State = StateUnknown
	}
	return r
}

func classifyMT5OrderType(t mt5.OrderType) string {
	switch t {
	case mt5.OrderType_OrderType_Balance:
		return "BALANCE"
	case mt5.OrderType_OrderType_Credit:
		return "CREDIT"
	default:
		return "TRADE"
	}
}

func safeSymbol(s string) string { return s }
