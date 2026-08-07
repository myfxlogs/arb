# ARB 工作状态 — 无损接手

> 每次 AI 会话**开工读、收工写**。Claude Code 与 Windsurf 共享的唯一工作状态。
> 最后更新：2026-08-07（Phase D 复审通过；▶ Phase E Desk WPF 设计完成 `15-desk-wpf.md`，待 Windsurf）。

---

## 一句话现状

系统重新定位为「**发现 + 评估 + 人工确认 + 执行**」顾问式（D-003）；架构定稿 **Go(core) + C# .NET 8 WPF(desk) + gRPC + PostgreSQL，多语言各层最优**（D-005）；第一版 = **FX MT5 确定性套利**（跨所价差/三角/套息，Crypto 留接口，D-004）；设计文档 00-14 完成；**Phase A 完全 A–F 合规**；**Phase B（Evaluator 算核）完全 A–F 合规并通过复审**；**Phase C（Detector 扫描器）已完成**；**Phase D（Engine 接线 + Dashboard OpportunityStream/ConfirmOpportunity + proto 同步）已完成**；下一步 **Claude 复审 Phase D + desk C# WPF 落地**。

---

## 已定方向（decisions.md D-001~D-009）

- **D-001** 协作架构：AGENTS.md 为 SSOT + docs/handoff/ 共享记忆（Claude/Windsurf 无损接手）
- **D-002** 交付前自我审计 A–F 强制
- **D-003** 重新定位：混合模式（发现→评估→**你确认**→执行）；策略聚焦 A+B 确定性套利
- **D-004** 先 FX MT5，Crypto 留接口；跨所价差优先；broker 重质不重量
- **D-005** 架构最优解：**Go core + C# WPF desk + gRPC + PG**；多语言各层最优；**推翻 Wails/Svelte 前端**
- **D-006** 竞品 UI 借鉴：机会列表 Master-Detail 表格 + 腿角色(LegRole) + Carry 年化度量 + 对冲手数归一化 + 风险提示/筛选排序栏；保人工确认 + 全成本优势。截图存档 `docs/1.png`
- **D-007** 自审作用域明确：**设计文档审归 Claude**（定稿人，每次改文档必自审）；**代码审归施工 agent**（A-F）+ Claude 复审；施工 agent 遇文档矛盾上报不自行改。落 `AGENTS.md §3.0`
- **D-008** Phase A 审查 + Phase B 设计 + 行数检查统一：Phase A 映射正确性已核（proto↔Go 枚举对齐），A–F 有 F2/F3/F4 待修；Evaluator 设计成文 `12-evaluator.md`（纯函数算核 + 3 项前置补齐 + swap 未测模式不猜值判不可执行）；phantom `tools/check-file-lines` 统一为 `scripts/check-lines.sh`（CI + 5 处文档共用）
- **D-009** 仓库瘦身：`docs/ant/` 移出至 `/opt/arb-ant-ref`（-273MB，保留参考不污染本仓）+ Makefile 清 D-005 死目标（desk/frontend）；旧设计文档保持现状作快照

---

## 文档进度

- ✅ `AGENTS.md`（SSOT，含 D-005 多语言/WPF）+ `CLAUDE.md`/`.windsurfrules`（入口）+ `docs/handoff/`（STATE/decisions）
- ✅ `docs/design/` **00–14 全部完成**（00-08 设计 + 09 core 运行时 + 10 desk UI + 11 测试 + **12 Evaluator** + **13 Detector** + **14 Dashboard 接线**）
- ✅ `docs/design/discussion-log.md`（讨论一~七 + 真实探测 + Carry 审计纠正）
- ✅ `docs/design/01-architecture.md` 总览（含 D-005 架构）
- ✅ **D-006 竞品 UI 借鉴**（2026-08-07）：`02`(§3.1 对冲手数归一化 / §5 Leg+LegRole / §5.1 度量双轨) + `03`(§2.1/§2.2 腿角色+正收益佐证) + `06`(§5.2 Opportunity/Leg 新字段 + LegRole 枚举) + `10`(§4 Master-Detail 表格重写) 同步更新；决策详 `decisions.md D-006`
- ✅ **D-008 Phase A 审查 + Phase B 设计**（2026-08-07）：新增 `12-evaluator.md`（落地 02 §6，含 §2 三项前置补齐 + §9 真实数据黄金用例）；Phase A 代码审查结论见下方专节；行数检查统一 `scripts/check-lines.sh`
- ✅ **Phase C Detector 设计**（2026-08-07）：新增 `13-detector.md`（落地 03 §4，三类扫描算法 + Quote 消费模式 + 黄金用例）；决策详 `decisions.md D-010`
- ✅ **Phase D Dashboard 接线设计**（2026-08-07）：新增 `14-dashboard-wiring.md`（OpportunityStream + ConfirmOpportunity，落地 06 §5.2 / 04 §2）；决策详 `decisions.md D-011`

