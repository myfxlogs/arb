# 全网套利系统 — 实现规范

> 对应评估框架全部 73 项，约束文档全部规则  
> 两个二进制：`core`（守护进程）+ `desk`（Fyne 桌面）  
> 本文件是实现的唯一权威参考

---

## 1. 项目结构

```
arb/
├── cmd/
│   ├── core/
│   │   └── main.go                    # 守护进程入口
│   └── desk/
│       └── main.go                    # Fyne 桌面应用入口
├── internal/
│   ├── adapter/
│   │   ├── adapter.go                 # PlatformAdapter 接口
│   │   ├── mt5.go                     # MT5Adapter 实现
│   │   ├── mt4.go                     # MT4Adapter 实现
│   │   └── reconnect.go              # 重连状态机
│   ├── bus/
│   │   └── quote_bus.go              # QuoteBus
│   ├── engine/
│   │   ├── engine.go                  # Strategy 接口 + Engine 调度
│   │   ├── triangular.go             # 三角套利
│   │   ├── cross_exchange.go         # 跨所套利
│   │   └── statistical.go            # 统计套利
│   ├── execute/
│   │   ├── pipeline.go               # 执行管线（4-phase）
│   │   ├── executor.go               # OrderExecutor（channel 信号量）
│   │   └── idempotency.go            # ClientID 幂等去重
│   ├── risk/
│   │   ├── gate.go                    # CapitalGate
│   │   ├── limiter.go                # AdaptiveRateLimiter
│   │   ├── circuit_breaker.go        # 策略级 + 全局熔断
│   │   └── kill_switch.go            # Kill Switch
│   ├── dashboard/
│   │   ├── server.go                  # DashboardService gRPC server
│   │   ├── matrix.go                  # 价差矩阵计算
│   │   └── position.go               # 持仓聚合
│   ├── store/
│   │   ├── pg.go                      # PostgreSQL 连接池
│   │   ├── ticks.go                   # tick 写入 + 查询
│   │   ├── signals.go                # 信号记录
│   │   ├── orders.go                  # 订单同步
│   │   └── daily.go                   # 日汇总
│   ├── audit/
│   │   └── audit.go                   # 审计日志（protobuf 追加写）
│   ├── decimalutil/
│   │   └── decimalutil.go            # float64↔decimal 统一转换
│   └── errclass/
│       └── errclass.go               # MT4/MT5 错误码分类
├── desk/
│   ├── app.go                         # Fyne 应用初始化 + Tab 容器
│   ├── matrix/
│   │   └── matrix.go                  # 价差矩阵 Tab
│   ├── positions/
│   │   └── positions.go              # 持仓 Tab
│   ├── trading/
│   │   └── trading.go                # 交易 Tab
│   └── history/
│       └── history.go                # 历史查询 Tab
├── proto/
│   └── dashboard/
│       └── dashboard.proto           # DashboardService 定义
├── config/
│   └── default.textproto              # 默认配置（protobuf text format）
├── docs/
│   ├── evaluation-framework.md
│   ├── constraints.md
│   └── implementation.md              # 本文件
├── go.mod
├── go.sum
├── Dockerfile.core
└── Makefile
```

---

## 2. 核心类型

```go
// internal/bus/types.go

type PlatformType int32
const (
    PlatformMT4 PlatformType = iota
    PlatformMT5
    PlatformBinance
)

type Quote struct {
    Symbol   string
    Bid      float64       // Hot Path 直接使用
    Ask      float64
    Time     time.Time     // 服务器时间戳
    Broker   string
    Platform PlatformType
}

type Account struct {
    Balance    decimal.Decimal
    Equity     decimal.Decimal
    Margin     decimal.Decimal
    FreeMargin decimal.Decimal
    Currency   string
}
```

---

## 3. decimalutil

```go
package decimalutil

import (
    "strconv"
    "github.com/shopspring/decimal"
)

func FromFloat64(f float64, digits int32) decimal.Decimal {
    s := strconv.FormatFloat(f, 'f', int(digits), 64)
    d, _ := decimal.NewFromString(s)
    return d
}

func ToFloat64(d decimal.Decimal) float64 {
    f, _ := strconv.ParseFloat(d.String(), 64)
    return f
}

func FromString(s string) (decimal.Decimal, error) {
    return decimal.NewFromString(s)
}
```

