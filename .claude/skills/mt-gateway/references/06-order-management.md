---
name: mt-gateway-06-order-management
description: >
  MT4/MT5 订单全生命周期管理。涵盖持仓查询（OpenedOrders）、历史订单
  （OrderHistory/ClosedOrders）、订单修改（OrderModify）、平仓（OrderClose）、
  删除挂单（OrderDelete）。MT4 Op 与 MT5 OrderType 枚举对照。
---

# 06 — 订单管理

## RPC 概览

| RPC | 平台 | 用途 |
|-----|------|------|
| `OpenedOrders` | MT4/MT5 | 当前持仓列表 |
| `OpenedOrder` | MT5 only | 按 ticket 查单个持仓 |
| `OpenedOrdersTickets` | MT5 only | 仅返回 ticket 号列表 |
| `OrderHistory` | MT4/MT5 | 历史订单（按时间范围） |
| `OrderHistoryPagination` | MT5 only | 分页历史订单 |
| `PendingOrderHistory` | MT5 only | 历史挂单 |
| `ClosedOrders` | MT4 only | 最近 10 单已平仓 |
| `OrderModify` | MT4/MT5 | 修改止损/止盈/价格 |
| `OrderClose` | MT4/MT5 | 平仓 |
| `OrderDelete` | MT4 only | 删除挂单 |
| `OrderCloseBy` | MT4 only | 反向对冲平仓 |

## OpenedOrders — 当前持仓

MT4 路径：`/mt4grpc.MT4/OpenedOrders`
MT5 路径：`/mt5grpc.MT5/OpenedOrders`

```go
resp, err := client.OpenedOrders(ctx, &pb.OpenedOrdersRequest{Id: sessionID})
orders := resp.GetResult()  // []*Order
```

### MT4 Order 结构（21 字段）

| 字段 | 类型 | 说明 |
|------|------|------|
| `Ticket` | int32 | 成交单号（唯一标识） |
| `Type` | Op enum | 订单类型 |
| `Symbol` | string | 品种名 |
| `Lots` | double | 手数 |
| `OpenPrice` | double | 开仓价 |
| `OpenTime` | Timestamp | 开仓时间 |
| `ClosePrice` | double | 平仓价（持仓中为 0） |
| `CloseTime` | Timestamp | 平仓时间 |
| `StopLoss` | double | 止损价 |
| `TakeProfit` | double | 止盈价 |
| `Profit` | double | 盈亏 |
| `Swap` | double | 库存费 |
| `Commission` | double | 佣金 |
| `MagicNumber` | int32 | 魔术号 |
| `Comment` | string | 备注 |
| `PlacedType` | enum | 下单来源 |
| `Expiration` | Timestamp | 挂单过期时间 |
| `RateOpen` | double | 开仓汇率 |
| `RateClose` | double | 平仓汇率 |
| `RateMargin` | double | 保证金汇率 |

### MT5 Order 结构（37 字段）

MT5 Order 包含更多字段，关键差异：

| 字段 | 类型 | 说明 |
|------|------|------|
| `Ticket` | int64 | 成交单号（MT4 是 int32） |
| `OrderType` | OrderType enum | 订单类型 |
| `State` | OrderState enum | 订单状态机 |
| `Lots` | double | 手数 |
| `CloseLots` | double | 已平仓手数 |
| `CloseVolume` | uint64 | 已平仓量 |
| `ContractSize` | double | 合约大小 |
| `Digits` | int32 | 小数位 |
| `FillPolicy` | FillPolicy enum | 成交策略 |
| `ExpirationType` | ExpirationType enum | 过期类型 |
| `DealType` | DealType enum | 交易类型 |
| `DealInternalIn/Out` | DealInternal | 成交内部分配详情 |
| `OrderInternal` | OrderInternal | 订单内部分配 |
| `OpenTimestampUTC` | int64 | 开仓 UTC 时间戳 |
| `RequestId` | int32 | 请求 ID |
| `StopLimitPrice` | double | 止损限价 |
| `ProfitRate` | double | 盈亏汇率 |
| `PartialCloseDeals` | []DealInternal | 部分平仓记录 |