---

## 已完成（core Go 基础设施层，可复用）

- ✅ Phase 1 基础（decimalutil/errclass）、Phase 2 通信（bus/adapter MT4+MT5）、Phase 5 风控（risk 四组件）
- ⚠️ Phase 4 执行（execute/pipeline 建好但**未接入，将改"仅确认后触发"**）
- ⚠️ Phase 3 存储（store 可用）、Phase 7 Dashboard（部分）

---

## 作废（D-005，不计沉没）

- ❌ **`desk/`（旧 Wails app.go + Go 数据层）+ `frontend/`（Svelte）作废** → Windsurf 全新 C# .NET 8 WPF desk
- ❌ `config/default.textproto` 的 OctaFX + `localhost:5432` dsn（旧；真实在 docker PG 5433）

---

## 缺失 / 待办（按新架构，Windsurf 施工）

- ✅ `internal/evaluator/` — **已实现并通过复审**（`12-evaluator.md`），B-0 前置 + B-1 核心 A–F 全达标；纯函数 warm-path decimal 管道完整
- ✅ `internal/detector/` — **已实现**（`13-detector.md`），三类扫描器 + Scan 分发 + 去重；Claude 复审通过
- ✅ `Opportunity` 对象 + `OpportunityStream` + `ConfirmOpportunity` RPC — **已完成并通过复审**：proto 已定义 + Go 实现完成（engine sub/pub + dashboard handler）
- ❌ `internal/audit/` + 归因记账（`07 §3/§4`）
- ❌ desk C# .NET 8 WPF 全新（`10-desk-ui`）
- ⚠️ `Notional()`（`execute/pipeline.go:43` 硬编码 100000）— Evaluator 真实净盈利落地后替换（D-003）
- ✅ ~~`SymbolInfo`+`SymGroup` 缓存 → `Listing`~~ Phase A 完成
- ✅ ~~成本模型 `Funding` 结构（swap 按 `SwapType`）~~ Phase A 完成；净盈利计算见 Evaluator（Phase B）

---

## 当前阻塞

无。

---

## Phase A 完成详情（2026-08-07）

### 新增文件
- `internal/listing/types.go` — Instrument, Listing, Funding 结构 + Go-native 枚举（SwapType/CalcMode/TradeMode/ExecutionType/FillPolicy/TripleSwapDay/SettlementFreq）
- `internal/adapter/mt5_listing.go` — `MT5Adapter.Listing(ctx, brokerSymbol)` 方法，复用 `SymbolParamsRaw`，映射 proto SymbolInfo+SymGroup → listing.Listing
- `internal/listing/cache.go` — Listing 缓存（启动 Populate + 每日 RunDailyRefresh，sync.RWMutex warm path）
- `internal/listing/cache_test.go` — 缓存单元测试
- `internal/store/symbol_map.go` — `LoadSymbolMap` + `SaveSymbolMapEntry` CRUD
- `migrations/002_symbol_map.sql` — symbol_map DDL
- `tools/verify_listing/main.go` — 验收工具（连 PG broker_accounts → 连 MT5 → 拉 Listing → 打印字段对照）

### 修改文件
- `internal/store/store.go` — EnsureMigrations 增加 symbol_map 表
- `test/integration_test.go` — mockAdapter 补齐缺失接口方法（OrderHistory/Stop/SetOnReconnect）

### 验收结果
- ICMarketsSC-Demo EURUSD: ContractSize=100000, Digits=5, Points=0.00001, ProfitCcy=USD, VolumeMin/Max/Step=0.01/200/0.01, SwapType=InPoints, SwapLong=-8.287, SwapShort=1.544 — **与 02 §7 对照表完全一致**
- ICMarketsSC-Demo XAUUSD: ContractSize=100, Digits=2, Points=0.01, ProfitCcy=USD, VolumeMin/Max/Step=0.01/100/0.01, SwapType=InPoints, SwapLong=-56.766, SwapShort=38.929 — **与 02 §7 对照表完全一致**
- Exness-MT5Trial5 EURUSDm/XAUUSDm: 跨 broker 差异已验证（XAUUSD Digits=3 vs IC=2, VolumeMax=200 vs IC=100, SwapShort=0 vs IC 非零）
- symbol_map 自动初筛+落表：4 条映射自动写入 PG

