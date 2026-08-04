package adapter

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"arb/internal/bus"
	mt4 "arb/proto/gen/mtapi/mt4"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
)

// MT4Adapter connects to an MT4 broker via mtapi.io gRPC proxy.
// MT4 has no unified Events stream — uses separate OnQuote, OnOrderUpdate, OnOrderProfit.
type MT4Adapter struct {
	brokerName string
	host       string
	port       int32
	user       int64
	password   string
	server     string
	token      string

	conn     *grpc.ClientConn
	connMgr  mt4.ConnectionClient
	mt4      mt4.MT4Client
	trading  mt4.TradingClient
	streams  mt4.StreamsClient
	subs     mt4.SubscriptionsClient
	digits   map[string]int32

	rsm     *reconnectStateMachine
	execSem chan struct{}

	onReconnect func(ctx context.Context) error
}

// NewMT4Adapter creates a new MT4Adapter.
func NewMT4Adapter(brokerName, host, server string, port int32, user int64, password string, maxConcurrentOrders int) *MT4Adapter {
	a := &MT4Adapter{
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
		slog.Error("MT4 emergency close", "broker", brokerName)
	}))
	return a
}

func (a *MT4Adapter) BrokerName() string         { return a.brokerName }
func (a *MT4Adapter) Platform() bus.PlatformType { return bus.PlatformMT4 }

func (a *MT4Adapter) Connect(ctx context.Context) (string, error) {
	a.rsm.setState(stateConnecting)

	gateway := "mt4grpc3.mtapi.io:443"
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
		return "", fmt.Errorf("mt4 dial %s: %w", gateway, err)
	}
	a.conn = conn
	a.connMgr = mt4.NewConnectionClient(conn)
	a.mt4 = mt4.NewMT4Client(conn)
	a.trading = mt4.NewTradingClient(conn)
	a.streams = mt4.NewStreamsClient(conn)
	a.subs = mt4.NewSubscriptionsClient(conn)

	token, err := a.authenticate(ctx)
	if err != nil {
		a.rsm.setState(stateDisconnected)
		conn.Close()
		return "", fmt.Errorf("mt4 auth %s: %w", a.brokerName, err)
	}
	if token == "" {
		a.rsm.setState(stateDisconnected)
		conn.Close()
		return "", fmt.Errorf("mt4 auth %s: empty token", a.brokerName)
	}
	a.token = token
	a.rsm.setState(stateConnected)
	a.rsm.resetRetries()
	slog.Info("MT4 connected", "broker", a.brokerName, "gateway", gateway, "brokerHost", a.host)
	return token, nil
}

// withSessionMD returns a context with gRPC metadata headers set.
// All post-connect RPCs must use this — mtapi.io routes by the `id` header.
func (a *MT4Adapter) withSessionMD(ctx context.Context) context.Context {
	md := metadata.New(map[string]string{"id": a.token})
	return metadata.NewOutgoingContext(ctx, md)
}

func (a *MT4Adapter) authenticate(ctx context.Context) (string, error) {
	tempID := "mdgw-" + strconv.FormatInt(a.user, 10)
	loginCtx := metadata.NewOutgoingContext(ctx, metadata.New(map[string]string{"id": tempID}))
	if a.server != "" {
		resp, err := a.connMgr.ConnectEx(loginCtx, &mt4.ConnectExRequest{
			User:     int32(a.user),
			Password: a.password,
			Server:   a.server,
			Id:       &tempID,
		})
		if err != nil {
			return "", err
		}
		if resp.Error != nil && resp.Error.Code != mt4.ErrorCode_INTERNAL_ERROR {
			return "", fmt.Errorf("connectEx: %s", resp.Error.Message)
		}
		return resp.Result, nil
	}
	resp, err := a.connMgr.Connect(loginCtx, &mt4.ConnectRequest{
		User:     int32(a.user),
		Password: a.password,
		Host:     a.host,
		Port:     a.port,
		Id:       &tempID,
	})
	if err != nil {
		return "", err
	}
	if resp.Error != nil && resp.Error.Code != mt4.ErrorCode_INTERNAL_ERROR {
		return "", fmt.Errorf("connect: %s", resp.Error.Message)
	}
	return resp.Result, nil
}

func (a *MT4Adapter) Disconnect() error {
	a.rsm.setState(stateDisconnected)
	if a.conn != nil {
		return a.conn.Close()
	}
	return nil
}

func (a *MT4Adapter) HealthCheck(ctx context.Context) error {
	if !a.rsm.isConnected() {
		return ErrNotConnected
	}
	_, err := a.connMgr.CheckConnect(a.withSessionMD(ctx), &mt4.CheckConnectRequest{Id: a.token})
	return err
}

func (a *MT4Adapter) Subscribe(ctx context.Context, symbols []string) error {
	_, err := a.subs.SubscribeMany(a.withSessionMD(ctx), &mt4.SubscribeManyRequest{
		Id:      a.token,
		Symbols: symbols,
	})
	return err
}

// QuoteStream starts 3 independent goroutines for MT4's separate stream endpoints.
func (a *MT4Adapter) QuoteStream(ctx context.Context, b *bus.QuoteBus) {
	go a.runOnQuote(ctx, b)
	go a.runOnOrderUpdate(ctx)
	go a.runOnOrderProfit(ctx)
}

func (a *MT4Adapter) runOnQuote(ctx context.Context, b *bus.QuoteBus) {
	for {
		if err := ctx.Err(); err != nil {
			return
		}
		if !a.rsm.isConnected() {
			if err := a.reconnect(ctx); err != nil {
				return
			}
		}
		stream, err := a.streams.OnQuote(a.withSessionMD(ctx), &mt4.OnQuoteRequest{Id: a.token})
		if err != nil {
			slog.Warn("MT4 OnQuote error", "broker", a.brokerName, "error", err)
			if err := a.reconnect(ctx); err != nil {
				return
			}
			continue
		}
		for {
			msg, err := stream.Recv()
			if err != nil {
				slog.Warn("MT4 quote recv error", "broker", a.brokerName, "error", err)
				break
			}
			if msg.Error != nil {
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
				Platform: bus.PlatformMT4,
			})
		}
	}
}

func (a *MT4Adapter) runOnOrderUpdate(ctx context.Context) {
	for {
		if err := ctx.Err(); err != nil {
			return
		}
		if !a.rsm.isConnected() {
			if err := a.reconnect(ctx); err != nil {
				return
			}
		}
		stream, err := a.streams.OnOrderUpdate(a.withSessionMD(ctx), &mt4.OnOrderUpdateRequest{Id: a.token})
		if err != nil {
			if err := a.reconnect(ctx); err != nil {
				return
			}
			continue
		}
		for {
			_, err := stream.Recv()
			if err != nil {
				break
			}
		}
	}
}

func (a *MT4Adapter) runOnOrderProfit(ctx context.Context) {
	for {
		if err := ctx.Err(); err != nil {
			return
		}
		if !a.rsm.isConnected() {
			if err := a.reconnect(ctx); err != nil {
				return
			}
		}
		stream, err := a.streams.OnOrderProfit(a.withSessionMD(ctx), &mt4.OnOrderProfitRequest{Id: a.token})
		if err != nil {
			if err := a.reconnect(ctx); err != nil {
				return
			}
			continue
		}
		for {
			_, err := stream.Recv()
			if err != nil {
				break
			}
		}
	}
}

func (a *MT4Adapter) reconnect(ctx context.Context) error {
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
