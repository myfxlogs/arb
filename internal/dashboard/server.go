package dashboard

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"arb/internal/adapter"
	"arb/internal/bus"
	"arb/internal/risk"
	"arb/internal/store"

	dashpb "arb/proto/gen/dashboard"
)

// Server implements DashboardServiceServer.
type Server struct {
	dashpb.UnimplementedDashboardServiceServer

	mu                  sync.RWMutex
	bus                 *bus.QuoteBus
	adapters            map[string]adapter.PlatformAdapter
	store               *store.Store
	kill                *risk.KillSwitch
	breaker             *risk.CircuitBreaker
	symbols             map[string]bool
	strategies          map[string]*strategyState
	quoteCache          *quoteCache
	maxConcurrentOrders int
}

type strategyState struct {
	enabled         bool
	consecutiveLoss int32
	windowPnL       float64
	tradesToday     int32
	pnlToday        float64
}

// Deps holds dependencies for the dashboard server.
type Deps struct {
	Bus                 *bus.QuoteBus
	Adapters            map[string]adapter.PlatformAdapter
	Store               *store.Store
	KillSwitch          *risk.KillSwitch
	Breaker             *risk.CircuitBreaker
	Symbols             []string
	MaxConcurrentOrders int
}

// NewServer creates a DashboardServiceServer.
func NewServer(deps Deps) *Server {
	syms := make(map[string]bool, len(deps.Symbols))
	for _, s := range deps.Symbols {
		syms[s] = true
	}
	strats := map[string]*strategyState{
		"triangular":     {enabled: true},
		"cross_exchange": {enabled: false},
		"statistical":    {enabled: false},
	}
	return &Server{
		bus:                 deps.Bus,
		adapters:            deps.Adapters,
		store:               deps.Store,
		kill:                deps.KillSwitch,
		breaker:             deps.Breaker,
		symbols:             syms,
		strategies:          strats,
		quoteCache:          newQuoteCache(),
		maxConcurrentOrders: deps.MaxConcurrentOrders,
	}
}

// StartFeeder starts the background quote cache feeder for all subscribed symbols.
// Must be called after NewServer.
func (s *Server) StartFeeder() {
	symbols := make([]string, 0, len(s.symbols))
	for sym := range s.symbols {
		symbols = append(symbols, sym)
	}
	s.quoteCache.startFeeder(s.bus, symbols)
}

// SpreadMatrix streams spread matrix snapshots at the requested interval.
func (s *Server) SpreadMatrix(req *dashpb.SpreadMatrixRequest, stream dashpb.DashboardService_SpreadMatrixServer) error {
	interval := time.Duration(req.RefreshIntervalMs) * time.Millisecond
	if interval == 0 {
		interval = 100 * time.Millisecond
	}
	ctx := stream.Context()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			reply := s.buildSpreadMatrix()
			if err := stream.Send(reply); err != nil {
				return err
			}
		}
	}
}

// PositionWatch streams position and account data at the requested interval.
func (s *Server) PositionWatch(req *dashpb.PositionWatchRequest, stream dashpb.DashboardService_PositionWatchServer) error {
	interval := time.Duration(req.RefreshIntervalMs) * time.Millisecond
	if interval == 0 {
		interval = 500 * time.Millisecond
	}
	ctx := stream.Context()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			reply := s.buildPositionWatch(ctx)
			if err := stream.Send(reply); err != nil {
				return err
			}
		}
	}
}

// BuildSpreadMatrixForTest is an exported wrapper for testing.
func (s *Server) BuildSpreadMatrixForTest() *dashpb.SpreadMatrixReply {
	return s.buildSpreadMatrix()
}

