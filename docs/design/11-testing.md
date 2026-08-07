# 11 · 测试策略

> 定义新架构（detector + evaluator + 机会闭环）的测试规范。本文是 `02/03/04/09` 的测试落地，遵守 `AGENTS.md §10`（Before Commit：`go test -race` 强制）。
> 依据：02（成本模型 / 字段对照）、03（三类 Detector）、04（pipeline / all-or-nothing / 对冲）、09（QuoteBus canonical / 背压）、AGENTS §10（race / vet / file-lines）。
> 实现者（Windsurf）照本文落地测试用例；命名 `*_test.go`，table-driven，与产品代码同包。

---

## 1. 测试原则（强制）

1. **`go test -race -count=1 ./...` 必过**（AGENTS §10）。race 检测是并发包（bus/detector/evaluator/pipeline）的硬门槛——09 §1 ~50 常驻 goroutine，无 race 才放心。
2. **精度分层**：hot path 测试断言用 float64 容差（`math.Sqrt` 量级误差）；warm/cold path（decimal / 金额 / bps）断言用 `decimal.Decimal` 或字符串精确比较——**不**对金额用 float 断言（constraints §四）。
3. **demo 验证优先**（08 §4）：单元/integration 跑 demo（ICMarketsSC-Demo 52993526 + Exness-MT5Trial5 277842155），不裸奔实盘。
4. **真实数据回归基线**：02 §7 字段对照表（EURUSD/XAUUSD/GBPJPY）是 Evaluator 的回归基线，必须有用例断言（§4）。
5. **table-driven**：输入 / 期望输出表格化，每行一个 case，新增 case 不改测试骨架。
6. **无外部依赖的单测**：broker / PG / mtapi.io 一律 mock；integration 测试单独 build tag（`//go:build integration`），CI 默认不跑（demo 凭证不进 CI）。

---

## 2. 单元测试

### 2.1 detector（`internal/detector/`）

Detector 纯函数（03 §4：输入 quotes+listings，输出 Candidate），易测。

| 用例 | 输入 | 期望 | 依据 |
|---|---|---|---|
| **CrossExchange 真实报价 mock** | ICMarkets EURUSD Ask=1.1000 + Exness EURUSDm Bid=1.1004（讨论七数据） | 候选 1 条：ICMarkets Buy + Exness Sell，毛价差 = (1.1004−1.1000)×100000×lots | 03 §2.1 |
| CrossExchange 无价差 | A.Ask ≥ B.Bid（同向） | 0 候选 | 03 §2.1 |
| CrossExchange 单 broker（无对家） | 仅 ICMarkets 有报价 | 0 候选（跨所需 2 broker） | 03 §2.1 |
| CrossExchange 跨 broker contractSize 一致校验 | 两腿 Listing.ContractSize 一致（100000） | 候选腿等量对冲 | 02 §3、讨论七 |
| **Triangular 同 broker 闭环** | ICMarkets EURUSD/GBPUSD/EURGBP 交叉汇率偏差 | 候选 1~2 条（3 腿闭环） | 03 §2.3 |
| Triangular 汇率一致 | EURUSD == GBPUSD × EURGBP | 0 候选 | 03 §2.3 |
| **Carry 净 swap 为负（实测基线）** | GBPJPY ICMarkets 做多(+11.67) + Exness 做空(−39.8) | 候选产出（净 swap 负，但 detector 仍产出供 evaluator 过滤） | 03 §2.2 / 讨论七 |
| Carry swap 倒挂（构造正收益） | broker A swapLong=+20 + broker B swapShort=+5 | 候选产出（净 swap +25，evaluator 推机会） | 03 §2.2 |
| 本地 map[broker]Quote 无竞态 | 并发 Publish + Scan | `go test -race` 无报告 | 09 §3 |
| 盲区跳过 | broker blind 标志置位 | 该 broker 不参与扫描，0 候选 | 09 §10 |

### 2.2 evaluator（`internal/evaluator/`）

Evaluator 是「准确无误」的命门（02 §4），**回归基线最重**（§4）。

