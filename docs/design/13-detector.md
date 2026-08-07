# 13 · Detector — 机会发现扫描器

> 「发现」层：轻量纯函数扫描最新报价 + Listing → 仅毛价差的 `Candidate`（03 §1 职责边界）。
> Detector **不算成本、不判可执行、不调 I/O**——那是 Evaluator 的职责（12）。
> 依据：`03`（三类 + Candidate 接口 + 确定性分级）、`00 公理①②`（统一市场抽象 + 等价度量）、B-0 `CanonicalIndex`（已有输入）。
> 施工：Windsurf。纯函数、warm path decimal、高频轻量。

---

## 0. 定位

```
QuoteBus（最新报价）───┐
CanonicalIndex（品种视图）─┼──► [Detector] ──► []Candidate（仅毛价差）
                          │       │ 纯函数，无副作用
                          │       ▼
                  [Evaluator] 扣成本 + 可执行性 → Opportunity
```

- Detector = **发现**（价差存在吗？/ swap 结构正收益吗？）。
- Evaluator = **评估**（成本后还赚吗？可执行吗？）。
- 分离好处：Detector 高频每 tick 扫描（~microsecond），重计算只对产出 Candidate 做（Evaluator）。
- Detector 不调 broker I/O、不发 RPC、不写 DB。它消费已有的 QuoteBus 快照 + CanonicalIndex 视图。

---

## 1. 包与接口

包：`internal/detector/`（Layer 3，依赖 `listing` / `bus` / `evaluator` [仅 Candidate 类型共享]，**不依赖** `execute`/`adapter`——单向）。

Candidate 类型已在 `evaluator` 包定义（`evaluator.Candidate` / `CandidateLeg`）——Detector **不重建类型**，直接 import `evaluator`。走 import 的理由：Detector→Evaluator 是单向数据流，Candidate 是它们之间的契约类型；Evaluator 定义它（Evaluator 是消费者，定义输入格式），Detector 导入它（生产者）。不违反 `code-map` Layer 3→Layer 3 互导规则（同层、单向）。

> 若 `evaluator.Candidate` 目前缺字段通过 B-1 时未纳入，**本任务在 evaluator 包追加**，
> 然后 Detector import 同一个类型。禁止在 detector 包重复定义 Candidate。

```go
// Detector 扫描最新行情发现候选机会（03 §4）。
type Detector interface {
    Type() evaluator.OppType
    Scan(ctx context.Context, quotes map[string]bus.Quote,
         listings map[listing.CanonicalKey]*listing.Listing) ([]evaluator.Candidate, error)
}

// NewCrossExchange 创建跨所价差检测器。
func NewCrossExchange() Detector

// NewCarry 创建套息检测器。
func NewCarry() Detector

// NewTriangular 创建三角检测器。
func NewTriangular() Detector
```

- `quotes`：key = brokerSymbol（raw），value = 最新 bus.Quote。
- `listings`：key = (broker, canonical)，value = *Listing（Instrument 已填）。用 `CanonicalIndex`（B-0）产出。
- `Scan` 纯函数：同一入参恒定输出。易测。
- 候选手数 = 基准腿的 `VolumeMin`（最小有意义单位）；Evaluator 归一化。

---

## 2. 前置依赖（Phase B → C 的补齐）

B-0/B-1 已落地：`CanonicalIndex`（品种→broker 视图）、`evaluator.Candidate/CandidateLeg` 类型。还需：

### 2.1 运行框架（main.go 或 engine 层）
Detector.Scan 是纯函数——谁调用它？需要一个轻量的**扫描循环**挂在 QuoteBus 上：

```
// 伪代码，在 cmd/core/main.go 或 engine 层实现
for {
    quotes := quoteBus.Snapshot(ctx, allBrokerSymbols)
    listings := cache.CanonicalIndex(symMap)
    for _, det := range detectors {
        candidates := det.Scan(ctx, quotes, listings)
        for _, c := range candidates {
            eval.Evaluate(ctx, c)  // → Opportunity → push desk
        }
    }
    time.Sleep(scanInterval)  // 或 select on ticker
}
```

- 扫描间隔：初值 ~100ms（QuoteBus 快照是 RWMutex protected，轻量；扫描 O(B²×S) 但 B=2~4、S=10~20，远未到瓶颈）。
- **本任务只做 Detector 纯函数 + 测试**；循环框架由 Windsurf 在 Phase C 收尾或后续 engine 接线时加（类似 B-0 的 proto 加载模式——proto 定义+生成在 B-0，使用在 B-1）。