### MT5 OrderState 状态机

```protobuf
enum OrderState {
  OrderState_Started          = 0;  // 已启动
  OrderState_Placed           = 1;  // 已挂单
  OrderState_Cancelled        = 2;  // 已取消
  OrderState_Partial          = 3;  // 部分成交
  OrderState_Filled           = 4;  // 完全成交
  OrderState_Rejected         = 5;  // 已拒绝
  OrderState_Expired          = 6;  // 已过期
  OrderState_RequestAdding    = 7;  // 正在添加
  OrderState_RequestModifying = 8;  // 正在修改
  OrderState_RequestCancelling = 9; // 正在取消
}
```

### MT5 FillPolicy

```protobuf
enum FillPolicy {
  FillPolicy_FillOrKill        = 0;
  FillPolicy_ImmediateOrCancel = 1;
  FillPolicy_FlashFill         = 2;
  FillPolicy_Any               = 3;
}
```

## MT4 Op vs MT5 OrderType 对照

| MT4 `Op` | MT5 `OrderType` | 说明 |
|----------|-----------------|------|
| `Op_Buy` (0) | `OrderType_Buy` (0) | 市价做多 |
| `Op_Sell` (1) | `OrderType_Sell` (1) | 市价做空 |
| `Op_BuyLimit` (2) | `OrderType_BuyLimit` (2) | 限价买入 |
| `Op_SellLimit` (3) | `OrderType_SellLimit` (3) | 限价卖出 |
| `Op_BuyStop` (4) | `OrderType_BuyStop` (4) | 突破买入 |
| `Op_SellStop` (5) | `OrderType_SellStop` (5) | 突破卖出 |
| — | `OrderType_BuyStopLimit` (6) | **MT5 独有**：止损限价买入 |
| — | `OrderType_SellStopLimit` (7) | **MT5 独有**：止损限价卖出 |
| — | `OrderType_CloseBy` (8) | **MT5 独有**：对冲平仓 |

MT5 独有 3 种订单类型：BuyStopLimit、SellStopLimit、CloseBy。

## OrderModify — 修改订单

MT4/MT5 均支持，修改持仓或挂单的止损/止盈/价格。

```go
resp, err := tradingCli.OrderModify(ctx, &pb.OrderModifyRequest{
    Id:        sessionID,
    Ticket:    206501996,
    StopLoss:  76000.0,
    TakeProfit: 77000.0,
    Price:     0,  // 市价单不设
})
```

MT4 的 `OrderModifyRequest.Ticket` 是 `int32`，MT5 是 `int64`。

## OrderClose — 平仓

```go
resp, err := tradingCli.OrderClose(ctx, &pb.OrderCloseRequest{
    Id:       sessionID,
    Ticket:   206501996,
    Volume:   0.01,
    Price:    0,       // 市价平仓
    Slippage: 100,
})
```

MT4 `OrderCloseRequest` 无 `Volume` 字段（整单平），MT5 有 `Volume`（支持部分平仓）。

## OrderDelete — 删除挂单（MT4 only）

仅删除未成交的挂单（BuyLimit/SellLimit/BuyStop/SellStop）。

```go
resp, err := tradingCli.OrderDelete(ctx, &pb.OrderDeleteRequest{
    Id:     sessionID,
    Ticket: 206501997,
})
```

## 实时订单更新

两个平台都支持通过 stream 接收订单变更：

| Stream | 平台 | 内容 |
|--------|------|------|
| `OnOrderUpdate` | MT4/MT5 | 单个订单变更事件 |
| `Events` | MT5 only | 统一事件流（含订单 + 报价） |
| `OnOpenedOrdersTickets` | MT5 only | 持仓 ticket 号变更 |

### MT4 OrderUpdateSummary

`OnOrderUpdate` 推送的是 `OrderUpdateSummary`，包含**完整账户快照**：

```protobuf
message OrderUpdateSummary {
  OrderUpdateEventArgs Update = 1;    // 变更的订单 + action
  double Balance = 2;
  double Equity = 5;
  double Margin = 6;
  double FreeMargin = 7;
  double MarginLevel = 8;
  double Leverage = 9;
  string Currency = 10;
  repeated Order OpenedOrders = 13;   // 全部当前持仓
}
```

