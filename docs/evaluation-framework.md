# 全网套利系统 — 四维评估框架

> 技术栈：Go + mtapi.io (gRPC)  
> 状态：架构设计阶段  
> 风险等级：🔴 高 — 上线前必须解决 | 🟡 中 — 纳入迭代计划 | 🟢 低 — 优化项

---

## 一、架构与性能

### 1.1 gRPC 流式处理 — 终局架构

> **状态：✅ 已决策**  
> **决策日期：2026-08-03**

#### 架构全景图

```
┌──────────────────────────────────────────────────────────────┐
│                        QuoteBus                              │
│         sync.RWMutex + map[symbol][]chan Quote               │
│         Publish() 零锁竞争（仅读锁），无中心 goroutine         │
└──────┬───────────────────────┬───────────────────┬───────────┘
       ↑ Publish()             ↑ Publish()         ↑ Publish()
       │                       │                   │
┌──────┴──────────┐  ┌─────────┴────────┐  ┌──────┴──────────┐
│  MT5 Adapter    │  │  MT5 Adapter     │  │  MT4 Adapter    │
│  Broker A       │  │  Broker B        │  │  Broker C       │
│                 │  │                  │  │                 │
│ recvLoop() ────→│  │ recvLoop() ─────→│  │ recvLoop() ────→│
│ stream.Recv()   │  │ stream.Recv()    │  │ stream.Recv()   │
│                 │  │                  │  │                 │
│ Events(统一流)   │  │ Events(统一流)    │  │ OnQuote         │
│                 │  │                  │  │ OnOrderUpdate   │
│                 │  │                  │  │ OnOrderProfit   │
└─────────────────┘  └──────────────────┘  └─────────────────┘
       │                       │                   │
       │ mt5grpc               │ mt5grpc           │ mt4grpc
       │ ClientConn            │ ClientConn        │ ClientConn
       └───────────────────────┴───────────────────┘
                  完全独立，零共享（仅统一 Quote 结构体）
```

#### 核心组件

**PlatformAdapter 接口**

```go
type PlatformAdapter interface {
    Connect(ctx context.Context) error
    Disconnect() error
    HealthCheck(ctx context.Context) error
    Subscribe(ctx context.Context, symbols []string) error
    QuoteStream(ctx context.Context, bus *QuoteBus)
    PlaceOrder(ctx context.Context, req OrderRequest) (*OrderResult, error)
    CancelOrder(ctx context.Context, ticket int64) error
    ModifyOrder(ctx context.Context, req ModifyRequest) (*OrderResult, error)
    AccountSummary(ctx context.Context) (*Account, error)
    Platform() PlatformType
    BrokerName() string
}
```

**QuoteBus（无中心 goroutine，始终保持最新 tick）**

```go
type QuoteBus struct {
    mu          sync.RWMutex
    subscribers map[string][]chan Quote  // channel cap = 1, 每个策略只存最新一条
}

func (b *QuoteBus) Publish(q Quote) {
    b.mu.RLock()
    chs := b.subscribers[q.Symbol]
    b.mu.RUnlock()
    for _, ch := range chs {
        select {
        case ch <- q:               // fast path: channel 空，直接发送
        default:                    // channel 满（消费慢），排空旧消息，写入新消息
            select { case <-ch: default: }  // 排空最旧的一条
            select { case ch <- q: default: } // 写入最新（极端慢时放弃）
        }
    }
}
```

**背压语义**：channel cap = 1，始终只保留最新一条 tick。慢消费者醒来后读到的是最新价格，而非积压的过期数据。对于套利系统，过期报价比没有报价更危险——它会触发基于错误价格的假信号。

**统一数据模型**

```go
type Quote struct {
    Symbol   string
    Bid      float64    // Hot Path 直接使用
    Ask      float64
    Time     time.Time
    Broker   string
    Platform PlatformType
}

type Account struct {
    Balance    decimal.Decimal  // Warm/Cold Path 使用 decimal
    Equity     decimal.Decimal
    Margin     decimal.Decimal
    FreeMargin decimal.Decimal
    Currency   string
}
```

#### 关键设计决策

| # | 决策点 | 答案 | 理由 |
|---|--------|------|------|
| 1 | 为什么 QuoteBus 而非中心 Hub | 无中心 goroutine，消除单点瓶颈 | Publish 仅读锁，写入不同 symbol 零竞争 |
| 2 | 为什么每连接独立 goroutine | `stream.Recv()` 是阻塞调用 | 必须独立 goroutine，否则阻塞整个 adapter |
| 3 | MT4/MT5 能否共享连接 | **绝对不可** | 不同 proto package、不同服务端、不同消息类型 |
| 4 | 慢消费者处理 | channel cap=1，drain-then-replace：丢弃旧数据保留最新 | 旧数据比无数据更危险（触发假信号） |
| 5 | 时间对齐 | 策略引擎自行处理 | 不同策略对齐需求不同（窗口大小、容忍度） |

#### MT4 vs MT5 Adapter 差异

| | MT4 Adapter | MT5 Adapter |
|--|------------|------------|
| gRPC endpoint | `mt4grpc3.mtapi.io:443` | `mt5grpc3.mtapi.io:443` |
| Go package | `mt4grpc` | `mt5grpc` |
| Stream 数量 | 3 个独立流（Quote/Order/Profit） | 1 个 Events 统一流（或独立流） |
| 订单 Ticket 类型 | `int32` | `int64` |
| 平仓方式 | `OrderClose` / `OrderCloseBy` / `OrderDelete` | `OrderClose` only |
| 历史数据 | Bar 级别 `QuoteHistory` | Bar + Tick 级别 |

#### 评估检查项

| # | 检查项 | 如何解决 | 风险 | 状态 |
|---|--------|----------|------|------|
| 1.1.1 | Stream 生命周期 | 每个 adapter 独立 context；ConnectManager 统一 cancel | 🔴 | ✅ |
| 1.1.2 | 背压处理 | QuoteBus non-blocking send；channel buffer 上限 + 丢弃 + 告警 | 🔴 | ✅ |
| 1.1.3 | 多流复用 | MT4=3流，MT5=1 Events 统一流；adapter 内部封装差异 | 🟡 | ✅ |
| 1.1.4 | 重连与断线恢复 | 每 adapter 独立重连状态机（见 1.1.4） | 🔴 | ➡️ 待深入 |
| 1.1.5 | 消息序列化 | proto 无 HOL 问题；QuoteBus 按 symbol 分流 | 🟡 | ✅ |

### 1.1.4 重连状态机（深入）

> **状态：✅ 已决策**  
> **决策日期：2026-08-03**

#### 状态机定义

```
                    ┌──────────┐
          Start ───→│Disconnected│
                    └─────┬────┘
                          │ Dial + Connect(token)
                          ▼
                    ┌──────────┐
                    │Connected │←──────────────┐
                    └─────┬────┘                │
                          │ Subscribe+Stream    │
                          ▼                     │
                    ┌──────────┐   成功重连     │
                    │Running   │───────────────┘
                    └─────┬────┘
                          │ Recv() error / CheckConnect 失败
                          ▼
                    ┌──────────┐
                    │Reconnecting│
                    └─────┬────┘
                          │ backoff(1s → 30s cap, jitter ±25%)
                          │ CheckConnect() 探测
                          ▼
                   ┌─────────────┐
                   │ 重连超限？   │
                   │ > 10次/分钟  │──Yes──→ 熔断 + 告警 + StopTrading
                   └─────┬───────┘
                         │ No
                         ▼
                    (重试循环 → Connected)
```

#### 参数配置

| 参数 | 值 | 理由 |
|------|-----|------|
| 初始 backoff | 1s | 快速恢复瞬断 |
| 最大 backoff | 30s | 避免频繁重连打爆 mtapi 服务端 |
| Jitter | ±25% | 避免多 adapter 同时重连产生 thundering herd |
| 重连上限 | 10 次/分钟 | 超限视为持续性故障，触发熔断 |
| gRPC KeepaliveTime | 30s | 定期 ping 检测死连接 |
| gRPC KeepaliveTimeout | 10s | 超时未响应视为断开 |
| gRPC KeepalivePermitWithoutStream | true | 空闲连接也发 ping（防止 LB 切断） |

#### 重连期间的仓位安全

```
Recv() error 触发 → adapter 进入 Reconnecting
    ├── 立即标记该 broker 所有在途订单为 "未知状态"
    ├── 暂停该 broker 所有新开仓请求
    ├── 启动重连循环
    │
    ├── 重连成功 →
    │   ├── 全量同步 OpenedOrders() → 补全订单状态
    │   ├── 全量同步 AccountSummary() → 校验余额一致性
    │   ├── 恢复所有 Subscribe(symbols)
    │   ├── 恢复 QuoteStream
    │   └── 解除该 broker 交易禁令
    │
    └── 重连超限（> 10次/分钟）→ 熔断 →
        ├── 立即调用 OrderClose() 平掉该 broker 所有持仓（best effort）
        ├── 告警 + 通知人工介入
        ├── 标记该 broker 为 "Suspended"
        └── 等待人工确认后手动恢复
```

