# 02 · 品种模型与机会评估

> 这是「准确无误」的核心落地：**机会的数据结构 + 净盈利计算**。
> 依据 `00-north-star.md` 公理①②③④；所有字段基于真实 MT5 探测数据（见 `discussion-log.md` 讨论七）。
> 实现者（Windsurf）照本文档落地；字段名对齐 proto 真实字段，不臆造。

---

## 1. 两层品种模型（公理①）

套利的本质是「同一逻辑品种，在不同 broker 实例之间」。模型必须分两层——否则要么看不见价差（无逻辑层），要么算错盈亏（参数混用，即当前 `Notional()` 硬编码 `*100000` 的根因）。

### 1.1 `Instrument`（逻辑品种，跨 broker 共享）

回答"这是什么"。很薄，定义身份（base/quote 是符号组成）；盈亏计价货币以 `Listing.ProfitCurrency` 为准（broker 间可能不同，公理②换算用）。

```go
type Instrument struct {
    Symbol     string  // 规范化逻辑符号，如 "EURUSD"、"XAUUSD"
    AssetClass string  // "FX" | "CRYPTO"
    Base       string  // "EUR" / "XAU" / "BTC"
    Quote      string  // "USD" / "USDT"
    Kind       string  // "SPOT"（FX 现货）/ "PERP"（永续，Crypto）
}
```

跨所比较时，靠它判定"ICMarkets 的 EURUSD 和 Exness 的 EURUSDm 是同一逻辑品种"。

### 1.2 `Listing`（某 broker 上的具体实例）

回答"在这个 broker 上它真实长什么样"。**所有"以实际为准"的真实参数都在这里**（公理①）。字段**直接映射** MT5 `SymbolParams` 返回的 `SymbolInfo` + `SymGroup`：

```go
type Listing struct {
    Broker       string         // "ICMarketsSC-Demo"
    BrokerSymbol string         // 原始符号，下单用，如 "EURUSDm"（不归一化，原样透传）
    Instrument   *Instrument    // 指向逻辑品种

    // —— 来自 SymbolInfo ——
    ContractSize   decimal.Decimal // 100000 (EURUSD) / 100 (XAUUSD)
    Digits         int32           // 5 / 2 / 3
    Points         decimal.Decimal // point size: 0.00001 / 0.01 / 0.001
    ProfitCurrency string          // "USD" / "JPY"   ★公理②换算关键
    MarginCurrency string          // "EUR" / "XAU" / "GBP"
    CalcMode       CalculationMode // 盈亏/保证金计算模式

    // —— 来自 SymGroup ——
    VolumeMin  decimal.Decimal // 0.01
    VolumeMax  decimal.Decimal // 200 / 100
    VolumeStep decimal.Decimal // 0.01
    Swap       Funding         // 见 §4.2
    InitMargin decimal.Decimal
    TradeMode     TradeMode      // FullAccess / Disabled / ...
    ExecutionType ExecutionType  // Market / ...
    FillPolicy    FillingFlags   // IOC / ...
    TripleSwapDay V3DaysSwap     // 三倍 swap 日（通常周三）

    // 动态报价 bid/ask/time 不在这里——走 QuoteBus 流（hot path, float64）
}
```

> 真实数据印证（ICMarkets vs Exness）：`ContractSize` 因品种异（100000/100）→ 每品种须独立 Listing；`ProfitCurrency` 因品种异（USD/JPY）→ 盈亏须换算；`Digits` 跨 broker 可能不同（XAUUSD ICMarkets=2, Exness=3）。

---

## 2. 符号归一化（公理①的配套）

不同 broker 同品种符号不同（ICMarkets `EURUSD`、Exness `EURUSDm`；少数特例（如某些 broker 用 `GOLD` 代 `XAUUSD`，非本次探测样本））。系统须一张**符号映射表**：`brokerSymbol → 逻辑 Symbol`。

- **不违反** `constraints` 的"raw broker symbol = canonical"——原始 `BrokerSymbol` 仍原样用于**下单**；映射只用于**比较/发现机会**。
- 维护方式：**人工**（用户确认差异小，主要后缀 + 少量特例）。存 PG 表或配置。
- 实现提示：可参考 `tools/probe` 的 `findSym`（按 base 尝试 `""`/`m`/`z`/`pro` 后缀）做自动初筛，人工确认落表。

---

## 3. 盈亏换算（公理②）

公理②是**换算器，不是抹平器**（见 `00 §2` 公理②澄清）。各 Listing 的真实参数如实保留，盈亏按下式换算到统一度量 USD：

```
每腿本币盈亏 = 价差 × ContractSize × Lots          [本币 = ProfitCurrency]
统一盈亏(USD) = 本币盈亏 × 汇率(ProfitCurrency → USD)
统一度量      = USD 绝对值  +  bps(净盈利 / 名义价值)
```

