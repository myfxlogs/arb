package dashboard

import (
	"context"
	"fmt"
	"time"

	"arb/internal/adapter"
	"arb/internal/decimalutil"

	dashpb "arb/proto/gen/dashboard"
)

// SubmitOrder handles manual order submission.
func (s *Server) SubmitOrder(ctx context.Context, req *dashpb.ManualOrderRequest) (*dashpb.ManualOrderReply, error) {
	a, ok := s.adapters[req.BrokerName]
	if !ok {
		return &dashpb.ManualOrderReply{Status: "Rejected", Error: "broker not found"}, nil
	}

	op := adapter.OpBuy
	if req.Side == "Sell" {
		op = adapter.OpSell
	}
	result, err := a.PlaceOrder(ctx, adapter.OrderRequest{
		ClientID:  req.ClientId,
		Symbol:    req.Symbol,
		Operation: op,
		Volume:    decimalutil.FromFloat64(req.Lots, 8),
		Price:     req.Price,
		Slippage:  req.Slippage,
	})
	if err != nil {
		return &dashpb.ManualOrderReply{ClientId: req.ClientId, Status: "Rejected", Error: err.Error()}, nil
	}
	status := "Unknown"
	switch result.State {
	case adapter.StateFilled:
		status = "Filled"
	case adapter.StatePartial:
		status = "Partial"
	case adapter.StateRejected:
		status = "Rejected"
	}
	errMsg := ""
	if result.Error != nil {
		errMsg = result.Error.Error()
	}
	return &dashpb.ManualOrderReply{
		ClientId: result.ClientID,
		Ticket:   result.Ticket,
		Status:   status,
		Error:    errMsg,
	}, nil
}

// ClosePosition handles manual position close.
func (s *Server) ClosePosition(ctx context.Context, req *dashpb.ClosePositionRequest) (*dashpb.ClosePositionReply, error) {
	a, ok := s.adapters[req.BrokerName]
	if !ok {
		return &dashpb.ClosePositionReply{Status: "Rejected", Error: "broker not found"}, nil
	}
	result, err := a.CloseOrder(ctx, req.Ticket, decimalutil.FromFloat64(req.Lots, 8), req.Price, req.Slippage)
	if err != nil {
		return &dashpb.ClosePositionReply{Ticket: req.Ticket, Status: "Rejected", Error: err.Error()}, nil
	}
	status := "Filled"
	if result.State == adapter.StateRejected {
		status = "Rejected"
	}
	return &dashpb.ClosePositionReply{Ticket: result.Ticket, Status: status}, nil
}

// CancelOrder handles pending order cancellation.
func (s *Server) CancelOrder(ctx context.Context, req *dashpb.CancelOrderRequest) (*dashpb.CancelOrderReply, error) {
	a, ok := s.adapters[req.BrokerName]
	if !ok {
		return &dashpb.CancelOrderReply{Success: false, Error: "broker not found"}, nil
	}
	if err := a.CancelOrder(ctx, req.Ticket); err != nil {
		return &dashpb.CancelOrderReply{Success: false, Error: err.Error()}, nil
	}
	return &dashpb.CancelOrderReply{Success: true}, nil
}

// GetSignalHistory queries historical signals from the store.
func (s *Server) GetSignalHistory(ctx context.Context, req *dashpb.SignalHistoryRequest) (*dashpb.SignalHistoryReply, error) {
	if s.store == nil {
		return &dashpb.SignalHistoryReply{}, nil
	}
	from := time.UnixMilli(req.FromUnixMs)
	to := time.UnixMilli(req.ToUnixMs)
	limit := req.Limit
	if limit == 0 {
		limit = 100
	}
	signals, err := s.store.QuerySignals(ctx, from, to, limit)
	if err != nil {
		return nil, fmt.Errorf("query signals: %w", err)
	}
	items := make([]*dashpb.SignalHistoryReply_SignalItem, 0, len(signals))
	for _, sig := range signals {
		items = append(items, &dashpb.SignalHistoryReply_SignalItem{
			Id:        sig.ID,
			Strategy:  sig.Strategy,
			LegsJson:  sig.Legs,
			Executed:  sig.Status == "executed",
		})
	}
	return &dashpb.SignalHistoryReply{Items: items}, nil
}

// GetOrderHistory queries historical orders from the store.
func (s *Server) GetOrderHistory(ctx context.Context, req *dashpb.OrderHistoryRequest) (*dashpb.OrderHistoryReply, error) {
	if s.store == nil {
		return &dashpb.OrderHistoryReply{}, nil
	}
	from := time.UnixMilli(req.FromUnixMs)
	to := time.UnixMilli(req.ToUnixMs)
	limit := req.Limit
	if limit == 0 {
		limit = 100
	}
	orders, err := s.store.QueryOrders(ctx, from, to, limit)
	if err != nil {
		return nil, fmt.Errorf("query orders: %w", err)
	}
	items := make([]*dashpb.OrderHistoryReply_OrderItem, 0, len(orders))
	for _, o := range orders {
		item := &dashpb.OrderHistoryReply_OrderItem{
			ClientId: o.ClientID,
			Broker:   o.Broker,
			Symbol:   o.Symbol,
			Side:     o.Side,
			Volume:   o.Volume,
			OpenPrice: o.Price,
		}
		if o.Ticket != nil {
			item.Ticket = *o.Ticket
		}
		items = append(items, item)
	}
	return &dashpb.OrderHistoryReply{Items: items}, nil
}

// GetDailySummary queries daily P&L summaries.
func (s *Server) GetDailySummary(ctx context.Context, req *dashpb.DailySummaryRequest) (*dashpb.DailySummaryReply, error) {
	if s.store == nil {
		return &dashpb.DailySummaryReply{}, nil
	}
	from := time.UnixMilli(req.FromUnixMs)
	to := time.UnixMilli(req.ToUnixMs)
	summaries, err := s.store.QueryDailySummary(ctx, from, to)
	if err != nil {
		return nil, fmt.Errorf("query daily summary: %w", err)
	}
	items := make([]*dashpb.DailySummaryReply_DailyItem, 0, len(summaries))
	for _, ds := range summaries {
		items = append(items, &dashpb.DailySummaryReply_DailyItem{
			Date:    ds.Day.Format("2006-01-02"),
			Pnl:     ds.TotalPnL,
		})
	}
	return &dashpb.DailySummaryReply{Items: items}, nil
}

// GetAccountSnapshots returns current account snapshots for all brokers.
func (s *Server) GetAccountSnapshots(ctx context.Context, req *dashpb.AccountSnapshotRequest) (*dashpb.AccountSnapshotReply, error) {
	items := make([]*dashpb.AccountSnapshotReply_AccountSnapshotItem, 0, len(s.adapters))
	for name, a := range s.adapters {
		item := &dashpb.AccountSnapshotReply_AccountSnapshotItem{
			BrokerName:  name,
			IsConnected: true,
		}
		acct, err := a.AccountSummary(ctx)
		if err != nil {
			item.IsConnected = false
		} else {
			item.Equity = float64(acct.Equity.InexactFloat64())
			item.FreeMargin = float64(acct.FreeMargin.InexactFloat64())
		}
		orders, err := a.OpenOrders(ctx)
		if err == nil {
			item.OpenPositions = int32(len(orders))
		}
		items = append(items, item)
	}
	return &dashpb.AccountSnapshotReply{Items: items}, nil
}