---

## 4. QuoteBus

```go
package bus

type QuoteBus struct {
    mu          sync.RWMutex
    subscribers map[string][]chan<- Quote
}

func New(symbols []string) *QuoteBus {
    return &QuoteBus{
        subscribers: make(map[string][]chan<- Quote, len(symbols)),
    }
}

func (b *QuoteBus) Subscribe(symbol string) (<-chan Quote, func()) {
    ch := make(chan Quote, 1)
    b.mu.Lock()
    b.subscribers[symbol] = append(b.subscribers[symbol], ch)
    b.mu.Unlock()
    return ch, func() {
        b.mu.Lock()
        subs := b.subscribers[symbol]
        for i, c := range subs {
            if c == ch {
                b.subscribers[symbol] = append(subs[:i], subs[i+1:]...)
                return
            }
        }
        b.mu.Unlock()
    }
}

// Publish — drain-then-replace。cap=1，始终保留最新 tick。
func (b *QuoteBus) Publish(q Quote) {
    b.mu.RLock()
    chs := b.subscribers[q.Symbol]
    b.mu.RUnlock()
    for _, ch := range chs {
        select {
        case ch <- q:
        default:
            select { case <-ch: default: }
            select { case ch <- q: default: }
        }
    }
}

// Snapshot 返回所有 symbol 的最新行情快照（供 Dashboard 和预成交校验使用）
func (b *QuoteBus) Snapshot(ctx context.Context, symbols []string) map[string]Quote {
    result := make(map[string]Quote, len(symbols))
    for _, sym := range symbols {
        ch, cancel := b.Subscribe(sym)
        select {
        case q := <-ch:
            result[sym] = q
        case <-ctx.Done():
            return result
        }
        cancel()
    }
    return result
}
```

---

## 5. PlatformAdapter

```go
package adapter

type PlatformAdapter interface {
    Connect(ctx context.Context) (token string, err error)
    Disconnect() error
    HealthCheck(ctx context.Context) error

    Subscribe(ctx context.Context, symbols []string) error
    QuoteStream(ctx context.Context, bus *bus.QuoteBus)

    AccountSummary(ctx context.Context) (*Account, error)
    OpenOrders(ctx context.Context) ([]Order, error)
    AllSymbols(ctx context.Context) ([]string, error)
    SymbolDigits(ctx context.Context, symbols []string) (map[string]int32, error)

    PlaceOrder(ctx context.Context, req OrderRequest) (*OrderResult, error)
    CancelOrder(ctx context.Context, ticket int64) error
    CloseOrder(ctx context.Context, ticket int64, lots decimal.Decimal, price float64, slippage int32) (*OrderResult, error)

    Platform() PlatformType
    BrokerName() string
}

type OrderRequest struct {
    ClientID  string
    Symbol    string
    Operation OrderOperation
    Volume    decimal.Decimal
    Price     float64
    Slippage  int32
}

type OrderOperation int32
const (
    OpBuy  OrderOperation = iota
    OpSell
)

type OrderResult struct {
    ClientID    string
    Ticket      int64
    Symbol      string
    Operation   OrderOperation
    State       OrderState
    Volume      decimal.Decimal
    CloseVolume decimal.Decimal
    Error       error
}

type OrderState int32
const (
    StateFilled  OrderState = iota
    StatePartial
    StateRejected
    StateUnknown
)

func (r *OrderResult) IsFullFill() bool {
    return r.State == StateFilled && r.CloseVolume.Equal(r.Volume)
}

type Order struct {
    Ticket   int64
    Symbol   string
    Type     OrderOperation
    Lots     decimal.Decimal
    State    OrderState
    Comment  string
}
```

---

## 6. MT5Adapter