**熔断后为什么需要平仓**：重连超限意味着连接已持续不可用超过 ~3 分钟（1+2+4+8+16+30×5）。在此期间订单状态完全不可知——可能已成交、可能未成交、可能部分成交。保守策略是平掉所有已知持仓，消除风险敞口，等待人工判断。

**为什么不在重连成功后立刻恢复交易**：重连后 AccountSummary + OpenedOrders 全量同步是必需的——MT5 在断线期间的成交/撤单状态只存在于服务端，本地缓存已失效。必须先做全量 diff，确认没有异常仓位残留，再恢复。

#### 与 Stream 错误码的联动

| Recv() 返回 | 含义 | 动作 |
|------------|------|------|
| `status.Code == Unavailable` | mtapi 服务不可达 | 进入 Reconnecting，backoff 重试 |
| `status.Code == DeadlineExceeded` | 请求超时 | 检查 Keepalive 参数，进入 Reconnecting |
| `status.Code == Canceled` | 流被取消（通常是服务端主动断） | 检查是否 token 过期，进入 Reconnecting |
| `io.EOF` | 流正常关闭 | 进入 Reconnecting |
| `status.Code == Unauthenticated` | token 失效 | 需要完整重连（重新 Connect），不走 CheckConnect |

---

### 1.2 并发模型 — 终局方案

> **状态：✅ 已决策**  
> **决策日期：2026-08-03**

#### 1.2.1 Goroutine 池化

**不需要第三方 goroutine pool。**

穷举系统所有 goroutine 来源：

| 来源 | 数量 | 生命周期 | 需池化？ |
|------|------|---------|---------|
| Adapter recvLoop | 1/adapter | 永久 | ❌ 固定 |
| Strategy eval loop | 1/策略 | 永久 | ❌ 固定 |
| OrderSend gRPC | 0~N | 瞬时 | ⚠️ 需限流 |
| 日志/指标 | 0~N | 瞬时 | ❌ goroutine 便宜 |
| 健康检查 | 1 | 永久 | ❌ 固定 |

唯一需要控制的是 OrderSend：不是 goroutine 太多，而是 **MT5 服务端有并发限流**。用 Go 惯用法——buffered channel 做信号量：

```go
type OrderExecutor struct {
    sem chan struct{}  // cap = MaxConcurrentOrders (default: 5)
}

func (e *OrderExecutor) Execute(ctx context.Context, fn func() error) error {
    select {
    case e.sem <- struct{}{}:
    case <-ctx.Done():
        return ctx.Err()
    }
    go func() {
        defer func() { <-e.sem }()
        fn()
    }()
    return nil
}
```

每 adapter 一个 `OrderExecutor`。`MaxConcurrentOrders=5` 来源于 MT5 错误码 `TOO_MANY_TRADE_REQUESTS (10024)`。

#### 1.2.2 锁竞争分析

全系统共享状态及保护策略：

| 共享状态 | 访问模式 | 方案 | 热路径开销 |
|----------|---------|------|-----------|
| QuoteBus.subscribers | 多读（每 tick）少写 | `sync.RWMutex` | 读锁 = atomic add，纳秒级 |
| Adapter.connState | 读写交织 | `atomic.Int32` 枚举 | 单条 CPU 指令 |
| AccountSummary cache | 多读少写 | `atomic.Value` | 指针 swap，无锁 |
| Strategy 内部状态 | 单 goroutine 持有 | 无锁，channel 通信 | 零 |

**QuoteBus.Publish() 在持读锁期间不做任何阻塞操作**。channel send 是 non-blocking 的（drain-then-replace），读锁持有时间 = 几次 channel send = 纳秒级。10 个 adapter 同时 Publish 读锁不互斥。

#### 1.2.3 无锁数据结构

**不需要。** Go 的 RWMutex 读锁是纯用户态 atomic add，在 99% 读场景下等价于无锁。保留此检查项作为观测点——如果 `go tool pprof` 显示 RWMutex 进入 CPU profile top 10，再考虑 lock-free queue 替换。

#### 1.2.4 内存模型正确性

四条铁律：

```
Rule 1: 每个可变状态有且仅有一个 owner goroutine，其他人通过 channel 通信
Rule 2: 必须共享时，显式加锁（RWMutex / atomic），不允许裸读写
Rule 3: Quote 是值类型，Publish 时拷贝，消除共享
Rule 4: CI 强制执行 go test -race，零容忍
```

最危险场景已消除：`connState` 用 `atomic.Int32`，消除 recvLoop 和健康检查之间的 data race。

#### 1.2.5 Fan-out/Fan-in

行情侧已在 1.1 架构覆盖。补充 **订单结果反向通道**：

```
策略 goroutine ──orderReq──→ OrderExecutor ──gRPC──→ MT5
      ↑                                                    │
      └────────────orderResult─────────────────────────────┘
```

策略统一 event loop：

```go
func (s *Strategy) loop() {
    for {
        select {
        case q := <-s.quoteCh:       s.evaluate(q)       // Hot Path
        case result := <-s.orderResultCh: s.handleResult(result)  // 订单结果
        case <-s.ctx.Done():         return
        }
    }
}
```

#### 评估检查项

| # | 检查项 | 决策 | 风险 | 状态 |
|---|--------|------|------|------|
| 1.2.1 | Goroutine 池化 | 不需要第三方池，channel 信号量限流 OrderSend | 🔴 | ✅ |
| 1.2.2 | 锁竞争分析 | RWMutex + atomic + 无锁策略状态，热路径零竞争 | 🟡 | ✅ |
| 1.2.3 | 无锁数据结构 | 不需要，profile 驱动再考虑 | 🟢 | ✅ |
| 1.2.4 | 内存模型正确性 | 四条铁律 + `-race` CI 零容忍 | 🔴 | ✅ |
| 1.2.5 | Fan-out/Fan-in | QuoteBus + 订单结果反向通道 + 策略统一 event loop | 🟡 | ✅ |

### 1.3 内存计算 — 终局方案

> **状态：✅ 已决策**  
> **决策日期：2026-08-03**

#### 数据流内存足迹

```
gRPC wire bytes
  │
  ▼  protobuf unmarshal → heap alloc (~200 bytes, gRPC 内部)
stream.Recv() → *mt5grpc.Quote
  │
  ▼  adapter 读取字段 → 栈分配 (~64 bytes)
Quote{Bid, Ask, Symbol, Time, Broker, Platform}
  │
  ▼  channel send → 值拷贝到 channel buffer (~64 bytes)
QuoteBus.Publish()
  │
  ▼  策略从 channel 读取 → 栈上局部变量 → 函数返回栈回收
GC eventually collects proto message
```

#### 1.3.1 零拷贝路径

**已经是零拷贝。** 验证每个环节：

| 环节 | 拷贝？ | 说明 |
|------|--------|------|
| `q.Symbol` (string) | **零拷贝** | Go string 是 `{ptr, len}` header，赋值仅拷贝 16 字节 |
| `Quote{}` 构造 | 栈分配 | 64 bytes，无堆逃逸 |
| `Publish()` 发 channel | 值拷贝 | 一次 `MOV` 序列，~64 bytes |
| `[]byte ↔ string` 转换 | **不存在** | 无手动 proto 解析，不操作原始字节 |

路径已达最小值，无需改动。

#### 1.3.2 对象池化

**不需要 `sync.Pool`。**

| 对象 | 分配频率 | 需池化？ |
|------|---------|---------|
| `*mt5grpc.Quote` (proto) | ~300/sec | ❌ gRPC 内部管理 |
| `Quote` 结构体 | ~300/sec | ❌ 栈分配，零 GC 压力 |
| `OrderRequest/Result` | 几次/分钟 | ❌ 频率太低 |
| 策略 ring buffer | 启动一次 | ❌ 预分配 |

每秒堆分配 ≈ 60KB。Go GC 在 50MB 堆上暂停 ~0.5ms，不会丢 tick。

#### 1.3.3 内存预分配

仅一处需要：**QuoteBus map 启动时预分配**，防止运行时 rehash 产生延迟尖刺。

```go
func NewQuoteBus(symbols []string) *QuoteBus {
    return &QuoteBus{
        subscribers: make(map[string][]chan Quote, len(symbols)),
    }
}
```

其余数据结构（ring buffer、channel buffer）设计上就是固定大小。

#### 1.3.4 GC 调优

保守起点，不激进：

```
GOGC=50       # 更频繁但更短的暂停（~0.5ms vs default ~2ms）
GOMEMLIMIT=   # 容器内存上限的 80%，防 OOM Kill
```