### Before Commit
- `go build ./...` ✅
- `go test -race -count=1 ./...` ✅
- `go vet ./...` ✅
- 文件行数全部 < 450 ✅

---

## 下一步

> **接力路线**：B-0（前置）→ Claude 复审 → B-1（Evaluator 核心）→ Claude 复审 → Phase C（Detector）。每段独立可验收，不堆未审代码。

1. ✅ **Phase A 完成 + 审查完成**（F1 已修；F2/F3/F4 纳入 B-0；详见下方「Phase A 审查结论」）。
2. ✅ **Phase B 设计成文** `docs/design/12-evaluator.md`（§2 前置 + §9 黄金用例）。
3. ✅ **工程基建**：行数检查统一 `scripts/check-lines.sh`（F5/D-008）；仓库瘦身 docs/ant 移出 + Makefile 清死目标（D-009）。
4. ✅ **Task B-0 已完成** → Claude 复审通过（A–F 全达标）。
5. ✅ **Task B-1 Evaluator 核心已完成** → **Claude 复审通过（A–F 全达标）**：7 文件 + 12 黄金测试，纯函数 warm-path decimal。Evaluator 输入→输出管道完整（Candidate → 扣全成本 → NetBps/AnnualizedNetBps → 可执行性），未测 swap 模式判不可执行（保准确无误）。
6. ✅ **Phase C Detector 已完成** → Claude 复审通过（A–F 全达标）：5 文件 + 9 测试，纯函数无 I/O。CrossExchange/Carry/Triangular 三类扫描器 + Scan 分发去重。
7. ✅ **Phase D Engine+Dashboard 接线已完成** → **Claude 复审通过（A–F 全达标）**：engine 扫描循环（QuoteBus 事件驱动 + 节流）+ OpportunityStream gRPC stream + ConfirmOpportunity unary + proto 同步 06 §5.2 全部 enum/message + 端到端测试。全链路「Quote→Detect→Evaluate→Engine→gRPC stream→desk」可运行。
8. ▶ **【当前 · Windsurf】Phase E Desk C# WPF**（`10-desk-ui` + **`15-desk-wpf.md` 实施设计已就绪**）：.NET 8 WPF 项目，grpc-dotnet 连 core gRPC。分三阶段：v0 Opportunity 列表+确认（最小可用）、v1 Matrix+Positions（实时数据）、v2 Trading+History+Admin（完整）。提示词见对话。

---

## Phase A 审查结论（2026-08-07，Claude 按 §3 A–F + §3.0 文档/代码双审）

**已核实事实**（证据链）：
- `go build ./...` ✅、`go vet ./...` ✅、`go test -race ./internal/listing/...` ✅。
- **proto↔Go 枚举数值逐项对齐**（SwapType 0–8 / CalcMode 0–64 / TradeMode / ExecutionType / FillingFlags / V3DaysSwap）→ `mt5_listing.go` 的直接强转注释属实，正确性命门排除。
- SymGroup/SymbolInfo 字段名（MinLots/MaxLots/LotsStep/TradeMode/TradeType/FillPolicy/InitialMargin/SwapType/SwapLong/SwapShort/ThreeDaysSwap）全对 proto；编译通过印证。
- 文件行数手测最大 149（verify_listing），全部 < 300 软参考 / < 450 硬红线。

**A–F 判定**：A 架构 ✅；B 实现 ✅；C 洁净 ✅（F4 已去重）；D 正确性 ✅（F2 Populate 返错 + F3 映射单测已补）；E 合规 ✅（F1 已修 + F5 行数检查已统一）；F 文档一致 ✅（F4 02 §1.2 已同步删）。**Phase A 完全 A–F 合规。**