```go
type MT5Adapter struct {
    brokerName  string
    host        string
    port        int32
    user        int64
    password    string
    token       string
    conn        *grpc.ClientConn
    client      mt5grpc.MT5Client
    trading     mt5grpc.TradingClient
    streams     mt5grpc.StreamsClient
    subs        mt5grpc.SubscriptionsClient
    digits      map[string]int32
    connState   atomic.Int32
    dedup       *DedupCache
    execLimiter chan struct{}
    mu          sync.RWMutex
}

func (a *MT5Adapter) Connect(ctx context.Context) (string, error) {
    // 铁律：gRPC Dial 目标永远是 mtapi 网关，不是 broker IP。
    // ConnectRequest.Host 填 broker 的真实地址。
    // 二者永远不同：Dial → mtapi，Host → broker。
    gateway := "mt5grpc3.mtapi.io:443"
    config := &tls.Config{MinVersion: tls.VersionTLS13}
    conn, err := grpc.DialContext(ctx, gateway,
        grpc.WithKeepaliveParams(keepalive.ClientParameters{
            Time: 30 * time.Second, Timeout: 10 * time.Second, PermitWithoutStream: true,
        }),
    )
    // ... connect, load symbols, etc.
}

func (a *MT5Adapter) QuoteStream(ctx context.Context, bus *QuoteBus) {
    stream, _ := a.streams.OnQuote(ctx, &mt5grpc.OnQuoteRequest{Id: a.token})
    for {
        msg, err := stream.Recv()
        if err != nil {
            a.reconnect(ctx, bus)
            return
        }
        bus.Publish(Quote{
            Symbol: msg.Result.Symbol, Bid: msg.Result.Bid, Ask: msg.Result.Ask,
            Time: msg.Result.Time.AsTime(), Broker: a.brokerName, Platform: PlatformMT5,
        })
    }
}

func (a *MT5Adapter) PlaceOrder(ctx context.Context, req OrderRequest) (*OrderResult, error) {
    if cached, ok := a.dedup.Get(req.ClientID); ok {
        return cached, nil
    }

    select {
    case a.execLimiter <- struct{}{}:
        defer func() { <-a.execLimiter }()
    case <-ctx.Done():
        return nil, ctx.Err()
    }

    resp, err := a.trading.OrderSend(ctx, &mt5grpc.OrderSendRequest{
        Id: a.token, Symbol: req.Symbol,
        Operation: toMT5Op(req.Operation),
        Volume:    decimalutil.ToFloat64(req.Volume),
        Slippage:  proto.Uint64(uint64(req.Slippage)),
        Comment:   proto.String(req.ClientID),
    })
    // ... error handling, convert result
}
```

---

## 7. MT4Adapter（关键差异）

```go
type MT4Adapter struct {
    // gRPC endpoint: mt4grpc3.mtapi.io
    // 无 Events 统一流 → 3 个独立 goroutine (OnQuote, OnOrderUpdate, OnOrderProfit)
    // Ticket: int32 → int64
    // Order 响应无 State → Ticket != 0 = 成交
    // 有 OrderDelete (MT5 没有)
    // Balance/Credit 订单: Op_Balance(6) / Op_Credit(7) — 无 symbol，必须特殊处理
}

func (a *MT4Adapter) PlaceOrder(ctx context.Context, req OrderRequest) (*OrderResult, error) {
    resp, err := a.trading.OrderSend(ctx, &mt4grpc.OrderSendRequest{
        Id: a.token, Symbol: req.Symbol,
        Operation: toMT4Op(req.Operation),
        Volume:    decimalutil.ToFloat64(req.Volume),
        Slippage:  req.Slippage,
        Comment:   req.ClientID,
    })
    result := &OrderResult{Ticket: int64(resp.Result.Ticket), ClientID: req.ClientID}
    if result.Ticket != 0 {
        result.State = StateFilled
        result.CloseVolume = result.Volume
    }
    return result, nil
}

func (a *MT4Adapter) QuoteStream(ctx context.Context, bus *QuoteBus) {
    go a.runOnQuote(ctx, bus)
    go a.runOnOrderUpdate(ctx)
    go a.runOnOrderProfit(ctx)
}

// mapOrderType 处理 MT4 特殊订单类型。
// Op_Balance(6) 和 Op_Credit(7) 是入金/出金/利息操作，没有 symbol。
// 必须特殊处理，否则会被当作普通交易导致 symbol 为空的 panic。
func toMT4Op(op OrderOperation) mt4grpc.Op {
    switch op {
    case OpBuy:  return mt4grpc.Op_Buy
    case OpSell: return mt4grpc.Op_Sell
    default:     return mt4grpc.Op_Buy
    }
}

// classifyMT4OrderType 在读取订单历史时识别 Balance/Credit 类型。
func classifyMT4OrderType(t mt4grpc.Op) string {
    switch t {
    case mt4grpc.Op_Balance: return "BALANCE"
    case mt4grpc.Op_Credit:  return "CREDIT"
    case mt4grpc.Op_Buy:     return "BUY"
    case mt4grpc.Op_Sell:    return "SELL"
    default:                 return "UNKNOWN"
    }
}
```

