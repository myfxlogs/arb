# 12 · Evaluator — 机会评估算核

> 「准确无误」的算核：`Candidate`（Detector 产出的**仅毛价差**）→ 扣全成本 → 净盈利 + 可执行性 → `Opportunity`。
> 依据：`02 §3/§4/§5/§5.1/§6`（换算/成本/Opportunity/度量双轨/评估流程）、`03 §1/§4`（Candidate 来源与职责边界）、`06 §5.2`（proto 字段级定义）、`07 §1`（风控三项）、`00 公理③④`。
> 施工：Windsurf。**纯函数 + warm path decimal**，无副作用，易测。第一版只做 MT5（D-004）。

---

## 0. 一句话定位

```
Detector（发现，高频轻量）─► Candidate（毛价差）
                                    │
                                    ▼
              Evaluator（本文件）─► Opportunity（扣全成本 + 可执行性）
                                    │ Executable=true
                                    ▼
                              推送 desk（04）
```

Evaluator **不算价差是否存在**（那是 Detector），只算**扣完整摩擦成本后还赚不赚、能不能执行**。两者分离（03 §1）：Detector 可对每 tick 扫描，重计算只在候选上发生。

---

## 1. 包与接口

包：`internal/evaluator/`（Layer 3，依赖 `listing` / `bus` / `decimalutil` / `risk`，**不依赖** `execute`/`detector`——单向）。

```go
// Deps 是 Evaluator 的只读依赖（构造时注入，Evaluate 内不 mutate）。
type Deps struct {
    Listings *listing.Cache       // 各腿真实参数（ContractSize/Points/Swap/...）
    Bus      *bus.QuoteBus        // 各腿最新报价（公理④新鲜度）
    Rates    *RateResolver        // ProfitCurrency → USD 交叉汇率（§4.3）
    Gate     *risk.CapitalGate    // 风控三项快照（§6）
    Cfg      Config               // 阈值/滑点/新鲜度/容差（§7）
    Now      func() time.Time     // 注入时钟，可测
}

// Evaluate 把一个候选评估为机会。不可执行/过期/成本无法确定 → 返回 (nil, nil)；
// 真实错误（如缺 Listing）→ 返回 (nil, err)。纯函数：同一输入恒定输出。
func (e *Evaluator) Evaluate(ctx context.Context, c Candidate) (*Opportunity, error)
```

- `Candidate` 来自 `03 §4`（`Type / Legs[] / GrossProfit / QuoteTime`），Leg 至少含 `Broker / BrokerSymbol / Canonical / Direction / Lots（基准，可能被归一化改写）/ EstPrice`。
- `Opportunity` 见 `02 §5`（成本拆解 + Carry 年化字段 + 可执行性），proto 透传见 `06 §5.2`。
- Evaluator 内部不调任何 broker I/O、不发 RPC、不写 DB（评估纯计算；落库由上游/归因负责，`07 §4`）。

---

## 2. 前置依赖（Phase A → B 的补齐）

Phase A 奠定了 `Listing`/`Cache`/`symbol_map`，但 Evaluator 还缺三块。**这三块必须在 Evaluator 之前落地**（建议作为 Phase B 的前置子任务，或并入 Phase A 收尾）：

### 2.1 `Listing` 缺 `Commission`（02 §4.4 vs §1.2 不一致）
`02 §4.4` 明确「commission 人工录入 Listing」，但 `02 §1.2` 的 `Listing` 结构与 Phase A 的 `internal/listing/types.go` **都没有 commission 字段**。补：

```go
// 加入 Listing（types.go）。MT5 SymbolInfo 不提供（02 §4.4），人工录入 + 成交校准。
CommissionMode  CommissionMode   // PerLot（每手固定，FX 主流）/ PerNotionalBps（名义 bps）
CommissionRate  decimal.Decimal  // PerLot: 利润币/手；PerNotionalBps: bps。默认 0（未录入）
```
- 默认 0 = **诚实但会高估净盈利**：未录入 commission 的机会，CommissionCost=0，desk 详情面板须标注「手续费未配置」。这是 MT5 固有限制（02 §4.4），设计接受，不抹平。

### 2.2 `Listing.Instrument` 仍为 nil（Instrument 解析器缺失）
Phase A 的 `Listing.Instrument` 留 nil（cache 按 `broker/brokerSymbol` 存，未解析逻辑品种）。但 Detector（`03 §4` 入参 `map[(broker,canonical)]*Listing`）与 Evaluator（跨 broker 配对、汇率换算）**都依赖 Instrument**。补一个解析器（建议 `internal/listing/resolver.go`，属 listing 包）：

