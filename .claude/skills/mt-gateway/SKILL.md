---
name: mt-gateway
description: >
  MT4/MT5 网关全部操作的总入口。基于原始 proto 文件（mt4 43 RPC / mt5 57 RPC）
  的完整知识，7 个功能域：连接认证(01)、品种列表(02)、实时报价(03)、下单交易(04)、
  账户状态(05)、订单管理(06)、MT5高级功能(07)。所有操作共享同一 gRPC 会话，
  通过 mtapi.io 两层代理连接真实交易服务器。
  当需要实现、调试、审查任何 MT4/MT5 后端操作时使用此 skill。
---

# MT 网关操作

## 参考文档

| 编号 | 功能 | 文件 | 说明 |
|------|------|------|------|
| 01 | 连接认证 | [01-connection.md](references/01-connection.md) | 拨号、Connect/ConnectEx、CheckConnect、Disconnect、session |
| 02 | 品种列表 | [02-symbols.md](references/02-symbols.md) | Symbols、SymbolList、SymbolParamsMany、SymbolInfo 50+ 字段 |
| 03 | 实时报价 | [03-quotes.md](references/03-quotes.md) | SubscribeMany、OnQuote stream、tick 转换、重连 |
| 04 | 下单交易 | [04-trading.md](references/04-trading.md) | OrderSend、MT4 Op vs MT5 OrderType、已验证示例 |
| 05 | 账户状态 | [05-account.md](references/05-account.md) | AccountSummary、Account(MT5)、余额/净值/保证金映射 |
| 06 | **K线历史** | [**06-kline.md**](references/06-kline.md) | **PriceHistory/QuoteHistory、MT4/MT5 5 大差异、8 个陷阱、新增周期 checklist** |
| 07 | 订单管理 | [07-order-management.md](references/07-order-management.md) | 持仓/历史查询、修改、平仓、OrderUpdateSummary、状态机 |
| 08 | MT5 高级 | [08-mt5-advanced.md](references/08-mt5-advanced.md) | TickHistory、MarketWatch、Sessions、Events、MT5 差异能力 |

## 覆盖率状态

基于原始 proto 分析（`/opt/ant/grpc/mt4.proto` 2028 行，`/opt/ant/grpc/mt5.proto` 2986 行）：

| | MT4 | MT5 |
|---|---|---|
| Proto 定义 RPC 总数 | ~43 (6 Service) | ~57 (7 Service) |
| 生产环境已实现 | 5 | 5 |
| Legacy 包已封装 | ~15 | ~12 |
| 覆盖率 | ~9% | ~7% |

当前生产环境仅实现行情管道：Connect → SubscribeMany → OnQuote + QuoteHistory/PriceHistory。
Legacy 包（`/opt/ant/backend/legacy/mt4client/`、`mt5client/`）封装了 AccountSummary、OpenedOrders、
Symbols、OrderSend/Modify/Close 等，但按 v2 架构 invariant #8 不直接调用，需通过 L5 mthub 或 L2 adapter 集成。

## 共享概念

### 两层连接架构

所有操作都经过 mtapi.io 代理：

```
AlphaForge 适配器 (L2)
    │  grpc.Dial("mt4grpc3.mtapi.io:443")
    ▼
mtapi.io 网关
    │  ConnectRequest{Host: "43.199.125.167:443"}
    ▼
Exness MT 服务器
```

### Session 模式

每个 MT 账户有一个 gRPC 连接和一个 sessionID。所有 RPC 调用共用同一个 sessionID，必须通过 metadata 的 `id` header 传递。

```
登录(Connect) → 获得 sessionID → 所有后续 RPC 携带 id: <sessionID>
```

### 七层架构中的位置