### 7.1 MT5 Balance/Credit 处理

MT5 proto 定义了 `OrderType_Balance = 100` 和 `OrderType_Credit = 101`。
读取 OpenedOrders 时，这些订单没有 symbol 字段。施工 agent 必须处理：

```go
func classifyMT5OrderType(t mt5grpc.OrderType) string {
    switch t {
    case mt5grpc.OrderType_Balance: return "BALANCE"
    case mt5grpc.OrderType_Credit:  return "CREDIT"
    default:                        return "TRADE"
    }
}

func safeSymbol(o *mt5grpc.Order) string {
    if o.Symbol == "" {
        return "" // Balance/Credit 订单无 symbol，合法
    }
    return o.Symbol
}
```

---

## 8. 执行管线

```go
package execute

func (p *ExecutionPipeline) Execute(ctx context.Context, opp ArbitrageOpportunity) error {
    // Phase 1: Pre-trade revalidation
    if err := p.revalidate(ctx, opp); err != nil {
        return err
    }
    // Phase 1.5: Capital gate
    if err := p.gate.Allow(opp); err != nil {
        return err
    }
    // Phase 2: Concurrent submit
    ctx, cancel := context.WithTimeout(ctx, opp.Params.OrderTimeout)
    defer cancel()
    results := make(chan LegResult, len(opp.Legs))
    for _, leg := range opp.Legs {
        go func(l Leg) { results <- p.executeLeg(ctx, l) }(leg)
    }
    // Phase 3: Collect — all or nothing
    var filled, failed []LegResult
    for i := 0; i < len(opp.Legs); i++ {
        select {
        case r := <-results:
            if r.IsFullFill() { filled = append(filled, r) }
            else { failed = append(failed, r) }
        case <-ctx.Done():
            failed = append(failed, LegResult{Error: ErrOrderTimeout})
        }
    }
    // Phase 4: Hedge on failure
    if len(failed) > 0 {
        for _, leg := range filled { p.hedge(ctx, leg) }
        for _, leg := range failed { p.cancel(ctx, leg) }
        return fmt.Errorf("arb failed: %d/%d filled", len(filled), len(opp.Legs))
    }
    return nil
}
```

---

## 9. 资金门禁 + 熔断 + Kill Switch

```go
package risk

type CapitalGate struct {
    maxNotionalPerTrade  decimal.Decimal
    maxExposurePerSymbol map[string]decimal.Decimal
    currentExposure      map[string]decimal.Decimal
    rateLimiter          *AdaptiveRateLimiter
    mu                   sync.RWMutex
}

type StrategyCircuitBreaker struct {
    consecutiveLosses    int
    maxConsecutiveLosses int
    windowPnL            decimal.Decimal
    maxWindowLoss        decimal.Decimal
    state                atomic.Int32
    mu                   sync.Mutex
}

type GlobalCircuitBreaker struct {
    dailyPnL       decimal.Decimal
    dailyLossLimit decimal.Decimal
    maxDrawdown    decimal.Decimal
    peakEquity     decimal.Decimal
    state          atomic.Int32
    mu             sync.Mutex
}

var KillSwitch atomic.Bool

// Kill 持久化到 .kill_switch 文件
func Kill() {
    KillSwitch.Store(true)
    os.WriteFile(".kill_switch", []byte("1"), 0o600)
}

// IsKilled 启动时也检查持久化文件
func IsKilled() bool {
    if KillSwitch.Load() { return true }
    if _, err := os.Stat(".kill_switch"); err == nil {
        KillSwitch.Store(true)
        return true
    }
    return false
}
```

