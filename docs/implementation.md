# 全网套利系统 — 实现规范

> 对应评估框架全部 73 项，约束文档全部规则  
> 两个二进制：`core`（守护进程）+ `desk`（Wails v3 桌面）  
> 本文件是实现的唯一权威参考

---

## 1. 项目结构

```
arb/
├── cmd/
│   ├── core/
│   │   └── main.go                    # 守护进程入口
│   └── desk/
│       └── main.go                    # Wails 桌面应用入口
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
│   ├── app.go                         # Wails 应用初始化 + Go 绑定函数
│   ├── matrix/
│   │   └── matrix.go                  # 价差矩阵 Go 数据层
│   ├── positions/
│   │   └── positions.go              # 持仓 Go 数据层
│   ├── trading/
│   │   └── trading.go                # 交易 Go 数据层
│   ├── history/
│   │   └── history.go                # 历史查询 Go 数据层
│   └── admin/
│       └── admin.go                   # 管理 Go 数据层
├── frontend/                          # Svelte 前端（构建时编译为静态文件）
│   ├── package.json
│   ├── vite.config.js
│   └── src/
│       ├── App.svelte                 # 根组件 + 5 Tab 容器
│       ├── lib/
│       │   ├── backend.js            # 平台无关 IPC 抽象层（★ 强制，组件不得越过此层直接调 wails.*）
│       │   └── stores.js             # Svelte stores (响应式状态)
│       ├── tabs/
│       │   ├── Matrix.svelte          # 价差矩阵 Tab
│       │   ├── Positions.svelte       # 持仓 Tab
│       │   ├── Trading.svelte         # 交易 Tab
│       │   ├── History.svelte         # 历史 Tab
│       │   └── Admin.svelte           # 管理 Tab
│       └── components/
│           ├── Card.svelte            # 液态玻璃卡片
│           ├── StatCard.svelte        # 数据卡片
│           ├── Skeleton.svelte        # 骨架屏动画
│           └── DataTable.svelte        # 数据表格
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

## 15. 桌面应用（Wails v3 + Svelte 5）

### 15.1 架构概述

Desk 是 Wails v3 桌面应用。Go 后端负责所有网络 I/O（gRPC 到 Core、PG 直连），Svelte 前端负责 UI 渲染。两端通过 Wails IPC 桥接（进程内函数调用 + 事件推送，非网络协议）。

```
desk.exe (单进程)
┌──────────────────────────────────────────────────────┐
│ Go 后端                                              │
│ ├── gRPC client ──── gRPC ────→ core:50051          │
│ ├── PG 直连 (历史查询)                                │
│ ├── Wails 绑定函数 (供前端调用)                       │
│ └── Wails EventsEmit (推送实时数据)                   │
│         ↕ Wails IPC (进程内)                          │
│ Svelte 前端 (WebView2 渲染)                          │
│ ├── wails.Call("method", args) → Go 函数             │
│ └── wails.Events.On("event", cb) ← Go 推送           │
└──────────────────────────────────────────────────────┘
```

### 15.2 IPC 抽象层（强制）

**施工 agent 必须遵守**：所有 Svelte 组件**禁止**直接调用 `wails.Call()` / `wails.Events.On()`。
必须通过 `frontend/src/lib/backend.js` 这一层间接调用。

**设计意图**：
- `backend.js` 是平台无关的接口定义。桌面端实现为 Wails IPC，未来手机端实现为 Capacitor Plugin / gRPC-Web stub。
- 换平台时只改 `backend.js`，不改任何组件。
- 接口命名以业务语义为准（`submitOrder`），不暴露 IPC 机制（不是 `wailsCallSubmitOrder`）。

**接口骨架（施工 agent 按此实现）**：

| 方法 | 方向 | 对应 gRPC |
|------|------|-----------|
| `backend.submitOrder(req)` | 前端→Go | `SubmitOrder` unary |
| `backend.closePosition(ticket)` | 前端→Go | `ClosePosition` unary |
| `backend.getSignalHistory(query)` | 前端→Go | `GetSignalHistory` unary |
| `backend.getAccountSnapshots()` | 前端→Go | `GetAccountSnapshots` unary |
| `backend.toggleStrategy(name, enabled)` | 前端→Go | `ToggleStrategy` unary |
| `backend.kill()` | 前端→Go | `Kill` unary |
| `backend.resume()` | 前端→Go | `Resume` unary |
| `backend.onSpreadMatrix(callback)` | Go→前端 | `SpreadMatrix` stream |
| `backend.onPositionWatch(callback)` | Go→前端 | `PositionWatch` stream |
| `backend.onAdminLogs(callback)` | Go→前端 | `TailLogs` stream |

**验收标准**（Claude 审查时检查）：
- [ ] `grep -r "wails\." frontend/src/tabs/ frontend/src/components/` 返回空
- [ ] `grep -r "wails\." frontend/src/lib/backend.js` 返回所有引用
- [ ] 所有方法名是业务动词（`submitOrder`），不含 `wails`/`Call`/`Events` 字样

### 15.3 响应式布局规范（强制）

**施工 agent 必须遵守**：所有 CSS 布局使用 `minmax()` + `auto-fill` 自适应方案，禁止固定宽度。
这是 `backend.js` 抽象层的视觉对应物——不改代码，自动适配不同屏幕。

```css
/* ✅ 正确 — 自动适配桌面/平板/手机 */
.grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); gap: 16px; }