### 2.2 无需额外补齐
CanonicalIndex（B-0）、evaluator.Candidate、Listing.Commission 均已就位。Detector 直接消费。

---

## 3. CrossExchange 扫描（01 优先实现，03 §2.1）

**逻辑**：同一 canonical 品种在两个 broker 间，一侧 Ask < 另一侧 Bid → 价差为正。

### 3.1 算法

```
输入: quotes[brokerSymbol]bus.Quote, listings[(broker,canonical)]*Listing
输出: []Candidate

1. 按 canonical 分组：收集每个 canonical 的 broker 列表。
   每组含 (broker, brokerSymbol, Listing, Quote)。
2. 对每个 canonical 的每对 broker (A, B):
   a) 方向 A Buy + B Sell：askA=Quotes[A].Ask, bidB=Quotes[B].Bid
      若 askA < bidB:
         spread = bidB − askA
         grossProfit = spread × Listing_A.ContractSize × Lots_A
         其中 Lots_A = Listing_A.VolumeMin
         → 产出 Candidate{CrossExchange, legs: [A/Buy/Lots_A/Ask_A, B/Sell/Lots_B_hint/Bid_B], GrossProfit}
   b) 方向 B Buy + A Sell：同理（bidA vs askB）
   c) Lots_B_hint = Listing_B.VolumeMin（Estimator 归一化后才定最终手数）
      Detector 不归一化——那是 Evaluator 的职责（03 §1）。
3. 去重：同一 canonical 同一 broker pair 只产一个 Candidate（选价差更大者）。
   若 GrossProfit ≤ 0 → 不产。
```

- **复杂度**：对 canonical 数 C、每 canonical 平均 B 个 broker → O(C × B²)。
  第一版 C≈10（EURUSD/GBPUSD/USDJPY/XAUUSD 等）、B=2~3 → ~60 对，< 1ms。
- **无方向风险**：两腿对冲锁价差，不赌方向。
- **引用**：02 §3.1（对冲手数归一化，Evaluator 做）、03 §2.1。

### 3.2 边界
- broker 对不对称（如 ICM 有 EURUSD 但 Exness 只有 EURUSDm）→ symbol_map 已归一为同一 canonical，CanonicalIndex 解决了此问题。
- 仅一个 broker 有某 canonical → 跳过（无配对）。
- Quote.Time 陈旧 → 由 Evaluator 新鲜度检查丢弃。Detector 不判新鲜度（保持轻量）。

---

## 4. Carry 扫描（03 §2.2）

**逻辑**：寻找对冲后**净 swap 为正**的 broker 对。

### 4.1 算法

```
1. 按 canonical 分组（同 §3.1）。
2. 对每个 canonical 的每对 broker (A, B):
   两方向：
   a) A Buy(Buy_A_long_swap) + B Sell(Sell_B_short_swap):
      dailyA = SwapLong_A × Points_A × ContractSize_A × VolumeMin_A  [本币]
      dailyB = SwapShort_B × Points_B × ContractSize_B × VolumeMin_B  [本币]
      注：SwapShort 用于 Sell（MT5 约定：SwapShort 是持有空头仓位时的 swap rate）
      注：dailyA/B 的正负取决于 Swap 符号——正值=收入，负值=支出
      netSwapCcy = dailyA + dailyB  （同品种 canonical，ProfitCcy 相同）
      if netSwapCcy > 0 → produce Candidate{Carry, GrossProfit=0}.
   b) B Buy + A Sell：同理（用 SwapLong_B + SwapShort_A）。
```

> **诚实记录**（03 §2.2）：ICMarkets + Exness 的 EURUSD/GBPJPY 实测，**所有对冲组合 netSwap 为负**。Carry Detector 对当前数据**产出 0 个 Candidate**——这是系统的正确行为（不推假机会）。4 个 broker+更多品种覆盖后可能出现正收益组合；外部竞品佐证（D-006）也确认正收益对冲套息真实存在。

- **Swap 公式**：与 `evaluator.DailySwap()` 一致（InPoints = S × Points × ContractSize × Lots）。不重复实现——本包 import evaluator 调 `evaluator.DailySwap()`。
- **GrossProfit = 0**：Carry 无价差利润，利润来自 swap（SwapCost 在 Evaluator 为负/收入）。
- **腿角色**：收息腿→Income，付息腿→Hedge。Detector 不标（Evaluator 的 carry.Compute 标）。
- **不调 I/O**：所有 swap 数据已在 Listing.Swap（Phase A 缓存，每日刷新）。