---

## 10. 错误码分类

```go
package errclass

type Action int
const (
    Retry      Action = iota
    RetryFresh
    Abort
    Halt
)

func ClassifyMT5(code mt5grpc.ErrorCode) Action {
    switch code {
    case mt5grpc.ErrorCode_REQUOTE, mt5grpc.ErrorCode_PRICE_CHANGED:
        return RetryFresh
    case mt5grpc.ErrorCode_OFF_QUOTES, mt5grpc.ErrorCode_NO_PRICES:
        return Retry
    case mt5grpc.ErrorCode_MARKET_CLOSED, mt5grpc.ErrorCode_TRADE_DISABLED:
        return Halt
    case mt5grpc.ErrorCode_NO_MONEY:
        return Abort
    case mt5grpc.ErrorCode_TOO_MANY_TRADE_REQUESTS:
        return Retry
    default:
        return Abort
    }
}
```

---

## 11. 幂等去重

```go
package execute

type DedupCache struct {
    cache map[string]*OrderResult
    mu    sync.RWMutex
}

func NewDedupCache() *DedupCache {
    d := &DedupCache{cache: make(map[string]*OrderResult)}
    go d.cleanupLoop(1 * time.Hour)
    return d
}

func (d *DedupCache) Get(clientID string) (*OrderResult, bool) {
    d.mu.RLock(); defer d.mu.RUnlock()
    r, ok := d.cache[clientID]
    return r, ok
}

func (d *DedupCache) Set(clientID string, r *OrderResult) {
    d.mu.Lock()
    d.cache[clientID] = r
    d.mu.Unlock()
}

func (d *DedupCache) SyncFromOrders(orders []Order) {
    d.mu.Lock(); defer d.mu.Unlock()
    for _, o := range orders {
        if o.Comment != "" {
            d.cache[o.Comment] = &OrderResult{Ticket: o.Ticket, State: o.State}
        }
    }
}
```

---

## 12. DashboardService gRPC

### 12.1 Proto 定义

```protobuf
syntax = "proto3";
package dashboard;

service DashboardService {
  rpc SpreadMatrix(SpreadMatrixRequest) returns (stream SpreadMatrixReply);
  rpc PositionWatch(PositionWatchRequest) returns (stream PositionWatchReply);
  rpc SubmitOrder(ManualOrderRequest) returns (ManualOrderReply);
  rpc ClosePosition(ClosePositionRequest) returns (ClosePositionReply);
  rpc GetSignalHistory(SignalHistoryRequest) returns (SignalHistoryReply);
}

message SpreadMatrixRequest {
  int32 refresh_interval_ms = 1;  // 刷新间隔，默认 100ms
}

message SpreadMatrixReply {
  repeated BrokerRow rows = 1;
  string best_bid_broker = 2;     // 全市场最优买价 broker
  string best_ask_broker = 3;     // 全市场最优卖价 broker

  message BrokerRow {
    string broker_name = 1;
    string base_currency = 2;
    double daily_swap_long_bps = 3;
    double daily_swap_short_bps = 4;
    repeated SpreadCell cells = 5;
  }

  message SpreadCell {
    string symbol = 1;
    double bid = 2;
    double ask = 3;
    double spread_to_best_ask_bps = 4;
    double spread_to_best_bid_bps = 5;
    bool is_arbitrageable = 6;
    double estimated_net_profit_bps = 7;
  }
}

message PositionWatchReply {
  string broker_name = 1;
  repeated Position positions = 2;
  double total_margin_used = 3;
  double total_floating_pnl = 4;
  double equity = 5;

  message Position {
    int64 ticket = 1;
    string symbol = 2;
    string side = 3;
    double lots = 4;
    double open_price = 5;
    double current_price = 6;
    double floating_pnl = 7;
    double swap_accrued = 8;
    double commission = 9;
  }
}
```

### 12.2 Server 实现