```go
// 从 symbol_map(brokerSymbol→canonical) + Listing cache，构建 canonical 视图：
// key = (broker, canonical)，value = Listing（Instrument 已填）。
// Instrument 由 canonical 符号推导：EURUSD→{FX,EUR,USD,SPOT}；XAUUSD→{FX,XAU,USD,SPOT}。
func (c *Cache) CanonicalIndex(symMap map[string]map[string]string) map[CanonicalKey]*Listing
```
- canonical→{base,quote,assetclass} 的推导规则集中在此（FX：6 字符拆 3+3，特殊如 XAUUSD/XAGUSD 按已知贵金属前缀；Crypto 留接口）。
- 这是 Phase A 数据源地基的自然延伸，不引入新概念。

### 2.3 `config.proto` 缺机会阈值 / 滑点预估参数（02 §6 vs config 不一致）
`02 §6` 要「净盈利主度量 > 阈值（NetBps ≥ 3bp；Carry 用 AnnualizedNetBps）」+「滑点预估」，但 `config.proto` 的 `RiskConfig` 只有执行/熔断参数（`revalidate_max_slippage_bps` 是**执行前**价偏，非评估滑点预估），**没有评估阈值**。补 `EvaluatorConfig`（§7）。

---

## 3. Evaluate 流程（落地 02 §6 七步）

```
Candidate 进入
 │
1. 新鲜度校验（公理④）── 各腿 Quote.Time 距 Now ≤ freshness_ttl？否 → 丢弃(nil)
 │
2. 对冲手数归一化（§4.2）── 名义价值相等；取整后校验偏差 ≤ tolerance，否则 Executable=false
 │
3. 成本计算（§4.4）── 点差 / 手续费 / 滑点 / swap（swap 按 §4.1 各 SwapType）
 │
4. 盈亏换算（§4.3）── 各腿本币 → USD（RateResolver）→ NetProfit / NetBps
 │
5. Carry 专用（§5，仅 Type==Carry）── NetSwapPerDay / AnnualizedNetBps / 各腿 LegRole+DailySwap
 │
6. 可执行性预检（§6）── 主度量阈值 + 风控三项 + 盘口宽度
 │
7. 设 ExpiresAt / Confidence（§7）
 │
▼
Executable=true → *Opportunity（status=Pushed）；false → 仍产出 *Opportunity 但 Executable=false（desk 可见不可点，10 §4.1 风险提示列）
真实异常（缺 Listing/Quote）→ (nil, err)
```

> 「Executable=false 也产出」：让 desk 看到被拒原因（价偏/超敞口/低置信），呼应 D-006 风险提示列与「不推假机会但要透明」。

---

## 4. 子模型（各自纯函数 + 独立单测）

### 4.1 swap 换算（9 种 SwapType，逐枚举）

**主公式（InPoints=1，已用真实数据验证，02 §4.2）**：

```
daily_swap(本币) = SwapLong|Short × Points × ContractSize × Lots
  EURUSD ICMarkets 多 1 手：-8.287 × 0.00001 × 100000 × 1 = -8.287 USD/天  ✓
  XAUUSD ICMarkets 多 1 手：-56.766 × 0.01 × 100 × 1     = -0.56766... → 校验成交后校准
```

**全枚举**（SwapValue 取 SwapLong/Short 由 Direction 决定；Price=Pr，ContractSize=C，Lots=L，Points=P）：

| SwapType | daily_swap（腿本币 = ProfitCurrency） | 实测状态 |
|---|---|---|
| `SwapNone` (0) | `0` | — |
| `InPoints` (1) | `S × P × C × L` | ✅ EURUSD/XAUUSD 已验证 |
| `MarginCurrency` (3) | `S × L`（S 为保证金币金额 → 经 RateResolver 换利润币） | 未实测 |
| `Currency` (4) | `S × L`（S 为利润币金额，直接用） | 未实测 |
| `PercCurPrice` (5) | `S% × Pr(当前) × C × L / 100 / 365`（年率日化） | 未实测 |
| `PercOpenPrice` (6) | `S% × Pr(开仓) × C × L / 100 / 365` | 未实测 |
| `SymInfo_s408` (2) / `PointClosePrice` (7) / `PointBidPrice` (8) | **未实测** → 见下 | 未实测 |