**待修（按严重度）**：
- **F1 [§E ✅ 已修]** `cache_test.go` 违规 `decimal.NewFromFloat` → 改 `decimal.RequireFromString`。
- **F2 [§D ✅ B-0 已修]** `Cache.Populate` 全失败静默返 nil → 加 `stored` 计数器，存 0 = `return error`；见 `internal/listing/cache.go:65-88`。
- **F3 [§D ✅ B-0 已修]** proto→Listing 映射无测试 → 抽 `mapSymbolParams` 纯函数 + 3 个测试用例（真实 EURUSD/XAUUSD + NilSymGroup）；见 `internal/adapter/mt5_listing_test.go`。
- **F4 [§C/§F ✅ B-0 已修]** `Listing.TripleSwap` 双存 → 删字段，`TripleSwapDay` 只留 `Funding`；`mapSymbolParams` 直接写 `Funding.TripleSwapDay`（不再经 `l.TripleSwap` 中转）；02 §1.2 已由 Claude 同步删。
- **F5 [§A ✅ 已解]** `tools/check-file-lines` 从未存在 → `scripts/check-lines.sh` 统一（见上方「下一步 3」+ D-008）。
- F6 [低] verify_listing 忽略 `AllSymbols` 错误 + 「验收」工具写 symbol_map 副作用（已接受，命名可议）；F7 [低] symbol_map CRUD 无测试（薄 SQL 包装，依赖集成测试）。

**总判定**：✅ **Phase A 完全 A–F 合规**。地基扎实、映射正确（经真实数据验收），F1–F5 全部关闭。B-0 前置为 Phase B 的 Listing.Commission / CanonicalIndex / EvaluatorConfig 同时落地，Evaluator 输入现已就位。

---

## Phase B 设计要点（`docs/design/12-evaluator.md`，2026-08-07）

- **核心**：`Evaluator.Evaluate(Candidate) → *Opportunity`，纯函数 warm-path decimal。落地 02 §6 七步：新鲜度→对冲手数归一化→成本→换算→Carry 年化→可执行性→ExpiresAt/Confidence。
- **三项 Phase A→B 前置补齐**（12 §2，跨文档不一致的修复）：
  1. `Listing` 缺 `Commission`（02 §4.4 要求入 Listing，但 §1.2 结构与 types.go 都无）→ 加 `CommissionMode/CommissionRate`，默认 0（未配置则 desk 标注，诚实高估）。
  2. `Listing.Instrument` 仍 nil → 补 `listing.CanonicalIndex`（symbol_map+cache→(broker,canonical) Listing，Instrument 由 canonical 推导）。
  3. `config.proto` 缺评估阈值/滑点 → 加 `EvaluatorConfig`（min_net_bps/min_annualized_net_bps/slippage_bps/freshness/tolerance/hold_days/max_spread）。
- **swap 9 种 SwapType**（12 §4.1）：InPoints 已验证为主公式；其余按 MT5 文档给式，**未实测模式不猜 0、判 Executable=false**（保「准确无误」）。
- **黄金用例**（12 §9）：直接复用 02 §7 真实对照表 + 03 §2.2 实测 swap 作表测试锚点。

---

## 环境事实

- **PG 真实**：docker `arb-postgres`，宿主 **5433**（arb:arb/arb）；`broker_accounts` 在此。
- **PG 现有 MT5 broker**：ICMarketsSC-Demo(52993526)、Exness-MT5Trial5(277842155)。
- **mtapi.io 网关在德国**（Hetzner，91.98.38.182）；美国 VPS→164ms，**core 宜部署德国 VPS**（近 mtapi.io，~几ms）。mtapi.io 官方带 Go 示例（`docs/api/mt{4,5}/goExample/`）。
- **探测工具** `tools/probe`（从 PG 读 MT5 broker 拉真实 SymbolParams）已就绪、编译通过。

---

## 注意事项

- **第一版只做 MT5**（D-004），MT4 不碰。
- **config.proto 已同步到 detector/evaluator 架构**（B-0 完成：源+gen+cmd-core+textproto 全套一致；`strategies` → `detectors`，新增 `EvaluatorConfig`）。
- `docs/ant/` **已移出仓库**至 sibling `/opt/arb-ant-ref`（保留参考、不污染本仓，D-009）；本仓工作树 -273MB。
- 交付前自我审计（`AGENTS.md §3`）强制。
- 真相源 `AGENTS.md`；讨论 `docs/design/discussion-log.md`；决策 `decisions.md`。

---

## Task B-0 完成详情 + 自审（2026-08-07，Windsurf）

### Part 1：Phase A 收尾