```go
package dashboard

type Server struct {
    dashboard.UnimplementedDashboardServiceServer
    bus      *bus.QuoteBus
    adapters map[string]adapter.PlatformAdapter
    store    *store.PG
}

func (s *Server) SpreadMatrix(req *dashboard.SpreadMatrixRequest, stream dashboard.DashboardService_SpreadMatrixServer) error {
    ticker := time.NewTicker(time.Duration(req.RefreshIntervalMs) * time.Millisecond)
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            matrix := s.computeMatrix()
            stream.Send(matrix)
        case <-stream.Context().Done():
            return nil
        }
    }
}

// computeMatrix 从 QuoteBus 获取所有 broker 所有 symbol 的最新行情，计算价差矩阵。
// 复杂度：O(B² × S)，B=broker 数，S=symbol 数。对 15×30=450 格，每 100ms 一次足够。
func (s *Server) computeMatrix() *dashboard.SpreadMatrixReply {
    // ...
}
```

---

## 13. PostgreSQL Store

```go
package store

type PG struct {
    pool *pgxpool.Pool
}

func Connect(ctx context.Context, dsn string) (*PG, error) {
    cfg, _ := pgxpool.ParseConfig(dsn)
    cfg.MaxConns = 10
    pool, _ := pgxpool.NewWithConfig(ctx, cfg)
    return &PG{pool: pool}, nil
}

// WriteTick 批量插入 tick，使用 COPY 协议
func (p *PG) WriteTicks(ctx context.Context, ticks []Tick) error {
    return p.pool.AcquireFunc(ctx, func(conn *pgxpool.Conn) error {
        return conn.CopyFrom(
            ctx, pgx.Identifier{"ticks"},
            []string{"ts", "broker", "symbol", "bid", "ask"},
            pgx.CopyFromSlice(len(ticks), func(i int) ([]any, error) {
                t := ticks[i]
                return []any{t.TS, t.Broker, t.Symbol, t.Bid, t.Ask}, nil
            }),
        )
    })
}

// QuerySignals 历史信号查询（desk 历史 Tab 使用）
func (p *PG) QuerySignals(ctx context.Context, from, to time.Time, strategy string) ([]Signal, error) {
    rows, _ := p.pool.Query(ctx,
        `SELECT id, ts, strategy, legs, gross_bps, net_bps, executed
         FROM signals WHERE ts BETWEEN $1 AND $2 AND ($3 = '' OR strategy = $3)
         ORDER BY ts DESC LIMIT 1000`, from, to, strategy)
    // ...
}
```

---

## 14. 审计日志

```go
package audit

type Logger struct {
    f  *os.File
    mu sync.Mutex
}

type Event struct {
    Timestamp time.Time
    Broker    string
    EventType string
    Symbol    string
    Price     decimal.Decimal
    Volume    decimal.Decimal
    ClientID  string
    PnL       decimal.Decimal
    Error     string
}

func (l *Logger) Log(e Event) {
    l.mu.Lock()
    defer l.mu.Unlock()
    // protobuf binary 序列化 + varint length prefix → 追加写
    // 不使用 JSON
    b, _ := proto.Marshal(e.toProto())
    var buf [binary.MaxVarintLen64]byte
    n := binary.PutUvarint(buf[:], uint64(len(b)))
    l.f.Write(buf[:n])
    l.f.Write(b)
}
```

---

## 15. 桌面应用（Fyne）

```go
package main

import "fyne.io/fyne/v2"

func main() {
    app := fyne.NewApp()
    window := app.NewWindow("ARB Desk")

    // gRPC 连接 core
    conn, _ := grpc.Dial("localhost:50051",
        grpc.WithTransportCredentials(insecure.NewCredentials()))
    client := dashboard.NewDashboardServiceClient(conn)

    // 四个 Tab
    tabs := container.NewAppTabs(
        container.NewTabItem("价差矩阵", matrix.New(client)),
        container.NewTabItem("持仓", positions.New(client)),
        container.NewTabItem("交易", trading.New(client)),
        container.NewTabItem("历史", history.New(client)),
    )

    window.SetContent(tabs)
    window.Resize(fyne.NewSize(1400, 900))
    window.ShowAndRun()
}
```

### 价差矩阵 Tab

