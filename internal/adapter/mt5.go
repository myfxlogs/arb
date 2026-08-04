package adapter

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"arb/internal/bus"
	mt5 "arb/proto/gen/mtapi/mt5"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
)

// MT5Adapter connects to an MT5 broker via mtapi.io gRPC proxy.
type MT5Adapter struct {
	brokerName string
	host       string
	port       int32
	user       int64
	password   string
	server     string
	token      string

	conn     *grpc.ClientConn
	connMgr  mt5.ConnectionClient
	mt5      mt5.MT5Client
	trading  mt5.TradingClient
	streams  mt5.StreamsClient
	subs     mt5.SubscriptionsClient
	digits   map[string]int32

	rsm     *reconnectStateMachine
	execSem chan struct{}

	// onReconnect is called after a successful reconnection to re-subscribe.
	onReconnect func(ctx context.Context) error
}

// NewMT5Adapter creates a new MT5Adapter. maxConcurrentOrders controls the
// channel semaphore for concurrent order execution.
func NewMT5Adapter(brokerName, host, server string, port int32, user int64, password string, maxConcurrentOrders int) *MT5Adapter {
	a := &MT5Adapter{
		brokerName: brokerName,
		host:       host,
		port:       port,
		user:       user,
		password:   password,
		server:     server,
		digits:     make(map[string]int32),
		execSem:    make(chan struct{}, maxConcurrentOrders),
	}
	a.rsm = newReconnectStateMachine(defaultReconnectConfig(func() {
		slog.Error("MT5 emergency close", "broker", brokerName)
	}))
	return a
}

func (a *MT5Adapter) BrokerName() string         { return a.brokerName }
func (a *MT5Adapter) Platform() bus.PlatformType { return bus.PlatformMT5 }

func (a *MT5Adapter) Connect(ctx context.Context) (string, error) {
	a.rsm.setState(stateConnecting)

	gateway := "mt5grpc3.mtapi.io:443"
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS13}
	conn, err := grpc.DialContext(ctx, gateway,
		grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg)),
		grpc.WithBlock(),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(16*1024*1024)),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                30 * time.Second,
			Timeout:             10 * time.Second,
			PermitWithoutStream: true,
		}),
	)
	if err != nil {
		a.rsm.setState(stateDisconnected)
		return "", fmt.Errorf("mt5 dial %s: %w", gateway, err)
	}
	a.conn = conn
	a.connMgr = mt5.NewConnectionClient(conn)
	a.mt5 = mt5.NewMT5Client(conn)
	a.trading = mt5.NewTradingClient(conn)
	a.streams = mt5.NewStreamsClient(conn)
	a.subs = mt5.NewSubscriptionsClient(conn)

	token, err := a.authenticate(ctx)
	if err != nil {
		a.rsm.setState(stateDisconnected)
		conn.Close()
		return "", fmt.Errorf("mt5 auth %s: %w", a.brokerName, err)
	}
	if token == "" {
		a.rsm.setState(stateDisconnected)
		conn.Close()
		return "", fmt.Errorf("mt5 auth %s: empty token", a.brokerName)
	}
	a.token = token
	a.rsm.setState(stateConnected)
	a.rsm.resetRetries()
	slog.Info("MT5 connected", "broker", a.brokerName, "gateway", gateway, "brokerHost", a.host)
	return token, nil
}

// withSessionMD returns a context with gRPC metadata headers set.
// All post-connect RPCs must use this — mtapi.io routes by the `id` header.
func (a *MT5Adapter) withSessionMD(ctx context.Context) context.Context {
	md := metadata.New(map[string]string{"id": a.token})
	return metadata.NewOutgoingContext(ctx, md)
}

