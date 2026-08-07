# ARB 设计讨论纪要

> 本文件记录系统设计讨论的**推演过程**（含被否方案、关键洞察、未决疑问），与 `decisions.md`（最终决策）互补：
> 这里记"**怎么想到的**"，`decisions.md` 记"**决定了什么**"，`STATE.md` 记"**现在到哪了**"。
> 创建：2026-08-07。持续追加，新讨论接在末尾。

## 如何使用
- 翻阅设计思路 / 取舍理由 → 按主题查本文件。
- 查最终拍板 → `docs/handoff/decisions.md`（D-001…）。
- 查当前状态 → `docs/handoff/STATE.md`。

---

## 讨论一 · 第一性审视：系统的北极星

**北极星（用户定义）**：给"下单者"（用户自己）提供**准确无误**的**盈利机会**。

**审视发现（代码 + 设计文档两面核实）**：
1. **方向冲突**：设计文档原意是「Core 全自动交易、人只监督」(`evaluation-framework.md:1394`)，与"给我提供机会、我来决策"直接冲突。这是设计方向 vs 用户目的的冲突，不是代码 bug。
2. **成本模型**：设计完整（§4.3.1：点差+手续费+滑点+swap），但**代码零实现**；`Notional()` 硬编码 `*100000` 且算的是名义价值不是盈利。
3. **套息缺失**：用户要"套息"，但设计里 swap 只是成本项，无 carry 策略。
4. **Opportunity 半定义**：持久化 `signals` 表有 `gross_bps/net_bps`，但 Go struct 字段、净利润公式都没定义。
5. **期现名存实亡**：评估框架列为四类之一，`implementation.md` 连策略文件都没有。
6. **物理事实**：「准确无误」分两层——**发现时准确**（可做到）vs **执行后准确**（跨 broker 同时成交不可能 100%，`evaluation-framework.md:523`）。人工确认恰好夹在两层之间，多争取一层确定性。

**结论**：系统重新定位为「发现+评估+人工确认+执行」（混合模式），见 D-003。

---

## 讨论二 · 发现机会的四条公理

从北极星倒推：机会永远来自"比较"，而"全网跨品种"意味着比较对象是**异构**的。由此推出不可断裂的公理链——**缺一环，机会就不成立**：

| 公理 | 含义 | 缺了会怎样 |
|---|---|---|
| ① 统一市场抽象 | 任意市场的任意品种用同一模型描述（报价/合约/成本/规则） | 看不见、无从比较 |
| ② 统一盈亏度量 | 换算到统一计价货币/收益率（换算非抹平，见 00 §2） | 比不出大小 |
| ③ 净盈利真实计算 | 毛 − 全成本（含融资/汇兑/机会成本） | 机会是幻觉 |
| ④ 时间一致性 | 可信时间戳 + 新鲜度度量 + 淘汰过期数据 | 拿历史当现在（假信号） |

**关键洞察**：四条公理的共同形态 = 一个"统一的、归一化的、含全成本、带时间校准的**机会评估坐标系**"。策略（套利/套息）只是跑在这个坐标系上的判定函数。**坐标系本身是系统最核心的资产。**

### 补充 · 公理②澄清：换算 vs 抹平（2026-08-07，用户质疑推动）

**用户质疑**：各 broker 合约大小/报价/利息都不同，"统一价值尺度"是不是和"以实际情况为准"矛盾？

**澄清**：不矛盾，是串联两步。
- 公理① = **如实保留**每个 broker 的真实参数（contractSize/swap/pointValue 各不同，原样，以实际为准）。
- 公理② = **换算器**：把基于真实参数的本币盈亏映射到统一度量（USD + bps），用于跨 broker/跨品种**比较与排序**。

**统一的是"度量尺子和换算函数"，不是各 broker 的实际参数。** 抹平参数 = 盈亏失真 = 违反"准确无误"。原文公理②表述（"同一合约规模模型"）有歧义，已修订为"统一盈亏度量（换算，非抹平）"。

