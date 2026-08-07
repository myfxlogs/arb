# 08 · 落地路线

> 给 Windsurf 的施工顺序。依据 D-004（先 FX MT5）+ 全部设计文档（00–07）。
> **真实代码由 Windsurf 施工**；Claude 只出设计。每阶段须 demo 跑通 + 自我审计（AGENTS §3 A–F）才进下一阶段。

---

## 1. 总原则
- **先 FX MT5，Crypto 留接口**（D-004）。抽象兼容 Crypto，实现分阶段。
- **先地基（数据源/Listing）→ 业务（Detector/Evaluator）→ 闭环（Stream/Confirm）→ 风控审计**。
- **demo 验证优先**：不裸奔实盘；每阶段在 ICMarkets+Exness demo 跑通。
- **每阶段收工**：更新 `STATE.md`（交接）+ 过自我审计。

---

## 2. Phase 顺序

### Phase A · 数据源地基
- `adapter.Listing(ctx, brokerSymbol)`（MT5：复用 `SymbolParamsRaw` → 映射 `SymbolInfo`+`SymGroup` → `02 §1.2` 的 Listing）。
- Listing 缓存：启动为订阅品种拉取，每日刷新 swap。
- PG `symbol_map` 表 + 启动加载（人工维护映射，如 Exness `EURUSDm`→`EURUSD`）。
- **验收**：拉出 ICMarkets+Exness 的 EURUSD/XAUUSD Listing，字段与 `02 §7` 对照表一致。

### Phase B · 评估核心
- `Instrument` + `Listing` 模型（02 §1）。
- `Evaluator`（02 §6）：成本模型四要素 + swap 按 `SwapType`（02 §4.2，先实现 `InPoints`）+ 盈亏换算 profitCurrency→USD（02 §3）。
- **验收**：给定候选（用真实报价/Listing），算出 NetProfit，与手算一致（含 EURUSD/GBPJPY 的 JPY 换算）。

### Phase C · 闭环骨架（CrossExchange）
- `CrossExchange` Detector（03 §2.1）。
- PG `opportunities` 表 + 机会仓库（内存 + 持久化）。
- `OpportunityStream` + `ConfirmOpportunity` gRPC（06）。
- `pipeline` 改为**仅 ConfirmOpportunity 触发**（04 §7，删 `_ = pipeline` 悬空）。
- desk WPF（C#，全新，不复用旧 Wails）：机会列表视图 + 确认按钮（ICommand → grpc-dotnet `ConfirmOpportunityAsync`），`OpportunityStream` 经 `ResponseStream.ReadAllAsync()` 推送到 ObservableCollection。
- **验收**：demo 全链路跑通——发现跨所价差→评估→推送 desk WPF→你确认→执行→归因回填。

### Phase D · Carry Detector
- 对冲套息 Detector（03 §2.2），swap 差真实可得。
- 长期持仓监控（套息天~周）。
- **验收**：发现 GBPJPY 等 swap 差机会，对冲套息净盈利正确。

### Phase E · Triangular Detector
- 三角套利（03 §2.3），执行最难，待 pipeline 稳定后做。

### Phase F · 风控完善 + 审计 + 归因
- `CapitalGate` 加"单平台 ≤ 40%"；`CircuitBreaker` 日亏 ≤ 3%。
- 新建 `internal/audit/`（Opportunity/Order 审计日志）。
- 归因：成交回填实际值 → 校准 Evaluator 滑点/swap/commission（07 §4）。
- **验收**：预估 vs 实际偏差被记录并反馈到阈值/成本。

### Phase G · Crypto 接口预留（不实现）
- `Listing.Swap(Funding)` 结构已为 Crypto 预留（02 §4.3，swap/funding 统一）；`Instrument.Kind` 预留 SPOT/PERP。Binance adapter/detector 留接口，本期不写。

---

## 3. 与旧 code-map Phase 的关系
- 旧 Phase 1–5（基础/通信/存储/执行/风控）**已完成**，新架构在其上叠加：
  - 旧 `engine`（规划但未实现）→ **不建**，改为 `detector` + `evaluator`（Phase B/C）。
  - 新增机会闭环（Phase C）、审计归因（Phase F）。
- 旧 `Notional()`（pipeline.go:43）**Phase B 删除/替换**为真实净盈利。

---

## 4. 每阶段强制项
1. **demo 验证**（ICMarkets 52993526 + Exness 277842155）。
2. **自我审计 A–F**（AGENTS §3）。
3. **更新 `docs/handoff/STATE.md`**（进度/阻塞/下一步，交接 Windsurf 或回 Claude）。
4. **Before Commit**（AGENTS §10：build/vet/test-race/check-file-lines/govulncheck）。

---

## 5. 回溯
- 先 FX / 跨所优先 → D-004
- Phase 内容 → 02/03/05/06/07
- 自我审计 / 交接 → AGENTS §3/§4