// buildSpreadMatrix constructs a spread matrix snapshot from cached quotes.
func (s *Server) buildSpreadMatrix() *dashpb.SpreadMatrixReply {
	s.mu.RLock()
	symbols := make([]string, 0, len(s.symbols))
	for sym := range s.symbols {
		symbols = append(symbols, sym)
	}
	s.mu.RUnlock()

	// Get cached quotes organized by broker -> symbol -> Quote
	cached := s.quoteCache.snapshot()

	type brokerQuote struct {
		bid, ask float64
		has      bool
	}
	quotes := make(map[string]map[string]brokerQuote) // broker -> symbol -> quote
	for broker, syms := range cached {
		quotes[broker] = make(map[string]brokerQuote)
		for sym, q := range syms {
			quotes[broker][sym] = brokerQuote{bid: q.Bid, ask: q.Ask, has: true}
		}
	}

	// Find best bid/ask per symbol across all brokers
	bestBid := make(map[string]float64)
	bestAsk := make(map[string]float64)
	for _, sym := range symbols {
		bestBid[sym] = 0
		bestAsk[sym] = 1e18
		for _, bqs := range quotes {
			if q, ok := bqs[sym]; ok && q.has {
				if q.bid > bestBid[sym] {
					bestBid[sym] = q.bid
				}
				if q.ask < bestAsk[sym] {
					bestAsk[sym] = q.ask
				}
			}
		}
	}

	// Build rows
	rows := make([]*dashpb.SpreadMatrixReply_BrokerRow, 0, len(quotes))
	bestBidBroker, bestAskBroker := "", ""
	bestBidVal, bestAskVal := 0.0, 1e18

	for brokerName, bqs := range quotes {
		cells := make([]*dashpb.SpreadMatrixReply_SpreadCell, 0, len(symbols))
		for _, sym := range symbols {
			q, ok := bqs[sym]
			if !ok || !q.has {
				continue
			}
			spreadToBestAskBps := 0.0
			spreadToBestBidBps := 0.0
			if bestAsk[sym] < 1e18 && bestAsk[sym] > 0 {
				spreadToBestAskBps = (q.ask - bestAsk[sym]) / bestAsk[sym] * 10000
			}
			if bestBid[sym] > 0 {
				spreadToBestBidBps = (q.bid - bestBid[sym]) / bestBid[sym] * 10000
			}
			isArb := bestBid[sym] > 0 && bestAsk[sym] < 1e18 && bestBid[sym] > bestAsk[sym]
			netProfitBps := 0.0
			if isArb && bestAsk[sym] > 0 {
				netProfitBps = (bestBid[sym] - bestAsk[sym]) / bestAsk[sym] * 10000
			}
			cells = append(cells, &dashpb.SpreadMatrixReply_SpreadCell{
				Symbol:                sym,
				Bid:                   q.bid,
				Ask:                   q.ask,
				SpreadToBestAskBps:    spreadToBestAskBps,
				SpreadToBestBidBps:    spreadToBestBidBps,
				IsArbitrageable:       isArb,
				EstimatedNetProfitBps: netProfitBps,
			})
			if q.bid > bestBidVal {
				bestBidVal = q.bid
				bestBidBroker = brokerName
			}
			if q.ask < bestAskVal {
				bestAskVal = q.ask
				bestAskBroker = brokerName
			}
		}
		rows = append(rows, &dashpb.SpreadMatrixReply_BrokerRow{
			BrokerName:  brokerName,
			IsConnected: true,
			Cells:       cells,
		})
	}

	return &dashpb.SpreadMatrixReply{
		TimestampUnixMs: time.Now().UnixMilli(),
		Rows:            rows,
		BestBidBroker:   bestBidBroker,
		BestAskBroker:   bestAskBroker,
		TotalSymbols:    int32(len(symbols)),
	}
}

// buildPositionWatch constructs a position watch snapshot.
func (s *Server) buildPositionWatch(ctx context.Context) *dashpb.PositionWatchReply {
	s.mu.RLock()
	adapters := make(map[string]adapter.PlatformAdapter, len(s.adapters))
	for k, v := range s.adapters {
		adapters[k] = v
	}
	s.mu.RUnlock()
	brokers := make([]*dashpb.PositionWatchReply_BrokerPosition, 0, len(adapters))
	for name, a := range adapters {
		bp := &dashpb.PositionWatchReply_BrokerPosition{
			BrokerName:  name,
			IsConnected: true,
		}
		acct, err := a.AccountSummary(ctx)
		if err != nil {
			slog.Warn("dashboard accountSummary", "broker", name, "error", err)
			bp.IsConnected = false
		} else {
			bp.Equity = float64(acct.Equity.InexactFloat64())
			bp.Balance = float64(acct.Balance.InexactFloat64())
			bp.MarginUsed = float64(acct.Margin.InexactFloat64())
			bp.MarginFree = float64(acct.FreeMargin.InexactFloat64())
			bp.Currency = acct.Currency
			bp.Credit = acct.Credit
			bp.TotalFloatingPnl = acct.Profit
			bp.MarginLevelPct = acct.MarginLevel
			bp.Leverage = acct.Leverage
			bp.Platform = acct.Platform
			bp.Login = acct.Login
		}
		orders, err := a.OpenOrders(ctx)
		if err == nil {
			for _, o := range orders {
				side := "Buy"
				if o.Type == adapter.OpSell {
					side = "Sell"
				}
				bp.Positions = append(bp.Positions, &dashpb.PositionWatchReply_Position{
					Ticket:         o.Ticket,
					Symbol:         o.Symbol,
					Side:           side,
					Lots:           float64(o.Lots.InexactFloat64()),
					OpenPrice:      o.OpenPrice,
					StopLoss:       o.StopLoss,
					TakeProfit:     o.TakeProfit,
					FloatingPnl:    o.Profit,
					SwapAccrued:    o.Swap,
					Commission:     o.Commission,
					OpenTimeUnixMs: o.OpenTime.UnixMilli(),
					Comment:        o.Comment,
				})
			}
		}
		brokers = append(brokers, bp)
	}
	return &dashpb.PositionWatchReply{
		TimestampUnixMs: time.Now().UnixMilli(),
		BrokerPositions: brokers,
	}
}