| 用例 | 输入 | 期望 | 依据 |
|---|---|---|---|
| **成本模型四要素** | 候选 + Listing + quote | NetProfit = Gross − Spread − Commission − Slippage − Swap，逐项断言 | 02 §4.1 |
| **swap InPoints 主公式（EURUSD ICMarkets）** | SwapLong=−8.287, Points=0.00001, ContractSize=100000, Lots=1 | swap = −8.287 USD/天 | 02 §4.2 / 讨论七 |
| **swap InPoints（XAUUSD Exness）** | SwapLong=−509.9, Points=0.001, ContractSize=100, Lots=1 | swap = −50.99 USD/天 | 02 §4.2 / 讨论七 |
| **swap 各 SwapType 换算**（参数化 9 种） | SwapType ∈ {MarginCurrency, Currency, PercCurPrice, PercOpenPrice, PointClosePrice, PointBidPrice, SymInfo_s408, SwapNone} | 按 MT5 文档公式断言 | 02 §4.2 |
| **JPY 计价品种盈亏换算** | GBPJPY ProfitCurrency=JPY，本币盈亏 + USDJPY 汇率 | USD 盈亏 = JPY 盈亏 / USDJPY | 02 §3 |
| **统一度量 bps** | NetProfit + Notional | NetBps = NetProfit / Notional × 10000 | 02 §3 |
| 可执行性预检：NetBps < 阈值 | NetBps = 1bp（阈值 3bp） | Executable=false，丢弃 | 02 §6 / 讨论五① |
| 可执行性预检：风控三项超限 | 单机会敞口 > 5% / 并发 > 5 / 单平台 > 40% | Executable=false | 07 §1 |
| 报价新鲜度（ExpiresAt） | quote_time 比 now 旧 5s | 判 Expired，丢弃 | 02 §6 / 公理④ |
| Confidence 占位 | 默认算法 | 初值基于新鲜度+盘口宽度（非 NaN / [0,1]） | 02 §6 |
| TickValue 不依赖 | TickValue=0（实测 broker 未填） | 仍能算出净盈利（用 ContractSize×Points） | 02 §3 / 讨论七 |

### 2.3 pipeline（`internal/execute/`）

pipeline 改为「仅 ConfirmOpportunity 触发」（04 §7），保留 revalidate / gate / 并发下单 / 对冲。

| 用例 | 输入 | 期望 | 依据 |
|---|---|---|---|
| **revalidate 价偏放弃** | 确认后重拉报价，价偏 > revalidate_max_slippage_bps | 机会 Expired，不下单 | 04 §3 |
| revalidate 通过 | 价偏 ≤ 阈值 | 进入 gate + 下单 | 04 §3 |
| **all-or-nothing 全成交** | N 腿全部 Filled | 机会 Filled，归因回填 | 04 §3 |
| **失败对冲**（hedge） | N−1 腿成交 + 1 腿失败 | 成交腿反向对冲平仓，机会 Failed | 04 §3 / 07 §1 |
| **单边敞口存活上限** | 失败腿后 > max_leg_exposure_duration 未对冲 | 强制对冲触发 | 07 §1 |
| 并发腿信号量 ≤5 | 一次确认 N>5 腿（构造三角 + 多组合） | 同时执行腿 ≤5 | 09 §6 |
| 幂等（ClientID 去重） | 同一 ClientID 重复提交 | 第二次 no-op | 01 §1（orders PK） |
| Kill 中断 | 执行中触发 Kill Switch | 撤未成交腿 + 平已成交腿 | 04 §4 / 07 §2 |
| 资金门禁拒绝 | CapitalGate 判敞口超限 | 机会 Expired + 告警 | 07 §1 |

### 2.4 QuoteBus（`internal/bus/`）

| 用例 | 输入 | 期望 | 依据 |
|---|---|---|---|
| **canonical 路由**（09 §2） | Publish EURUSD(ICMarkets) + EURUSDm(Exness) 经 symbol_map 映射 canonical=EURUSD | Subscribe("EURUSD") 收到 2 条（Broker 字段区分） | 09 §2 |
| **cap=1 drain-then-replace** | 慢消费者 + 高频 Publish | 消费者拿到最新一条（旧丢） | 09 §6 / 公理④ |
| **Snapshot/LatestOrWait** | revalidate 查 latest[canonical][broker] | 返回各 broker 最新报价 | 09 §2 |
| 无竞态（并发 Publish + Subscribe） | N producer + M consumer | `go test -race` 无报告 | 09 §7 |

### 2.5 risk（`internal/risk/`）

| 用例 | 输入 | 期望 | 依据 |
|---|---|---|---|
| CapitalGate：单机会敞口 ≤5% | 占用 6% | 拒绝 | 07 §1 |
| CapitalGate：并发 ≤5 | 已 5 个未平仓 | 拒绝 | 07 §1 |
| CapitalGate：**单平台 ≤40%**（新） | 某 broker 占 45% | 拒绝 | 07 §1 / 讨论五② |
| CircuitBreaker：日亏 ≤3% | 日亏 3.5% | 熔断开 + 暂停新机会 | 07 §1 |
| CircuitBreaker 须人工恢复 | 熔断后自动时间到 | 仍熔断，须 Resume | 07 §2 |
| KillSwitch 文件触发 | 创建 .kill_switch | atomic.Bool 置位 + 撤单平仓 | 07 §2 |
| AdaptiveRateLimiter 反馈限流 | 连续失败 | 速率下降 | 07 §2 |

---

## 3. 集成测试（`//go:build integration`）

需要真实 demo broker / PG，本地或手动跑，CI 默认跳过。

