# ARB 工作状态 — 无损接手

> 每次 AI 会话**开工读、收工写**。Claude Code 与 Windsurf 共享的唯一工作状态。
> 最后更新：2026-08-07（Phase A 完成 + D-006 竞品 UI 借鉴 + D-007 自审作用域明确）。

---

## 一句话现状

系统重新定位为「**发现 + 评估 + 人工确认 + 执行**」顾问式（D-003）；架构定稿 **Go(core) + C# .NET 8 WPF(desk) + gRPC + PostgreSQL，多语言各层最优**（D-005）；第一版 = **FX MT5 确定性套利**（跨所价差/三角/套息，Crypto 留接口，D-004）；设计文档 00-11 完成；**Phase A（数据源地基）已完成并通过真实验收**；**D-006 竞品 UI 借鉴已落地到 02/03/06/10**（机会列表 Master-Detail 表格 + 腿角色 + Carry 年化 + 对冲手数归一化，保人工确认 + 全成本优势）。

---

## 已定方向（decisions.md D-001~D-005）

- **D-001** 协作架构：AGENTS.md 为 SSOT + docs/handoff/ 共享记忆（Claude/Windsurf 无损接手）
- **D-002** 交付前自我审计 A–F 强制
- **D-003** 重新定位：混合模式（发现→评估→**你确认**→执行）；策略聚焦 A+B 确定性套利
- **D-004** 先 FX MT5，Crypto 留接口；跨所价差优先；broker 重质不重量
- **D-005** 架构最优解：**Go core + C# WPF desk + gRPC + PG**；多语言各层最优；**推翻 Wails/Svelte 前端**
- **D-006** 竞品 UI 借鉴：机会列表 Master-Detail 表格 + 腿角色(LegRole) + Carry 年化度量 + 对冲手数归一化 + 风险提示/筛选排序栏；保人工确认 + 全成本优势。截图存档 `docs/1.png`
- **D-007** 自审作用域明确：**设计文档审归 Claude**（定稿人，每次改文档必自审）；**代码审归施工 agent**（A-F）+ Claude 复审；施工 agent 遇文档矛盾上报不自行改。落 `AGENTS.md §3.0`

---

## 文档进度

- ✅ `AGENTS.md`（SSOT，含 D-005 多语言/WPF）+ `CLAUDE.md`/`.windsurfrules`（入口）+ `docs/handoff/`（STATE/decisions）
- ✅ `docs/design/` **00–11 全部完成**（00-08 设计 + 09 core 运行时 + 10 desk UI + 11 测试；基于真实探测数据 + 三视角审计修复）
- ✅ `docs/design/discussion-log.md`（讨论一~七 + 真实探测 + Carry 审计纠正）
- ✅ `docs/design/01-architecture.md` 总览（含 D-005 架构）
- ✅ **D-006 竞品 UI 借鉴**（2026-08-07）：`02`(§3.1 对冲手数归一化 / §5 Leg+LegRole / §5.1 度量双轨) + `03`(§2.1/§2.2 腿角色+正收益佐证) + `06`(§5.2 Opportunity/Leg 新字段 + LegRole 枚举) + `10`(§4 Master-Detail 表格重写) 同步更新；决策详 `decisions.md D-006`

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

- ❌ `internal/detector/` + `internal/evaluator/`（替代未建的 engine，D-003）
- ❌ `Opportunity` 对象 + `OpportunityStream` + `ConfirmOpportunity` RPC
- ❌ 成本模型代码（`Listing.Swap(Funding)`，swap 按 `SwapType`）
- ❌ `SymbolInfo`+`SymGroup` 缓存（→ `Listing`）
- ❌ `internal/audit/` + 归因记账
- ❌ desk C# .NET 8 WPF 全新
- ⚠️ `Notional()`（pipeline.go:43）须替换为真实净盈利
- ✅ ~~`SymbolInfo`+`SymGroup` 缓存（→ `Listing`）~~ Phase A 完成
- ✅ ~~成本模型代码（`Listing.Swap(Funding)`，swap 按 `SwapType`）~~ Phase A 完成（Funding 结构定义）

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

1. **Phase A 已完成**（Listing/缓存/symbol_map，真实验收通过）+ **D-006 竞品借鉴已落文档**（02/03/06/10）。
2. ✅ **系统架构设计完整**：外部（D-005 语言/通信/部署）+ 内部（`09` 并发/延迟/容灾/恢复/扩展）+ desk（`10`）+ 测试（`11`）。
3. **设计层后续**：Phase A 代码质量审查（Claude）→ Phase B（Evaluator：成本模型 + 净利润 + 可执行性，依 `02 §6` + D-006 新字段 `NetSwapPerDay`/`annualized`/`LegRole`）设计 → 交接 Windsurf 施工。
4. Windsurf 施工前须同步 `06 §5.2` 新 proto 字段（Opportunity `net_swap_per_day`/`hold_days_hint`/`annualized_net_bps`、Leg `role`/`daily_swap`/`annualized_bps`、`LegRole` 枚举）。

---

## 环境事实

- **PG 真实**：docker `arb-postgres`，宿主 **5433**（arb:arb/arb）；`broker_accounts` 在此。
- **PG 现有 MT5 broker**：ICMarketsSC-Demo(52993526)、Exness-MT5Trial5(277842155)。
- **mtapi.io 网关在德国**（Hetzner，91.98.38.182）；美国 VPS→164ms，**core 宜部署德国 VPS**（近 mtapi.io，~几ms）。mtapi.io 官方带 Go 示例（`docs/api/mt{4,5}/goExample/`）。
- **探测工具** `tools/probe`（从 PG 读 MT5 broker 拉真实 SymbolParams）已就绪、编译通过。

---

## 注意事项

- **第一版只做 MT5**（D-004），MT4 不碰。
- **config.proto 已回退到 strategies**（保持源/gen/cmd-core/textproto 一致；detectors 迁移由 Windsurf 全套同步——源+gen+cmd-core+textproto 一起改，勿半截改源，否则 `buf generate` 会让 core 编译崩）。
- `docs/ant/` **保留**（成功案例参考 / 可迁移复用，用户定）。
- 交付前自我审计（`AGENTS.md §3`）强制。
- 真相源 `AGENTS.md`；讨论 `docs/design/discussion-log.md`；决策 `decisions.md`。
