# Code Map — 包依赖与数据流

> ⚠️ 本文件的 engine/risk 暴露面/文件名等已过时。**架构以 `docs/design/01-architecture.md` 为准**；本文件保留为历史依赖图参考。
> ⚠️ desk 部分（Layer 6 / §7 Phase 8）按 D-005 更新为 **C# .NET 8 WPF**（取代旧 Wails + Svelte，旧 `desk/` Wails app.go 与 `frontend/` Svelte 作废）。

> 施工 agent 在写任何代码之前必须先读本文档。
> 依赖方向 = 实施顺序。被依赖的先写。

---

## 1. 依赖层级图

```
Layer 0: 零依赖（标准库 only）
  decimalutil/
  errclass/

Layer 1: 类型 + 通信基础
  proto/config/   proto/dashboard/   (生成代码)
  bus/                               依赖: 无 (仅标准库 + Quote 类型定义)
  listing/                           依赖: decimal (Instrument/Listing/Funding 领域模型)

Layer 2: 外部通信
  adapter/                           依赖: bus, decimalutil, errclass, listing, proto(外部mtapi)
  store/                             依赖: decimalutil (PG 读写 decimal)

Layer 3: 业务逻辑
  execute/                           依赖: adapter, risk, decimalutil
  risk/                              依赖: decimalutil
  audit/                             依赖: (仅 protobuf + os)

Layer 4: 聚合服务
  engine/                            依赖: bus, adapter, execute, risk, store
  dashboard/                         依赖: bus, adapter, store, risk, engine

Layer 5: 入口
  cmd/core/                          依赖: engine, dashboard, adapter, bus, store, risk

Layer 6: 桌面客户端（C# .NET 8 WPF，独立项目，D-005）
  desk/ (Desk.csproj)                依赖: core 的 gRPC server（grpc-dotnet client）
    ViewModels/ Views/ Services/ Proto/
```

### 依赖关系图（箭头 = 依赖方向）