> **未实测模式的诚实处理**：评估遇到 SwapType ∉ {0,1,3,4,5,6} 时，**无法保证成本准确 = 无法保证「准确无误」** → 该机会 `Executable=false` 并在 reason 标注「swap 模式未校准」。**禁止猜 0**（swap 多为成本，猜 0 会造假机会）。Phase F 归因/扩大 broker 覆盖后用实测值精确化。

### 4.2 对冲手数归一化（02 §3.1，delta-neutral 落地）

```
名义价值(腿) = ContractSize × Lots × Price        ← 两腿须相等
给定基准腿 A 的手数 L_A（Detector 给或取 VolumeMin）：
  notional_A = C_A × L_A × Pr_A
  腿 B 原始手数  L_B_raw = notional_A / (C_B × Pr_B)
  腿 B 实际手数  L_B     = round(L_B_raw → VolumeStep_B)   ← 向 VolumeStep 取整
再校验：|notional_A − C_B × L_B × Pr_B| / notional_A ≤ tolerance（初值 1%）
  超 tolerance → 两腿名义对不齐 = 对冲留敞口 → Executable=false
两腿均须落在 [VolumeMin, VolumeMax]；越界 → Executable=false
```
- FX 同品种跨 broker（第一版主流）：`C_A == C_B` → `L_B ≈ L_A`（1:1）。
- 异规模（如未来 UKOIL↔XBRUSD 1:10、Crypto 永续）：上式自动反比，不假设 1:1。
- 取整导致的名义偏差是真实的（公理②换算非抹平），超容差即判不可执行——不留敞口。

### 4.3 盈亏换算到 USD（02 §3）+ RateResolver

```
腿本币盈亏 = 价差类 × C × L                          [本币 = ProfitCurrency]
统一 USD   = 本币盈亏 × rate(ProfitCurrency → USD)
```
- **不依赖 TickValue**（实测 broker 未填，02 §3）。
- **JPY 计价**（GBPJPY/USDJPY，ProfitCurrency=JPY）：先得 JPY，再经 USDJPY 汇率换 USD。
- `RateResolver`：从 `QuoteBus.Snapshot` 取交叉汇率。`rate(JPY→USD) = 1 / USDJPY_ask`；`rate(USD→USD)=1`。所需交叉对须在订阅集内；缺失 → Executable=false（不能换算 = 不能比较）。Crypto 接入后同机制（USDT≈USD）。

### 4.4 成本各项（02 §4.1）

```
NetProfit = GrossProfit − SpreadCost − CommissionCost − SlippageCost − SwapCost
NetBps    = NetProfit / Notional × 10000
```

| 项 | 公式 | 来源 |
|---|---|---|
| **点差** SpreadCost | `Σ_legs (Ask−Bid) × C × L`（每腿按盘口半价 ×2 近似，或各自 ask/bid） | 实时 quote |
| **手续费** CommissionCost | PerLot: `Σ L × rate`；PerNotionalBps: `Σ notional × rate/10000` → 换 USD | `Listing.Commission`（§2.1，默认 0） |
| **滑点** SlippageCost | `n_legs × slippage_bps × Notional / 10000`（每腿入场各承担一次） | `Cfg.SlippageBps`（初值保守，Phase F → 归因 P95） |
| **swap** SwapCost | `NetSwapPerDay × effective_hold_days`，换 USD | `Listing.Swap`（§4.1）；hold_days 见 §5 |

- **Notional 定义**：归一化后两腿名义相等（§4.2 前提）→ `Notional = notional_A`（USD）。三腿三角取三条腿名义之和的 1/2（闭环特性，闭环相等时三腿名义同量级；以最大腿名义为基准，实现时精确化并单测）。
- **SwapCost 可为负**（Carry 净 swap 为收入，02 §5）：此时它**增加** NetProfit——swap 是 Carry 的利润来源。
- **GrossProfit 语义随策略**：CrossExchange/Triangular = Detector 给的毛价差（>0）；Carry = 0（无价差优势，利润来自 swap）。

### 4.5 Carry 专用：年化度量（02 §5.1）

