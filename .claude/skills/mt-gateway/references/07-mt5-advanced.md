---
name: mt-gateway-07-mt5-advanced
description: >
  MT5 独有高级功能。涵盖 TickHistory（tick 级历史回溯）、MarketWatch
  （市场报价板）、Sessions（交易/报价时段查询）、统一 Events 流、
  ConnectEx（按服务器名连接）。这些是 MT5 相对 MT4 的差异化能力。
---

# 07 — MT5 高级功能

MT5 proto 比 MT4 多出约 14 个 RPC + 3 个独有 Service。以下按功能域组织。

## TickHistory — Tick 级历史回溯

**MT5 独有服务**，提供历史 tick 数据流。MT4 无对应功能。

```protobuf
service TickHistory {
  rpc TickHistoryRequest(TickHistoryRequestRequest) returns (TickHistoryRequestReply);
  rpc TickHistoryStop(TickHistoryStopRequest) returns (TickHistoryStopReply);
  rpc OnTickHistory(OnTickHistoryRequest) returns (stream OnTickHistoryReply);
}
```

### 流程

```
TickHistoryRequest(from, symbol) → 启动后端采集
    │
    ▼
OnTickHistory stream → 接收 tick 数据
    │
    ▼
TickHistoryStop → 停止采集
```

### 用途

- 回测：获取指定品种的历史 tick 级数据
- 精确复盘：比 PriceHistory 的 OHLC 粒度更细
- 注意：需要在 MT5 Streams 服务中也订阅 `OnTickHistory`

## MarketWatch — 市场报价板

**MT5 独有**。获取 MT5 终端"市场报价"窗口的全部品种快照。

```go
resp, err := mt5Client.MarketWatchMany(ctx, &pb.MarketWatchManyRequest{Id: sessionID})
```

```protobuf
message MarketWatch {
  string Symbol = 1;
  double Bid = 2;
  double Ask = 3;
  double Spread = 4;
  int32 Digits = 5;
  double SessionBuy = 6;
  double SessionSell = 7;
  int64 Time = 8;
  double Volume = 9;
}
```

与 `GetQuoteMany` 的区别：
- `GetQuoteMany`：返回指定的若干品种报价
- `MarketWatchMany`：返回 MT5 终端市场报价窗口**所有**品种
- `MarketWatch` 多了 `Spread`、`SessionBuy/Sell`、`Volume`

## Sessions — 交易/报价时段查询

**MT5 独有**。查询品种的交易时段和报价时段，判断当前是否可交易。

### IsTradeSession / IsTradeSessionMany

```go
resp, err := mt5Client.IsTradeSession(ctx, &pb.IsTradeSessionRequest{
    Id:     sessionID,
    Symbol: "BTCUSDm",
})
canTrade := resp.GetResult()  // bool
```

### IsQuoteSession / IsQuoteSessionMany

```go
resp, err := mt5Client.IsQuoteSession(ctx, &pb.IsQuoteSessionRequest{
    Id:     sessionID,
    Symbol: "BTCUSDm",
})
hasQuotes := resp.GetResult()  // bool
```

### SymbolSessionsEx / SymbolSessionsExMany

返回品种的详细交易/报价时段信息（起止时间、星期几）。

```protobuf
message SymbolSessionsEx {
  string Symbol = 1;
  // 交易时段
  uint64 TradeStarts = 2;   // 起始时间（秒 from midnight）
  uint64 TradeEnds = 3;     // 结束时间（秒 from midnight）
  bool TradeToday = 4;      // 今天是否有交易日
  bool TradeMonday-Sunday = ...;
  // 报价时段
  uint64 QuoteStarts/Ends = ...;
  bool QuoteToday/Monday-Sunday = ...;
}
```

### 使用场景

- 前端「当前是否可交易」状态灯
- 定时任务：只在交易时段执行操作
- 风控：报价时段外不接单

## Events — 统一事件流

**MT5 独有**。单一 stream 接收所有事件类型（替代 MT4 的多个独立 stream）。

```protobuf
service Streams {
  rpc Events(EventsRequest) returns (stream EventsReply);
}
```