---

## 5. Triangular 扫描（03 §2.3，最后实现）

**逻辑**：同一 broker 内三个货币对的交叉汇率偏差。闭环对冲，无方向风险，但**三腿同时成交最难**（03 §2.3）→ 优先级最低。

### 5.1 算法

```
输入: 同一 broker 的所有品种。
前提: 对每 broker，构建以 base/quote 为节点的有向图。
      Triangular 需要三个货币(A,B,C)和三对(A/B, B/C, A/C)都在此 broker 存在。

对每个 broker，找所有互异的货币三元组 (A,B,C) 满足 broker 有 A/B, B/C, A/C。

每条边的报价方向：
  A/B 表示 "CCY1=A, CCY2=B"。如果 broker 直接挂牌 A/B，用其 Bid/Ask。
  否则看挂牌格式：EURUSD = EUR/USD → base=EUR, quote=USD。

对每条闭环：
  方向 1（正循环 A→B→C→A）：
    leg1: Buy  A/B @ Ask_AB     → 1 A = Ask_AB B
    leg2: Buy  B/C @ Ask_BC     → Ask_AB B = Ask_AB × Ask_BC C
    leg3: Sell A/C @ Bid_AC     → 回 A: Ask_AB × Ask_BC / Bid_AC? 换个角度：
    从 1 A 出发：
      A→B: Buy  A/B → 1 A 得 Ask_AB 单位 B ✓
      B→C: Buy  B/C → Ask_AB 单位 B 得 Ask_AB × Bid_BC 单位 C？不——
      这里关键区别：如果挂牌是 B/C（base=B, quote=C），Buy B/C @ Ask_BC ：
        付出 C 得到 B → 1 B = 1/Ask_BC C？反过来：
        Ask_BC 是 "买 1 单位 B 需要支付多少 C"。
        所以 Ask_AB 单位 B → 需要 Ask_AB × Ask_BC 单位 C。支出 = Ask_AB × Ask_BC C。
      C→A: Sell A/C @ Bid_AC：
        A/C = A as base, C as quote. Sell @ Bid_AC 意味着我们卖出 A、收入 C。
        卖出 1 A 得到 Bid_AC C。我们想从 C 换回 A：
        需要多少 C 买回 1 A？Ask_AC。但我们卖了——这是最后一步。
      
      完整：从 1 A 出发 → 经 A/B + B/C 合成 C 头寸 → 经反向 A/C 转回 A。
      
      ★ 标准三边公式（以 A/B, B/C, A/C 全为 "base/quote" 挂牌）：
      收益 A = (Bid_AC × 1) − (Ask_AB × Ask_BC × 1)？不。
      
      实际上从 A 开始：用 A 买 B(A/B) → 用 B 买 C(B/C) → 用 C 买回 A(C/A)。
      
      但 C/A 并不挂牌！挂牌是 A/C。所以 Sell A/C = Buy C/A 的反向。
      Buy C/A @ price = 1/Ask_AC（你用 C 买东西要付多少 C per 单位 A）
      Sell A/C @ Bid_AC — 你卖 A 得到 C per A。
      
      更简洁（以 B/C 的 base=CCY2, quote=CCY3）:
      
      假设 broker 有这三对:
        A/B: base=A, quote=B  (如 EUR/USD)
        B/C: 牌价 GBP/USD——但这是 B=GBP, C=USD，base=B, quote=C。对齐。
        C/A: 不存在！但 A/C 存在。A/C 的 base=A, quote=C。
      
      **实现级简化**（不要求全自动图搜索——初版可以假定一个货币图，三角手动枚举配合交叉配对）：
      
      鉴于初版 broker 少（2-4）、品种有限（~10 FX pairs），三角三元组**人工枚举**，避免复杂图搜索。
      枚举已知 FX 三角：{EUR,USD,GBP}、{EUR,USD,JPY}、{EUR,USD,CHF}、{EUR,GBP,JPY}等。
      每个三元组取三个 pair，找正向挂牌的换算公式。
    
    ★ 具体实现策略（初版确定性的枚举方式）：
    
    以 {EUR,USD,GBP} 为例，broker 有 EURUSD(A/C)、GBPUSD(B/C)、EURGBP(A/B)：
      这三对全是标准 "base/quote" 挂牌。A=EUR, B=GBP, C=USD。
      
      方向 1（EUR→GBP→USD→EUR / 卖出 EURGBP、买入 EURUSD、买入 GBPUSD?）:
        用实价表达：
        EURUSD bid = 价格(卖 EUR 买 USD)
        EURUSD ask = 价格(买 EUR 付 USD)
        
        一个环：买 EUR 用 USD → …… 不对，从开始：
        
        **固定实现**（不写通用图搜索；初版 3 个手动公式即可）:
        
        三元组 1：{EUR,USD,GBP} —— pairs: EURUSD(A/C), GBPUSD(B/C), EURGBP(A/B)
        方向：
          op_1: 买 EURUSD @ Ask, 买 EURGBP @ Ask, 卖 EURGBP @ Bid → 闭合价差扫描。
        
        更好的固定方向写法：
          环1) Buy EURUSD(Ask), Buy GBPUSD(Ask), Sell EURGBP(Bid)
            每 1 EUR 出发 → USD → GBP → EUR
            美金金额 per 1 EUR = 1 / Ask_EURUSD // 买 EURUSD 是付 USD 得 EUR，反过来是 1 EUR 得 1/Ask USD。等一下我用 direction 固定一下：
            
              EURUSD direction=Buy  → 付美元得欧元？不对。
              外汇惯例：Buy EURUSD = 买 EUR、卖 USD，用 Ask price。按 Ask=1.08，1 EUR 需 1.08 USD。
              所以如果方向是 Buy EURUSD @ Ask，含义是 **我们把 EUR 兑换成 USD** 是逆的。
              
              ═══ 显式表达 ═══
              从 1 EUR 出发：
                把 EUR 换成 USD：这等价于 **卖出 EURUSD**（卖 base、得 quote），用 Bid。
                → 得到 Bid_EURUSD USD。
                
                把 USD 换成 GBP：等价于 **卖出 GBPUSD**？不对——
                GBPUSD = GBP/USD。把 USD 换成 GBP = **买入 GBPUSD**（花 USD 买 GBP/买 base 付 quote），用 Ask。
                → 需要多少 USD 买 1 GBP？ Ask_GBPUSD。
                → Bid_EURUSD USD 能买 Bid_EURUSD / Ask_GBPUSD 英镑。
                
                把 GBP 换成 EUR：= **卖出 EURGBP**（卖 base EUR 得 quote GBP？不是——EURGBP = EUR/GBP。
                卖出 EURGBP = 卖 EUR、收入 GBP。这是最后一步的反向：我们需要用 GBP 买入 EUR，
                即 **买入 EURGBP**（用 GBP 买 EUR/买 base 付 quote），用 Ask_EURGBP。
                → GBP amount / Ask_EURGBP EUR 回来。
                
              最终收益 = Bid_EURUSD / Ask_GBPUSD / Ask_EURGBP EUR per 1 EUR 出发。
              净收益 = 最终 - 1。
              若 > 1 → 存在套利。GrossProfit = (net) × ContractSize × baseLots。
            
           但上面的分析容易出错——让 windsorfer 实现时查 MQL5 或经典三角套利公式，
           本文档只定方向、不要求全自动推导。Windsofer 可用枚举三元组+手动公式实现。

           **简化落地指令**：
           - Detector 枚举 3–5 个已知 FX 三角（EUR/USD/GBP、EUR/USD/JPY、EUR/USD/CHF、EUR/GBP/JPY、USD/GBP/JPY）
           - 每三角对 broker 报价取 Bid/Ask，按标准三边公式算两个闭环方向，哪边 > 1 即 Candidate
           - 不写通用图搜索——初版 broker+品种少，枚举 > 通用性；后期（Crypto 接入、更多 broker）可重构
```