这意味着订阅 `OnOrderUpdate` 后，无需单独调 `AccountSummary` + `OpenedOrders` — 每次订单变更都会附带完整状态。

### UpdateAction 枚举

```protobuf
enum UpdateAction {
  UpdateAction_PositionOpen   = 0;  // 新持仓
  UpdateAction_PositionClose  = 1;  // 持仓平仓
  UpdateAction_PositionModify = 2;  // 持仓修改
  UpdateAction_PendingOpen    = 3;  // 新挂单
  UpdateAction_PendingClose   = 4;  // 挂单删除
  UpdateAction_PendingModify  = 5;  // 挂单修改
}
```

## 参考代码

- MT4 proto：`/opt/ant/grpc/mt4.proto` L703-1846
- MT5 proto：`/opt/ant/grpc/mt5.proto` L927-1259
- MT4 适配器：`internal/mdgateway/adapter/mt4/gateway.go` → `SubscribeOrderUpdate` / `orderUpdateRecvLoop`
- MT5 适配器：`internal/mdgateway/adapter/mt5/gateway.go` → `SubscribeOrderUpdate` / `orderUpdateRecvLoop`
- 类型定义：`internal/mdgateway/adapter/mdtick/mdtick.go` → `OrderUpdateHandler` / `OrderUpdate` / `OrderUpdatePosition`
- mthub broker：`internal/mthub/types.go` → `PositionSnapshot` / `PositionSnapshotBroker`
- stream handler：`internal/connect/stream_handler.go` → `SubscribeEvents` / `orderRecordToUpdateEvent`
- Legacy 封装：`/opt/ant/backend/legacy/mt4client/trading_methods.go`
- Legacy 封装：`/opt/ant/backend/legacy/mt5client/trading_methods.go`
- 下单测试：`/opt/ant/backend/cmd/mt4trade/main.go`

## Gateway 实现模式 — OnOrderUpdate

### 接口定义

`manager.go` 中 Gateway 接口增加：

```go
SubscribeOrderUpdate(ctx context.Context, handler mdtick.OrderUpdateHandler) error
```

### 内部类型（`mdtick` 包）

```go
type OrderUpdateHandler func(o *OrderUpdate)

type OrderUpdate struct {
    AccountID, Platform string
    UpdateTicket  int64; UpdateType, UpdateSymbol string
    UpdateVolume, UpdateOpenPrice, UpdateClosePrice float64
    UpdateProfit, UpdateSwap, UpdateCommission      float64
    UpdateComment string; UpdateOpenTime, UpdateCloseTime int64
    UpdateSL, UpdateTP float64
    Balance, Credit, Equity, Margin, FreeMargin, MarginLevel, Profit float64
    Positions []OrderUpdatePosition   // 全部持仓（完整 OpenedOrders）
}

type OrderUpdatePosition struct {
    Ticket int64; Symbol, Type string
    Volume, OpenPrice, CurrentPrice, StopLoss, TakeProfit float64
    Profit, Swap, Commission float64
    Comment string; OpenTime int64
}
```

### MT5 实现模式

```go
func (g *Gateway) SubscribeOrderUpdate(ctx context.Context, handler mdtick.OrderUpdateHandler) error {
    go g.orderUpdateRecvLoop(ctx, handler)
    return nil
}

func (g *Gateway) orderUpdateRecvLoop(ctx context.Context, handler mdtick.OrderUpdateHandler) {
    // 订阅 OnOrderUpdate stream，带指数退避重连
    stream, _ := sc.OnOrderUpdate(subCtx, &pb.OnOrderUpdateRequest{Id: sid})
    for {
        resp, _ := stream.Recv()
        s := resp.GetResult()
        
        // 1. 单笔变动（可选）
        update := s.GetUpdate()
        // 提取 updateTicket/updateType/updateSymbol/... 从 update.GetOrder()
        
        // 2. ⚠️ 全部持仓 — 迭代 s.GetOpenedOrders()，映射到 OrderUpdatePosition
        for _, o := range s.GetOpenedOrders() {
            positions = append(positions, OrderUpdatePosition{
                Ticket: o.GetTicket(), Symbol: o.GetSymbol(),
                Type: mt5OrderTypeLabel(o.GetOrderType()), // ← 含买卖方向!
                Volume: o.GetLots(),                        // ← MT5 用 Lots，非 Volume
                OpenPrice: o.GetOpenPrice(), CurrentPrice: o.GetClosePrice(),
                StopLoss: o.GetStopLoss(), TakeProfit: o.GetTakeProfit(),
                Profit: o.GetProfit(), Swap: o.GetSwap(), Commission: o.GetCommission(),
            })
        }
        
        // 3. 一次回调，包含全部数据
        handler(&OrderUpdate{..., Positions: positions})
    }
}
```