- **不依赖 `TickValue`**：实测 broker 未填该字段（零值）。用 `ContractSize × Points × ProfitCurrency` 手算。
- **JPY 计价品种**（GBPJPY/USDJPY，ProfitCurrency=JPY）：盈亏先得 JPY，再按 USDJPY 汇率换 USD。

---

## 4. 成本模型（公理③ —— "准确无误"的命门）

机会 = 毛利差 − **全部**成本。漏一项 = 假机会。

### 4.1 净盈利公式

```
NetProfit = GrossProfit − SpreadCost − CommissionCost − SlippageCost − SwapCost
NetBps    = NetProfit / Notional × 10000
```

| 成本项 | 公式 | 数据来源 |
|---|---|---|
| **点差** | `(Ask − Bid) × ContractSize × Lots`（每腿） | 实时 quote |
| **手续费** | `Lots × CommissionRate` | Listing（broker 配置；MT5 SymbolInfo 未提供，见下） |
| **滑点** | 实测滑点分布 P95（从归因记账校准，初值保守估） | 归因 |
| **swap** | 按持仓天数 × SwapType 换算（§4.2） | Listing.Swap |

> **公理③完整性**：上表是交易摩擦成本。公理③还列了**汇兑/出入金成本**与**资金占用机会成本**——第一版按经验值估算（汇兑按 broker 出入金费率；机会成本 = 占用资金 × 无风险利率 × 天数，主要影响长期持仓的 Carry），Phase F 归因校准真实值。不忽略、不抹平。

### 4.2 swap 换算（按 proto `SwapType` 枚举，9 种）

实测两边都是 `InPoints`，但跨 broker/品种可能不同，evaluator 须覆盖全枚举。**主公式（InPoints，已用真实数据验证）**：

```
SwapType_InPoints:
  swap货币/天 = SwapLong(或 Short) × Points × ContractSize × Lots
  例 EURUSD ICMarkets 做多 1 手隔夜:
      -8.287 × 0.00001 × 100000 × 1 = -8.287 USD/天   ✓
  例 XAUUSD Exness(XAUUSDm) 做多 1 手隔夜:
      -509.9 × 0.001 × 100 × 1 = -50.99 USD/天        ✓
```

其余模式（实现时按 MT5 `ENUM_SYMBOL_SWAP_MODE` 精确化）：
- `MarginCurrency` / `Currency`：`SwapValue` 本身是货币 → `swap = SwapValue × Lots`（按结算货币）。
- `PercCurPrice` / `PercOpenPrice`：百分比模式 → `swap = SwapValue% × Price × ContractSize × Lots`（注意年化/日化的换算口径，按 MT5 文档）。
- `PointClosePrice` / `PointBidPrice` / `SymInfo_s408`：按 MT5 文档实现。
- `SwapNone`：不计 swap。

### 4.3 funding 统一抽象（FX swap ≡ Crypto funding rate）

FX 的 swap 和 Crypto 的 funding rate 本质同——"持仓融资成本"。统一建模为 `Funding`：

```go
type Funding struct {
    SwapType       SwapType        // 决定换算公式（FX）；Crypto 用 PERCENTAGE
    SwapLong       decimal.Decimal
    SwapShort      decimal.Decimal
    SettlementFreq SettlementFreq  // DAILY（FX 隔夜）/ EVERY_8H（Binance 永续）
    TripleSwapDay  int32           // FX 三倍 swap 日；Crypto 无
}
```

→ FX 套息和 Crypto 资金费率套利**共享同一套成本计算代码**，只是 `SwapType`/`SettlementFreq` 不同。这正是统一地基（公理①）让跨资产成为可能的落点。

### 4.4 手续费的来源（现实约束）

MT5 `SymbolInfo`/`SymGroup` **不提供 commission**（实测）。commission 是 broker 账户/协议级别设定。获取方式：
- **下单前（评估）**：用 broker 公布的 commission 率（**人工录入 Listing**，静态参数）。
- **下单后（校准）**：成交记录 `Order.Commission` 有实际值（归因记账校准预估）。

> 即：commission 和 swap 一样，是"人工静态参数 + 成交校准"，不是 API 实时拉。这是 MT5/mtapi.io 的固有限制，设计接受。

---

## 5. `Opportunity` 对象（准确无误的物理化身）

```go
type Opportunity struct {
    ID        string
    Type      OppType    // CrossExchange / Carry / Triangular
    Legs      []Leg      // 每腿：Listing + 方向(Buy/Sell) + Lots + 预估价
    QuoteTime time.Time  // 价格采样时刻（公理④新鲜度基准）

    // —— 成本拆解（warm path, decimal）——
    GrossProfit    decimal.Decimal
    SpreadCost     decimal.Decimal
    CommissionCost decimal.Decimal
    SlippageCost   decimal.Decimal
    SwapCost       decimal.Decimal // 按预期持仓时长预估
    NetProfit      decimal.Decimal // = Gross − 上述全部
    NetBps         decimal.Decimal // 统一度量（跨机会排序用）

    // —— 准确性（公理④）——
    ExpiresAt  time.Time // 报价新鲜度有效期
    Executable bool      // 可执行性预检结果（盘口宽度/资金/净值>阈值）
    Confidence float64

    Status OppStatus // Pushed → Confirmed → Executing → Filled/Failed/Expired（Candidate 无状态，见下状态机）
}
```

