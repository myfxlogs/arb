package dashboard

import (
	"context"
	"log/slog"

	"arb/internal/adapter"
	dashpb "arb/proto/gen/dashboard"
)

// Kill triggers the global kill switch.
func (s *Server) Kill(ctx context.Context, req *dashpb.KillRequest) (*dashpb.KillReply, error) {
	if s.kill == nil {
		return &dashpb.KillReply{Success: false, Error: "kill switch not configured"}, nil
	}
	if err := s.kill.Activate(); err != nil {
		return &dashpb.KillReply{Success: false, Error: err.Error()}, nil
	}
	positionsClosed := int32(0)
	ordersCancelled := int32(0)
	s.mu.RLock()
	adapters := make(map[string]adapter.PlatformAdapter, len(s.adapters))
	for k, v := range s.adapters {
		adapters[k] = v
	}
	s.mu.RUnlock()
	for name, a := range adapters {
		orders, err := a.OpenOrders(ctx)
		if err != nil {
			slog.Warn("kill: openOrders", "broker", name, "error", err)
			continue
		}
		for _, o := range orders {
			if err := a.CancelOrder(ctx, o.Ticket); err != nil {
				slog.Warn("kill: cancelOrder", "broker", name, "ticket", o.Ticket, "error", err)
			} else {
				ordersCancelled++
			}
		}
	}
	slog.Info("kill switch activated",
		"positions_closed", positionsClosed, "orders_cancelled", ordersCancelled)
	return &dashpb.KillReply{
		Success:         true,
		OrdersCancelled: ordersCancelled,
	}, nil
}

// Resume deactivates the kill switch.
func (s *Server) Resume(ctx context.Context, req *dashpb.ResumeRequest) (*dashpb.ResumeReply, error) {
	if s.kill == nil {
		return &dashpb.ResumeReply{Success: false}, nil
	}
	if err := s.kill.Deactivate(); err != nil {
		return &dashpb.ResumeReply{Success: false}, nil
	}
	return &dashpb.ResumeReply{Success: true}, nil
}

// SubscribeSymbols adds symbols to the subscription set.
func (s *Server) SubscribeSymbols(ctx context.Context, req *dashpb.SubscribeSymbolsRequest) (*dashpb.SubscribeSymbolsReply, error) {
	for _, sym := range req.Symbols {
		s.symbols[sym] = true
	}
	return &dashpb.SubscribeSymbolsReply{Success: true}, nil
}

// UnsubscribeSymbols removes symbols from the subscription set.
func (s *Server) UnsubscribeSymbols(ctx context.Context, req *dashpb.UnsubscribeSymbolsRequest) (*dashpb.UnsubscribeSymbolsReply, error) {
	for _, sym := range req.Symbols {
		delete(s.symbols, sym)
	}
	return &dashpb.UnsubscribeSymbolsReply{Success: true}, nil
}

// ListSubscribedSymbols returns all subscribed symbols.
func (s *Server) ListSubscribedSymbols(ctx context.Context, req *dashpb.ListSymbolsRequest) (*dashpb.ListSymbolsReply, error) {
	syms := make([]string, 0, len(s.symbols))
	for sym := range s.symbols {
		syms = append(syms, sym)
	}
	return &dashpb.ListSymbolsReply{Symbols: syms}, nil
}

// GetStrategyStatus returns the status of all strategies.
func (s *Server) GetStrategyStatus(ctx context.Context, req *dashpb.StrategyStatusRequest) (*dashpb.StrategyStatusReply, error) {
	items := make([]*dashpb.StrategyStatusReply_StrategyItem, 0, len(s.strategies))
	for name, st := range s.strategies {
		if req.Strategy != "" && req.Strategy != name {
			continue
		}
		cbOpen := false
		if s.breaker != nil {
			cbOpen = s.breaker.State() != 0 // CircuitClosed = 0
		}
		items = append(items, &dashpb.StrategyStatusReply_StrategyItem{
			Name:               name,
			Enabled:            st.enabled,
			CircuitBreakerOpen: cbOpen,
			ConsecutiveLosses:  st.consecutiveLoss,
			WindowPnl:          st.windowPnL,
			TradesToday:        st.tradesToday,
			PnlToday:           st.pnlToday,
		})
	}
	return &dashpb.StrategyStatusReply{Items: items}, nil
}