```go
package matrix

import (
    "fyne.io/fyne/v2"
    "fyne.io/fyne/v2/widget"
    "fyne.io/fyne/v2/canvas"
    "image/color"
)

type Matrix struct {
    widget.Table
    client dashboard.DashboardServiceClient
    data   *dashboard.SpreadMatrixReply
    mu     sync.RWMutex
}

func New(client dashboard.DashboardServiceClient) *Matrix {
    m := &Matrix{client: client}
    m.Table = *widget.NewTable(
        func() (int, int) { return m.rows(), m.cols() },
        func() fyne.CanvasObject { return canvas.NewText("", color.White) },
        m.updateCell,
    )
    go m.streamLoop()
    return m
}

func (m *Matrix) updateCell(id widget.TableCellID, cell fyne.CanvasObject) {
    txt := cell.(*canvas.Text)
    cell := m.data.Rows[id.Row].Cells[id.Col]
    txt.Text = fmt.Sprintf("%.2f", cell.SpreadToBestAskBps)

    // 颜色编码
    switch {
    case cell.IsArbitrageable:
        txt.Color = color.RGBA{0, 180, 0, 255}   // 绿
    case cell.EstimatedNetProfitBps > 0:
        txt.Color = color.RGBA{200, 180, 0, 255}  // 黄
    default:
        txt.Color = color.RGBA{200, 60, 60, 255}   // 红
    }
    txt.Refresh()
}
```

### 管理 Tab（Admin）

第五个 Tab。聚合 broker 状态、策略管理、熔断控制、Kill Switch、实时日志。

```go
package admin

type Admin struct {
    widget.List                     // broker+策略状态列表
    client    dashboard.DashboardServiceClient
    mu        sync.RWMutex
}

func New(client dashboard.DashboardServiceClient) *Admin {
    a := &Admin{client: client}
    // 布局（从上到下）：
    //
    // ┌──────────────────────────────────────────┐
    // │ 🔴 Kill Switch: ACTIVE                   │  ← 全宽红色横幅，仅激活时显示
    // ├──────────────────────────────────────────┤
    // │  Broker     │ 状态  ●/○  │ Equity │ PnL │  ← 可点击展开详情
    // │  OctaFX     │ Connected  │ $5,230 │ +12 │
    // │  RoboForex  │ Discon.    │   —   │  —  │
    // ├──────────────────────────────────────────┤
    // │  Strategy    │ 启用 │ 熔断 │ 亏损 │ PnL  │
    // │  Triangular  │  ✅  │  —  │  0   │ +18 │
    // │  Cross-Exch  │  ✅  │  ■  │  5   │ -42 │  ← 熔断行红色
    // │  Statistical │  ❌  │  —  │  0   │  0  │  ← 已禁用灰色
    // ├──────────────────────────────────────────┤
    // │  [Refresh]  [Kill]  [Resume]             │  ← 操作按钮栏
    // ├──────────────────────────────────────────┤
    // │  14:32:01 ERROR octafx reconnect ...    │  ← 日志尾行（只读）
    // │  14:31:55 INFO  engine  signal detected │
    // └──────────────────────────────────────────┘

    go a.pollLoop()
    go a.tailLogs()
    return a
}

func (a *Admin) pollLoop() {
    ticker := time.NewTicker(1 * time.Second)
    for range ticker.C {
        a.refreshBrokers()
        a.refreshStrategies()
        a.refreshKillSwitch()
    }
}

func (a *Admin) refreshBrokers() {
    snap, _ := a.client.GetAccountSnapshots(ctx, &dashboard.AccountSnapshotRequest{})
    // 逐行渲染：broker name, ●/○ connected, equity, free_margin
}

func (a *Admin) refreshStrategies() {
    status, _ := a.client.GetStrategyStatus(ctx, &dashboard.StrategyStatusRequest{})
    // 逐行渲染：strategy name, [▶] enabled toggle, circuit_breaker, losses, pnl
}

func (a *Admin) refreshKillSwitch() {
    ks, _ := a.client.GetKillSwitchStatus(ctx, &dashboard.KillSwitchStatusRequest{})
    if ks.Active {
        a.banner.Show() // 红色横幅
    } else {
        a.banner.Hide()
    }
}

func (a *Admin) tailLogs() {
    stream, _ := a.client.TailLogs(ctx, &dashboard.TailLogsRequest{
        TailLines: 50,
        LevelFilter: "",
    })
    for {
        msg, _ := stream.Recv()
        a.logBox.Append(fmt.Sprintf("%s %s %s: %s",
            msg.Timestamp, msg.Level, msg.Source, msg.Message))
    }
}
```