不使用 `madvdontneed`——内存足迹小且稳定。实际值生产环境跑一周后按 GC CPU 占比和 P99 暂停调优。

#### 1.3.5 Arena 分配

**不适用于实时交易路径。** Arena 用于"大量同生命周期对象集体释放"——回测场景可能用到（留给 4.2）。

#### 评估检查项

| # | 检查项 | 决策 | 风险 | 状态 |
|---|--------|------|------|------|
| 1.3.1 | 零拷贝路径 | 已最小化，string header 传递，无 `[]byte↔string` 转换 | 🟡 | ✅ |
| 1.3.2 | 对象池化 | 不需要，栈分配 + 低频分配 | 🟡 | ✅ |
| 1.3.3 | 内存预分配 | QuoteBus map 启动时预分配 | 🟢 | ✅ |
| 1.3.4 | GC 调优 | GOGC=50 + GOMEMLIMIT，生产跑一周后调优 | 🟡 | ✅ |
| 1.3.5 | Arena | 不适用实时路径，回测可考虑 | 🟢 | ✅ |

### 1.4 延迟优化 — 终局方案

> **状态：✅ 已决策**  
> **决策日期：2026-08-03**

#### 1.4.1 e2e 延迟度量

**可测与不可测**：

```
t0 ─────── t1 ───── t2 ───── t3 ───── t4 ───── t5
mtapi      Recv()   Publish  Strategy OrderSend  gRPC
收到tick   返回     完成     决策完成  发出        返回
  可测      可测     可测     可测      可测       可测
  
  Quote.Time (服务器时间戳，用于计算 server_age)
```

**埋点方案**：每层打 Prometheus histogram，精度微秒级：

```go
// adapter
recvLatency.Observe(time.Since(tRecvStart).Microseconds())

// 策略
serverAge := time.Since(quote.Time.AsTime())  // tick 从服务器到现在的总延迟

// 订单
orderLatency.Observe(time.Since(tOrderStart).Microseconds())
```

| 指标 | 含义 | 告警 |
|------|------|------|
| `recv_latency_us` | Recv() 耗时（含网络） | P99 > 50ms |
| `server_age_us` | tick 服务器时间 → 本地现在 | P99 > 100ms |
| `signal_latency_us` | 策略决策耗时 | P99 > 1ms |
| `order_latency_us` | 下单往返耗时 | P99 > 200ms |
| `e2e_us` | tick→决策总延迟 | P99 > 150ms |

#### 1.4.2 网络拓扑

启动前用 `PingHost` RPC 探测 RTT，决定部署区域：

```
PingHost RTT < 5ms   → 同区域，最优
PingHost RTT 5-20ms  → 同大陆，可接受
PingHost RTT > 50ms  → 跨洋，三角套利和跨所套利不可靠
```

gRPC 默认已启 `TCP_NODELAY`。无需额外客户端负载均衡（每 endpoint 是单 host）。Keepalive 参数见 1.1.4。

#### 1.4.3 系统调用

**热路径上系统调用：零。**

| 操作 | 系统调用？ | 说明 |
|------|-----------|------|
| `stream.Recv()` | `epoll_wait` | Go netpoller，不可避免 |
| channel send/recv | 无 | Go runtime 用户态调度 |
| `time.Now()` | 无 | Linux vDSO，~20ns |

Go epoll-based netpoller 已是最优。不需要 `io_uring`——它面向数千并发连接，我们是几个长连接。

#### 1.4.4 时钟精度

- **交易决策只用服务器时间戳 `Quote.Time`**，不依赖本地时钟
- `time.Since()` 使用 monotonic clock，不受墙钟调整（NTP 步进）影响
- 部署 chrony/ntpd，多 broker 间时钟偏差 < 1ms
- 闰秒：Go UTC 正确处理；MT5 服务端大概率 smear，对毫秒级决策无影响

#### 1.4.5 CPU 亲和性

**不需要。** CPU pinning 的收益场景是 L1/L2 cache 命中率主导的紧循环。我们的 Hot Path 数据小到天然 fit L1，Go 调度器 NUMA-aware 已足够。`unix.SchedSetaffinity` 引入的 cgo 开销和复杂度远超收益。

#### 评估检查项

| # | 检查项 | 决策 | 风险 | 状态 |
|---|--------|------|------|------|
| 1.4.1 | e2e 延迟度量 | 5 项 Prometheus histogram，微秒精度 | 🔴 | ✅ |
| 1.4.2 | 网络拓扑 | PingHost 探测 → 选区域；TCP_NODELAY 默认已启用 | 🔴 | ✅ |
| 1.4.3 | 系统调用 | 已最优，零用户态 syscall | 🟢 | ✅ |
| 1.4.4 | 时钟精度 | 服务器时间戳决策 + monotonic clock 度量 + chrony | 🟡 | ✅ |
| 1.4.5 | CPU 亲和性 | 不需要，复杂度远超收益 | 🟢 | ✅ |

---

## 二、风控与执行

### 2.0 执行管线 — 统一方案

> **状态：✅ 已决策**  
> **决策日期：2026-08-03**
>
> 本方案统一覆盖：2.1.1~2.1.4（滑点检查）、2.2.1（价格校验）、2.3.1（部分成交）、2.3.2（订单超时）

#### 第一性分析

套利的物理本质是**同时买入和卖出等价物**。在分布式系统中（跨 broker/跨交易所），真正的同时不可能实现。因此唯一的正确语义是：**全部成交，或全部不成交**。任一腿失败 = 立即对冲已成交的腿，消除方向性风险敞口。

#### API 约束

经 proto 验证：

| 能力 | MT4 | MT5 |
|------|-----|-----|
| OrderSend 中指定 FillOrKill | ❌ 无此字段 | ❌ 无此字段 |
| Order 响应中查看 FillPolicy | ❌ 无 | ✅ `FillPolicy` 枚举 |
| Order 响应中查看成交状态 | ❌ 仅有 Ticket | ✅ `State` + `CloseVolume` |
| Slippage 参数 | ✅ `int32` (点数) | ✅ `uint64` (点数) |

**核心结论**：FillOrKill 不可依赖。保护机制 = **Slippage 参数 + 成交后状态校验 + 失败对冲**。

#### 执行管线

```
Strategy.execute()
  │
  ▼
┌─────────────────────────────────────────────┐
│ Phase 1: Pre-Trade Revalidation             │
│  获取所有腿的最新 tick                        │
│  重新计算套利条件                              │
│  任一腿价格偏离 > MaxSlippageBps → 放弃       │
└─────────────────────┬───────────────────────┘
                      │ 通过
                      ▼
┌─────────────────────────────────────────────┐
│ Phase 2: Concurrent Submit                  │
│  所有腿同时发出 OrderSend（不等确认）          │
│  slippage = 基于 symbol Digits 换算点数       │
│  ctx Timeout = OrderTimeout (500ms)          │
│                                              │
│  MT5: 返回后检查 State + CloseVolume          │
│  MT4: 返回后检查 Ticket ≠ 0（市价单通常已成交） │
└─────────────────────┬───────────────────────┘
                      │
                      ▼
┌─────────────────────────────────────────────┐
│ Phase 3: Result Collection                  │
│  全部 Filled + CloseVolume==Volume → ✅      │
│  任一腿非 Filled / 部分成交 / 超时 → ❌       │
└─────────────────────┬───────────────────────┘
                      │ 失败
                      ▼
┌─────────────────────────────────────────────┐
│ Phase 4: Hedge on Failure                   │
│  已成交的腿 → 立即反向平仓（限价=当前bid/ask） │
│  未成交挂单 → CancelOrder                    │
│  超时无响应 → 标记uncertain + 不断轮询状态     │
│  记录滑点事件 + 指标 + 告警                   │
└─────────────────────────────────────────────┘
```

#### 核心代码

```go
func (e *Executor) Execute(ctx context.Context, opp ArbitrageOpportunity) error {
    // Phase 1: Pre-trade revalidation
    latest := e.bus.GetLatest(opp.LegSymbols())
    if !opp.IsStillValid(latest, opp.Params.MaxSlippageBps) {
        return ErrPreTradeSlippage
    }

    // Phase 2: Concurrent submit all legs
    ctx, cancel := context.WithTimeout(ctx, opp.Params.OrderTimeout)
    defer cancel()

    results := make(chan LegResult, len(opp.Legs))
    for _, leg := range opp.Legs {
        go func(l Leg) { results <- e.submit(ctx, l) }(leg)
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

    // Phase 4: Hedge any partial fills
    if len(failed) > 0 {
        for _, leg := range filled { e.hedge(leg) }
        for _, leg := range failed { e.cancel(leg) }
        return fmt.Errorf("%d/%d legs filled", len(filled), len(opp.Legs))
    }
    return nil
}
```

#### 策略参数

