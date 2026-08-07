# 09 · core 运行时架构（内部）

> core 单进程内的**并发模型 / 延迟预算 / 容灾恢复 / 扩展点**。Windsurf 实现 core 内部的依据。
> 依据 `00` 公理 + `01` 总览 + `02/03/05`。`01` 是架构总览，本文是 core 内部运行时细化。
> 议题 3（并发模型）2026-08-07 定；议题 4–8（延迟/容灾/恢复/扩展）待续。

---

## 1. goroutine 拓扑

```
main ── signal wait ── graceful shutdown（先停 adapter/stream，再关 PG）

├── adapter recvLoop × N（每 broker 1 个）          常驻
│     └─ OnQuote stream.Recv() → 映射 brokerSymbol→canonical → QuoteBus.Publish
│
├── detector × M（每 canonical 品种 1 个）          常驻
│     └─ range QuoteBus.Subscribe(canonical) → 更新本地 map[broker]Quote → 扫描 → Candidate
│
├── evaluator × 1（串行）                           常驻
│     └─ range Candidate chan → 扣成本算 NetProfit → 可执行性预检 → Opportunity → 仓库
│
├── OpportunityStream × 1                          常驻
│     └─ 订阅仓库变更 → gRPC server stream → desk
│
├── dashboard SpreadMatrix / PositionWatch × 2     常驻（现有）
│
├── Listing 刷新 × 1（每日 ticker）                常驻（Push-First 合法例外）
├── DedupCache cleanup × 1（每 1h）                常驻
│
└── ExecutionPipeline（按需，每确认 1 次）          非常驻
      └─ leg goroutines（信号量 ≤5 并发）→ audit + 归因
```

**常驻 goroutine ≈ N(15) + M(~30 品种) + 1 + 1 + 2 + 1 + 1 ≈ 50**；按需 pipeline leg 受并发上限 ≤5。**无无界增长**（constraints §并发合规）。

---

## 2. QuoteBus 路由（关键改动，跨所的前提）

### 问题（现有缺陷）
现有 `subscribers map[brokerSymbol]`：ICMarkets `EURUSD` 和 Exness `EURUSDm` 路由到不同订阅者。跨所 detector 得分别订阅 + 手动聚合，每个 detector 重复聚合——**跨所逻辑被憋在 detector 里**。

### 定夺：按 canonical 路由
- `Quote` 加字段：`Canonical`（路由 key）、`Broker`、`BrokerSymbol`（下单用）。
- adapter Publish 前，用 `symbol_map` 把 `BrokerSymbol→Canonical`，填入 Quote。
- `QuoteBus` 按 `Canonical` 分发 + `latest[Canonical] = map[Broker]Quote`（per-broker 最新，revalidate 用）。

```go
type Quote struct {
    Canonical    string      // 路由 key，如 "EURUSD"（brokerSymbol→canonical 映射）
    Broker       string      // 来源 broker，如 "ICMarketsSC-Demo"
    BrokerSymbol string      // 原始符号，下单用，如 "EURUSDm"
    Bid, Ask     float64     // hot path
    Time         time.Time   // 公理④新鲜度
    Platform     PlatformType
}
```

→ 跨所 detector 订阅 `Subscribe("EURUSD")`，range channel 即收到该品种**所有 broker** 的报价（带 Broker 字段区分），一次聚合完成。

### 保留
- `cap=1` drain-then-replace（防假信号、慢消费者不阻塞，公理④）。
- `Snapshot`/`LatestOrWait`（revalidate 用，改查 `latest[Canonical][Broker]`）。
- subscribers/latest 分两把锁（细粒度）。

---

## 3. detector（事件驱动，每 canonical 1 goroutine）

```go
// 每 canonical 品种 1 个 goroutine，本地状态无锁。
func runDetector(canonical string, ch <-chan Quote, listings, out chan<- Candidate) {
    latest := make(map[string]Quote)   // broker → 最新报价（goroutine 局部，无共享无锁）
    for q := range ch {
        latest[q.Broker] = q
        scanCrossExchange(canonical, latest, listings, out)  // 跨所：比各 broker bid/ask
        scanCarry(canonical, latest, listings, out)          // 套息：比 swap（Listing）
    }
}
```
- **事件驱动**（range channel，Push-First，报价来即扫）。
- **本地 `map[broker]Quote`**（goroutine 局部，无锁、无竞态）。
- 扫描产出 `Candidate`（毛价差 > 0）→ Candidate channel。
- 三角 detector（跨品种闭环）单独模型，按 broker 分组（见 `03`）。

---

## 4. evaluator（1 个串行 goroutine，可扩展）