### MT4 实现模式

与 MT5 结构相同，关键差异：
- Ticket 从 `int32` cast 到 `int64`
- `UpdateAction` 枚举映射（PositionOpen→"open", PositionClose→"close", PendingClose→"pending_close"...）
- `Op` 枚举映射（用 `mt4OrderOpLabel`，含买卖方向）

## 数据映射规则（已验证）

### 规则 1：Type 必须组合 Side + OrderType

❌ **错误**：只用 OrderType 枚举
```go
Type: orderTypeLabel(rec.OrderType)  // "market", "limit", "stop" — 缺少方向!
```

✅ **正确**：组合 Side 和 OrderType
```go
func orderSideTypeLabel(side mthub.Side, ot mthub.OrderType) string {
    prefix := "buy"
    if side == mthub.SideSell { prefix = "sell" }
    switch ot {
    case mthub.OrderMarket:    return prefix           // "buy", "sell"
    case mthub.OrderLimit:     return prefix + "_limit"    // "buy_limit", "sell_limit"
    case mthub.OrderStop:      return prefix + "_stop"     // "buy_stop", "sell_stop"
    case mthub.OrderStopLimit: return prefix + "_stop_limit"
    default:                   return prefix
    }
}
```

原因：前端 `normalizePositionSide()` 依赖此值判断买卖方向。若值不含 "sell" 则默认为 "buy"。

### 规则 2：Canonical 必须与 SymbolRaw 同时设置

❌ **错误**：`FetchOpenedOrders` 只设 `SymbolRaw`
```go
OrderRecord{Ticket: ..., SymbolRaw: o.GetSymbol()}  // Canonical 为空!
```

✅ **正确**：同时设置 Canonical
```go
OrderRecord{Ticket: ..., SymbolRaw: o.GetSymbol(), Canonical: o.GetSymbol()}
```

原因：`orderRecordToUpdateEvent` 使用 `Canonical` 填充 `Symbol` 字段，空值导致前端显示无品种名。

### 规则 3：MT5 手数用 Lots，非 Volume

❌ **错误**：`Volume: o.GetVolume()` — uint64，tick 成交量
✅ **正确**：`Volume: o.GetLots()` — double，交易手数

## 前后端数据流

```
MT5/MT4 gRPC OnOrderUpdate stream
  │  OrderUpdateSummary { Update, OpenedOrders[], Balance... }
  ▼
Gateway.orderUpdateRecvLoop
  │  构建 mdtick.OrderUpdate（含全部持仓 + 账户指标）
  ▼
main.go OnOrderUpdate callback
  │  ├─ accountBroker.Publish(AccountProfitEvent)    → SSE profit_update
  │  └─ snapshotBroker.Publish(PositionSnapshot)     → SSE position_snapshot
  ▼
stream_handler.go SubscribeEvents
  │  ├─ 首次: OpenedOrders RPC (一次性) → position_snapshot
  │  └─ 实时: snapCh → position_snapshot (批量，全部持仓在一条消息)
  ▼
前端 SSE → onPositionSnapshot → setPositions() (单次批量替换 → 一次渲染)
```

关键约束：持仓数据**始终批量发送**，不允许逐条推送。初始加载用 `OpenedOrders` RPC，后续用 `OnOrderUpdate` stream。前端永不用 `fetchPositions` RPC（数据全走 stream）。