| 策略 | MaxSlippageBps | OrderTimeout | 理由 |
|------|---------------|-------------|------|
| 三角套利 | 0.5 | 500ms | 利差薄（1-3 pips），滑点吃全部利润 |
| 跨所套利 | 1.0 | 500ms | 利差稍大（2-5 pips） |
| 统计套利 | 2.0 | 1000ms | 均值回归，不要求瞬时 |
| 期现套利 | 1.5 | 1000ms | 基差收敛，时间窗口稍宽 |

#### 覆盖的评估项

| # | 检查项 | 如何覆盖 |
|---|--------|---------|
| 2.1.1 | 信号-执行延时滑点 | Phase 1 预成交价格重校验 |
| 2.1.2 | 盘口深度滑点 | Slippage 参数换算为 symbol 点数 + 交易所限价保护 |
| 2.1.3 | 历史滑点回测 | 同一套管线回测时注入模拟延迟（见 4.2.3） |
| 2.1.4 | 多腿滑点一致性 | Phase 2 并发下单 + Phase 3 全部或全不 |
| 2.2.1 | 价格校验层 | Phase 1 就是二次校验 |
| 2.3.1 | 部分成交处理 | Phase 4 hedge-on-failure |
| 2.3.2 | 订单超时 | context.WithTimeout + Phase 4 cancel |

### 2.1 滑点检查

> 已合并至 2.0 执行管线。

### 2.2 二次校验 — 资金门禁

> **状态：✅ 已决策**  
> **决策日期：2026-08-03**
>
> 2.2.1 已合并至 2.0 执行管线 Phase 1。

#### 2.2.2 单笔限额

**第一性**：套利容量的真实限制是盘口深度，不是固定金额。Slippage 参数已动态控制这一点——下单量超过深度 → 成交价偏离 > slippage → 订单被拒。单笔限额的唯一存在理由是**防止配置错误**（手误 100 lot 写成 0.1 lot）。它是一道 sanity check，不是核心风控。

#### 2.2.4 资金校验

**第一性**：`FreeMargin` 来自 MT5 服务器，是权威值。校验与下单之间的时间窗口无法消除，最坏结果是 `NO_MONEY (10019)`——安全失效，不丢钱。不需要本地锁或预占机制。

#### 2.2.3 频率限制

**第一性**：MT5 不公开精确限流值。固定速率是猜的。正确做法——**自适应限流 (AIMD)**：

```
正常速率发送
  → TOO_MANY_TRADE_REQUESTS → 速率减半
  → 无错误 → 速率缓慢线性恢复
```

TCP 拥塞控制的经典算法，针对未知上限的最优解。

```go
type AdaptiveRateLimiter struct {
    current float64
    max     float64
    min     float64
    mu      sync.Mutex
}

func (l *AdaptiveRateLimiter) OnSuccess() {
    l.mu.Lock()
    l.current = min(l.current * 1.1, l.max)  // additive recovery
    l.mu.Unlock()
}

func (l *AdaptiveRateLimiter) OnRateLimit() {
    l.mu.Lock()
    l.current = max(l.current * 0.5, l.min)  // multiplicative backoff
    l.mu.Unlock()
}
```

#### 2.2.5 自成交检查

**第一性**：三角套利（3 个不同 symbol）天然免疫。统计套利可能在同一 symbol 发反向信号，但执行管线保证上一笔订单已结算后才发起新订单。自成交结构上不可能。保留检查作为纵深防御，成本为零。

#### 资金门禁（统一实现）

在 2.0 执行管线 Phase 1 和 Phase 2 之间执行：

```go
func (g *CapitalGate) Allow(opp ArbitrageOpportunity, acct Account, orders []Order) error {
    // 1. 单笔 sanity check（防配置错误，不是容量控制）
    if opp.MaxNotional() > g.maxNotionalPerTrade {
        return ErrExceedsNotionalLimit
    }

    // 2. 保证金（尽力而为，最坏 OrderSend 被拒）
    if opp.RequiredMargin().GreaterThan(acct.FreeMargin) {
        return ErrInsufficientMargin
    }

    // 3. 频率（自适应限流）
    if !g.rateLimiter.Allow() {
        return ErrRateLimited
    }

    // 4. 自成交（纵深防御，结构上不应触发）
    for _, leg := range opp.Legs {
        for _, o := range orders {
            if o.Symbol == leg.Symbol && o.IsPending() && o.IsOpposite(leg.Direction) {
                return ErrSelfTrade
            }
        }
    }

    return nil
}
```

#### 评估检查项

| # | 检查项 | 决策 | 风险 | 状态 |
|---|--------|------|------|------|
| 2.2.2 | 单笔限额 | Sanity check，不是容量控制；真实限制由 slippage 动态提供 | 🔴 | ✅ |
| 2.2.3 | 频率限制 | 自适应 AIMD，根据 `TOO_MANY_TRADE_REQUESTS` 反馈动态调整 | 🔴 | ✅ |
| 2.2.4 | 资金校验 | 尽力而为，最坏 `NO_MONEY` 安全失败 | 🔴 | ✅ |
| 2.2.5 | 自成交检查 | 结构上免疫，保留作为纵深防御 | 🟡 | ✅ |

### 2.3 异常处理 — 终局方案

> **状态：✅ 已决策**  
> **决策日期：2026-08-03**
>
> 2.3.1（部分成交）和 2.3.2（订单超时）已合并至 2.0 执行管线。

#### 2.3.3 gRPC 错误码映射

**第一性**：130+ 错误码的应对策略只有 4 种。

```go
type ErrorAction int
const (
    Retry      ErrorAction = iota // 退避后重试
    RetryFresh                    // 重新询价后重试
    Abort                         // 放弃本次交易
    Halt                          // 暂停策略/熔断
)
```

| MT5 错误码 | 动作 | 理由 |
|-----------|------|------|
| `REQUOTE (10004)` / `PRICE_CHANGED (10020)` | RetryFresh | 价格变了，拿新报价再试一次 |
| `OFF_QUOTES` / `NO_PRICES (10021)` | Retry | 暂时无报价，等行情恢复 |
| `MARKET_CLOSED (10018)` | Abort | 市场已关，今天不会再开 |
| `TRADE_DISABLED (10017)` | Halt | 交易权限被禁，需人工 |
| `NO_MONEY (10019)` | Abort | 资金不足，立刻停止该策略 |
| `TOO_MANY_TRADE_REQUESTS (10024)` | Retry + 速率减半 | 自适应限流反馈 |
| `TRADE_TIMEOUT (0x80)` / `REQUEST_TIMEOUT (10012)` | Retry (仅1次) | 瞬态 |
| `INVALID_VOLUME (10014)` / `INVALID_PRICE (10015)` / `INVALID_STOPS (10016)` | Abort | 参数错误，重试无用 |
| `ORDER_LOCKED (139)` / `TRADE_CONTEXT_BUSY (146)` | Retry | 订单处理中 |
| `REQUEST_CANCELLED (10007)` | Abort | 已被取消 |
| `INVALID_TOKEN (0x10000)` | Adapter 完整重连 | Token 过期 |
| `CONNECT_ERROR (0x10004)` / `NO_CONNECTION (10)` | Adapter 重连 | 连接级 |
| `NOT_MONEY (134)` | Abort | MT4 版资金不足 |

```go
func Classify(err *mt5grpc.Error) ErrorAction {
    switch err.Code {
    case mt5grpc.ErrorCode_REQUOTE, mt5grpc.ErrorCode_PRICE_CHANGED:
        return RetryFresh
    case mt5grpc.ErrorCode_OFF_QUOTES, mt5grpc.ErrorCode_NO_PRICES:
        return Retry
    case mt5grpc.ErrorCode_MARKET_CLOSED:
        return Abort
    case mt5grpc.ErrorCode_TRADE_DISABLED:
        return Halt
    case mt5grpc.ErrorCode_NO_MONEY:
        return Abort
    case mt5grpc.ErrorCode_TOO_MANY_TRADE_REQUESTS:
        return Retry  // + 触发 rate limiter 减半
    case mt5grpc.ErrorCode_TRADE_TIMEOUT, mt5grpc.ErrorCode_REQUEST_TIMEOUT:
        return Retry
    case mt5grpc.ErrorCode_INVALID_TOKEN:
        return Halt  // 触发 adapter 重连
    default:
        return Abort  // 未识别的错误，保守放弃
    }
}
```

#### 2.3.4 幂等性设计

**第一性**：OrderSend 是网络调用，网络不可靠，必然有超时重试。proto 中无原生幂等键。必须应用层构建。

最危险场景：`OrderSend → 网络超时 → 实际上已成交 → 重试 → 重复下单`。

**方案：ClientID + comment 去重**。

