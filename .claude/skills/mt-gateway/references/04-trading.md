---
name: mt-gateway-04-trading
description: >
  MT4/MT5 下单交易操作。涵盖市价单、挂单、OrderSend 完整参数、
  MT4 Op vs MT5 OrderType 对照、已验证示例。订单修改/平仓/查询
  见 06-order-management。
---

# 04 — 下单交易

## RPC 概览

| RPC | 用途 |
|-----|------|
| `OrderSend` | 开仓（市价/挂单） |

> 订单修改、平仓、删除、查询见 [06-order-management.md](06-order-management.md)

## OrderSend — 下单

MT4 路径：`/mt4grpc.Trading/OrderSend`
MT5 路径：`/mt5grpc.Trading/OrderSend`

```go
resp, err := tradingCli.OrderSend(ctx, &pb.OrderSendRequest{
    Id:        sessionID,
    Symbol:    "BTCUSDm",
    Operation: pb.Op_Op_Buy,   // MT4: Op enum
    Volume:    0.01,           // 0.01 手
    Slippage:  100,
    Comment:   "ant-test",
})
```

### 操作类型对照

| MT4 `Op` | MT5 `OrderType` | 说明 |
|----------|-----------------|------|
| `Op_Buy` (0) | `OrderType_Buy` (0) | 市价买入 |
| `Op_Sell` (1) | `OrderType_Sell` (1) | 市价卖出 |
| `Op_BuyLimit` (2) | `OrderType_BuyLimit` (2) | 限价买入 |
| `Op_SellLimit` (3) | `OrderType_SellLimit` (3) | 限价卖出 |
| `Op_BuyStop` (4) | `OrderType_BuyStop` (4) | 突破买入 |
| `Op_SellStop` (5) | `OrderType_SellStop` (5) | 突破卖出 |
| — | `OrderType_BuyStopLimit` (6) | **MT5 独有**：止损限价买入 |
| — | `OrderType_SellStopLimit` (7) | **MT5 独有**：止损限价卖出 |
| — | `OrderType_CloseBy` (8) | **MT5 独有**：对冲平仓 |

### 请求参数

| 字段 | 必填 | MT4 | MT5 | 说明 |
|------|------|-----|-----|------|
| `Id` | ✓ | string | string | sessionID |
| `Symbol` | ✓ | string | string | 品种名，如 "BTCUSDm" |
| `Operation` / `OrderType` | ✓ | Op enum | OrderType enum | 操作类型 |
| `Volume` / `Lots` | ✓ | double | double | 手数（如 0.01） |
| `Price` | | double | double | 挂单价（市价单可不填） |
| `Slippage` | | int32 | int32 | 滑点容忍（点数） |
| `StopLoss` | | double | double | 止损价 |
| `TakeProfit` | | double | double | 止盈价 |
| `Comment` | | string | string | 备注 |
| `Magic` | | int32 | int64 | 魔术号（MT5 是 int64） |
| `Expiration` | | string | string | 挂单过期时间（yyyy-MM-ddTHH:mm:ss） |
| `PlacedType` | | PlacedType | PlacedType | 下单来源标识 |
| `FillPolicy` | | — | FillPolicy | **MT5 独有**：成交策略 |

### MT5 FillPolicy 枚举

```protobuf
enum FillPolicy {
  FillPolicy_FillOrKill        = 0;  // 全成交或取消
  FillPolicy_ImmediateOrCancel = 1;  // 立即成交或取消
  FillPolicy_FlashFill         = 2;  // 闪电成交
  FillPolicy_Any               = 3;  // 任意
}
```

### 响应

```go
type OrderSendReply struct {
    Result *Order
    Error  *Error
}
```

MT4 Order 关键字段：
| 字段 | 说明 |
|------|------|
| `Ticket` | 成交单号（int32） |
| `Symbol` | 品种名 |
| `Lots` | 实际手数 |
| `OpenPrice` | 开仓价 |
| `OpenTime` | 开仓时间 |
| `ClosePrice` | 平仓价（持仓中为空） |
| `Profit` | 浮动盈亏 |

MT5 Order 有 37 个字段（详见 [06-order-management.md](06-order-management.md)），Ticket 是 int64。

## 已验证示例

MT4 测试账户、BTCUSDm 0.01 手市价做多：

```
Connect: session="mdgw-95172262"
OrderSend: BTCUSDm BUY 0.01 lots
  → Ticket: 206501996
  → OpenPrice: 76503.16
  → Status: 持仓中
```

完整测试代码：`/opt/ant/backend/cmd/mt4trade/main.go`

## 参考代码

- 测试验证：`/opt/ant/backend/cmd/mt4trade/main.go`（MT4 市价单 + 连接验证）
- MT4 适配器：`/opt/ant/backend/internal/mdgateway/adapter/mt4/gateway.go`
- Legacy 封装：`/opt/ant/backend/legacy/mt4client/trading_methods.go`
- Legacy 封装：`/opt/ant/backend/legacy/mt5client/trading_methods.go`
- 订单管理：`/opt/ant/.claude/skills/mt-gateway/references/06-order-management.md`