仅 `Type==Carry`：
```
NetSwapPerDay    = Σ legs daily_swap_usd     （收息腿 +，对冲腿 −，组合净日 swap）
AnnualizedNetBps = NetSwapPerDay × 365 / Notional × 10000
HoldDaysHint     = Cfg.DefaultHoldDays（年化换算的持仓天数预设，运营可调）
各腿 LegRole      = Income（正 swap 腿）/ Hedge（负 swap 腿）
各腿 DailySwap    = §4.1 算出的该腿日 swap（USD）
各腿 AnnualizedBps = Leg.DailySwap × 365 / leg_notional × 10000   （UI 拆分展示，竞品借鉴 D-006）
SwapCost(Carry)  = NetSwapPerDay × HoldDaysHint   （为负 = 收入）
```
- 年化**只用于 Carry 展示与排序**，不替代 NetProfit（实际盈亏仍以 NetProfit 计，公理③）。
- CrossExchange/Triangular：`effective_hold_days = 0`（日内，不隔夜）→ SwapCost ≈ 0，年化字段零值不用。

---

## 5. 可执行性预检（02 §6 step6 + 07 §1）

全部通过 → `Executable=true`：

1. **主度量阈值**：CrossExchange/Triangular → `NetBps ≥ Cfg.MinNetBps`（初值 3 bp，02 §6）；Carry → `AnnualizedNetBps ≥ Cfg.MinAnnualizedNetBps`（年化口径，初值待 Phase F 校准）。
2. **风控三项**（查 `risk.CapitalGate` + 各 broker 账户余额快照，07 §1）：
   - 单机会敞口 ≤ `max_exposure_per_opportunity_pct`（5%）
   - 并发未平仓 ≤ `max_concurrent_opportunities`（5）
   - 单平台资金占比 ≤ `single_broker_exposure_pct`（40%）
3. **盘口宽度**：各腿 `(Ask−Bid)/Mid ≤ Cfg.MaxSpreadBps`（过宽 = 流动性差，滑点将吞利润）。
4. **成本可确定性**：swap 模式已校准（§4.1）、汇率可取（§4.3）、commission 已配置或显式标注。

任一不过 → `Executable=false`，`reason` 字段写明（desk 风险提示列映射，10 §4.1）。

---

## 6. 时间与置信（公理④）

- `ExpiresAt = QuoteTime + Cfg.QuoteFreshnessTtl`（报价有效期；desk 倒计时，过期 → Expired）。
- `Confidence`：P1 占位，初值 = `f(新鲜度余量, 盘口宽度/阈值比)`，Phase F 归因数据驱动校准（07 §4）。**非计价**，仅排序/展示。

---

## 7. 配置（config.proto 新增 `EvaluatorConfig`）

`SystemConfig` 加 `EvaluatorConfig evaluator = 6;`（02 §6 阈值 + 滑点的 config 归宿，§2.3）：

```protobuf
message EvaluatorConfig {
  // 主度量阈值（02 §6）。CrossExchange/Triangular 用 min_net_bps；Carry 用 min_annualized_net_bps。
  double min_net_bps              = 1;  // 初值 3.0
  double min_annualized_net_bps   = 2;  // Carry 年化口径，初值待 Phase F
  // 滑点预估（02 §4.1，归因 P95 校准前用保守静态值）
  double slippage_bps             = 3;  // 每腿入场滑点，保守初值（如 1.0）
  // 报价新鲜度（公理④）
  google.protobuf.Duration quote_freshness_ttl = 4;  // 初值 2s
  // 对冲手数归一化容差（02 §3.1）
  double hedge_notional_tolerance_pct = 5;  // 初值 1.0
  // Carry 默认预期持仓天数（年化换算分母，02 §5.1）
  int32  carry_default_hold_days  = 6;  // 初值 7
  // 盘口宽度上限（§5 第3条）
  double max_spread_bps           = 7;
}
```
> textproto + 生成代码 + cmd-core 加载须**全套同步**（STATE 注意事项：勿半截改 proto 源，否则 `buf generate` 崩 core 编译）。

---

## 8. 文件布局（300 软参考 / 450 硬红线，AGENTS §7）