```go
type OrderRequest struct {
    ClientID string // UUID，客户端生成，全局唯一
    // ...
}

func (a *MT5Adapter) PlaceOrder(ctx context.Context, req OrderRequest) (*OrderResult, error) {
    // Step 1: 检查去重缓存
    if cached, ok := a.dedup.Get(req.ClientID); ok {
        return cached, nil
    }

    // Step 2: ClientID 写入 comment（MT5 服务端持久化）
    req.Comment = req.ClientID

    // Step 3: 发送
    resp, err := a.trading.OrderSend(ctx, req)
    if err != nil {
        // 网络错误 → 不缓存，重试前先查 OpenedOrders
        return nil, fmt.Errorf("order send failed, clientID=%s: %w", req.ClientID, err)
    }

    // Step 4: 缓存成功结果（TTL = 1h）
    a.dedup.Set(req.ClientID, fromProto(resp))
    return fromProto(resp), nil
}

// 网络超时后的重试前检查
func (a *MT5Adapter) CheckDuplicate(ctx context.Context, clientID string) (*OrderResult, bool) {
    // 查询最近订单，按 comment 匹配 ClientID
    orders, _ := a.mt5.OpenedOrders(ctx, &mt5grpc.OpenedOrdersRequest{Id: a.token})
    for _, o := range orders.Result {
        if o.Comment == clientID {
            return fromOrder(o), true  // 幂等命中
        }
    }
    return nil, false  // 未找到，可安全重试
}
```

**为什么这是最优解**：MT5 proto 不给幂等键字段，我们只能利用已有的持久化字段（comment）。MT5 的 comment 在 Order 和 OrderHistory 中均可追溯，重连后同步时可做全量去重。替代方案不存在。

#### 2.3.5 数据断流

行情流断开 → 该 broker 进入 **blind mode**：

```
OnQuote/Events 流 Recv() 返回 error
  →
  1. adapter 标记为 blind
  2. 该 broker 所有活跃挂单 → CancelOrder（best effort，网络可能已断）
  3. 该 broker 所有新开仓 → 拒绝
  4. 启动重连（1.1.4 状态机）
  →
  重连成功 → 同步 OpenedOrders + AccountSummary → 解除 blind
  重连超限 → 熔断（2.4.2）
```

blind mode 是二进制状态：要么看到一切，要么什么都不做。不存在"部分可见"的中间态。

#### 评估检查项

| # | 检查项 | 决策 | 风险 | 状态 |
|---|--------|------|------|------|
| 2.3.3 | gRPC 错误码映射 | 4 级分类：Retry/RetryFresh/Abort/Halt | 🔴 | ✅ |
| 2.3.4 | 幂等性设计 | ClientID + comment 去重 + 重试前查 OpenedOrders | 🔴 | ✅ |
| 2.3.5 | 数据断流 | Blind mode：取消挂单 + 拒绝新仓 + 重连 | 🔴 | ✅ |

### 2.4 熔断机制 — 终局方案

> **状态：✅ 已决策**  
> **决策日期：2026-08-03**

#### 第一性

熔断不是风控策略，是生存策略。唯一目的是**在市场或系统杀死你之前，你先杀死自己**。推论：

1. 阈值运行前设定，不在亏损中动态放宽
2. 触发后动作必须自治（不依赖网络、不依赖人）
3. 状态必须持久化（重启不重置）

#### 四层熔断体系

```
Level 1: 策略级   → 单策略连续亏损 → 暂停该策略
Level 2: Broker级 → 连接/交易所异常 → 暂停该 broker 所有策略
Level 3: 全局级   → 日亏损/总回撤超限 → 停止所有交易
Level 4: Kill Switch → 覆盖所有自动逻辑，立即撤单平仓
```

#### 2.4.1 策略级

```go
type StrategyCB struct {
    ConsecutiveLosses    int
    MaxConsecutiveLosses int             // 默认 5
    WindowPnL            decimal.Decimal
    MaxWindowLoss        decimal.Decimal // 策略分配资金上限
    State                CBState         // Closed → Open
}
```

触发条件（OR，任一满足）：连续亏损 ≥ 5 笔，或窗口累计亏损 > 上限。触发后取消该策略所有挂单 + 平仓 + 标记 Suspended。

#### 2.4.2 全局级

```go
type GlobalCB struct {
    DailyPnL       decimal.Decimal
    DailyLossLimit decimal.Decimal // 如 $5000
    TotalDrawdown  decimal.Decimal
    MaxDrawdown    decimal.Decimal // 本金 × 20%
    State          CBState
    persisted      bool            // 写磁盘，重启不重置
}
```

触发后：所有 broker 全量 CancelOrder + OrderClose → 所有 adapter Suspended → 持久化 `.circuit_breaker_state` → 告警。

#### 2.4.3 + 2.4.4 Broker 级

已在现有设计中覆盖：

| 场景 | 机制 | 位置 |
|------|------|------|
| 网络不可用 | 重连超限 → adapter Suspended | 1.1.4 |
| 交易所维护/关闭 | `TRADE_DISABLED`/`MARKET_CLOSED` → Halt | 2.3.3 |

在熔断体系中统一归属为 Level 2 broker 级。

#### 2.4.5 恢复策略

**不自动恢复。** 连续亏损触发的熔断 = 策略逻辑可能有问题，自动重开 = 继续亏。网络问题触发 = 重连状态机已经尽力了。所有恢复必须人工确认。唯一例外：Kill Switch 解除后系统回到全局 Suspended，策略需逐个手动启用。

#### 2.4.6 Kill Switch

最高优先级，全局 `atomic.Bool`，所有策略 event loop 和订单执行路径必须检查：

```go
var killSwitch atomic.Bool

func (m *CBManager) Kill() {
    killSwitch.Store(true)

    for _, a := range m.adapters {
        go func(a PlatformAdapter) {
            tickets, _ := a.GetOpenOrderTickets(context.Background())
            for _, t := range tickets {
                a.CancelOrder(context.Background(), t)
            }
            orders, _ := a.GetOpenedOrders(context.Background())
            for _, o := range orders {
                a.CloseOrder(context.Background(), o.Ticket, o.Lots, 0, 0)
            }
        }(a)
    }

    m.persistState() // 持久化，重启后保持
}

// 所有热路径必须检查
func (s *Strategy) evaluate(q Quote) {
    if killSwitch.Load() { return }
    // ...
}
```

#### 评估检查项

| # | 检查项 | 决策 | 风险 | 状态 |
|---|--------|------|------|------|
| 2.4.1 | 单策略熔断 | 连续亏损 ≥5 笔 OR 窗口亏损 > 上限 → Suspended | 🔴 | ✅ |
| 2.4.2 | 全局熔断 | 日亏损/总回撤超限 → 全量平仓 + 持久化 | 🔴 | ✅ |
| 2.4.3 | 网络熔断 | 已由 1.1.4 覆盖，归属 Broker 级 | 🟡 | ✅ |
| 2.4.4 | 交易所级熔断 | 已由 2.3.3 覆盖，`TRADE_DISABLED` → Halt | 🟡 | ✅ |
| 2.4.5 | 恢复策略 | 人工确认，不自动恢复 | 🟡 | ✅ |
| 2.4.6 | Kill Switch | `atomic.Bool` + 全路径检查 + 持久化 | 🔴 | ✅ |

---

## 三、安全与合规

### 3.1 API 密钥管理 — 终局方案

> **状态：✅ 已决策**  
> **决策日期：2026-08-03**

#### 3.1.1 密钥存储

**第一性**：凭证的生命周期应该是 secrets manager → 内存 → Connect()，不落盘。

| 环境 | 方案 |
|------|------|
| 生产 | Vault / AWS Secrets Manager / GCP Secret Manager，启动时拉取 |
| 开发 | `.env` 文件，必须在 `.gitignore` 中 |

```go
type CredentialSource interface {
    Fetch(ctx context.Context) (*Credentials, error)
}

type VaultSource struct { ... }
type EnvFileSource struct { ... }

// 启动时
creds, _ := source.Fetch(ctx)
token, _ := adapter.Connect(ctx, creds)
// creds 对象随后离开作用域，等待 GC 回收
```

#### 3.1.2 内存安全

**第一性**：Go 的 GC 可能移动内存，OS 可能 swap。但 `mlock` 需要 cgo + `CAP_IPC_LOCK`，引入的复杂度远超收益。正确做法：

```
GOTRACEBACK=0       # 禁用 core dump 中的 goroutine 栈回溯
ulimit -c 0          # 完全禁用 core dump
容器/VM 关闭 swap    # 防止凭证被写入 swap
```

凭证在内存中的生命周期很短（启动时拉取 → Connect → 丢弃），攻击窗口极小。

#### 3.1.3 密钥轮转

MT5 `ChangePassword` RPC 支持在线轮转。Adapter 接口预留：

```go
type PlatformAdapter interface {
    // ...
    UpdateCredentials(ctx context.Context, password string) error
}
```