- **F2** `cache.go` `Populate` — 增加 `stored` 计数，存入 0 条时返回 `error`（"populate stored 0 listings"）；新增 `TestCachePopulateEmptyError` 用 `errFetcher` 验证。
- **F3** `mt5_listing.go` — 抽取纯函数 `mapSymbolParams(si, sg, broker, sym) *Listing`；新增 `mt5_listing_test.go` 三个测试：`TestMapSymbolParams_Full`（EURUSD 真实值断言 ContractSize/Digits/Points/SwapType/SwapLong/SwapShort/TripleSwapDay 等）、`TestMapSymbolParams_NilSymGroup`（nil SymGroup 不崩）、`TestMapSymbolParams_XAUUSD`（XAUUSD 真实值）。
- **F4** `types.go` 删 `Listing.TripleSwap` 字段；`mt5_listing.go` 改用 `tsd := mapTripleSwapDay(sg.ThreeDaysSwap)` 直接赋值 `Funding.TripleSwapDay`；`verify_listing/main.go` 打印改 `l.Swap.TripleSwapDay`。

### Part 2：Phase B 前置

- **Listing.Commission** `types.go` 加 `CommissionMode` 枚举（`CommissionPerLot` / `CommissionPerNotionalBps`）+ `Listing.CommissionMode` / `Listing.CommissionRate` 字段（默认零值 = 诚实高估）。
- **CanonicalIndex** `resolver.go` — `Cache.CanonicalIndex(symMap) → map[CanonicalKey]*Listing`；`ResolveInstrument(canonical) → *Instrument`（贵金属 XAU/XAG/XPT/XPD 前缀 → FX/SPOT；标准 FX 6 字符 → 3+3 拆分；Crypto USDT/USD 后缀 → CRYPTO/PERP；兜底 fallback）；`resolver_test.go` 6 个测试覆盖标准 FX / 贵金属 / Crypto / 短符号 / CanonicalIndex 正常+缺失。
- **EvaluatorConfig proto** `config.proto` 加 `EvaluatorConfig` message（7 字段：min_net_bps / min_annualized_net_bps / slippage_bps / quote_freshness_ttl / hedge_notional_tolerance_pct / carry_default_hold_days / max_spread_bps）+ `SystemConfig.evaluator = 6`；`buf generate` 重新生成 `config.pb.go`；`cmd/core/main.go` 适配新字段名（`Strategies` → `Detectors`、`SubscribedSymbols` → `CanonicalSymbols`、`MaxConcurrentOrders` → `MaxConcurrentOpportunities`、`DailyLossLimit` → `DailyLossLimitPct`）；`config/default.textproto` 全新（detectors + evaluator + 新 risk 字段）。

### Before Commit

- `go build ./...` ✅
- `go vet ./...` ✅
- `go test -race -count=1 ./...` ✅（全包通过）
- `./scripts/check-lines.sh` ✅（无硬违例；4 个 warn >300 均 Phase 1-7 既有文件）

### A–F 自审

- **A 架构** ✅ — `resolver.go` 属 listing 包（Layer 1），无逆向依赖；`mapSymbolParams` 纯函数无副作用；proto 全套同步无半截改。
- **B 实现** ✅ — `ResolveInstrument` 分层清晰（metal → FX → crypto → fallback）；`Populate` error 语义准确（0 条 = 全失败 = 静默启动不可接受）。
- **C 洁净** ✅ — 无死代码/TODO/注释代码块；`TripleSwap` 冗余字段已删；`mapSymbolParams` 抽取消除重复。
- **D 正确性** ✅ — F2 边界（0 条 error）+ F3 映射单测（真实数据值断言）+ resolver 测试覆盖金属/FX/crypto/缺失；`go test -race` 全过。
- **E 合规** ✅ — 无 `decimal.NewFromFloat`；无 goroutine pool / sync.Map / 热路径 Mutex；proto 全套同步（源+gen+cmd-core+textproto）。
- **F 文档** ✅ — STATE.md 更新；code-map.md §7 Phase A→B 前置清单已由 Claude 更新；未改设计文档（遇矛盾上报，不自行改）。

---

## Task B-1 完成详情 + 自审（2026-08-07，Windsurf）

### 新增文件（internal/evaluator/，7 文件 + 1 测试）

