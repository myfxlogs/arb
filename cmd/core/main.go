package main

import (
	"context"
	"flag"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"

	"arb/internal/adapter"
	"arb/internal/bus"
	config "arb/internal/config"
	"arb/internal/dashboard"
	"arb/internal/execute"
	"arb/internal/risk"
	"arb/internal/store"

	configpb "arb/proto/gen/config"
	dashpb "arb/proto/gen/dashboard"
)

func main() {
	configPath := flag.String("config", "config/default.textproto", "path to config file")
	flag.Parse()

	// Load config
	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("load config", "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		slog.Info("shutting down...")
		cancel()
	}()

	// 1. Collect all symbols
	allSymbols := collectSymbols(cfg)

	// 2. QuoteBus
	quoteBus := bus.New(allSymbols)

	// 3. Kill switch
	killSwitch := risk.NewKillSwitch("/tmp/arb_kill_switch")

	// 4. Circuit breaker
	breaker := risk.NewCircuitBreaker(
		cfg.Risk.MaxConsecutiveLosses,
		30*time.Second,
		cfg.Risk.DailyLossLimit,
		cfg.Risk.MaxWindowLoss,
	)

	// 5. Connect adapters
	adapters := make(map[string]adapter.PlatformAdapter)
	for _, bc := range cfg.Brokers {
		var a adapter.PlatformAdapter
		switch bc.Platform {
		case configpb.PlatformType_PLATFORM_TYPE_MT4:
			a = adapter.NewMT4Adapter(bc.Name, bc.Host, bc.Server, bc.Port, bc.User, bc.Password, int(cfg.Risk.MaxConcurrentOrders))
		case configpb.PlatformType_PLATFORM_TYPE_MT5:
			a = adapter.NewMT5Adapter(bc.Name, bc.Host, bc.Server, bc.Port, bc.User, bc.Password, int(cfg.Risk.MaxConcurrentOrders))
		default:
			slog.Warn("unknown platform", "broker", bc.Name, "platform", bc.Platform)
			continue
		}
		token, err := a.Connect(ctx)
		if err != nil {
			slog.Error("connect broker", "broker", bc.Name, "error", err)
			continue
		}
		slog.Info("broker connected", "broker", bc.Name, "token", token)
		adapters[bc.Name] = a

		// Subscribe to symbols
		symbols := allSymbols
		if err := a.Subscribe(ctx, symbols); err != nil {
			slog.Warn("subscribe", "broker", bc.Name, "error", err)
		}
		// Start quote stream
		go a.QuoteStream(ctx, quoteBus)
	}

	// 6. Store (optional — skip if no DSN)
	var st *store.Store
	if cfg.Database != nil && cfg.Database.Dsn != "" {
		st, err = store.New(ctx, cfg.Database.Dsn)
		if err != nil {
			slog.Warn("connect store", "error", err)
		} else {
			if err := st.EnsureMigrations(ctx); err != nil {
				slog.Warn("migrations", "error", err)
			}
			if err := st.EnsureCurrentPartitions(ctx); err != nil {
				slog.Warn("partitions", "error", err)
			}
		}
	}

	// 7. Execution pipeline
	dedup := execute.NewDedupCache()
	pipeline := execute.NewPipeline(execute.PipelineDeps{
		Bus:      quoteBus,
		Dedup:    dedup,
		Adapters: adapters,
	})
	_ = pipeline // used by strategy engine (Phase 8 integration)

	// 8. Dashboard gRPC server
	dashServer := dashboard.NewServer(dashboard.Deps{
		Bus:        quoteBus,
		Adapters:   adapters,
		Store:      st,
		KillSwitch: killSwitch,
		Breaker:    breaker,
		Symbols:    allSymbols,
	})
	dashServer.StartFeeder()

	lis, err := net.Listen("tcp", cfg.Dashboard.ListenAddress)
	if err != nil {
		slog.Error("listen", "error", err)
		os.Exit(1)
	}
	grpcServer := grpc.NewServer()
	dashpb.RegisterDashboardServiceServer(grpcServer, dashServer)

	slog.Info("dashboard listening", "addr", cfg.Dashboard.ListenAddress)
	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			slog.Error("grpc serve", "error", err)
		}
	}()

	<-ctx.Done()
	slog.Info("stopping gRPC server")
	grpcServer.GracefulStop()

	// Disconnect all adapters
	for name, a := range adapters {
		if err := a.Disconnect(); err != nil {
			slog.Warn("disconnect", "broker", name, "error", err)
		}
	}
	if st != nil {
		st.Close()
	}
	slog.Info("shutdown complete")
}

func collectSymbols(cfg *configpb.SystemConfig) []string {
	seen := make(map[string]bool)
	var symbols []string
	for _, sc := range cfg.Strategies {
		for _, s := range sc.SubscribedSymbols {
			if !seen[s] {
				seen[s] = true
				symbols = append(symbols, s)
			}
		}
	}
	if len(symbols) == 0 {
		symbols = []string{"EURUSD", "GBPUSD", "USDJPY"}
	}
	return symbols
}