默认 90 天手动轮转。不自动执行——错误的自动轮转会导致所有连接同时断开。

#### 3.1.4 最小权限

- **交易账户**：使用 Master Password（OrderSend 权限），无权提币
- **行情账户**：使用 Investor Password（只读），无权交易
- 启动时检查 `IsInvestor` flag，确保交易账户确实是交易权限

#### 3.1.5 IP 白名单

broker 侧设置，非代码控制。部署时确认出口 IP 已加入 broker 白名单。

#### 评估检查项

| # | 检查项 | 决策 | 风险 | 状态 |
|---|--------|------|------|------|
| 3.1.1 | 密钥存储 | Vault/Secrets Manager → 内存，不落盘；dev=.env | 🔴 | ✅ |
| 3.1.2 | 内存安全 | 禁用 core dump + swap，优于 mlock | 🟡 | ✅ |
| 3.1.3 | 密钥轮转 | ChangePassword RPC，90 天手动，不自动 | 🟡 | ✅ |
| 3.1.4 | 最小权限 | Master=交易, Investor=只读；启动时校验 IsInvestor | 🔴 | ✅ |
| 3.1.5 | IP 白名单 | Broker 侧配置 | 🟡 | ✅ |

---

### 3.2 防注入与代码安全 — 终局方案

> **状态：✅ 已决策**  
> **决策日期：2026-08-03**

#### 3.2.1 策略热加载

**当前不涉及。** 策略编译进 Go binary——没有热加载 = 没有注入面。如果未来引入 Go plugin/WASM/脚本引擎，需独立安全审计（沙箱隔离、资源限制、系统调用过滤）。此项标记为 N/A，未来若引入则升级为 🔴。

#### 3.2.2 输入校验

```go
// Adapter 层校验所有跨信任边界的字段
func validateProto(req *mt5grpc.OrderSendRequest) error {
    if !knownSymbols[req.Symbol] { return ErrInvalidSymbol }
    if req.Volume <= 0 || req.Volume > maxVolume { return ErrInvalidVolume }
    if req.Price != nil && *req.Price < 0 { return ErrInvalidPrice }
    return nil
}
```

symbol 白名单来自 `SymbolList` RPC，启动时加载。

#### 3.2.3 日志脱敏

```go
type Redacted string  // 实现 fmt.Stringer, 总是打印 "***REDACTED***"

type Credentials struct {
    User     Redacted
    Password Redacted
}
```

结构化日志（`slog`）自动脱敏：所有包含凭证、账户 ID、IP 地址的字段使用 Redacted 类型。不允许使用 `fmt.Sprintf("%v", creds)`——编译器不会保护你，靠 code review + lint 规则。

#### 3.2.4 依赖安全

```
CI pipeline:
  go mod tidy -diff    # 确保 go.sum 完整
  govulncheck ./...    # 扫描已知漏洞
  go mod verify        # 校验依赖完整性
```

#### 评估检查项

| # | 检查项 | 决策 | 风险 | 状态 |
|---|--------|------|------|------|
| 3.2.1 | 策略热加载 | N/A — 策略编译进 binary，无热加载 | 🔴 | ✅ |
| 3.2.2 | 输入校验 | Adapter 层 symbol 白名单 + 范围校验 | 🟡 | ✅ |
| 3.2.3 | 日志脱敏 | Redacted 类型 + slog 结构化日志 | 🔴 | ✅ |
| 3.2.4 | 依赖安全 | govulncheck + go mod verify in CI | 🟡 | ✅ |

---

### 3.4 合规性 — 终局方案

> **状态：✅ 已决策**  
> **决策日期：2026-08-03**

#### 3.4.1 交易所服务条款

MT4/MT5 提供 gRPC API 本身就意味着允许程序化交易。但仍需**逐 broker 检查 ToS**——部分 broker 禁止特定策略类型（如高频 scalping、套利）。此项为外部依赖，非代码问题。

#### 3.4.2 审计日志

全链路不可变日志：

```
tick → signal → decision → OrderSend → fill → PnL
```

每条记录包含：timestamp, broker, symbol, action, price, volume, clientID（幂等键）。

```go
type AuditLog struct {
    Timestamp  time.Time       `json:"ts"`
    Broker     string          `json:"broker"`
    EventType  string          `json:"event"`  // tick/signal/decision/order/fill/pnl
    Symbol     string          `json:"symbol,omitempty"`
    Price      decimal.Decimal `json:"price,omitempty"`
    Volume     decimal.Decimal `json:"volume,omitempty"`
    ClientID   string          `json:"client_id,omitempty"`
    PnL        decimal.Decimal `json:"pnl,omitempty"`
    Error      string          `json:"error,omitempty"`
}
```

输出到 JSONL 文件，每行一条。用于事后争议、回测复盘、税务报告。

#### 3.4.3 KYC/AML

**铁律**：所有 broker 账户必须是同一所有人。跨所套利在不同 KYC 身份之间 = 违法。代码不做自动校验（无法访问外部 KYC 数据库），但在配置文件中显式声明账户归属。

#### 3.4.4 税务报告

每笔平仓生成 PnL 记录（含时间、symbol、开仓价、平仓价、数量、手续费、净盈亏）。按日/月/年聚合导出 CSV。适配所在司法管辖区格式要求。

#### 3.4.5 数据留存

审计日志本地保留 30 天（滚动），归档到冷存储保留 7 年。归档文件脱敏：去除凭证字段、保留交易数据。

#### 评估检查项

| # | 检查项 | 决策 | 风险 | 状态 |
|---|--------|------|------|------|
| 3.4.1 | 交易所 ToS | 逐 broker 检查，API 存在 ≠ 允许所有策略 | 🔴 | ✅ |
| 3.4.2 | 审计日志 | 全链路 JSONL，不可变，clientID 串联 | 🔴 | ✅ |
| 3.4.3 | KYC/AML | 所有账户同一所有人，配置文件显式声明 | 🔴 | ✅ |
| 3.4.4 | 税务报告 | 平仓 PnL 自动导出，按日/月/年聚合 CSV | 🟡 | ✅ |
| 3.4.5 | 数据留存 | 本地 30 天 + 冷存储 7 年，归档脱敏 | 🟡 | ✅ |

---

## 四、商业价值评估

### 4.0 可实现的套利策略（可行性分析）

> **状态：✅ 已确认**  
> **决策日期：2026-08-03**

| 策略 | 所需连接 | 延迟敏感 | 容量 | 可行性 |
|------|---------|---------|------|--------|
| 三角套利 | 单所 | 中 | 中 | ✅ 核心 |
| 跨所同品种套利 | 多 MT5 连接 | 高 | 中 | ✅ 核心 |
| 跨品种统计套利 | 单所 | 低 | 大 | ✅ 辅助 |
| CFD 期现/基差套利 | 单所 | 低 | 中 | ✅ 辅助 |
| 资金费率套利 | 需 Binance API | 低 | 大 | 🔮 未来（需接入 Binance） |
| 延迟套利 | 需直连 | 极高 | 小 | ❌ 放弃（mtapi 多一跳） |
| 做市/对冲 | 单所 | 极高 | 中 | ❌ 放弃（门槛过高） |

**架构影响**：需要同时管理多个 MT5 连接 + 未来 Binance 连接，每个连接独立 goroutine 消费行情流，汇聚到中心 OrderBook。

### 4.1 策略正期望值验证 — 终局方案

> **状态：✅ 已决策**  
> **决策日期：2026-08-03**

#### 第一性

套利不是预测——是发现错误定价。纯套利（三角/跨所）是无风险利润，Sharpe Ratio 是错误指标（分母趋近于零，比值无意义）。正确指标因策略而异。

#### 4.1.1 统计显著性

| 策略类型 | 正确指标 | 最低门槛 |
|----------|---------|---------|
| 三角套利 / 跨所 | 扣费后净利差均值 | > 0（连续 100+ 笔为正）|
| 统计套利 | Sharpe Ratio / Sortino | Sharpe > 1.5, Sortino > 2.0 |
| 期现套利 | 扣费后基差收敛收益 | > 0，且收敛时间 < 持有成本 |

#### 4.1.2 套利概率

三角套利：从历史 tick 数据统计 `|implied - actual| > transactionCost` 的 tick 占比。这不是概率问题——是市场微观结构的经验事实。需要 MT5 `OnTickHistory` 提供的 tick 级数据支撑。

#### 4.1.3 容量测算

**API 限制**：MT5 Quote/MarketWatch 不提供订单簿深度，只给 Best Bid/Ask。无法计算真实容量上限。保守策略：固定小额下单（0.1-1 lot），根据实际成交率逐步放大。MT5 `SymbolInfo.DepthOfMarket` 字段可以告诉你交易所是否提供深度数据，但 mtapi 的 Quote 流不传输深度。

#### 4.1.4 衰减分析