// authenticate calls Connect or ConnectEx depending on whether a server name is set.
func (a *MT5Adapter) authenticate(ctx context.Context) (string, error) {
	tempID := "mdgw-" + strconv.FormatInt(a.user, 10)
	loginCtx := metadata.NewOutgoingContext(ctx, metadata.New(map[string]string{"id": tempID}))
	if a.server != "" {
		resp, err := a.connMgr.ConnectEx(loginCtx, &mt5.ConnectExRequest{
			User:     uint64(a.user),
			Password: a.password,
			Server:   a.server,
			Id:       &tempID,
		})
		if err != nil {
			return "", err
		}
		if resp.Error != nil && resp.Error.Code != mt5.ErrorCode_DONE {
			return "", fmt.Errorf("connectEx: %s", resp.Error.Message)
		}
		return resp.Result, nil
	}
	resp, err := a.connMgr.Connect(loginCtx, &mt5.ConnectRequest{
		User:     uint64(a.user),
		Password: a.password,
		Host:     a.host,
		Port:     a.port,
		Id:       &tempID,
	})
	if err != nil {
		return "", err
	}
	if resp.Error != nil && resp.Error.Code != mt5.ErrorCode_DONE {
		return "", fmt.Errorf("connect: %s", resp.Error.Message)
	}
	return resp.Result, nil
}

// Disconnect closes the gRPC connection.
func (a *MT5Adapter) Disconnect() error {
	a.rsm.setState(stateDisconnected)
	if a.conn != nil {
		return a.conn.Close()
	}
	return nil
}

func (a *MT5Adapter) HealthCheck(ctx context.Context) error {
	if !a.rsm.isConnected() {
		return ErrNotConnected
	}
	_, err := a.connMgr.CheckConnect(a.withSessionMD(ctx), &mt5.CheckConnectRequest{Id: a.token})
	return err
}

// Subscribe subscribes to real-time quotes for the given symbols.
func (a *MT5Adapter) Subscribe(ctx context.Context, symbols []string) error {
	for _, sym := range symbols {
		_, err := a.subs.Subscribe(a.withSessionMD(ctx), &mt5.SubscribeRequest{
			Id:     a.token,
			Symbol: sym,
		})
		if err != nil {
			return fmt.Errorf("subscribe %s: %w", sym, err)
		}
	}
	return nil
}

// QuoteStream starts the OnQuote stream and publishes quotes to the bus.
// On stream error, triggers reconnection.
func (a *MT5Adapter) QuoteStream(ctx context.Context, b *bus.QuoteBus) {
	for {
		if err := ctx.Err(); err != nil {
			return
		}
		if !a.rsm.isConnected() {
			if err := a.reconnect(ctx); err != nil {
				return
			}
		}
		stream, err := a.streams.OnQuote(a.withSessionMD(ctx), &mt5.OnQuoteRequest{Id: a.token})
		if err != nil {
			slog.Warn("MT5 OnQuote stream error", "broker", a.brokerName, "error", err)
			if err := a.reconnect(ctx); err != nil {
				return
			}
			continue
		}
		a.recvQuoteLoop(ctx, b, stream)
	}
}

// recvQuoteLoop reads quotes from the stream and publishes to the bus.
func (a *MT5Adapter) recvQuoteLoop(ctx context.Context, b *bus.QuoteBus, stream mt5.Streams_OnQuoteClient) {
	for {
		msg, err := stream.Recv()
		if err != nil {
			slog.Warn("MT5 quote recv error", "broker", a.brokerName, "error", err)
			return
		}
		if msg.Error != nil {
			slog.Warn("MT5 quote error", "broker", a.brokerName, "error", msg.Error.Message)
			continue
		}
		q := msg.Result
		if q == nil {
			continue
		}
		b.Publish(bus.Quote{
			Symbol:   q.Symbol,
			Bid:      q.Bid,
			Ask:      q.Ask,
			Time:     q.Time.AsTime(),
			Broker:   a.brokerName,
			Platform: bus.PlatformMT5,
		})
	}
}

// reconnect runs the reconnection state machine.
func (a *MT5Adapter) reconnect(ctx context.Context) error {
	connectFn := func() error {
		if a.conn != nil {
			a.conn.Close()
		}
		_, err := a.Connect(ctx)
		if err != nil {
			return err
		}
		if a.onReconnect != nil {
			return a.onReconnect(ctx)
		}
		return nil
	}
	return a.rsm.reconnectLoop(ctx, connectFn)
}