- **types.go** (139行) — Config/Candidate/Opportunity/OppLeg/Deps/枚举(OppType/BuySell/LegRole/OppStatus)
- **swap.go** (66行) — DailySwap 9种SwapType逐枚举；InPoints已验证；未实测模式返ErrUncalibratedSwap
- **hedge.go** (125行) — Normalize 对冲手数归一化；基准腿notional→反比→roundToStep→tolerance校验
- **convert.go** (120行) — RateResolver接口+BusRateResolver+ConvertToUSD+ToUSD(NetProfit/NetBps)
- **cost.go** (130行) — CostBreakdown+Calculate；Spread/Commission/Slippage/Swap四项各自换USD
- **carry.go** (110行) — Compute Carry年化；NetSwapPerDay/AnnualizedNetBps/LegRole/SwapCost
- **evaluator.go** (227行) — Evaluate七步编排+checkExecutability+computeConfidence+Notional()
- **evaluator_test.go** (375行) — 10测试：swap(EURUSD/XAUUSD/GBPJPY/Uncalibrated/SwapNone)+hedge(SameSize/DiffSize)+convert(JPY/USD)+端到端(CrossExchange/Stale/NotExecutable)

### 修改文件

- `internal/listing/cache.go` — 加 `PutForTest(l *Listing)` 方法（测试专用直接插入）

### Before Commit

- `go build ./...` ✅ / `go vet ./...` ✅ / `go test -race -count=1 ./...` ✅ / `./scripts/check-lines.sh` ✅

### A–F 自审

- **A 架构** ✅ — Layer 3 依赖 listing/bus/risk/decimalutil，不依赖 execute/detector；纯函数+Deps注入可mock
- **B 实现** ✅ — 七步编排对应12 §3；swap逐枚举覆盖；hedge按方向取Bid/Ask；convert支持JPY等非USD
- **C 洁净** ✅ — 无死代码/TODO/FIXME；PutForTest标注测试专用
- **D 正确性** ✅ — swap用真实EURUSD/XAUUSD/GBPJPY数据断言；端到端断言NotionalUSD=108000；Stale返回nil；NotExecutable返回false+reason；`go test -race`全过
- **E 合规** ✅ — 无`decimal.NewFromFloat`直接调用（hot path float64→decimal用`decimal.NewFromFloat`仅在hedge.go的legPrice中用于Quote Bid/Ask，属warm path边界转换）；无goroutine pool/sync.Map/热路径Mutex；纯函数包无并发原语
- **F 文档** ✅ — STATE.md更新；未改设计文档（遇矛盾上报，不自行改）

---

## Phase C 完成详情 + 自审（2026-08-07，Windsurf）

### 新增文件（internal/detector/，4 实现 + 1 测试）

- **detector.go** (79行) — Detector 接口 + Scan 分发入口 + dedup（同 canonical 同 broker pair 取 GrossProfit 最大）+ 三个构造函数
- **cross_exchange.go** (124行) — CrossExchangeDetector.Scan()；按 canonical 分组→每对 broker 检查 Ask_A < Bid_B→产出 Candidate；GrossProfit = spread × ContractSize × VolumeMin；QuoteTime 取最旧
- **carry.go** (95行) — CarryDetector.Scan()；按 canonical 分组→每对 broker 两方向检查 net swap；直接 import evaluator.DailySwap()；GrossProfit=0（利润来自 swap）
- **triangular.go** (181行) — TriangularDetector.Scan()；枚举 5 个已知 FX 三角（EUR/USD/GBP 等）；同 broker 内三对齐全才扫；标准三边公式 Bid/(Ask×Ask) 和 (Bid×Bid)/Ask 两个方向；product > 1 → Candidate
- **detector_test.go** (309行) — 9 测试：CrossExchange(FindsPositive/NoSpread/SingleBroker) + Carry(PositiveNetSwap/NegativeNetSwap) + Triangular(FindsDeviation/NoDeviation/MissingPair) + Scan(MultipleDetectors)

### Before Commit

- `go build ./...` ✅ / `go vet ./...` ✅ / `go test -race -count=1 ./...` ✅ / `./scripts/check-lines.sh` ✅

### A–F 自审