// ToggleStrategy enables or disables a strategy.
func (s *Server) ToggleStrategy(ctx context.Context, req *dashpb.ToggleStrategyRequest) (*dashpb.ToggleStrategyReply, error) {
	st, ok := s.strategies[req.Strategy]
	if !ok {
		return &dashpb.ToggleStrategyReply{Success: false, Error: "strategy not found"}, nil
	}
	st.enabled = req.Enabled
	return &dashpb.ToggleStrategyReply{Success: true}, nil
}

// ResumeStrategy re-enables a strategy after circuit breaker trip.
func (s *Server) ResumeStrategy(ctx context.Context, req *dashpb.ResumeStrategyRequest) (*dashpb.ResumeStrategyReply, error) {
	st, ok := s.strategies[req.Strategy]
	if !ok {
		return &dashpb.ResumeStrategyReply{Success: false, Error: "strategy not found"}, nil
	}
	st.enabled = true
	st.consecutiveLoss = 0
	return &dashpb.ResumeStrategyReply{Success: true}, nil
}

// ResetGlobalCircuitBreaker resets the global circuit breaker.
func (s *Server) ResetGlobalCircuitBreaker(ctx context.Context, req *dashpb.ResetCBRequest) (*dashpb.ResetCBReply, error) {
	if s.breaker != nil {
		s.breaker.ResetWindow()
	}
	return &dashpb.ResetCBReply{Success: true}, nil
}

// GetKillSwitchStatus returns the current kill switch status.
func (s *Server) GetKillSwitchStatus(ctx context.Context, req *dashpb.KillSwitchStatusRequest) (*dashpb.KillSwitchStatusReply, error) {
	if s.kill == nil {
		return &dashpb.KillSwitchStatusReply{Active: false}, nil
	}
	return &dashpb.KillSwitchStatusReply{
		Active:       s.kill.IsActive(),
		TriggeredBy:  "manual",
	}, nil
}

// TailLogs streams recent log entries (placeholder — returns empty stream).
func (s *Server) TailLogs(req *dashpb.TailLogsRequest, stream dashpb.DashboardService_TailLogsServer) error {
	ctx := stream.Context()
	<-ctx.Done()
	return ctx.Err()
}

// AddBroker connects a new broker adapter at runtime.
func (s *Server) AddBroker(ctx context.Context, req *dashpb.AddBrokerRequest) (*dashpb.AddBrokerReply, error) {
	s.mu.Lock()
	if _, exists := s.adapters[req.Name]; exists {
		s.mu.Unlock()
		return &dashpb.AddBrokerReply{Success: false, Error: "broker already exists"}, nil
	}
	s.mu.Unlock()

	maxOrders := s.maxConcurrentOrders
	if maxOrders <= 0 {
		maxOrders = 5
	}

	var a adapter.PlatformAdapter
	switch req.Platform {
	case 0: // MT4
		a = adapter.NewMT4Adapter(req.Name, req.Host, req.Server, req.Port, req.User, req.Password, maxOrders)
	case 1: // MT5
		a = adapter.NewMT5Adapter(req.Name, req.Host, req.Server, req.Port, req.User, req.Password, maxOrders)
	default:
		return &dashpb.AddBrokerReply{Success: false, Error: "unknown platform type"}, nil
	}

	token, err := a.Connect(ctx)
	if err != nil {
		return &dashpb.AddBrokerReply{Success: false, Error: err.Error()}, nil
	}

	s.mu.Lock()
	s.adapters[req.Name] = a
	symbols := make([]string, 0, len(s.symbols))
	for sym := range s.symbols {
		symbols = append(symbols, sym)
	}
	s.mu.Unlock()

	if err := a.Subscribe(ctx, symbols); err != nil {
		slog.Warn("addBroker subscribe", "broker", req.Name, "error", err)
	}
	go a.QuoteStream(ctx, s.bus)

	slog.Info("broker added", "broker", req.Name, "token", token)
	return &dashpb.AddBrokerReply{Success: true, Token: token}, nil
}

// RemoveBroker disconnects and removes a broker adapter.
func (s *Server) RemoveBroker(ctx context.Context, req *dashpb.RemoveBrokerRequest) (*dashpb.RemoveBrokerReply, error) {
	s.mu.Lock()
	a, exists := s.adapters[req.Name]
	if !exists {
		s.mu.Unlock()
		return &dashpb.RemoveBrokerReply{Success: false, Error: "broker not found"}, nil
	}
	delete(s.adapters, req.Name)
	s.mu.Unlock()

	if err := a.Disconnect(); err != nil {
		slog.Warn("removeBroker disconnect", "broker", req.Name, "error", err)
	}
	slog.Info("broker removed", "broker", req.Name)
	return &dashpb.RemoveBrokerReply{Success: true}, nil
}