```
internal/evaluator/
  evaluator.go      Evaluate 主流程（§3 七步编排）         ~150 行
  cost.go           成本四项 + NetProfit/NetBps（§4.4）    ~120 行
  swap.go           9 种 SwapType 换算（§4.1）             ~120 行
  hedge.go          对冲手数归一化（§4.2）                  ~90 行
  convert.go        USD 换算 + RateResolver（§4.3）        ~90 行
  carry.go          Carry 年化/LegRole（§4.5）             ~80 行
  types.go          Evaluator/Deps/Config/Opportunity 映射  ~80 行
  *_test.go         黄金用例（§9）                          按子模型
```
- listing 包补：`internal/listing/resolver.go`（§2.2 CanonicalIndex）、`types.go` 加 Commission 字段（§2.1）。
- 每文件单一职责，函数 ≤ 50 行；纯函数易测。

---

## 9. 测试计划（黄金用例 = 真实探测数据，锁定「准确无误」）

Evaluator 的正确性靠**真实数据驱动的表测试**锁死——这是 Phase A 验收数据（02 §7 对照表 + 03 §2.2 实测 swap）的直接复用：

| 用例 | 输入（真实） | 期望输出（手算） |
|---|---|---|
| `TestSwapInPoints_FX` | EURUSD ICMarkets：SwapLong=-8.287, Points=0.00001, C=100000, L=1, Buy | daily_swap = -8.287 USD |
| `TestSwapInPoints_XAU` | XAUUSD ICMarkets：SwapLong=-56.766, Points=0.01, C=100, L=1, Buy | daily_swap = -0.56766... USD（手算锁值） |
| `TestSwapInPoints_JPY` | GBPJPY：SwapShort=-23.186, Points=0.001, C=100000, L=1, Sell | -2318.6 JPY → 经 USDJPY 换 USD |
| `TestHedgeLots_SameContractSize` | 两腿 C=100000，L_A=1 | L_B=1（1:1，FX 主流） |
| `TestHedgeLots_DiffContractSize` | C_A=100, C_B=1000，L_A=10 | L_B=1（反比），取整后名义偏差 < 1% |
| `TestHedgeLots_OutOfTolerance` | 取整后偏差 > tolerance | Executable=false |
| `TestCarry_NegativeNetSwap` | GBPJPY ICMarkets做多(+11.67) + Exness做空(−39.8) | NetSwapPerDay < 0 → 不达年化阈值 → Executable=false（03 §2.2 实测：暂无正收益 Carry） |
| `TestConvert_JPYtoUSD` | 本币 JPY 金额 + USDJPY 报价 | USD 金额正确 |
| `TestFreshness_Stale` | Quote.Time 超 ttl | 返回 nil（丢弃） |
| `TestSwap_UnknownMode` | SwapType=SymInfo_s408 | Executable=false（不猜 0） |
| `TestExecutability_RiskGate` | 敞口超 5% / 并发超 5 / 单平台超 40% | 各自 Executable=false |

- `Evaluate` 端到端用一个构造的 CrossExchange 候选（两 broker EURUSD/EURUSDm），断言完整 `Opportunity` 字段。
- `go test -race -count=1 ./internal/evaluator/...` 必过（AGENTS §10）。

---

## 10. 回溯

- 成本模型 / swap / 净盈利 / 对冲手数 / 换算 → 公理②③（`00 §2`）、`02 §3/§4`
- 度量双轨 / LegRole / Carry 年化 → 竞品借鉴 `D-006`（`decisions.md`）、`02 §5/§5.1`
- 新鲜度 / ExpiresAt / 可执行性 → 公理④（`00 §2`）、`02 §6`
- 风控三项 → `07 §1`
- proto 字段（net_swap_per_day / annualized_net_bps / LegRole 等）→ `06 §5.2`
- Candidate 来源 / Detector 边界 → `03 §1/§4`
- 执行（仅确认后触发，all-or-nothing + 失败对冲）→ `04`、现有 `execute/pipeline.go`（其 `Notional()` 硬编码 100000 由本 Evaluator 的真实 Notional 取代，D-003）

---

## 11. 实现指引（Windsurf）

1. **先做前置**（§2）：`Listing` 加 Commission、`CanonicalIndex` 解析器、`EvaluatorConfig` proto 全套同步。前置不过，Evaluator 无输入可用。
2. 子模型按 §4 各文件独立实现 + 单测，最后 §3 主流程编排。
3. swap 换算**逐枚举覆盖**（§4.1），未实测模式走「Executable=false + 告警」，不猜值。
4. 黄金用例（§9）必须用 02 §7 真实数据——这是「准确无误」可核验的锚。
5. 交付前过 AGENTS §3 A–F（含 §10 机械检查）+ 更新 STATE.md。