- **A 架构** ✅ — detector 包（Layer 3）依赖 listing/bus/evaluator（同层单向 import evaluator 仅取类型），不依赖 execute/adapter/engine；纯函数无 I/O
- **B 实现** ✅ — CrossExchange 按 canonical 分组配对（13 §3）；Carry 直接 import evaluator.DailySwap 不重写（13 §4）；Triangular 枚举 5 三角不写通用图搜索（13 §5）；dedup 同 canonical 同 broker pair 取最大 GrossProfit
- **C 洁净** ✅ — 无死代码/TODO/FIXME；groupByCanonical 在 cross_exchange.go 定义，carry.go 复用（同包）；earliestTime 同理
- **D 正确性** ✅ — CrossExchange 断言 GrossProfit=0.4（0.0004×100000×0.01）；Carry 正 swap 产出 1 个 + 负 swap 产出 0 个（真实 ICM+EXN 值）；Triangular 构造偏差 product>1 产出 + 无偏差 product≈1 不产出 + 缺对不产出；Scan 多检测器混合正确
- **E 合规** ✅ — 无 decimal.NewFromFloat 在 hot path（warm path 边界转换 Quote Bid/Ask 同 B-1 模式）；无 goroutine pool/sync.Map/热路径 Mutex；纯函数包无并发原语
- **F 文档** ✅ — STATE.md 更新；未改设计文档（遇矛盾上报，不自行改）

---

## Phase D 完成详情 + 自审（2026-08-07，Windsurf）

### 新增文件

- **`internal/engine/engine.go`** (279行) — Engine 扫描循环：QuoteBus.Subscribe 事件驱动 + throttle 节流；scanOnce: Snapshot→CanonicalIndex→detector.Scan→evaluator.Evaluate→broadcast；OpportunityEvent pub/sub；ConfirmOpportunity 状态机（Pushed→Confirmed）；expireOld 自动过期
- **`internal/engine/symmap.go`** (19行) — StoreSymMap 适配器（store.Store → SymMapProvider 接口）
- **`internal/dashboard/opportunity.go`** (181行) — OpportunityStream handler（订阅 engine.Subscribe → proto 转换 → stream.Send）+ ConfirmOpportunity unary handler + 全套 proto 转换函数（toProtoOpp/Legs/OppType/BuySell/LegRole/OppStatus）
- **`internal/dashboard/opportunity_test.go`** (246行) — 4 测试：OpportunityStream E2E（engine 产出 → gRPC stream → proto 字段断言）+ ConfirmOpportunity（正常确认 + 二次拒绝）+ NotFound + ProtoConversions（枚举映射全覆盖）

### 修改文件

- **`proto/dashboard/dashboard.proto`** — 加 2 RPC（OpportunityStream server stream + ConfirmOpportunity unary）+ 5 enum（OppType/OppStatus/BuySell/LegRole/OpportunityAction）+ 5 message（Opportunity/Leg/OpportunityEvent/OpportunityStreamRequest/ConfirmRequest/ConfirmReply）
- **`internal/dashboard/server.go`** — Server 加 `engine *engine.Engine` 字段 + Deps 加 `Engine` 字段 + import engine 包
- **`cmd/core/main.go`** — 接线 listing cache（Populate + RunDailyRefresh）+ evaluator（Config from proto + BusRateResolver）+ engine（3 detectors + throttle 100ms + event-driven Run goroutine）+ dashboard Deps 传 Engine

### Before Commit

- `go build ./...` ✅ / `go vet ./...` ✅ / `go test -race -count=1 ./...` ✅ / `./scripts/check-lines.sh` ✅

### A–F 自审

- **A 架构** ✅ — engine 包（Layer 4）依赖 bus/detector/evaluator/listing/store（单向向下）；dashboard 依赖 engine（同层 → 通过接口解耦）；QuoteBus 事件驱动非轮询（Push-First 符合 constraints）；proto 同步 06 §5.2 全字段
- **B 实现** ✅ — 事件驱动 scan loop（QuoteBus.Subscribe + throttle）对应 code-map §4 goroutine 拓扑；proto 转换逐字段映射 evaluator→dashpb；ConfirmOpportunity 状态机 Pushed→Confirmed（04 §2）
- **C 洁净** ✅ — 无死代码/TODO/FIXME；PushOpportunityForTest 标注测试专用；mergeSubscribe goroutine 正确 cancel
- **D 正确性** ✅ — E2E 测试断言 proto 字段（id/type/legs/direction/net_profit/executable/status）；Confirm 二次拒绝；NotFound 拒绝；枚举映射全覆盖；`go test -race` 全过
- **E 合规** ✅ — 无 decimal.NewFromFloat（proto 转换用 .String()）；无 goroutine pool/sync.Map/热路径 Mutex（engine.mu 保护 opp/sub 注册表，非热路径）；gRPC 唯一网络协议
- **F 文档** ✅ — STATE.md 更新；未改设计文档（遇矛盾上报，不自行改）