```
L7  RPC 边界（ConnectRPC + SSE）
L6  应用编排（oms, marketplace, ai）
L5  会话/下单中心（mthub）          ← 统一下单、session 管理
L4  因子计算
L3  行情网关（mdgateway）            ← 当前已实现：tick 管道
L2  MT 适配（adapter/mt4, mt5/）    ← 纯翻译层，proto → DTO
L1  外部接口（mtapi.io gRPC）
```

invariant #8：业务代码 0 处直调 mt4client/mt5client，必须通过 L5 mthub 或 L2 adapter。

### 关键数据表

```
mt_accounts 表 — 账户凭据和 broker 地址
  ├── login          → tempID + ConnectRequest.User
  ├── password       → ConnectRequest.Password（明文策略见 mt-accounts skill）
  ├── broker_host    → ConnectRequest.Host（真实 IP）
  ├── broker_server  → 仅 UI 展示
  └── mt_token       → metadata authorization Bearer

brokers 表 — 经纪商配置
  └── mtapi_endpoint → grpc.Dial 目标（空=mtapi.io 公共网关）
```

### gRPC 元数据速查

| RPC | id | authorization |
|-----|-----|---------------|
| Connect / ConnectEx | `"mdgw-<login>"` | 不需要 |
| CheckConnect / Disconnect | `<sessionID>` | 不需要 |
| AccountSummary / Account | `<sessionID>` | 不需要 |
| Symbols / SymbolParams / SymbolList | `<sessionID>` | 不需要 |
| GetQuote / GetQuoteMany | `<sessionID>` | 不需要 |
| SubscribeMany / OnQuote / OnMarketWatch | `<sessionID>` | `"Bearer <mt_token>"` |
| OrderSend / OrderModify / OrderClose | `<sessionID>` | 不需要 |
| OpenedOrders / OrderHistory | `<sessionID>` | 不需要 |
| OnOrderUpdate / SubscribeOrderUpdate | `<sessionID>` | `"Bearer <mt_token>"` |
| PriceHistory / TickHistoryRequest | `<sessionID>` | `"Bearer <mt_token>"` |

### MT4/MT5 关键差异

| | MT4 | MT5 |
|---|---|---|
| gRPC 网关 | `mt4grpc3.mtapi.io:443` | `mt5grpc3.mtapi.io:443` |
| Proto 规模 | 2028 行, 6 Service, 43 RPC | 2986 行, 7 Service, 57 RPC |
| ConnectRequest.Id | **必须设置** (`&tempID`) | **无此字段** |
| ConnectRequest.User | `int32` | `uint64` |
| 订单类型枚举 | `Op` (6 values) | `OrderType` (11 values, 含 StopLimit/CloseBy) |
| Order Ticket | `int32` | `int64` |
| Order 字段 | 21 个 | 37 个（含 State/FillPolicy/DealInternal） |
| Bar 字段 | `Volume`(double) | `TickVolume`(uint64) + `Volume`(uint64) |
| AccountSummary | `Type` enum (Real/Contest/Demo) | `Type` string + `Method` enum (Netting/Hedging) |
| 独有功能 | OnQuoteHistory stream | TickHistory, MarketWatch, Sessions, Events stream |
| 独有 RPC | ClosedOrders, OrderDelete, OrderCloseBy | Account, SymbolList, ChangePassword, RequiredMargin |

## 参考代码

- MT4 适配器：`internal/mdgateway/adapter/mt4/gateway.go`
- MT5 适配器：`internal/mdgateway/adapter/mt5/gateway.go`
- 配置加载：`internal/mdgateway/runner.go` → `loadAccountConfigs()`
- 账户模型：`internal/mdgateway/adapter/mdtick/mdtick.go` → `AccountConfig`
- 架构设计：`/opt/ant/docs/architecture/02-overview.md`
- alfq 参考：`/opt/alfq/backend/go/internal/mdgateway/`
- Legacy 封装：`/opt/ant/backend/legacy/mt4client/`、`mt5client/`
- 原始 proto：`/opt/ant/grpc/mt4.proto`、`/opt/ant/grpc/mt5.proto`