**操作按钮 → gRPC 映射**：

| 按钮 | gRPC |
|------|------|
| 策略启用/禁用 toggle | `ToggleStrategy(name, enabled)` |
| 重置策略熔断 | `ResumeStrategy(name)` |
| 重置全局熔断 | `ResetGlobalCircuitBreaker()` |
| Kill | `Kill()` — 二次确认弹窗 |
| Resume | `Resume()` — 二次确认弹窗 |

**变更 Desk 主入口**：`tabs` 从 4 个增加到 5 个：

```go
tabs := container.NewAppTabs(
    container.NewTabItem("价差矩阵", matrix.New(client)),
    container.NewTabItem("持仓", positions.New(client)),
    container.NewTabItem("交易", trading.New(client)),
    container.NewTabItem("历史", history.New(client)),
    container.NewTabItem("管理", admin.New(client)),    // ← 新增
)
```

---

## 16. Core 主入口

```go
package main

func main() {
    if risk.IsKilled() { log.Fatal("kill switch active") }

    // 1. PostgreSQL
    pg, _ := store.Connect(ctx, cfg.DatabaseDSN)

    // 2. QuoteBus
    bus := bus.New(allSymbols)

    // 3. Adapters（15 个 MT4/MT5 broker）
    adapters := make(map[string]adapter.PlatformAdapter)
    for _, b := range cfg.Brokers {
        a := createAdapter(b)
        a.Connect(ctx)
        a.Subscribe(ctx, cfg.SubscribedSymbols)
        go a.QuoteStream(ctx, bus)
        adapters[b.Name] = a
    }

    // 4. 策略引擎
    engine := engine.New(bus, adapters, pg)

    // 5. DashboardService gRPC server
    dash := dashboard.NewServer(bus, adapters, pg)
    grpcServer := grpc.NewServer()
    dashboard.RegisterDashboardServiceServer(grpcServer, dash)
    go grpcServer.Serve(lis) // :50051

    // 6. Tick → PG 持久化
    go tickWriter(ctx, bus, pg)

    // 7. 等待信号
    sig := make(chan os.Signal, 1)
    signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
    <-sig

    // 8. 优雅关闭
    grpcServer.GracefulStop()
    for _, a := range adapters { a.Disconnect() }
    pg.Close()
}
```

---

## 17. 测试要求

| 测试 | 覆盖 |
|------|------|
| `quote_bus_test.go` | Publish/Subscribe 并发；drain-then-replace；Snapshot |
| `adapter_test.go` | Connect/Reconnect 状态机；Token 失效 |
| `pipeline_test.go` | 全部 Filled；部分 Filled+hedge；超时+hedge |
| `circuit_breaker_test.go` | 连续亏损触发；全局熔断；Kill Switch |
| `errclass_test.go` | 所有 ErrorCode 分类 |
| `idempotency_test.go` | ClientID 去重；重连同步 |
| `dashboard_test.go` | SpreadMatrix stream；PositionWatch stream |
| `store_test.go` | PG write/query；COPY 批量插入 |
| `benchmark/` | Hot Path 零堆分配；矩阵计算 O(B²×S) |

---

## 18. 构建

```makefile
.PHONY: test lint proto run-core run-desk

proto:
	buf generate proto/

test:
	go test -race -count=1 ./...

bench:
	go test -bench=. -benchmem ./internal/bus/ ./internal/execute/

run-core:
	go run ./cmd/core -config=config/default.textproto

run-desk:
	go run ./cmd/desk

build-core:
	CGO_ENABLED=0 go build -o bin/arb-core ./cmd/core

build-desk:
	go build -o bin/arb-desk ./cmd/desk
```