换算链：真实参数(各不同) → 本币盈亏 → 换算 → 统一度量(USD+bps)。系统比较机会、定统一阈值，都基于换算后的统一度量；但计算的一切输入都来自各 broker 的真实参数。

---

## 讨论三 · 范围边界：索罗斯 vs 套利，A+B 而非全网

**索罗斯辨析（关键澄清）**：用户崇拜的索罗斯本质是**宏观方向性押注者**，不是套利者（1992 英镑、1997 泰铢，靠"发现宏观失衡 + 敢下重注"）。他的哲学：*"判断对错不重要，关键是对的时候赚多少"*——重注、高赔率、依赖人的认知。这与"扫描发现套利"是**相反哲学**（每笔小、几乎不会错、靠频率累积）。两条路必须二选一，混了就是"太广、做不精"。

**盈利手段确定性谱系**：
| 谱系 | 手段 | 确定性 | 可扫描自动化 | 取舍 |
|---|---|---|---|---|
| A | 无风险套利：跨所价差/三角/**资金费率套利** | 高 | ✅ | **做** |
| B | 套息：FX swap 差/加密借贷利率差/对冲套息 | 中高 | ✅ | **做** |
| C | 统计/配对：协整/均值回归 | 低（概率+参数） | 半 | 排除 |
| D | 相对价值对冲：多金空银 | 低（赌回归） | 半 | 排除 |
| E | 宏观方向/事件（索罗斯式） | 认知驱动 | ❌ | 根本不是这个系统 |

**结论**：第一版 = **A+B**。A+B 是"准确无误"承诺**唯一能兑现**的区间；C/D/E 本质是概率/认知博弈，纳入会稀释可信度。

**被否方案**：
- ❌ 全自动交易（与"给下单者机会"冲突）。
- ❌ C/D/E（做不精、不"准确无误"）。
- ❌ 一上来全网（地基会塌）。

**两个落地洞察**：
1. **资金费率套利**（加密金矿）：永续资金费率为正且高时，做空永续 + 做多等量现货，对冲掉价格风险，**纯赚资金费率**。年化常 20–50%。FX 无此机制。
2. **swap ≡ funding rate**：FX 的 swap 和 Crypto 的 funding rate 本质同一（持仓融资成本），统一抽象层建模成同一字段（如 `Instrument.fundingRate` + 结算频率）。跨资产套息比较因此成为可能——这正是统一地基的价值。

---

## 讨论四 · 必备基础条件

**外部 P0（阻塞，用户须提供）**：
1. ≥2 FX broker 账户（跨所前提）；Binance 现货+永续+API key。
2. **资金预分布**：套利是同时双边，资金不能实时跨平台划转，每个平台须常驻保证金。
3. mtapi.io 凭证 + Binance API key。
4. **低延迟部署**：服务器靠近 broker/Binance，否则抢不到。
5. **真实费率数据**：成本模型输入必须真实，不能假设。
6. Demo/testnet 验证环境（不裸奔实盘）。

**核心风险（专业提醒）**：
- **资金预分布的代价**：分散是套利的必要条件，也是**对手方风险**的来源（FTX 前车之鉴）。须"单平台资金占比"硬上限。
- **单 Binance 局限**：只有 1 个加密所，**跨所搬砖做不了**；战场在 Binance 内部（资金费率/期现/三角）。
- **FX swap vs Crypto funding 结算时效不同**：FX 隔夜、Crypto 每 8h——统一抽象须建模"结算频率"。
- **broker 数量 ≠ 越多越好**：每个 broker 带接入/资金分散/对手方/运维成本。起步选 2–4 个**优质**的（点差低/swap 友好/稳定/**允许套利不封号**），质量优先。

**系统地基（对应四公理，施工时建）**：统一 Instrument 抽象、归一化尺度、全成本模型、时间同步、账户/资金/持仓可读通道、归因记账。

### 补充 · 用户提供的 5 个 broker 评估（2026-08-07）

用户可提供：Exness、XM、ICMarkets、PrimeXBT、CMC Markets。金融专家评估（针对套利）：

| Broker | 模式 | 套利友好 | MT4/5 | 结论 |
|---|---|---|---|---|
| IC Markets | 真 ECN/RAW，点差极低 | ✅ 高 | ✅ | **起步核心** |
| XM | STP/ECN，执行稳 | ✅ 高 | ✅ | **起步基准** |
| Exness | 即时执行+自营商，点差极低，报价差异大；swap 近 0 | ⚠️ 中 | ✅ | **起步加入**（跨所机会多，套息弱） |
| PrimeXBT | CFD（加密导向） | ❌ | ❌ 非 MT4/5（自研） | **暂缓**：mtapi.io 接不进 + CFD 定价不可比 |
| CMC Markets | 做市商（主推股/指） | ❌（requote/监控/封号风险） | △ | **暂缓**：做市商对套利不友好 |

**起步组合：IC Markets + XM + Exness**（3 个真 ECN/低点差/MT4-5）。理由：ECN↔ECN 价差是纯套利；ECN↔CFD/做市商的"价差"含对方定价风险，不是纯套利。统一走 **MT5**（字段更全，利于拉 SymbolInfo）。

**待用户确认**：每个 broker 的 MT5 server host + 凭证（Exness/XM/IC 均支持 MT5）。

---

## 讨论五 · P1 参数与自适应原则

**贯穿原则**：所有参数不能写死，须用"**预估 vs 实际成交**"偏差持续校准。归因记账从第一天起内建——它是 P1 自适应的数据源。

**① 机会阈值**：`net_bps ≥ 实测执行滑点(P95) + 安全垫`。不是常数，是"执行残余不确定性"的函数。初值 `net ≥ 3 bp`（未实测前保守值）。FX 主要货币对点差 0.1–1 pip，跨所机会 1–5 bp 量级。

**② 风控参数**（防"执行失败变方向敞口" + 对手方爆雷）：
| 参数 | 初值 |
|---|---|
| 单机会最大敞口 | ≤ 5% 总资金 |
| 最大并发未平仓机会 | ≤ 5 |
| 单平台资金占比 | ≤ 40% |
| 单边敞口存活上限 | ≤ 3 秒自动对冲/平仓 |
| 日亏损熔断 | 日亏 ≤ 3% 暂停 |
| 行情断流 | blind mode 撤单+拒新仓 |

**③ 资金分配**：初始各 broker 均分 + 20% 机动储备，受"单平台 ≤ 40%"硬上限约束（保命优先于效率）。跑起来按实测机会密度调整。

---

## 讨论六 · 实施顺序

**决策**（见 D-004）：
1. **先 FX，Crypto 留接口**：整套"发现→评估→确认→执行"链路先在 FX 跑通验证，再加 Crypto。
2. **抽象层仍按 FX+Crypto 设计**（`Listing.Swap(Funding)` 含 swapType/swapLong/Short/结算频率，见 02 §4.3；`Instrument.Kind` 预留），实现分阶段——扩展不改地基。
3. FX broker 充足（用户可按需提供），故**第一步做跨所价差套利**（最稳、最直接兑现"准确无误"），单 broker 内同时可做三角 + 套息。

---

## 待续 / 未决
- swap 的 InPoints → 货币成本的换算公式实现（evaluator）。
- 索罗斯式宏观判断是否未来作为独立"人的 alpha"层接入（不属本系统，但可互补）。

---

## 讨论七 · Instrument 抽象 + 探测验证（2026-08-07，真实数据定案）

写探测工具 `tools/probe`（从 PG 读 MT5 broker 拉真实 SymbolParams）。连 **ICMarketsSC-Demo(52993526)** + **Exness-MT5Trial5(277842155)** 两个 MT5 demo 对比。

**核心发现：`SymbolParams` 返回 `SymbolInfo` + `SymGroup` 两个结构**——旧 `SymbolDigits` 只取 SymbolInfo.Digits，第一版 probe 也漏了 SymGroup。补齐后字段齐全。

### 跨 broker 同品种对比（真实数据）

| 品种 | broker(符号) | contractSize | swapType | swapLong | swapShort | digits |
|---|---|---|---|---|---|---|
| EURUSD | ICMarkets(EURUSD) | 100000 | InPoints | -8.287 | +1.544 | 5 |
| EURUSD | Exness(EURUSDm) | 100000 | InPoints | -5.9 | 0 | 5 |
| GBPJPY | ICMarkets | 100000 | InPoints | +11.67 | -23.186 | 3 |
| GBPJPY | Exness(GBPJPYm) | 100000 | InPoints | 0 | -39.8 | 3 |
| XAUUSD | ICMarkets | 100 | InPoints | -56.766 | +38.929 | 2 |
| XAUUSD | Exness(XAUUSDm) | 100 | InPoints | -509.9 | 0 | 3 |

### 结论（数据定案）
1. **套息(B)机制可行，但实测对冲套息净 swap 为负**（审计纠正，见 03 §2.2）：同品种跨 broker swap 显著不同（EURUSD swapLong ICMarkets -8.287 vs Exness -5.9；GBPJPY ICMarkets +11.67 vs Exness 0），但**对冲套息净 swap 实测为负**——GBPJPY：ICMarkets做多(+11.67) + Exness做空(-39.8) = **-28.13/天**；EURUSD 同理负。原因：broker swap 定价使多空整体付 swap。**Carry detector 仍建（正收益才推机会），当前 2 broker 无正收益机会，需扩大覆盖**。裸 carry（GBPJPY 做多收 +11.67）有正 swap 但有汇率风险，不符合"准确无误"不做。
2. **跨所价差可比**：同品种 contractSize 跨 broker 一致（EURUSD 100000、XAUUSD 100）→ 同手数对冲可行。
3. **符号归一化必要**：ICMarkets 无后缀（EURUSD/XAUUSD），Exness 全 m 后缀（EURUSDm/...）。须符号映射表（人工维护，用户确认差异小）才能跨 broker 比较同品种。
4. **digits 跨 broker 可能不同**（XAUUSD ICMarkets=2, Exness=3）→ 报价比较须精度对齐。
5. **swap 建模用 proto `SwapType` 枚举**（7 种：InPoints/MarginCurrency/Currency/PercCurPrice/PercOpenPrice/PointClosePrice/SwapNone）。两边都 InPoints，但实现须覆盖全枚举（跨 broker/品种可能不同）。
6. **Listing 字段来源** = `SymbolInfo`(contractSize/digits/points/profitCurrency/marginCurrency/calcMode) + `SymGroup`(swapType/swapLong/swapShort/volumeMin-Max-Step/initialMargin/tradeMode/executionType/fillPolicy/threeDaysSwap)。
7. **公理②实证**：profitCurrency 因品种异（USD/JPY），盈亏须换算 USD。`TickValue` broker 未填(零值)→盈亏换算用 contractSize×points×profitCurrency 手算，不依赖 TickValue。
8. **两层模型实证**：contractSize 因品种异（100000/100）→ 每 broker 每品种须独立 Listing。当前 `Notional()` 硬编码 `*100000` 对 XAUUSD(100) 错。

### 设计验证状态：✅ 完成
第 3 点（swap）+ 第 4 点（两层模型）均有真实答案。可写 `02-opportunity.md`。

### 工具产出（探测工具，非产品代码）
- `internal/adapter/mt5_trading.go` 加 `SymbolParamsRaw`（返回完整 SymbolParamsReply）。
- `tools/probe/main.go`（遍历 MT5 broker、自动适配符号、对比关键字段）。
- PG 现有 2 个 MT5 broker：ICMarketsSC-Demo(52993526)、Exness-MT5Trial5(277842155)。

### 环境事实
PG 真实连接在 docker `arb-postgres`（宿主 5433，arb:arb/arb），`broker_accounts` 在此；`config` 的 5432/OctaFX 是旧的勿用。