机会半衰期必须 > 10x 系统 e2e 延迟才能可靠捕获。从回测数据中测量：对每个套利信号，追踪价差从触发阈值到消失的时间。画生存曲线。如果你的 e2e 是 50ms 而半衰期是 200ms，只有约 `exp(-50*ln(2)/200) ≈ 84%` 的机会仍存活——可接受。半衰期 10ms → 不可行。

#### 4.1.5 过拟合检测

| 策略类型 | 可调参数 | 过拟合风险 |
|----------|---------|-----------|
| 三角套利 | 0（交易成本阈值不算参数） | 零 |
| 跨所套利 | 0 | 零 |
| 统计套利 | 3-5（lookback、entry z-score、exit z-score） | 中等，需 WFO |
| 期现套利 | 1-2（基差阈值） | 低 |

无参数策略不存在过拟合。统计套利需 Walk-Forward Optimization 验证。

#### 评估检查项

| # | 检查项 | 决策 | 风险 | 状态 |
|---|--------|------|------|------|
| 4.1.1 | 统计显著性 | 纯套利看净利差均值，统计套利看 Sharpe > 1.5 | 🔴 | ✅ |
| 4.1.2 | 套利概率 | 历史 tick 数据驱动，不是先验概率 | 🔴 | ✅ |
| 4.1.3 | 容量测算 | MT5 无深度数据，保守固定量 + 渐进放大 | 🔴 | ✅ |
| 4.1.4 | 衰减分析 | 半衰期 > 10× e2e，生存曲线从回测测量 | 🔴 | ✅ |
| 4.1.5 | 过拟合检测 | 纯套利零参数零风险；统计套利需 WFO | 🔴 | ✅ |

---

### 4.2 回测框架 — 终局方案

> **状态：✅ 已决策**  
> **决策日期：2026-08-03**

#### 核心原则

回测的唯一目的是**告诉你实盘会亏多少**，不是**能赚多少**。因此必须向悲观方向建模。

#### 4.2.1 数据质量

| 数据源 | 粒度 | 用途 |
|--------|------|------|
| MT5 `OnTickHistory` | Tick (Bid/Ask/Last) | 三角/跨所套利回测 |
| MT5 `PriceHistory` | Bar (OHLC) | 统计套利初步筛选 |
| MT4 `QuoteHistory` | Bar (OHLC) | MT4 回测（无 tick 源） |

必须用 tick 数据做套利回测——套利信号存在于 bar 内部，用 bar 数据有 look-ahead bias。MT4 只有 bar 数据，精度受限。

#### 4.2.2 撮合仿真

不假设"cross 即成交"。回测按以下顺序模拟：

```
1. 信号触发时刻的价格 = tick.Bid / tick.Ask
2. 执行价格 = 信号价格 × (1 + slippage)，slippage 从历史分布采样
3. 成交量 = min(期望量, 保守深度假设 1 lot)
4. 手续费 = 成交量 × contractSize × 佣金率
5. 如果执行后价差 ≤ 0 → 该笔回测失败
```

#### 4.2.3 延迟注入

从实盘 Prometheus 指标取 `recv_latency_us` + `order_latency_us` 的 P50/P99 分布，回测时从该分布随机采样注入延迟。回测结果必须分别报告 P50 延迟和 P99 延迟下的收益。

#### 4.2.4 幸存者偏差

使用回测时段的历史 symbol 列表（从 `PriceHistory` 回溯），而非当前活跃 symbol。已退市品种在回测中保留。

#### 4.2.5 市场冲击

对 retail lot size（0.01-1 lot）在 major forex 上，冲击可忽略不计（市场深度通常 >100M 美元）。对 exotic pairs 或 >10 lot，按 `impact_bps = size / avg_depth × 0.5bps` 估算。

#### 4.2.6 样本外验证

```
纯套利（零参数）：
  全部历史数据作为单次验证 → 报告全时段扣费后净利差

统计套利（有参数）：
  60% 训练 / 20% 验证 / 20% 测试，严格时间顺序
  Walk-Forward Optimization：每季度重新训练
```

#### 评估检查项

| # | 检查项 | 决策 | 风险 | 状态 |
|---|--------|------|------|------|
| 4.2.1 | 数据质量 | MT5 tick 数据（必需），MT4 bar 数据（降级） | 🔴 | ✅ |
| 4.2.2 | 撮合仿真 | 悲观模型：slippage 采样 + 保守深度 + 手续费 | 🔴 | ✅ |
| 4.2.3 | 延迟注入 | 实盘延迟分布采样，分别报告 P50/P99 | 🟡 | ✅ |
| 4.2.4 | 幸存者偏差 | 历史 symbol 列表，含已退市品种 | 🟡 | ✅ |
| 4.2.5 | 市场冲击 | Retail lot 忽略，exotic/大单按公式估算 | 🟡 | ✅ |
| 4.2.6 | 样本外验证 | 纯套利全时段，统计套利 60/20/20 + WFO | 🔴 | ✅ |

---

### 4.3 ROI 测算 — 终局方案

> **状态：✅ 已决策**  
> **决策日期：2026-08-03**

#### 4.3.1 真实成本模型

```
每笔交易成本 = 点差成本 + 手续费 + 滑点 + (持仓 O/N 则 + swap)
点差成本 = (ask - bid) × contractSize × lots
手续费   = 成交量 × 佣金率（MT5 通常每 lot $3.5-$7 RT）
滑点     = 历史滑点分布均值（从回测取）
swap     = SymbolInfo.SwapLong/Short（仅跨夜持仓）
```

#### 4.3.2 基础设施成本（自用项目）

| 项目 | 月成本估算 |
|------|-----------|
| VPS（近 mtapi 区域） | $20-100 |
| mtapi 许可 | 待确认 |
| 监控（Grafana Cloud 免费层或自建） | $0-20 |
| 合计 | $20-120 + mtapi |

#### 4.3.3 资金成本

跨所套利的隐藏成本：需要在两个 broker 同时存入保证金。如果每所 $10,000 保证金、年化机会成本 5%，就是 $1,000/年的隐性成本。

#### 4.3.4 净收益测算

```
年化 ROE = (年毛收益 - 年总手续费 - 年总滑点 - 基础设施年费 - 资金机会成本)
            / 总部署资金

ROE > 无风险利率 × 2（即 > 10% currently）→ 值得做
ROE > 20% → 优秀
```

#### 4.3.5 压力情景

| 情景 | 最大亏损预估 | 应对 |
|------|------------|------|
| 闪崩（spread × 10） | 持仓 × 极端滑点 | 2.4.2 全局熔断 |
| 单 broker 断线 | 该 broker 全部持仓 | 2.3.5 Blind mode 平仓 |
| 策略逻辑错误 | 连续亏损 × N 笔 | 2.4.1 策略熔断 |
| 网络分区（部分 broker 可达） | 单腿成交无法对冲 | 2.0 Phase 4 hedge-on-failure |

#### 4.3.6 盈亏平衡点

```
最小本金   = sum(各 broker 保证金要求) + 最大历史回撤 × 2
最低日交易 = (日基础设施成本) / (每笔平均净利)
```

#### 评估检查项

| # | 检查项 | 决策 | 风险 | 状态 |
|---|--------|------|------|------|
| 4.3.1 | 真实成本模型 | 点差 + 手续费 + 滑点 + swap，四要素 | 🔴 | ✅ |
| 4.3.2 | 基础设施 | VPS $20-100/月 + mtapi | 🟡 | ✅ |
| 4.3.3 | 资金成本 | 双边保证金机会成本入账 | 🟡 | ✅ |
| 4.3.4 | 净收益测算 | ROE > 10% 值得，> 20% 优秀 | 🔴 | ✅ |
| 4.3.5 | 压力情景 | 4 种情景 + 已有防线映射 | 🔴 | ✅ |
| 4.3.6 | 盈亏平衡点 | 本金 = 保证金 + 2× 回撤，日交易 = 固定成本 / 单笔净利 | 🟡 | ✅ |

---

## 五、Dashboard & 桌面应用

> **状态：✅ 已决策**  
> **决策日期：2026-08-03**

### 5.0 决策

桌面应用是决策辅助工具，不是自动执行器。Core 执行管线自动下单，Dashboard 提供人工监控和干预通道。两者共享同一个 QuoteBus 和 PG。

### 5.1 架构

```
core (守护进程)                     desk (Fyne 桌面)
  DashboardService (gRPC server)      gRPC client
    SpreadMatrix stream ──────────────→ 价差矩阵 Tab
    PositionWatch stream ─────────────→ 持仓 Tab
    SubmitOrder unary ←─────────────── 手动下单
    ClosePosition unary ←───────────── 手动平仓
    GetSignalHistory unary ←────────── 历史查询
  PostgreSQL ←──────────────────────── 历史 tab (直连)
```

### 5.2 价差矩阵

