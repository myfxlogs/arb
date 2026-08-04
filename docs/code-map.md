# Code Map — 包依赖与数据流

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

Layer 2: 外部通信
  adapter/                           依赖: bus, decimalutil, errclass, proto(外部mtapi)
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
  cmd/desk/                          依赖: dashboard(proto client), store

Layer 6: 桌面 UI
  desk/app.go  desk/matrix/  desk/positions/
  desk/trading/ desk/history/ desk/admin/  依赖: dashboard(proto client), store
  frontend/src/                           依赖: Wails runtime (进程内 IPC)
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

  cmd/desk
    │
    ├── dashboard (proto client, gRPC)
    └── store (PG 直连, 历史查询)

  desk/*
    ├── dashboard (proto client, gRPC)
    └── store (PG 直连, 历史查询)

  frontend/ (Svelte 5, 构建时编译, 运行时嵌入 Go binary)
    ├── Wails runtime (进程内 IPC → Go 绑定函数)
    └── 无直接 gRPC/PG 访问（全部通过 Go 后端）
```

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
| `cmd/desk` | 桌面应用 | `main()` — Wails app 启动 | — |

### Layer 6

| 包 | 职责 | 暴露 | 不暴露 |
|----|------|------|--------|
| `desk/app.go` | Wails 应用初始化 + Go 绑定函数 | `NewApp(addr) *App`, `App.Run()` | WebView2 管理 |
| `desk/matrix` | 价差矩阵 Go 数据层 | `NewMatrixBridge(client) *MatrixBridge` | gRPC stream 细节, Wails Events emit |
| `desk/positions` | 持仓 Go 数据层 | `NewPositionsBridge(client) *PositionsBridge` | gRPC stream 细节 |
| `desk/trading` | 交易 Go 数据层 | `NewTradingBridge(client) *TradingBridge` | 表单校验, gRPC unary |
| `desk/history` | 历史 Go 数据层 | `NewHistoryBridge(client, pgDSN) *HistoryBridge` | PG 直连查询 |
| `desk/admin` | 管理 Go 数据层 | `NewAdminBridge(client) *AdminBridge` | gRPC stream + unary |
| `frontend/src/` | Svelte 前端 | `App.svelte` → 5 个 Tab 组件 | Svelte stores, 渲染细节 |

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

### 3.3 桌面流

```
desk 启动
  │
  ├── Go 后端 gRPC Dial("localhost:50051")
  │     ├── SpreadMatrix stream ──→ EventsEmit("spread-matrix") ──→ Svelte Matrix Tab
  │     ├── PositionWatch stream ──→ EventsEmit("positions") ──→ Svelte Positions Tab
  │     ├── SubmitOrder (unary) ←── Wails Call ←── Svelte Trading Tab
  │     ├── ClosePosition (unary) ←── Wails Call ←── Svelte Trading Tab
  │     └── GetSignalHistory (unary) ←── Wails Call ←── Svelte History Tab
  │
  ├── PG 直连 ──→ history Tab (复杂查询，不走 gRPC)
  │
  └── Wails 应用启动 → 加载 frontend/dist/ (Svelte 编译产物)
        │
        └── WebView2 渲染 5 个 Tab
              │
              ├── wails.Call("method", args) → Go 函数 → gRPC/PG
              └── wails.Events.On("event", callback) ← Go EventsEmit ← gRPC stream
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

## 6. 包导入规则

```
✅ 允许:
  Layer N 导入 Layer N-1 或更下层
  cmd/* 导入任何 internal/*
  desk/* 导入 proto 生成代码 + store
  同 Layer 内互相导入 (仅在必要时，如 engine → dashboard)

❌ 禁止:
  Layer N 导入 Layer N+1（循环依赖）
  internal/* 导入 cmd/*
  adapter 导入 engine（adapter 不知道策略的存在）
  bus 导入任何 internal 包（bus 是叶子层）
  desk/* 导入 internal/*（桌面独立，通过 gRPC 通信）
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
  cmd/core/main.go
  cmd/desk/main.go
  desk/app.go                         # Wails 应用 + Go 绑定函数
  desk/matrix/matrix.go               # 价差矩阵 Go 数据层
  desk/positions/positions.go         # 持仓 Go 数据层
  desk/trading/trading.go             # 交易 Go 数据层
  desk/history/history.go             # 历史 Go 数据层
  desk/admin/admin.go                 # 管理 Go 数据层
  frontend/                           # Svelte 前端
    package.json
    vite.config.js
    src/
      App.svelte                      # 根组件 + Tab 容器
      lib/
        backend.js                    # 平台无关 IPC 抽象层（★ 强制）
        stores.js                     # Svelte stores (响应式数据)
      tabs/
        Matrix.svelte                 # 价差矩阵 Tab
        Positions.svelte              # 持仓 Tab
        Trading.svelte                # 交易 Tab
        History.svelte                # 历史 Tab
        Admin.svelte                  # 管理 Tab
      components/
        Card.svelte                   # 液态玻璃卡片
        StatCard.svelte               # 数据卡片
        Skeleton.svelte               # 骨架屏
        DataTable.svelte               # 数据表格

Phase 9: 集成
  test/integration/mt5_connect_test.go
  test/integration/dashboard_test.go
  test/benchmark/hotpath_test.go
  Dockerfile.core
  Makefile
  config/default.textproto
```