```go
func runEvaluator(in <-chan Candidate, listings, oppStore, out) {
    for c := range in {
        opp := evaluate(c, listings)   // 扣全成本(decimal) + 可执行性预检 → Opportunity
        if opp.Executable {
            oppStore.Add(opp)           // → 仓库 → OpportunityStream 推 desk
        }
    }
}
```
- **串行 1 goroutine**：评估是 decimal 计算 + 缓存 Listing + 最新 quote，微秒~毫秒级，串行够。
- **接口可扩展**：`Candidate chan` → 1 consumer；若归因（`07`）显示评估成瓶颈，改 N 个 consumer（手写 worker，**非 pool 库**，合规）。
- 不在 detector 热路径算 decimal——重计算留给 evaluator，detector 只发现毛价差。

---

## 5. Opportunity 仓库

- 内存 `map[oppID]*Opportunity` + PG 持久化（预估 + 实际，`07`）。
- **`sync.RWMutex`**（非热路径——机会频率远低于报价，constraints 只禁热路径 Mutex）。
- evaluator Add / pipeline 更新状态 / OpportunityStream 订阅变更。

---

## 6. 背压与有界

| 组件 | 背压机制 |
|---|---|
| QuoteBus → detector/dashboard | cap=1 drain-then-replace（丢旧不阻塞） |
| detector → evaluator (Candidate chan) | cap=K channel；满则丢候选（候选可再生，丢无妨） |
| evaluator → 仓库 | 同步（仓库 RWMutex 短临界区） |
| pipeline leg | 信号量 ≤5（CapitalGate 并发上限） |

**有界**：常驻 ~50 goroutine + 按需 pipeline leg ≤5。无 `ants`/`conc` pool（合规）。

---

## 7. 同步规则（constraints §并发细化）

- **detector 本地状态无锁**（每 goroutine 独立 map[broker]Quote）。
- **QuoteBus** RWMutex（订阅增删 + latest；热路径但细粒度 + 短临界区，可接受——若实测竞争，改 atomic pointer 或 sharded map）。
- **Opportunity 仓库** RWMutex（非热路径）。
- **pipeline leg** 信号量（channel cap=5）。
- **禁** `sync.Map`、热路径 `sync.Mutex`、goroutine pool、裸无界 goroutine。

---

## 8. 延迟预算 SLO（议题 4，2026-08-07 定）

### 目标：core 本地（quote 到达 → 推送 desk）SLO < 20ms

| 段 | 目标 | 说明 |
|---|---|---|
| quote 到达 → QuoteBus.Publish | <1ms | channel + map 写 |
| QuoteBus → detector | <1ms | cap=1 channel |
| detector 扫描 → Candidate | <1ms | 本地 map[broker] 比较 |
| evaluator 评估 → Opportunity | <5ms | decimal 计算（warm path） |
| Opportunity → OpportunityStream | <5ms | gRPC stream send |
| **core 本地合计** | **<20ms** | |

### 瓶颈在外部（core 本地不瓶颈）
- **mtapi.io 链路**：core 放德国 ~几ms / 美国 164ms（D-005 定德国）。
- **desk ↔ core 公网**：~100ms（desk 仅展示，秒级不敏感）。
- **人类确认**：秒级（D-003 人确认，**主导延迟**）。
- **执行**（确认→下单）：revalidate <10ms + mtapi.io 下单往返。

→ 系统是**秒级+**（D-005），core 本地 <20ms 远小于外部 + 人类，**不是瓶颈**。**不为省微秒过度优化**（lock-free/对象池无意义，mtapi.io 占主导）。

### 实测验证
- `probe/latency`：测 core↔mtapi.io gRPC 往返（德国 vs 美国）。
- core 内部埋点：quote→detector→evaluator→stream 各段耗时（slog/metrics），验证 <20ms。
- 归因（`07`）：实际 vs SLO 偏差持续监控。

---

## 9. 回溯 + 待续
- canonical 路由 / cap=1 / 事件驱动 → 公理①④、Push-First
- detector/evaluator 拆分 → D-003（发现 vs 评估）
- 有界 goroutine → constraints §并发
- 延迟不瓶颈 / 秒级+ → D-005、mtapi.io 主导
- **待续**：议题 6 持久化恢复、议题 7 扩展点、议题 8 可用性

---

## 10. 容灾恢复（议题 5，2026-08-07 定）