> **说明**：三角的精确公式涉及挂牌方向（base/quote）的正反操作。上述伪代码是指引，精准公式由 Windsurf 实现时查 MQL5 三角套利文献确定，用 broker 真实 Ask/Bid 验算。初版枚举 3–5 三角，不写全自动图搜索。

### 5.2 复杂度
- 三角个数 × broker 数。枚举 5 三角 × 3 broker = 15 次检查。微不足道。
- **腿**：3 条（同 broker）。执行风险最高——三腿同时成交比两腿更难（03 §2.3）。Detector 只负责"发现"，风险在 Evaluator 的置信度 + 执行管线的 all-or-nothing（04 §6）。

---

## 6. Quote 消费与组合模式

Detector.Scan 是纯函数，入参 `quotes map[string]bus.Quote` 和 `listings map[CanonicalKey]*Listing`。调用方如何产生这两个 map：

1. **Snapshot**（推荐，符合 Push-First）：调用方定时（~100ms）从 `QuoteBus.Snapshot(ctx, allBrokerSyms)` 取最新报价快照，传给 Detector。
   - QuoteBus.Snapshot 是 RWMutex 保护的无锁 map 读取——轻量，热路径友好。
2. **Per-tick**：QuoteBus.Subscribe 逐品种 channel，每 tick 触发达标的 Detector。但需要按 canonical 路由（一个 canonical 的 quote 到达 → 只扫该 canonical 的 broker 对）。更精细但复杂度高——初版做 Snapshot 即可，per-tick 留后期优化（09 §2 canonical 路由）。