| 用例 | 步骤 | 验收 | 依据 |
|---|---|---|---|
| **adapter 连 demo 拉 Listing** | 连 ICMarketsSC-Demo + Exness-MT5Trial5，调 `Listing(ctx, brokerSymbol)` | 字段对照 02 §7（EURUSD/XAUUSD/GBPJPY），contractSize/swapLong/swapShort/digits 一致 | 05 §3 / 02 §7 |
| **QuoteBus canonical 路由端到端** | 两个 broker 真实 OnQuote stream → symbol_map 映射 → Subscribe("EURUSD") | 收到两 broker 报价（Broker 字段区分），server_age < 100ms | 09 §2 / 05 §2 |
| **OpportunityStream 推送** | 注入构造候选 → evaluator → 仓库 → OpportunityStream | desk（或测试 client）经 `await foreach` 收到 OpportunityEvent(action=PUSHED) | 04 §3 / 06 |
| **ConfirmOpportunity 全链路（demo 不成交）** | 推送机会 → 测试 client ConfirmOpportunity → pipeline revalidate | revalidate 在 demo 报价上跑通（可构造价偏让其 Expired，不必真成交） | 04 §3 |
| **symbol_map 加载** | PG 插入映射（Exness EURUSDm→EURUSD）→ core 启动 | 内存 map 含该映射，detector 跨所聚合成功 | 05 §5 |
| **启动对账**（09 §10） | 构造 PG 与 broker 持仓不一致 | 孤儿持仓告警 + 暂停新机会 | 09 §10 |

---

## 4. 回归基线（02 §7 字段对照表）

**这是"准确无误"的回归锚点**——Evaluator 的 Listing 映射必须与真实探测数据一致。每次改 evaluator 都跑这组断言。

```
table-driven fixture（testdata/listing_icmarkets_eurusd.json 等）：
  Listing 字段            | EURUSD(IC)       | XAUUSD(IC)      | GBPJPY(IC)
  ContractSize            | 100000           | 100             | 100000
  Digits                  | 5                | 2               | 3
  Points                  | 0.00001          | 0.01            | 0.001
  ProfitCurrency          | USD              | USD             | JPY
  SwapType                | InPoints         | InPoints        | InPoints
  SwapLong                | -8.287           | -56.766         | +11.67
  SwapShort               | +1.544           | +38.929         | -23.186
```

- **断言**：`adapter.Listing("ICMarketsSC-Demo","EURUSD")` 返回的上述字段 == fixture（integration 测试，连 demo）。
- **断言**：Evaluator 用 fixture 算 EURUSD 1 手隔夜 swap == −8.287 USD（unit 测试，不连 demo）。
- 新增 broker / 品种探测后，**追加 fixture**（02 §7 是基线起点，不是终点）。

---

## 5. 测试设施

| 设施 | 用途 |
|---|---|
| `testdata/*.json` | Listing / Quote / 候选 fixture（真实探测快照，02 §7） |
| mock mtapi client | adapter 单测用（不连真 broker）；mtapi gRPC interface mock |
| in-memory PG（`pgxmock` 或 docker PG） | store 单测（orders/opportunities/audit_events） |
| `//go:build integration` | integration 测试 build tag，CI 跳过 |
| table-driven helper | `cases := []struct{ name, in, want }`，统一骨架 |
| race detector | `go test -race`（AGENTS §10，强制） |

---

## 6. Before Commit（AGENTS §10 复述）

```bash
go build ./...                              # 编译通过
go test -race -count=1 ./...                # 全量 race 检测（强制）
go vet ./...                                # 静态分析
go run ./tools/check-file-lines --strict    # 文件规模（测试文件豁免）
govulncheck ./...                           # 已知漏洞
```

- 测试文件豁免 file-lines 检查（constraints §六），但**不豁免 race**。
- desk（C# WPF）测试不在本规范范围（第一版 desk 重 push 刷新 + 确认按钮，手动验收为主；后期可加 UI 自动化）。

---

## 7. 回溯

- `go test -race` 强制 / Before Commit → AGENTS §10
- Evaluator 回归基线（EURUSD/XAUUSD/GBPJPY 字段对照）→ 02 §7 / 讨论七真实探测
- 成本模型四要素 / swap InPoints 公式 / JPY 换算 → 02 §3 §4
- 三类 Detector 用例（CrossExchange/Carry/Triangular）→ 03 §2
- Carry 净 swap 为负用例 → 03 §2.2 / 讨论七（审计纠正）
- pipeline revalidate / all-or-nothing / 失败对冲 → 04 §3
- QuoteBus canonical 路由 / cap=1 → 09 §2 §6
- 风控 P1（单平台 ≤40% / 日亏 ≤3%）→ 07 §1 / 讨论五②
- demo 验证优先 / 不裸奔实盘 → 08 §4
