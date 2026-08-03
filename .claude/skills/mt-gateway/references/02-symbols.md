---
name: mt-gateway-02-symbols
description: >
  MT4/MT5 获取可交易品种列表。涵盖 Symbols（全量名称）、SymbolList（MT5）、
  SymbolParams/SymbolParamsMany（完整详情，50+ 字段）。用于品种选择器后端、
  交易品种校验。
---

# 02 — 品种列表

## RPC 概览

| RPC | 平台 | 用途 | 响应 |
|-----|------|------|------|
| `Symbols` | MT4/MT5 | 获取全部品种名+参数 | `[]*SymbolInfo` |
| `SymbolList` | MT5 only | 仅品种名列表 | `[]string` |
| `SymbolParams` | MT4/MT5 | 单品种详情 | `*SymbolInfo` |
| `SymbolParamsMany` | MT4/MT5 | 批量多品种详情 | `[]*SymbolInfo` |

## Symbols — 全量品种

MT4 路径：`/mt4grpc.MT4/Symbols`
MT5 路径：`/mt5grpc.MT5/Symbols`

```go
resp, err := client.Symbols(ctx, &pb.SymbolsRequest{Id: sessionID})
```

响应 MT4 返回 `[]string`（仅品种名），MT5 返回 `[]*SymbolInfo`（含完整参数）。

### MT4 响应

```go
type SymbolsReply struct {
    Result []string  // ["BTCUSDm", "ETHUSDm", "EURUSDm", ...]
    Error  *Error
}
```

### MT5 响应

```go
type SymbolsReply struct {
    Result []*SymbolInfo  // 全量品种+参数，数据量大
    Error  *Error
}
```

> MT5 `Symbols` 返回所有品种的全部参数，数据量可能很大。如只需品种名列表，用 `SymbolList`。

## SymbolList — 仅品种名（MT5 only）

```go
resp, err := mt5Client.SymbolList(ctx, &pb.SymbolListRequest{Id: sessionID})
// resp.GetResult() → ["BTCUSDm", "ETHUSDm", ...]
```

## SymbolParams — 单品种详情

```go
resp, err := client.SymbolParams(ctx, &pb.SymbolParamsRequest{
    Id:     sessionID,
    Symbol: "BTCUSDm",
})
```

## SymbolParamsMany — 批量品种详情

```go
resp, err := client.SymbolParamsMany(ctx, &pb.SymbolParamsManyRequest{
    Id:      sessionID,
    Symbols: []string{"BTCUSDm", "ETHUSDm", "XRPUSDm"},
})
```

## SymbolInfo 关键字段（MT4/MT5 共有）

交易参数：

| 字段 | 类型 | 说明 |
|------|------|------|
| `SymbolName` | string | 品种名（如 "BTCUSDm"） |
| `Digits` | int32 | 小数位 |
| `TickSize` | double | 最小变动单位 |
| `TickValue` | double | 每 tick 价值（账户货币） |
| `ContractSize` | double | 合约大小 |
| `MinLot` / `MaxLot` / `LotStep` | double | 手数限制 |
| `MinVolume` / `MaxVolume` / `VolumeStep` | double | 交易量限制（MT5） |

点差：

| 字段 | 说明 |
|------|------|
| `Spread` | 当前点差（点数） |
| `SpreadBalance` | 点差余额阈值 |

保证金：

| 字段 | 说明 |
|------|------|
| `MarginInitial` | 初始保证金 |
| `MarginMaintenance` | 维持保证金 |
| `MarginHedged` | 对冲保证金 |
| `MarginDivider` | 保证金除数 |

交易模式：

| 字段 | 说明 |
|------|------|
| `TradeMode` | 交易模式（无交易/仅平仓/完全交易等） |
| `TradeExecution` | 执行模式（请求执行/立即执行/市场执行/交易所执行） |
| `FillingMode` | 成交模式（MT5） |

限价/止损：

| 字段 | 说明 |
|------|------|
| `StopLimit` | 止损/止盈最小距离 |
| `FreezeLevel` | 冻结距离（接近市价不可修改订单） |
| `LimitLevel` | 挂单最小距离 |

交易时段（MT5 `SymbolSessionsEx` 返回更详细数据）：

| 字段 | 说明 |
|------|------|
| `SessionOpen` ~ `SessionClose` | 7 天交易时段 |
| `QuoteSessionOpen` ~ `QuoteSessionClose` | 7 天报价时段 |

掉期/库存费：

| 字段 | 说明 |
|------|------|
| `SwapLong` / `SwapShort` | 多/空库存费 |
| `SwapMode` | 库存费计算模式 |
| `Swap3Days` | 三重库存费日（周几） |

其他：

| 字段 | 说明 |
|------|------|
| `CurrencyBase` / `CurrencyProfit` / `CurrencyMargin` | 基础/盈亏/保证金货币 |
| `GtcMode` | GTC 订单模式 |
| `CalculationMode` | 盈亏计算模式 |
| `Description` | 品种描述 |

## MT4/MT5 SymbolInfo 关键差异

| | MT4 | MT5 |
|---|---|---|
| 总字段数 | ~30 | ~50+ |
| `Symbols` 返回 | `[]string` | `[]*SymbolInfo` |
| `SymbolList` | 无 | 有 |
| 交易时段 | 简单 7 天字段 | 另有 `SymbolSessionsEx` 独立 RPC |
| FillingMode | 无 | 有（FillPolicy） |

## 前端调用链

前端 `marketApi.getSymbols(accountId)` → `MarketService.GetSymbols` → MT `Symbols` RPC → 返回 `SymbolInfo[]`。

在 `ui-patterns` skill 中有前端选择器使用示例。

## 参考代码

- `internal/connect/mthub_service.go` → SymbolParams 接口
- `internal/mthub/service.go` → FetchSymbolParams
- Legacy：`/opt/ant/backend/legacy/mt5client/account_methods.go` → Symbols/SymbolList/SymbolParams
- MT4 proto：`/opt/ant/grpc/mt4.proto`（Symbols L92, SymbolParams L99）
- MT5 proto：`/opt/ant/grpc/mt5.proto`（Symbols L120, SymbolList L126, SymbolParamsMany L137）