- **当次扫描的去重**：同一 canonical 同一 broker pair 只产出**一个** Candidate（取净价差较大者）。更严格的跨 tick 去重（同一机会上一 tick 已产出）由 Evaluator 外层的去重缓存负责（不在 Detector 内）。

---

## 7. 文件布局（300 软参考 / 450 硬红线）

```
internal/detector/
  detector.go        ~150  — Detector 接口 + Scan 入口（quote→canonical 分组→分配子检测器）
  cross_exchange.go  ~120  — CrossExchangeDetector{}.Scan()
  carry.go           ~100  — CarryDetector{}.Scan()
  triangular.go      ~120  — TriangularDetector{}.Scan()
  detector_test.go   ~250  — 黄金用例（§8）
```

每文件单类扫描器。`detector.go` 含接口 + `Scan()` 分发逻辑（group by canonical → 调用子检测器）。不新建 orchestrator 文件（运行时框架在 main.go 或 engine 层）。

---

## 8. 测试计划（黄金用例 = 真实数据 + 构造偏差）

| 用例 | 输入 | 期望 |
|---|---|---|
| `TestCrossExchange_FindsPositive` | 2 brokers EURUSD: Ask_A=1.0800, Bid_B=1.0804, C=100000, Lots=0.01 | 产出 1 Candidate, GrossProfit>0 |
| `TestCrossExchange_NoSpread_NoCandidate` | Ask_A ≥ Bid_B for all pairs | 0 candidates |
| `TestCrossExchange_OffByOnePair` | 仅 1 broker 有 canonical | 0 candidates |
| `TestCarry_NegativeNetSwap_NoCandidate` | ICM+Exness 真实 swap（-8.287/+1.544, -5.0/+2.0）EURUSD | 0 candidates（当前数据预期） |
| `TestCarry_PositiveNetSwap_FindsCandidate` | 构造 broker A SwapLong=+100, broker B SwapShort=+50, C=100000, P=0.00001 | 1 Candidate, NetSwap>0 |
| `TestTriangular_FindsDeviation` | 1 broker, 构造 EUR/USD/GBP 报价使得跨率偏差 >1 | 1 Candidate |
| `TestTriangular_NoDeviation_NoCandidate` | 合理报价（跨率≈1） | 0 candidates |

- 真实 EURUSD/XAUUSD 值来自 02 §7 对照表。
- `bus.Quote` 构造取 `bus.Quote{Symbol, Broker, Bid, Ask, Time: time.Now()}`。
- `listing.Listing` 构造复用 B-1 的 `testListing()` 模式或直接用 `evaluator_test.go` 的 helper（跨包引用测试工具需在 adapter 或 listing 提供——Windsurf 在本包重写简单版即可）。

---

## 9. 实现指引（Windsurf）

1. **Intra-package import evaluator**：Candidate/CandidateLeg/OppType 在 `evaluator` 包。Detector import evaluator 单向，不违依赖方向。
2. 优先 **CrossExchange**（§3），再加 Carry + Triangular。
3. 三角枚举 3–5 已知三元组，不写通用图搜索。
4. 黄金用例优先 CrossExchange + Carry（三角复杂且优先级低，至少写一个基本面 case）。
5. 交付前过 AGENTS §3 A–F + §10（build/vet/test-race/check-lines）。
6. 不改设计文档（§3.0），遇矛盾上报 Claude。

---

## 10. 回溯

- Candidate 接口 + 三类 → 03 §1/§4
- 跨所价差优先、确定性分级 → 03 §2/§3
- CanonicalIndex（品种→broker 视图）→ B-0（`listing.CanonicalIndex`）
- Evaluator（接收 Candidate，扣成本）→ 12（`evaluator.Evaluate`）
- QuoteBus snapshot → 现有 `bus.QuoteBus.Snapshot`
- 执行风险（三腿 all-or-nothing）→ 04 §6、现有 `execute/pipeline.go`
