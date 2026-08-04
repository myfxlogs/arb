package main

import (
	"context"
	"crypto/tls"
	"flag"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/reflection"

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
	certFile := flag.String("cert", "", "TLS cert file (enables TLS when set)")
	keyFile := flag.String("key", "", "TLS key file")
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

	// 5. Store (optional — skip if no DSN)
	var st *store.Store
	dsn := cfg.Database.GetDsn()
	if envDSN := os.Getenv("DB_DSN"); envDSN != "" {
		dsn = envDSN
	}
	if dsn != "" {
		st, err = store.New(ctx, dsn)
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

	// 6. Connect adapters from textproto config
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

	// 6b. Load broker accounts from database and connect
	if st != nil {
		dbBrokers, dbErr := st.ListBrokerAccounts(ctx)
		if dbErr != nil {
			slog.Warn("list db broker accounts", "error", dbErr)
		}
		for _, db := range dbBrokers {
			if _, exists := adapters[db.Name]; exists {
				continue
			}
			var a adapter.PlatformAdapter
			switch db.Platform {
			case 0: // MT4
				a = adapter.NewMT4Adapter(db.Name, db.Host, db.Server, db.Port, db.Login, db.Password, int(cfg.Risk.MaxConcurrentOrders))
			case 1: // MT5
				a = adapter.NewMT5Adapter(db.Name, db.Host, db.Server, db.Port, db.Login, db.Password, int(cfg.Risk.MaxConcurrentOrders))
			default:
				slog.Warn("unknown platform in db broker", "broker", db.Name, "platform", db.Platform)
				continue
			}
			token, connErr := a.Connect(ctx)
			if connErr != nil {
				slog.Error("connect db broker", "broker", db.Name, "error", connErr)
				continue
			}
			slog.Info("db broker connected", "broker", db.Name, "token", token)
			adapters[db.Name] = a
			if err := a.Subscribe(ctx, allSymbols); err != nil {
				slog.Warn("subscribe db broker", "broker", db.Name, "error", err)
			}
			go a.QuoteStream(ctx, quoteBus)
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
		Bus:                 quoteBus,
		Adapters:            adapters,
		Store:               st,
		KillSwitch:          killSwitch,
		Breaker:             breaker,
		Symbols:             allSymbols,
		MaxConcurrentOrders: int(cfg.Risk.MaxConcurrentOrders),
	})
	dashServer.StartFeeder()

	lis, err := net.Listen("tcp", cfg.Dashboard.ListenAddress)
	if err != nil {
		slog.Error("listen", "error", err)
		os.Exit(1)
	}
	var grpcServer *grpc.Server
	if *certFile != "" && *keyFile != "" {
		tlsCfg, err := loadTLSConfig(*certFile, *keyFile)
		if err != nil {
			slog.Error("load TLS cert", "error", err)
			os.Exit(1)
		}
		creds := credentials.NewTLS(tlsCfg)
		grpcServer = grpc.NewServer(grpc.Creds(creds))
		slog.Info("gRPC server with TLS")
	} else {
		grpcServer = grpc.NewServer()
		slog.Info("gRPC server without TLS")
	}
	dashpb.RegisterDashboardServiceServer(grpcServer, dashServer)
	reflection.Register(grpcServer)

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

func loadTLSConfig(certFile, keyFile string) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, err
	}
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{"h2"},
		MinVersion:   tls.VersionTLS12,
	}, nil
}