```
                        cmd/core
                           │
              ┌────────────┼────────────┐
              ▼            ▼            ▼
          engine      dashboard       audit
              │            │
    ┌─────────┼──────┐     ├──────┬──────┐
    ▼         ▼      ▼     ▼      ▼      ▼
  execute   risk   store  bus  adapter store
    │         │      │           │
    │         │      │           │
    ▼         ▼      ▼           ▼
  adapter  decimalutil        decimalutil
    │                         errclass
    │
    ▼
  bus

  desk/ (C# .NET 8 WPF, 独立项目, D-005)
    │
    └── Proto/ 生成的 C# grpc-dotnet client ── gRPC ──► core DashboardService
        （不再有 cmd/desk、desk/app.go、frontend/；desk 不直连 PG，所有数据经 core）

---

## 2. 包职责 + 暴露面

### Layer 0

| 包 | 职责 | 暴露 | 不暴露 |
|----|------|------|--------|
| `decimalutil` | float64↔decimal 统一转换 | `FromFloat64`, `ToFloat64`, `FromString` | 无 |
| `errclass` | MT4/MT5 错误码分级 | `ClassifyMT5(code) Action`, `ClassifyMT4(code) Action` | 无 |

### Layer 1

| 包 | 职责 | 暴露 | 不暴露 |
|----|------|------|--------|
| `bus` | 行情总线 | `New(symbols) *QuoteBus`, `Subscribe`, `Publish`, `Snapshot` | `subscribers` map |
| `proto/*` | 生成代码 | 由 buf generate 生成，所有 message 和 service stub | — |

### Layer 2

| 包 | 职责 | 暴露 | 不暴露 |
|----|------|------|--------|
| `adapter` | MT4/MT5 连接管理 | `PlatformAdapter` 接口, `NewMT5Adapter`, `NewMT4Adapter`, `OrderRequest`, `OrderResult`, `Order`, `Account` | `MT5Adapter`/`MT4Adapter` 内部字段, `reconnect()` |
| `store` | PG 读写 | `Connect(dsn) *PG`, `WriteTicks`, `CreateSignal`, `UpsertOrder`, `QuerySignals`, `QueryDaily` | `*pgxpool.Pool` |

### Layer 3

| 包 | 职责 | 暴露 | 不暴露 |
|----|------|------|--------|
| `execute` | 执行管线 + 幂等去重 | `NewPipeline(...) *ExecutionPipeline`, `Execute(opp) error`, `DedupCache` | `executeLeg`, `hedge`, `revalidate` |
| `risk` | 风控门禁 + 熔断 + Kill Switch | `CapitalGate`, `AdaptiveRateLimiter`, `StrategyCircuitBreaker`, `GlobalCircuitBreaker`, `Kill()`, `IsKilled()` | 内部状态字段 |
| `audit` | 审计日志 | `NewLogger(path) *Logger`, `Log(Event)` | 文件句柄 |

### Layer 4

| 包 | 职责 | 暴露 | 不暴露 |
|----|------|------|--------|
| `engine` | 策略调度 | `New(bus, adapters, store) *Engine`, `Run(ctx)` | 各策略实现 |
| `dashboard` | gRPC server | `NewServer(bus, adapters, store, engine) *Server` | `computeMatrix`, `aggregatePositions` |

### Layer 5

| 包 | 职责 | 暴露 | 不暴露 |
|----|------|------|--------|
| `cmd/core` | 守护进程 | `main()` — 唯一入口，串联一切 | — |

> 旧 `cmd/desk`（Wails 入口）随 D-005 废除；desk 改为独立 C# .NET 8 WPF 项目（见 Layer 6）。

### Layer 6（C# .NET 8 WPF，独立项目，D-005）

> 不再是 Go 包，是 `desk/` 目录下的 C# 项目（`Desk.csproj`），通过 grpc-dotnet 连 core。
> 旧 `desk/app.go`（Wails Go 后端）+ `desk/{matrix,positions,trading,history,admin}/`（Go 数据层）+ `frontend/`（Svelte）全部作废。

| 组件 | 职责 | 暴露 | 不暴露 |
|----|------|------|--------|
| `desk/Services/` | gRPC client（封装 grpc-dotnet） | `DashboardClient`（OpportunityStream/SpreadMatrix/PositionWatch + ConfirmOpportunity/SubmitOrder unary） | channel 生命周期 |
| `desk/ViewModels/` | MVVM ViewModel（5 个） | `MatrixViewModel` / `PositionsViewModel` / `TradingViewModel` / `HistoryViewModel` / `AdminViewModel`（INotifyPropertyChanged / ObservableCollection） | 命令实现细节 |
| `desk/Views/` | XAML 视图（5 个） | `MatrixView.xaml` / `PositionsView.xaml` / `TradingView.xaml` / `HistoryView.xaml` / `AdminView.xaml` | 控件树 |
| `desk/Proto/` | 由 `proto/dashboard` 生成的 C# gRPC stub | — | — |

---

## 3. 数据流图

### 3.1 行情流（Hot Path）

```
MT5 server
  │ gRPC stream
  ▼
MT5Adapter.quoteStream()
  │ stream.Recv() → proto Quote
  │ 构造 bus.Quote (float64 Bid/Ask, 零堆分配)
  ▼
QuoteBus.Publish(Quote)
  │ drain-then-replace → cap=1 channel
  ├──→ Strategy.evaluate(q)         ← Hot Path (float64 乘除比较)
  │      │ 信号触发
  │      ▼
  │    ExecutionPipeline.Execute()
  │
  ├──→ Dashboard.computeMatrix()    ← 每 100ms 全量快照
  │      │
  │      ▼
  │    gRPC stream → desk/matrix Tab
  │
  └──→ TickWriter (批量缓冲)
         │ 100ms 批量 COPY
         ▼
       PostgreSQL ticks 表
```

### 3.2 订单流

```
Strategy 信号
  │
  ▼
ExecutionPipeline.Execute()
  ├── Phase 1: Snapshot (QuoteBus) → revalidate
  ├── Phase 1.5: CapitalGate.Allow()
  ├── Phase 2: concurrent PlaceOrder (gRPC to mtapi)
  ├── Phase 3: collect results
  └── Phase 4: hedge on failure
       │
       ▼
     AuditLogger.Log()
     PG.orders (UpsertOrder by ClientID)
     SignalBreaker.Record(PnL)
```

### 3.3 桌面流（C# .NET 8 WPF，D-005）

```
desk 启动（desk.exe，单进程）
  │
  └── Services/DashboardClient (grpc-dotnet) ── gRPC ──► core:50051
        │
        ├── OpportunityStream  ──→ await foreach ──→ OpportunityViewModel (ObservableCollection) ──→ WPF 视图
        ├── SpreadMatrix stream ──→ await foreach ──→ MatrixViewModel        ──→ WPF 视图
        ├── PositionWatch stream ──→ await foreach ──→ PositionsViewModel    ──→ WPF 视图
        ├── SubmitOrder (unary)        ←── ICommand ←── WPF TradingView 按钮
        ├── ConfirmOpportunity (unary) ←── ICommand ←── WPF 机会列表「确认执行」按钮
        ├── ClosePosition (unary)      ←── ICommand ←── WPF PositionsView 按钮
        └── GetSignalHistory (unary)   ←── ICommand ←── WPF HistoryView

  （desk 不直连 PostgreSQL；历史等复杂查询经 core gRPC unary，见 §1 of 06-interfaces.md）
```

---

## 4. goroutine 拓扑

```
main goroutine ──→ signal wait ──→ graceful shutdown

├── adapter recvLoop × N (每个 broker 一个)
│     └── QuoteBus.Publish() (同步，非阻塞)

├── Strategy eval loop × M (每个策略一个)
│     └── select { quoteCh, orderResultCh, ctx.Done }

├── ExecutionPipeline (按需，每笔套利一次)
│     └── leg goroutines (每个腿一个，并发下单)
│           └── OrderExecutor.Execute (信号量限流)

├── Dashboard gRPC server
│     └── SpreadMatrix goroutine (ticker → compute → send)
│     └── PositionWatch goroutine (ticker → aggregate → send)

├── TickWriter goroutine
│     └── ticker → batch COPY → PG

├── AuditLogger (同步，不加 goroutine)

└── DedupCache cleanup goroutine (每 1h)
```

goroutine 总数：N (broker) + M (策略) + 3 (matrix/position/tick writer) + 1 (dedup cleanup) + 按需执行腿

对于 15 broker + 5 策略：15 + 5 + 3 + 1 = 24 个常驻 goroutine。全部由设计约束，无无界增长。

---

## 5. channel 拓扑

```
QuoteBus
  subscribers map[string][]chan Quote
  ├── EURUSD → [strategy1.ch, strategy2.ch, dashboard.ch]
  ├── GBPUSD → [strategy1.ch]
  └── ...

每个 channel: cap=1, drain-then-replace

Strategy
  quoteCh      <-chan Quote        (cap=1, 从 QuoteBus)
  orderResultCh <-chan LegResult   (cap=10, 从 ExecutionPipeline)

ExecutionPipeline (每次 Execute 调用创建，函数返回后回收)
  results chan LegResult           (cap=len(legs), 从 腿 goroutines)

OrderExecutor (每 adapter 一个)
  sem chan struct{}                (cap=5, 并发限流)

Dashboard
  (gRPC stream, 非 Go channel)
```

---

## 6. 包导入规则（core / Go）

> desk 是独立 C# 项目（D-005），不在 Go 导入图内；与 core 仅通过 gRPC+protobuf 通信。
> desk 侧：由 `proto/dashboard` 用 `Grpc.Tools` 生成 C# gRPC client，Services 层封装调用，ViewModels 消费。

```
✅ 允许:
  Layer N 导入 Layer N-1 或更下层
  cmd/* 导入任何 internal/*
  同 Layer 内互相导入 (仅在必要时，如 engine → dashboard)

❌ 禁止:
  Layer N 导入 Layer N+1（循环依赖）
  internal/* 导入 cmd/*
  adapter 导入 engine（adapter 不知道策略的存在）
  bus 导入任何 internal 包（bus 是叶子层）
```

### 6.1 desk 侧（C# WPF）调用规则

```
✅ 允许:
  Services/ 只用 grpc-dotnet client 调 core
  ViewModels/ 依赖 Services/ + INotifyPropertyChanged / ObservableCollection
  Views/ 只绑定对应 ViewModel
  Proto/ 由 buf/Grpc.Tools 从 proto/dashboard 生成（与 Go 共享 .proto）

❌ 禁止:
  desk 直连 broker（所有 broker I/O 在 core）
  desk 直连 PostgreSQL（所有数据经 core gRPC）
  desk 启动任何 HTTP/WebSocket/REST listener
```

---

## 7. 文件清单对照

施工 agent：每完成一个 Phase，核对以下文件是否全部存在。

```
Phase 1: 基础
  internal/decimalutil/decimalutil.go
  internal/decimalutil/decimalutil_test.go
  internal/errclass/errclass.go
  internal/errclass/errclass_test.go

Phase 2: 通信
  internal/bus/quote_bus.go
  internal/bus/quote_bus_test.go
  internal/adapter/adapter.go           ← 接口定义
  internal/adapter/mt5.go
  internal/adapter/mt4.go
  internal/adapter/reconnect.go
  internal/adapter/reconnect_test.go
  internal/adapter/credentials.go

Phase 3: 存储
  internal/store/pg.go
  internal/store/ticks.go
  internal/store/signals.go
  internal/store/orders.go
  internal/store/daily.go
  internal/store/pg_test.go

Phase 4: 执行
  internal/execute/pipeline.go
  internal/execute/executor.go
  internal/execute/idempotency.go
  internal/execute/pipeline_test.go
  internal/execute/idempotency_test.go

Phase 5: 风控
  internal/risk/gate.go
  internal/risk/limiter.go
  internal/risk/circuit_breaker.go
  internal/risk/kill_switch.go
  internal/risk/circuit_breaker_test.go

Phase 6: 引擎
  internal/engine/engine.go
  internal/engine/triangular.go
  internal/engine/cross_exchange.go
  internal/engine/statistical.go

Phase 7: Dashboard
  internal/dashboard/server.go
  internal/dashboard/matrix.go
  internal/dashboard/position.go
  internal/dashboard/matrix_test.go

Phase 8: 审计 + 入口 + 桌面
  internal/audit/audit.go
  internal/audit/audit_test.go
  cmd/core/main.go                    # core 守护进程入口（Go）
  # cmd/desk、desk/app.go、frontend/（旧 Wails + Svelte）随 D-005 废除删除
  desk/                               # C# .NET 8 WPF 项目（全新，D-005）
    Desk.csproj                       # .NET 8 + WPF SDK + Grpc.Net.Client/Grpc.Tools/Google.Protobuf
    App.xaml / App.xaml.cs
    MainWindow.xaml / MainWindow.xaml.cs
    Services/
      DashboardClient.cs              # grpc-dotnet client 封装（stream + unary）
    ViewModels/
      MatrixViewModel.cs
      PositionsViewModel.cs
      TradingViewModel.cs
      HistoryViewModel.cs
      AdminViewModel.cs
      OpportunityViewModel.cs         # 机会列表（04 §4）
    Views/
      MatrixView.xaml
      PositionsView.xaml
      TradingView.xaml
      HistoryView.xaml
      AdminView.xaml
      OpportunityView.xaml            # 机会列表视图
    Proto/                            # 由 proto/dashboard 用 Grpc.Tools 生成

Phase A: 数据源地基（已完成 2026-08-07）
  internal/listing/types.go           # Instrument/Listing/Funding + Go-native 枚举
  internal/listing/cache.go           # Listing 缓存（Populate + RunDailyRefresh）
  internal/listing/cache_test.go
  internal/adapter/mt5_listing.go     # MT5Adapter.Listing() proto→listing 映射
  internal/store/symbol_map.go        # symbol_map CRUD (LoadSymbolMap/SaveSymbolMapEntry)
  migrations/002_symbol_map.sql       # symbol_map DDL
  tools/verify_listing/main.go        # 验收工具

Phase 9: 集成
  test/integration/mt5_connect_test.go
  test/integration/dashboard_test.go
  test/benchmark/hotpath_test.go
  Dockerfile.core
  Makefile
  config/default.textproto
```