**第一性**：15 个 broker × N 个品种的跨所价差矩阵。每个格子 = `|BrokerX.Bid - BrokerY.Ask| / BrokerY.Ask × 100%`。颜色编码：

| 颜色 | 条件 | 操作 |
|------|------|------|
| 绿 | 净利差 > 执行成本 | 可执行套利 |
| 黄 | 0 < 净利差 < 执行成本 | 接近但不可行 |
| 红 | 净利差 < 0 | 不可行 |

每行显示该 broker 的日 swap 利率（bps），高 swap 行高亮——如截图中"brent 这个品种 cmc 隔夜利息很高"，一眼排除该 broker 与所有其他 broker 的组合。

### 5.3 四个 Tab

| Tab | 内容 | 数据源 |
|-----|------|--------|
| 价差矩阵 | 15×N 网格，红黄绿色编码，按 broker 分组，swap 高亮 | `SpreadMatrix` gRPC stream |
| 持仓 | 所有 broker 当前持仓 + 浮动 PnL + 保证金占比 + 总风险敞口 | `PositionWatch` gRPC stream |
| 交易 | 手动开仓表单（选 broker、品种、方向、量）、ClientID、成交状态 | `SubmitOrder` / `ClosePosition` |
| 历史 | 信号列表、PnL 时间线、按策略/品种/broker 筛选 | PG 直连 |

### 5.4 PostgreSQL

**第一性**：套利机会是稀疏事件，需要长期历史积累来验证策略有效性。千万级 tick、百万级信号、万级订单。

```sql
-- tick 行情（按月分区，BRIN 索引）
CREATE TABLE ticks (
    ts       TIMESTAMPTZ NOT NULL,
    broker   TEXT NOT NULL,
    symbol   TEXT NOT NULL,
    bid      DECIMAL NOT NULL,
    ask      DECIMAL NOT NULL
) PARTITION BY RANGE (ts);

-- 套利信号
CREATE TABLE signals (
    id         UUID PRIMARY KEY,
    ts         TIMESTAMPTZ NOT NULL,
    strategy   TEXT NOT NULL,
    legs       JSONB NOT NULL,
    gross_bps  DECIMAL NOT NULL,
    net_bps    DECIMAL NOT NULL,
    executed   BOOL DEFAULT FALSE
);

-- 订单（ClientID 是主键 = 幂等键）
CREATE TABLE orders (
    client_id   UUID PRIMARY KEY,
    ticket      BIGINT NOT NULL,
    broker      TEXT NOT NULL,
    symbol      TEXT NOT NULL,
    side        TEXT NOT NULL,
    volume      DECIMAL NOT NULL,
    open_price  DECIMAL,
    close_price DECIMAL,
    open_time   TIMESTAMPTZ,
    close_time  TIMESTAMPTZ,
    pnl         DECIMAL,
    commission  DECIMAL,
    swap        DECIMAL,
    signal_id   UUID REFERENCES signals(id)
);

-- 每日资金汇总
CREATE TABLE daily_summary (
    date         DATE NOT NULL,
    broker       TEXT NOT NULL,
    start_equity DECIMAL,
    end_equity   DECIMAL,
    pnl          DECIMAL,
    PRIMARY KEY (date, broker)
);
```

### 5.5 评估检查项

| # | 检查项 | 决策 | 风险 | 状态 |
|---|--------|------|------|------|
| 5.0.1 | 前端技术 | Fyne Go GUI，4 个 Tab，直连 gRPC + PG | 🔴 | ✅ |
| 5.0.2 | 价差矩阵 | 15×N 网格，净值 > 成本 = 绿，0~成本 = 黄，<0 = 红 | 🔴 | ✅ |
| 5.0.3 | DashboardService | gRPC stream 推矩阵和持仓，unary 下单平仓 | 🔴 | ✅ |
| 5.0.4 | PostgreSQL | tick/信号/订单/日汇总，按月分区，BRIN 索引 | 🔴 | ✅ |
| 5.0.5 | 配置管理 | protobuf text format 配置文件，go:embed | 🟡 | ✅ |

---

## 附录：快速检查清单（按优先级排序）

### P0 — 上线前必须完成（所有 🔴 项）

- [x] 1.1.1 Stream 生命周期管理 — 每 adapter 独立 context + 统一 cancel
- [x] 1.1.2 背压处理 — QuoteBus non-blocking send + drop
- [x] 1.1.4 重连与断线恢复 — 状态机定稿，含 backoff/仓位安全/Stream 错误码联动
- [x] 1.2.1 Goroutine 池化 — channel 信号量限流 OrderSend，无需第三方池
- [x] 1.2.4 内存模型正确性（data race 检查）— 四条铁律 + -race CI
- [x] 1.4.1 e2e 延迟度量 — 5 项 Prometheus histogram，微秒精度
- [x] 1.4.2 网络拓扑 — PingHost 探测选区域，TCP_NODELAY 默认启用
- [x] 2.1.1 信号-执行延时滑点 — 预成交价格重校验
- [x] 2.1.2 盘口深度滑点 — Slippage 点数 + 成交后状态校验
- [x] 2.1.3 历史滑点回测 — 管线复用于回测
- [x] 2.1.4 多腿滑点一致性 — 并发下单 + all-or-nothing
- [x] 2.2.1 价格校验层 — Phase 1 预成交重校验
- [x] 2.2.2 单笔限额 — Sanity check，真实限制由 slippage 动态控制
- [x] 2.2.3 频率限制 — 自适应 AIMD，根据 TOO_MANY_TRADE_REQUESTS 反馈
- [x] 2.2.4 资金校验 — 尽力而为，最坏 NO_MONEY 安全失败
- [x] 2.3.1 部分成交处理 — Phase 4 hedge-on-failure
- [x] 2.3.2 订单超时 — context.WithTimeout + cancel + 轮询
- [x] 2.3.3 gRPC 错误码映射 — 4 级分类 Retry/RetryFresh/Abort/Halt
- [x] 2.3.4 幂等性设计 — ClientID + comment 去重 + 重试前查 OpenedOrders
- [x] 2.3.5 数据断流处理 — Blind mode: 取消挂单 + 拒绝新仓 + 重连状态机
- [x] 2.4.1 单策略熔断 — 连续亏损 ≥5 OR 窗口亏损 > 上限
- [x] 2.4.2 全局熔断 — 日亏损/总回撤超限 → 全量平仓 + 持久化
- [x] 2.4.6 Kill Switch — atomic.Bool + 全路径检查 + 持久化
- [x] 3.1.1 密钥存储 — Vault → 内存，不落盘
- [x] 3.1.4 最小权限 — Master=交易, Investor=只读
- [x] 3.2.1 策略热加载 — N/A，策略编译进 binary
- [x] 3.2.3 日志脱敏 — Redacted 类型 + slog
- [x] 3.3.1 禁用 float — 分层策略：Hot=float64, Warm/Cold=decimal
- [x] 3.3.2 protobuf 金额字段 — mtapi: FormatFloat中转; Binance: string零损失
- [x] 3.3.3 精度转换边界 — 三条铁律确立
- [x] 3.4.1 交易所服务条款 — 逐 broker 检查
- [x] 3.4.2 审计日志 — 全链路 JSONL，clientID 串联
- [x] 3.4.3 KYC/AML — 所有人一致，配置显式声明
- [x] 4.1.1 统计显著性 — 纯套利看净利差，统计套利 Sharpe > 1.5
- [x] 4.1.2 套利概率 — 历史 tick 数据驱动
- [x] 4.1.3 容量测算 — MT5 无深度数据，保守固定量 + 渐进放大
- [x] 4.1.4 Alpha 衰减 — 半衰期 > 10× e2e，生存曲线测量
- [x] 4.1.5 过拟合检测 — 纯套利零风险，统计套利 WFO
- [x] 4.2.1 数据质量 — MT5 tick 必需，MT4 bar 降级
- [x] 4.2.2 撮合仿真 — 悲观模型：slippage 采样 + 保守深度 + 手续费
- [x] 4.2.6 样本外验证 — 纯套利全时段，统计 60/20/20 + WFO
- [x] 4.3.1 真实成本模型 — 点差 + 手续费 + 滑点 + swap
- [x] 4.3.4 净收益测算 — ROE > 10% 值得，> 20% 优秀
- [x] 4.3.5 压力情景 — 4 种情景 + 已有防线映射
- [x] 5.0.1 前端技术 — Fyne Go GUI，4 Tab
- [x] 5.0.2 价差矩阵 — 15×N 网格，红黄绿编码
- [x] 5.0.3 DashboardService — gRPC stream + unary
- [x] 5.0.4 PostgreSQL — 时序分区 + BRIN 索引
- [x] 5.0.5 配置管理 — protobuf text format

### P1 — 第一次迭代纳入

- [ ] All 🟡 items

### P2 — 持续优化

- [ ] All 🟢 items
