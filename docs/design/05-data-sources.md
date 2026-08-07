# 05 · 数据源

> 定义系统的两类输入数据：**动态报价**与**静态品种参数**，以及它们的获取、分离、时间校准。
> 依据公理①（统一抽象）、④（时间一致性）+ `02`（Listing）+ 现有 `QuoteBus`/`adapter`。

---

## 1. 两类数据，两条路径

| | 动态报价 | 静态品种参数 |
|---|---|---|
| 内容 | bid/ask/time | contractSize/swap/lots/margin/execution |
| 变化频率 | 每 tick | 慢（swap 每日，其余几乎不变） |
| 获取 | **gRPC stream（Push-First）** | **定期拉取（Pull，Push-First 合法例外）** |
| 精度路径 | hot path, float64 | warm path, decimal |
| 载体 | `QuoteBus`（现有） | `Listing` 缓存（02 §1.2） |
| 落地 | `Quote` 结构 | `Listing` 结构 |

**分离原则**：动态数据不混入静态（避免每 tick 重复传 contractSize）；静态不污染 hot path（decimal 不进 float64 比较）。这是精度分层（constraints §精度）与性能的统一。

---

## 2. 动态报价流（现有，保留）

- **`adapter.QuoteStream`** → `OnQuote` gRPC stream → `QuoteBus.Publish(Quote)`。
- **`QuoteBus`**：`cap=1` drain-then-replace channel，**只保留每个品种最新 tick**。这是公理④的防假信号核心——过期报价比没报价更危险（`evaluation-framework.md:90`）。
- **`Quote` 结构**（`bus/types.go`）：`{Symbol, Bid, Ask, Time, Broker, Platform}`，float64。
- **新鲜度度量 `server_age`**：用 `Quote.Time`（broker 服务器时间戳）算 `time.Since(quote.Time)`；P99 > 100ms 告警（`evaluation-framework.md:460`）。**只用服务器时间戳**，不依赖本地时钟。

---

## 3. 静态品种参数（Listing 拉取，新增）

- **`adapter.Listing(ctx, brokerSymbol) (*Listing, error)`**（Windsurf 实现）：调 `SymbolParams`（复用已验证的 `SymbolParamsRaw`）→ 把 `SymbolInfo` + `SymGroup` 映射成 `02 §1.2` 的 `Listing`。
- **加载时机**：core 启动时，为所有订阅品种 × 每个已连 broker 拉一次。
- **刷新策略**：contractSize/digits/lots 几乎不变（启动拉即可）；**swap 每日变** → 每日（或开仓前）刷新相关品种的 Listing。频率匹配 `Funding.SettlementFreq`（FX=DAILY）。
- warm path, `decimal.Decimal`。

---

## 4. Push-First 的合法例外（constraints §七）

- 动态报价：**必须 stream**（套利抢的就是它）——遵守 Push-First。
- 静态参数：`SymbolInfo` 是**查询接口，MT5 无 push 能力**，且对延迟不敏感（一天变一次）→ **定期拉取是 Push-First 的合法例外**（constraints §七："仅当数据源无 push 能力且对延迟不敏感时例外"）。
- Crypto funding rate（Binance，未来）：`markPrice` stream 带 funding rate → **届时改 push**。设计上 `Funding` 抽象已预留。

---

## 5. 符号映射表

- PG 表 `symbol_map(broker, broker_symbol, canonical_symbol)`，**人工维护**（02 §2）。
- core 启动加载到内存 `map[(broker,brokerSymbol)]canonical`。
- 用途：Detector 按逻辑品种聚合跨 broker 报价；下单时仍用原始 `brokerSymbol`（透传，不归一化）。

---

## 6. 时间一致性（公理④）

- 所有报价带 **broker 服务器时间戳**（`Quote.Time`）。
- Evaluator 评估机会时，校验每腿报价 `server_age` 在阈值内，否则机会判 `Expired`（02 §6、04 §2）。
- 跨 broker 时间基准：各 broker 服务器时钟可能有微差，`server_age` 按**各自**服务器时间算；跨 broker 比较时接受这个微差（机会阈值含安全垫吸收，03/讨论五）。

---

## 7. 实现指引（Windsurf）

| 组件 | 动作 |
|---|---|
| `adapter`（MT5） | 加 `Listing(ctx, brokerSymbol) (*Listing, error)`：调 `SymbolParamsRaw` → 映射 `SymbolInfo`+`SymGroup` → `Listing`（字段对照见 02 §7）。 |
| `Listing` 缓存 | `internal/`新建轻量缓存：`map[(broker,canonical)]*Listing`，启动拉取 + 每日刷新 swap。 |
| `symbol_map` | PG 迁移加表 + 启动加载。 |
| `QuoteBus` | 保留现有（已满足公理④的 cap=1）。 |
| `server_age` | 指标暴露（`evaluation-framework.md:451` 的 `server_age_us`）。 |

---

## 8. 回溯
- Listing/符号归一化 → 公理①、02
- QuoteBus cap=1 / server_age / Expired → 公理④
- 静态定期拉取 → Push-First 合法例外
- 精度分层（hot float64 / warm decimal）→ constraints §精度