/* ❌ 禁止 — 固定列数，手机端崩坏 */
.grid { display: grid; grid-template-columns: repeat(3, 1fr); }
```

**验收标准**：将浏览器窗口从 1400px 缩到 375px（iPhone SE 宽度），所有 Tab 内容可见、可交互、无水平滚动条。

### 15.4 Go 后端入口 (cmd/desk/main.go)

```go
package main

import (
    "github.com/wailsapp/wails/v3/pkg/application"
    "arb/desk"
)

func main() {
    app := application.New(application.Options{
        Name:        "ARB Desk",
        Description: "ARB 交易终端",
        Width:       1400,
        Height:      900,
        Assets:      assets.EmbeddedAssets, // frontend/dist/ → embed.FS
    })

    deskApp, _ := desk.NewApp("localhost:50051")
    deskApp.Bind(app) // 注册 Go 函数到前端

    app.Run()
}
```

### 15.5 Go 后端绑定层 (desk/app.go)

```go
package desk

import (
    "context"
    "github.com/wailsapp/wails/v3/pkg/application"
    dashpb "arb/proto/gen/dashboard"
)

type App struct {
    client dashpb.DashboardServiceClient
    // ... PG pool, etc.
}

func NewApp(coreAddr string) (*App, error) {
    conn, _ := grpc.Dial(coreAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
    return &App{client: dashpb.NewDashboardServiceClient(conn)}, nil
}

// Bind 将所有 Go 方法注册到 Wails 前端。
func (a *App) Bind(app *application.App) {
    // 绑定函数（前端通过 wails.Call 调用）
    app.Bind("SubmitOrder", a.SubmitOrder)
    app.Bind("ClosePosition", a.ClosePosition)
    app.Bind("GetSignalHistory", a.GetSignalHistory)
    app.Bind("GetAccountSnapshots", a.GetAccountSnapshots)
    app.Bind("ToggleStrategy", a.ToggleStrategy)
    app.Bind("Kill", a.Kill)
    app.Bind("Resume", a.Resume)

    // 启动实时推送 goroutine
    go a.streamSpreadMatrix(app)
    go a.streamPositionWatch(app)
}
```

### 15.6 实时数据推送模式（Go → Svelte）

```go
// desk/matrix/matrix.go
package matrix

import (
    "github.com/wailsapp/wails/v3/pkg/application"
    dashpb "arb/proto/gen/dashboard"
)

type MatrixBridge struct {
    client dashpb.DashboardServiceClient
}

func NewMatrixBridge(client dashpb.DashboardServiceClient) *MatrixBridge {
    return &MatrixBridge{client: client}
}

// StartStream 启动 gRPC stream → Wails Events 推送。
func (m *MatrixBridge) StartStream(ctx context.Context, app *application.App) {
    stream, _ := m.client.SpreadMatrix(ctx, &dashpb.SpreadMatrixRequest{
        RefreshIntervalMs: 100,
    })
    for {
        msg, err := stream.Recv()
        if err != nil {
            return
        }
        // gRPC 消息直接通过 Wails Events 推送到 Svelte
        app.Events().Emit("spread-matrix", msg)
    }
}
```

### 15.7 Svelte 前端 — 实时数据接收

```js
// frontend/src/lib/stores.js
import { writable } from 'svelte/store';

export const spreadMatrix = writable({ rows: [] });

// 订阅 Wails 事件
export function initStores() {
    wails.Events.On('spread-matrix', (data) => {
        spreadMatrix.set(data);  // Svelte 自动触发最小 DOM 更新
    });
}
```

```svelte
<!-- frontend/src/tabs/Matrix.svelte -->
<script>
  import { spreadMatrix } from '../lib/stores.js';
  import Card from '../components/Card.svelte';
</script>

<div class="matrix-grid">
  {#each $spreadMatrix.rows as row}
    <Card>
      <div class="broker-name">{row.brokerName}</div>
      <div class="cells">
        {#each row.cells as cell}
          <div class="cell" class:arb={cell.isArbitrageable}
               style="color: {cell.isArbitrageable ? '#34c759' :
                      cell.estimatedNetProfitBps > 0 ? '#ffcc00' : '#ff453a'}">
            {cell.spreadToBestAskBps.toFixed(2)}
          </div>
        {/each}
      </div>
    </Card>
  {/each}
</div>

<style>
  .matrix-grid {
    display: grid;
    gap: 12px;
  }
  .cell {
    transition: color 0.3s ease;  /* CSS 原生过渡，GPU 加速 */
  }
</style>
```

### 15.8 Svelte 前端 — 用户操作

```svelte
<!-- frontend/src/tabs/Trading.svelte -->
<script>
  let symbol = 'EURUSD';
  let volume = 0.1;

  async function submitOrder() {
    const result = await wails.Call('SubmitOrder', {
      symbol, volume, operation: 'BUY'
    });
    if (result.ticket) {
      notification.success(`Order #${result.ticket} filled`);
    }
  }
</script>

<Card>
  <input bind:value={symbol} placeholder="Symbol" />
  <input bind:value={volume} type="number" step="0.01" />
  <button on:click={submitOrder}> Submit </button>
</Card>
```

### 15.9 Wails Call → gRPC unary 映射

| 前端调用 | Go 函数 | gRPC |
|----------|---------|------|
| `wails.Call("SubmitOrder", ...)` | `App.SubmitOrder()` | `DashboardService.SubmitOrder` |
| `wails.Call("ClosePosition", ...)` | `App.ClosePosition()` | `DashboardService.ClosePosition` |
| `wails.Call("GetSignalHistory", ...)` | `App.GetSignalHistory()` | `DashboardService.GetSignalHistory` |
| `wails.Call("ToggleStrategy", ...)` | `App.ToggleStrategy()` | `DashboardService.ToggleStrategy` |
| `wails.Call("Kill")` | `App.Kill()` | `DashboardService.Kill` |
| `wails.Call("Resume")` | `App.Resume()` | `DashboardService.Resume` |

### 15.10 Wails Events ← gRPC stream 映射

| Go goroutine | gRPC Stream | Wails Event | Svelte Store |
|-------------|-------------|-------------|-------------|
| `MatrixBridge.StartStream` | `SpreadMatrix` | `"spread-matrix"` | `spreadMatrix` |
| `PositionsBridge.StartStream` | `PositionWatch` | `"positions"` | `positions` |
| `AdminBridge.TailLogs` | `TailLogs` | `"admin-logs"` | `adminLogs` |

### 15.11 Admin Tab — Go 数据层

```go
// desk/admin/admin.go
package admin

type AdminBridge struct {
    client dashpb.DashboardServiceClient
}

func NewAdminBridge(client dashpb.DashboardServiceClient) *AdminBridge {
    return &AdminBridge{client: client}
}

// GetBrokers 供前端调用的绑定函数。
func (a *AdminBridge) GetBrokers() ([]BrokerStatus, error) {
    snap, err := a.client.GetAccountSnapshots(context.Background(),
        &dashpb.AccountSnapshotRequest{})
    if err != nil {
        return nil, err
    }
    // 转换为前端友好的结构体
    var brokers []BrokerStatus
    for _, s := range snap.Snapshots {
        brokers = append(brokers, BrokerStatus{
            Name:       s.BrokerName,
            Connected:  s.IsConnected,
            Equity:     s.Equity,
            FreeMargin: s.FreeMargin,
        })
    }
    return brokers, nil
}
```

Admin Tab 的 Svelte 端渲染逻辑与 Fyne 版本相同（broker 状态列表 + 策略列表 + Kill Switch 横幅 + 操作按钮 + 日志尾行），但使用 HTML/CSS 实现液态玻璃卡片、阴影、悬浮动效等视觉效果。

### 15.12 构建流程

```makefile
.PHONY: build-desk

build-desk:
	cd frontend && npm install && npm run build     # Svelte → frontend/dist/
	go build -o bin/arb-desk.exe ./cmd/desk          # Go + embed frontend/dist/

# desk 最终产物：单个 bin/arb-desk.exe (~20MB)
# 包含：Go 后端 + Svelte 编译产物（嵌入）+ WebView2 loader
```

WebView2 由 Windows 10/11 系统自带，不打包在 exe 中。Win 7 用户首次运行会自动下载 WebView2 Runtime（一次性）。

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
.PHONY: test lint proto run-core run-desk build-core build-desk

proto:
	buf generate proto/

test:
	go test -race -count=1 ./...

bench:
	go test -bench=. -benchmem ./internal/bus/ ./internal/execute/

run-core:
	go run ./cmd/core -config=config/default.textproto

run-desk:
	cd frontend && npm install && npm run build
	go run ./cmd/desk

build-core:
	CGO_ENABLED=0 go build -o bin/arb-core ./cmd/core

build-desk:
	cd frontend && npm install && npm run build
	go build -o bin/arb-desk.exe ./cmd/desk
```