`Events` 统一了报价、订单、市场报价板等事件。MT4 需要分别订阅 `OnQuote` + `OnOrderUpdate` + `OnTickValue` + `OnOrderProfit`，而 MT5 的 `Events` 一个流搞定。

> 注：MT5 仍然保留了独立的 `OnQuote`、`OnOrderUpdate` 等 stream 以兼容老代码，但推荐使用 `Events`。

## OnMarketWatch — 实时报价板推送

**MT5 独有 stream**。实时推送市场报价板品种的 bid/ask 变更。

```protobuf
rpc OnMarketWatch(OnMarketWatchRequest) returns (stream OnMarketWatchReply);
```

与 `OnQuote` 的差异：
- `OnQuote`：仅推送已订阅的品种
- `OnMarketWatch`：推送市场报价板所有品种
- `OnMarketWatch` 包含 `Spread`、`SessionBuy/Sell` 等额外字段

## 完整 Stream 清单（MT4 vs MT5）

| Stream | MT4 | MT5 | 说明 |
|--------|-----|-----|------|
| `OnQuote` | ✓ | ✓ | 实时报价 tick |
| `OnOrderUpdate` | ✓ | ✓ | 订单变更（MT4 含完整账户快照） |
| `OnTickValue` | ✓ | ✓ | tick 价值变更 |
| `OnOrderProfit` | ✓ | ✓ | 订单盈亏变更 |
| `OnMarketWatch` | — | ✓ | **MT5 独有**：报价板实时推送 |
| `OnTickHistory` | — | ✓ | **MT5 独有**：历史 tick 回放 |
| `OnMail` | — | ✓ | **MT5 独有**：账户邮件推送 |
| `OnOpenedOrdersTickets` | — | ✓ | **MT5 独有**：持仓 ticket 变更 |
| `Events` | — | ✓ | **MT5 独有**：统一事件流 |
| `OnQuoteHistory` | ✓ | — | **MT4 独有**：异步历史报价回调 |

## 辅助功能差异

| RPC | MT4 | MT5 | 说明 |
|-----|-----|-----|------|
| `ConnectEx` | ✓ | ✓ | 按服务器名连接（非 host:port） |
| `ChangePassword` | — | ✓ | MT5 可改密码 |
| `Mails` | — | ✓ | 账户邮件 |
| `RequiredMargin` | — | ✓ | 计算所需保证金 |
| `TickValueWithSize` | ✓ | ✓ | 指定手数的 tick 价值 |
| `ServerTimezone` | ✓ | ✓ | 服务器时区 |
| `IsInvestor` | ✓ | — | MT4 查询是否只读模式 |

## Service 层工具

两个平台都有 `Service`，但方法不同：

| RPC | MT4 | MT5 |
|-----|-----|-----|
| `Ping` | ✓ | ✓ |
| `PingHost` | ✓ | ✓ |
| `PingHostMany` | ✓ | ✓ |
| `MemorySnapshot` | ✓ | ✓ |
| `Search` | ✓ | ✓ |
| `GetClients` | ✓ | ✓ |
| `GetLogs` | ✓ | — |
| `GetLogsByUser` | ✓ | — |
| `MemoryUsage` | ✓ | — |
| `GetDemo` | — | ✓ |
| `Version` | — | ✓ |
| `Health` | — | ✓ |

## 架构影响

在 ant 七层架构中：

- L2 adapter 层：MT4 和 MT5 adapter 目前只用了 4 个 RPC（Connect/SubscribeMany/OnQuote/PriceHistory）
- 要利用这些 MT5 高级功能，需在 L2 adapter 增加对应的 gRPC 客户端和方法
- L5 `mthub` 负责 session 管理和统一下单，应该封装 Session/Order/Trade 等差异化调用
- invariant #8：业务代码不直调 mt4client/mt5client，必须通过 L5 mthub 或 L2 adapter

## 参考代码

- MT5 proto：`/opt/ant/grpc/mt5.proto`（2986 行，7 个 Service，57 个 RPC）
- MT4 proto：`/opt/ant/grpc/mt4.proto`（2028 行，6 个 Service，43 个 RPC）
- Legacy MT5：`/opt/ant/backend/legacy/mt5client/`
- 架构设计：`/opt/ant/docs/architecture/02-overview.md`
