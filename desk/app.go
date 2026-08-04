package desk

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	dashpb "arb/proto/gen/dashboard"
)

// App is the Wails desktop application service.
// It holds the gRPC client to core and streams real-time data to the frontend.
type App struct {
	app    *application.App
	client dashpb.DashboardServiceClient
	ctx    context.Context
	cancel context.CancelFunc
}

// NewApp creates a new desk application connected to the core gRPC server.
func NewApp(addr string) (*App, error) {
	var opts []grpc.DialOption
	if isLocal(addr) {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	} else {
		creds := credentials.NewTLS(&tls.Config{
			ServerName: hostFromAddr(addr),
		})
		opts = append(opts, grpc.WithTransportCredentials(creds))
	}
	conn, err := grpc.Dial(addr, opts...)
	if err != nil {
		return nil, fmt.Errorf("dial core %s: %w", addr, err)
	}
	client := dashpb.NewDashboardServiceClient(conn)
	ctx, cancel := context.WithCancel(context.Background())
	return &App{
		client: client,
		ctx:    ctx,
		cancel: cancel,
	}, nil
}

// ServiceStartup is called by Wails when the service starts.
// The app reference is stored for emitting events to the frontend.
func (a *App) ServiceStartup(ctx context.Context, options application.ServiceOptions) {
	// app reference is set via SetApp before Run
}

// SetApp stores the Wails application reference and starts stream goroutines.
func (a *App) SetApp(app *application.App) {
	a.app = app
	go a.streamSpreadMatrix()
	go a.streamPositionWatch()
}

// ServiceShutdown is called by Wails when the service stops.
func (a *App) ServiceShutdown() {
	if a.cancel != nil {
		a.cancel()
	}
}

// === Stream goroutines (Go → Wails Events → Svelte) ===

func (a *App) streamSpreadMatrix() {
	for {
		if a.ctx.Err() != nil {
			return
		}
		stream, err := a.client.SpreadMatrix(a.ctx, &dashpb.SpreadMatrixRequest{
			RefreshIntervalMs: 200,
		})
		if err != nil {
			slog.Error("matrix stream", "error", err)
			time.Sleep(2 * time.Second)
			continue
		}
		for {
			reply, err := stream.Recv()
			if err != nil {
				slog.Warn("matrix recv", "error", err)
				time.Sleep(time.Second)
				break
			}
			if a.app != nil {
				a.app.Event.Emit("spread-matrix", reply)
			}
		}
	}
}

func (a *App) streamPositionWatch() {
	for {
		if a.ctx.Err() != nil {
			return
		}
		stream, err := a.client.PositionWatch(a.ctx, &dashpb.PositionWatchRequest{
			RefreshIntervalMs: 500,
		})
		if err != nil {
			slog.Error("positions stream", "error", err)
			time.Sleep(2 * time.Second)
			continue
		}
		for {
			reply, err := stream.Recv()
			if err != nil {
				slog.Warn("positions recv", "error", err)
				time.Sleep(time.Second)
				break
			}
			if a.app != nil {
				a.app.Event.Emit("positions", reply)
			}
		}
	}
}

// === Wails service methods (frontend callable via bindings) ===

// SubmitOrder sends a manual order to core.
func (a *App) SubmitOrder(req *dashpb.ManualOrderRequest) (*dashpb.ManualOrderReply, error) {
	return a.client.SubmitOrder(a.ctx, req)
}

// ClosePosition closes a position.
func (a *App) ClosePosition(req *dashpb.ClosePositionRequest) (*dashpb.ClosePositionReply, error) {
	return a.client.ClosePosition(a.ctx, req)
}

// CancelOrder cancels a pending order.
func (a *App) CancelOrder(req *dashpb.CancelOrderRequest) (*dashpb.CancelOrderReply, error) {
	return a.client.CancelOrder(a.ctx, req)
}

// GetSignalHistory queries historical signals.
func (a *App) GetSignalHistory(req *dashpb.SignalHistoryRequest) (*dashpb.SignalHistoryReply, error) {
	return a.client.GetSignalHistory(a.ctx, req)
}

// GetOrderHistory queries historical orders.
func (a *App) GetOrderHistory(req *dashpb.OrderHistoryRequest) (*dashpb.OrderHistoryReply, error) {
	return a.client.GetOrderHistory(a.ctx, req)
}