### 故障场景 × 处置
| 场景 | 处置 |
|---|---|
| **单 broker 断线** | adapter reconnect（指数退避，已有）+ 该 broker blind → 不参与新机会 + 其未确认机会 Expire；重连后 Subscribe 恢复 |
| **mtapi.io 网关断**（全 broker 失联） | **全局 blind**：拒新机会 + 未确认 Expire + 撤能撤的挂单 + 告警（desk+log）；mtapi 恢复后重连全部 |
| **core 重启**（崩溃/升级） | 连 PG 恢复 + 连 broker + **对账**（见下）；孤儿持仓告警 + 暂停新机会 |
| **desk 断线** | core 独立继续（无人确认 → 机会自然 Expire）；desk 重连重订 OpportunityStream 恢复 |
| **PG 断** | 内存继续（quotes/发现/推送不依赖 PG）；但订单/audit 写失败 → 告警 + **暂停下单**（不能安全持久化订单就不下单） |

### blind mode（二进制状态）
- **非 blind** = broker 已连接 + quote 流新鲜（`server_age` < 阈值，如 5s）。
- **blind** = 断线 / quote 流断（`server_age` 超阈值）。
- blind broker：detector 跳过、其相关机会 Expire、撤该 broker 挂单。
- **全局 blind**（mtapi 断 / 多数 broker blind）：暂停整个系统新机会 + 告警 + Kill Switch 可触发。
- 实现：adapter reconnect 状态机（已有 `stateConnected/Disconnected`）+ `server_age` 监控 → blind 标志；detector/evaluator/pipeline 检查该标志。

### 启动对账（core 重启）
1. 连 PG → 读 `orders`/`positions`/`opportunities`（已知状态）。
2. 连各 broker → 拉 `OpenOrders`（实际持仓）。
3. 对比：
   - **broker 有、PG 无**（孤儿/重启期间残留未对冲腿）→ **告警 + 暂停新机会**（等你 broker 终端手动处理）。
   - PG 有、broker 无（已平）→ 更新 PG。
4. 未确认机会 → Expire（重启期间报价已过期）。
5. 对账通过 → 启动 detector。

> 对账是"core 挂了手动平"（D-005 你定的）的安全网——重启后系统不会在不知情下带着孤儿敞口继续。

---

## 11. 持久化恢复（议题 6）

### PG 持久化范围
| 数据 | 持久化 | 重启 |
|---|---|---|
| `broker_accounts` / `symbol_map` | ✅ PG | 启动加载（连 broker + 符号映射） |
| `opportunities`（预估+实际+偏差） | ✅ PG | 对账（议题 5） |
| `orders` | ✅ PG | 对账 |
| `audit` | ✅（PG 或 protobuf 文件，constraints §二） | 合规追溯 |
| `ticks`（报价时序） | ✅ PG（量大→二期 TimescaleDB） | 不重建（历史） |
| quotes 实时 / Listing 缓存 / detector 本地状态 | ❌ 内存 | 重启重建（重连 + 拉 Listing） |

### 原则
- **交易状态**（orders/opportunities）必持久化 → 重启不丢交易。
- **实时数据**（quotes/Listing）内存 → 重启重建（不持久化热数据）。
- **core 无状态化**（状态在 PG）→ 快重启。

---

## 12. 扩展点（议题 7）

加什么 = 加实例/插件，**不改 core 数据流**：
| 扩展 | 方式 | 改核? |
|---|---|---|
| 加 broker | PG `broker_accounts` + adapter 实例（启动加载） | ❌ |
| 加品种 | `symbol_map` + detector 自动（每 canonical goroutine） | ❌ |
| 加策略（detector 类型） | 实现 `Detector` 接口（`03`）+ 注册 | ❌ |
| 加 Crypto（Binance） | Binance adapter（实现 `PlatformAdapter`）+ `Instrument.Kind=PERP` + `Funding`（`02` 预留） | ❌（抽象兼容） |
| 加移动端 | core 暴露移动接口（后期独立） | ❌ |

→ 插件式（adapter/detector 接口），扩展不改 core。

---

## 13. 可用性（议题 8）

D-005 已定：**单实例 + 快重启**（core 挂手动平）。
- 不做 HA/多实例（用户明确不要，过度复杂）。
- **快重启**：无状态（PG）+ 连 broker + 拉 Listing + 对账（议题 5），秒~十秒级。
- **单点风险接受**：手动平（broker 终端）+ 启动对账（孤儿持仓告警）= 安全网。
- **Kill Switch**（文件触发）：紧急全局停止。

---

## 14. 回溯（议题 3-8 完成）
- canonical 路由 / cap=1 / 事件驱动 / 有界 goroutine → 公理①④、Push-First、constraints §并发
- detector/evaluator 拆分 → D-003
- 延迟不瓶颈 / 秒级+ → D-005、mtapi.io 主导
- blind mode / 启动对账 / 孤儿持仓 → 公理④、D-005 安全网
- 无状态化 + 快重启 → D-005
- 插件式扩展 → 公理①（统一抽象让扩展不改核）
