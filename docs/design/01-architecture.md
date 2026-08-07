# 01 · 系统架构（总览）

> 汇总 00–08，给出整体架构。读完本文件应能理解系统全貌。
> 本文件**更新** `docs/code-map.md` 的旧架构为新设计（detector+evaluator 替代 engine）；冲突处以本文件 + 各专题文档为准。

---

## 1. 分层（新架构）

```
Layer 0  decimalutil / errclass                    零依赖
Layer 1  bus(QuoteBus) / proto(生成)               类型 + 通信基础
Layer 2  adapter(MT5/Binance) / store(PG)          外部通信
Layer 3  execute / risk / audit                    业务逻辑
Layer 4  detector / evaluator                      ★新（替代旧 engine）
Layer 5  dashboard(gRPC server)                    聚合服务
Layer 6  cmd/core / cmd/desk / desk/* / frontend   入口 + 桌面 UI
```

> 关键变化：旧设计的 `engine/`（自动执行）**不建**，拆为 `detector/`（发现候选）+ `evaluator/`（评估净盈利），中间断开自动执行，改由人确认触发（D-003）。

---

## 2. 数据流

### 行情流（Hot Path, float64）
```
MT5 ×N ──OnQuote stream──► QuoteBus(cap=1，只留最新) ──► detector / dashboard
```

### 机会流（主流程）
```
QuoteBus + Listing缓存 ──► [Detector] ──候选──► [Evaluator] ──Opportunity──► 机会仓库
                                                                  │
                                                  OpportunityStream │ 推送
                                                                  ▼
                                                              desk（你确认）
                                                                  │ ConfirmOpportunity
                                                                  ▼
                                                          [ExecutionPipeline]（仅确认后）
```

### 订单流（执行，warm path, decimal）
```
ConfirmOpportunity → pipeline: revalidate → capital gate → 并发下单(all-or-nothing) → 失败对冲
                                                                        │
                                                                        ▼
                                                              audit.Log + store.opportunities(回填实际)
```

### 归因流（闭环）
```
成交实际值(Order.Swap/Commission/成交价) → opportunities 表(预估vs实际偏差) → 校准 Evaluator(滑点/swap/成本)
```

### 静态参数流（warm path）
```
启动: adapter.Listing(每品种) → Listing缓存(每日刷 swap)
符号: PG symbol_map → 内存映射(brokerSymbol→逻辑symbol)
```

---

## 3. goroutine 拓扑

```
main ── signal wait ── graceful shutdown
├── adapter recvLoop × N（每 broker 一个）→ QuoteBus.Publish
├── detector scan loop（事件驱动消费 QuoteBus：range over channel，非 time.Ticker 轮询；结合 Listing → 候选）
├── evaluator（按候选触发，算净盈利 → 机会仓库）
├── OpportunityStream goroutine（推 desk）
├── ExecutionPipeline（按需，每笔确认一次）→ leg goroutines（并发下单，信号量限流）
├── Listing 刷新（每日 ticker，Push-First 合法例外：静态参数非实时行情）
└── audit（同步，不加 goroutine）
```

goroutine 总数有界：N(broker) + 少量常驻(detector/evaluator/stream/listing刷新) + 按需执行腿。无无界增长（constraints §并发）。

---

## 4. 包依赖（更新 code-map）

```
cmd/core ─► detector, evaluator, dashboard, adapter, bus, store, risk, execute, audit
detector ─► bus, adapter(Listing), store         （发现）
evaluator ─► decimalutil, bus, adapter(Listing)  （评估，纯函数为主）
dashboard ─► bus, adapter, store, risk, execute  （gRPC，含 OpportunityStream/ConfirmOpportunity）
execute ─► adapter, risk, decimalutil, audit     （仅确认后触发）
```

- **`engine/` 不存在**（旧规划作废）→ 由 `detector/` + `evaluator/` 替代。
- `adapter` 新增 `Listing()` 拉取（05 §3）。
- `audit/` 新建（07 §3）。

---

## 5. 与旧架构的关键差异

| 维度 | 旧（evaluation-framework） | 新（本目录） |
|---|---|---|
| 定位 | Core 全自动交易，人监督 | 发现+评估+**人确认**+执行（D-003） |
| 品种模型 | Quote 平铺，contractSize 硬编码 | Instrument + Listing 两层（真实字段，02） |
| 机会 | signals 表，自动执行 | Opportunity 一等对象 + 推送/确认（04） |
| 净盈利 | Notional()（错的） | 成本模型四要素 + swap 按 SwapType（02 §4） |
| 引擎 | engine 自动 Execute | detector+evaluator，pipeline 仅确认后触发 |
| 套息 | 无（swap 仅成本） | Carry detector（03 §2.2） |

---

## 6. 回溯
- 分层/数据流/差异 → 00（公理）+ D-003/D-004 + 02/03/04/05/06/07
- 旧 code-map 的依赖方向规则仍适用（Layer N 只依赖下层）
- 精度分层（hot float64 / warm decimal）→ constraints §精度