// GetDailySummary queries daily PnL summary.
func (a *App) GetDailySummary(req *dashpb.DailySummaryRequest) (*dashpb.DailySummaryReply, error) {
	return a.client.GetDailySummary(a.ctx, req)
}

// GetAccountSnapshots returns all broker account snapshots.
func (a *App) GetAccountSnapshots(req *dashpb.AccountSnapshotRequest) (*dashpb.AccountSnapshotReply, error) {
	return a.client.GetAccountSnapshots(a.ctx, req)
}

// GetStrategyStatus returns strategy statuses.
func (a *App) GetStrategyStatus(req *dashpb.StrategyStatusRequest) (*dashpb.StrategyStatusReply, error) {
	return a.client.GetStrategyStatus(a.ctx, req)
}

// ToggleStrategy enables/disables a strategy.
func (a *App) ToggleStrategy(req *dashpb.ToggleStrategyRequest) (*dashpb.ToggleStrategyReply, error) {
	return a.client.ToggleStrategy(a.ctx, req)
}

// ResumeStrategy resumes a strategy after circuit breaker.
func (a *App) ResumeStrategy(req *dashpb.ResumeStrategyRequest) (*dashpb.ResumeStrategyReply, error) {
	return a.client.ResumeStrategy(a.ctx, req)
}

// ResetGlobalCircuitBreaker resets the global circuit breaker.
func (a *App) ResetGlobalCircuitBreaker(req *dashpb.ResetCBRequest) (*dashpb.ResetCBReply, error) {
	return a.client.ResetGlobalCircuitBreaker(a.ctx, req)
}

// GetKillSwitchStatus returns kill switch status.
func (a *App) GetKillSwitchStatus(req *dashpb.KillSwitchStatusRequest) (*dashpb.KillSwitchStatusReply, error) {
	return a.client.GetKillSwitchStatus(a.ctx, req)
}

// Kill triggers the global kill switch.
func (a *App) Kill(req *dashpb.KillRequest) (*dashpb.KillReply, error) {
	return a.client.Kill(a.ctx, req)
}

// Resume recovers from kill switch.
func (a *App) Resume(req *dashpb.ResumeRequest) (*dashpb.ResumeReply, error) {
	return a.client.Resume(a.ctx, req)
}

// SearchBroker searches for brokers by company name.
func (a *App) SearchBroker(req *dashpb.SearchBrokerRequest) (*dashpb.SearchBrokerReply, error) {
	return a.client.SearchBroker(a.ctx, req)
}

// AddBroker adds a new broker connection.
func (a *App) AddBroker(req *dashpb.AddBrokerRequest) (*dashpb.AddBrokerReply, error) {
	return a.client.AddBroker(a.ctx, req)
}

// RemoveBroker removes a broker connection.
func (a *App) RemoveBroker(req *dashpb.RemoveBrokerRequest) (*dashpb.RemoveBrokerReply, error) {
	return a.client.RemoveBroker(a.ctx, req)
}

// SubscribeSymbols subscribes to symbols.
func (a *App) SubscribeSymbols(req *dashpb.SubscribeSymbolsRequest) (*dashpb.SubscribeSymbolsReply, error) {
	return a.client.SubscribeSymbols(a.ctx, req)
}

// UnsubscribeSymbols unsubscribes from symbols.
func (a *App) UnsubscribeSymbols(req *dashpb.UnsubscribeSymbolsRequest) (*dashpb.UnsubscribeSymbolsReply, error) {
	return a.client.UnsubscribeSymbols(a.ctx, req)
}

// ListSubscribedSymbols lists subscribed symbols.
func (a *App) ListSubscribedSymbols(req *dashpb.ListSymbolsRequest) (*dashpb.ListSymbolsReply, error) {
	return a.client.ListSubscribedSymbols(a.ctx, req)
}

// === helpers ===

func isLocal(addr string) bool {
	return strings.HasPrefix(addr, "localhost") || strings.HasPrefix(addr, "127.")
}

func hostFromAddr(addr string) string {
	host, _, ok := strings.Cut(addr, ":")
	if !ok {
		return addr
	}
	return host
}