**状态机**：Detector 产出 `Candidate`（无状态，仅价差）→ Evaluator 评估，通过则产出 `Opportunity(status=Pushed)`、不通过则丢弃。Opportunity 状态：`Pushed`（推送 desk）→ `Confirmed`（你点确认）→ `Executing` → `Filled`/`Failed`/`Expired`。完整状态机见 `04 §2`。

---

## 6. 机会评估流程（Evaluator）

```
候选机会（Detector 产出，仅价差）
   │
   ▼
Evaluator 评估：
  1. 拉各腿最新 quote（公理④：校验新鲜度，过期则丢弃）
  2. 用各腿 Listing 真实参数算：毛利差 → 点差/手续费/滑点/swap → NetProfit（§4）
  3. 盈亏换算到 USD + bps（§3）
  4. 可执行性预检：净盈利 NetBps > 阈值(实测滑点P95+安全垫)；风控三项（见 07 §1）：单机会敞口 ≤5%、并发未平仓 ≤5、单平台资金占比 ≤40%
  5. 设 ExpiresAt（报价有效期）、Confidence（Confidence 算法 P1 占位：初值基于报价新鲜度+盘口宽度，Phase F 归因数据驱动校准）
   │
   ▼
Opportunity（Executable=true 的）→ 推送 desk（04-human-in-loop）
```

阈值（`discussion-log` 讨论五）：`NetBps ≥ 实测执行滑点P95 + 安全垫`，初值 `≥ 3 bp`，随归因自适应。

---

## 7. 实现指引（给 Windsurf）

| 组件 | 动作 |
|---|---|
| `adapter` | `SymbolParamsRaw` 已有（拉完整 `SymbolInfo`+`SymGroup`）。再封装一个 `Listing(ctx, brokerSymbol) (*Listing, error)`，把 proto 字段映射到 §1.2 的 `Listing` 结构。MT4 暂不实现（第一版 MT5）。 |
| `Listing` 缓存 | 启动时为所有订阅品种拉 Listing；swap 每日变 → 定期刷新（`SettlementFreq=DAILY`，每日/开仓前刷新）。warm path, decimal。 |
| 符号映射 | PG 表 `symbol_map(broker, broker_symbol, canonical_symbol)`，人工维护；启动加载。 |
| `Instrument` 仓库 | 逻辑品种目录（从 symbol_map + Listing 推导），跨 broker 共享。 |
| `Evaluator` | 纯函数：`(candidate, legs' Listings, quotes) → Opportunity`。无副作用，易测。swap 按 §4.2 各 SwapType 实现。 |
| 盈亏换算 | `decimalutil`（已有）。JPY 等非 USD 计价品种按 §3 换算。 |
| 归因 | 成交后用 `Order.Swap/Commission` 实际值校准 §4 的预估（swap/commission/滑点）。 |

### 已验证的真实字段对照（实现时照此映射）

| `Listing` 字段 | proto 来源 | EURUSD(IC) | XAUUSD(IC) | GBPJPY(IC) |
|---|---|---|---|---|
| ContractSize | SymbolInfo.ContractSize | 100000 | 100 | 100000 |
| Digits | SymbolInfo.Digits | 5 | 2 | 3 |
| Points | SymbolInfo.Points | 0.00001 | 0.01 | 0.001 |
| ProfitCurrency | SymbolInfo.ProfitCurrency | USD | USD | JPY |
| VolumeMin/Max/Step | SymGroup.{Min,Max}Lots/LotsStep | 0.01/200/0.01 | 0.01/100/0.01 | 0.01/100/0.01 |
| SwapType | SymGroup.SwapType | InPoints | InPoints | InPoints |
| SwapLong/Short | SymGroup.SwapLong/Short | -8.287/+1.544 | -56.766/+38.929 | +11.67/-23.186 |
| ExecutionType | SymGroup.TradeType | Market | Market | Market |

---

## 8. 回溯

- 两层模型 / 字段来源 / 符号归一化 → 公理①
- 盈亏换算 → 公理②（换算非抹平）
- 成本模型 / swap / 净盈利 → 公理③
- ExpiresAt / 新鲜度 / 可执行性预检 → 公理④
- Opportunity 推送-确认-执行链 → `04-human-in-loop.md`
- 执行（all-or-nothing + 失败对冲）→ 现有 `execute/pipeline.go`（保留，改为仅确认后触发）
