---
name: mt-gateway-05-account
description: >
  MT4/MT5 账户状态查询。涵盖 AccountSummary（余额/净值/保证金/杠杆/货币）、
  MT5 Account（完整账户信息含联系方式）。用于绑定账户时填充真实数据。
---

# 05 — 账户状态

## RPC 概览

| RPC | 平台 | 用途 |
|-----|------|------|
| `AccountSummary` | MT4/MT5 | 交易账户摘要（余额、净值、保证金等） |
| `Account` | MT5 only | 完整账户信息（含用户名、邮箱、国家等） |

## AccountSummary — 交易摘要

MT4 路径：`/mt4grpc.MT4/AccountSummary`
MT5 路径：`/mt5grpc.MT5/AccountSummary`

两个平台返回结构基本一致，MT5 多了 `Method` 和 `Type` 字段。

```go
resp, err := mt4Client.AccountSummary(ctx, &pb.AccountSummaryRequest{Id: sessionID})
if err != nil {
    return err
}
s := resp.GetResult()
```

### 响应字段

| 字段 | 类型 | MT4 | MT5 | 说明 |
|------|------|-----|-----|------|
| `Balance` | double | ✓ | ✓ | 账户余额 |
| `Credit` | double | ✓ | ✓ | 信用赠金 |
| `Profit` | double | ✓ | ✓ | 浮动盈亏 |
| `Equity` | double | ✓ | ✓ | 净值 (= Balance + Credit + Profit) |
| `Margin` | double | ✓ | ✓ | 已用保证金 |
| `FreeMargin` | double | ✓ | ✓ | 可用保证金 (= Equity - Margin) |
| `MarginLevel` | double | ✓ | ✓ | 保证金比例 (= Equity / Margin * 100) |
| `Leverage` | double | ✓ | ✓ | 杠杆倍数 |
| `Currency` | string | ✓ | ✓ | 账户货币 (USD/EUR/...) |
| `Type` | enum/string | AccountType | string | 账户类型（真实/模拟/竞赛） |
| `IsInvestor` | bool | ✓ | ✓ | 是否只读投资者 |
| `Method` | AccMethod | — | ✓ | MT5 独有：Netting/Hedging |

### MT4 AccountType 枚举

```protobuf
enum AccountType {
  AccountType_Real    = 0;
  AccountType_Contest = 1;
  AccountType_Demo    = 2;
}
```

### MT5 AccMethod 枚举

```protobuf
enum AccMethod {
  AccMethod_Default = 0;
  AccMethod_Netting = 1;  // 单边持仓
  AccMethod_Hedging = 2;  // 双边持仓
}
```

MT5 的 `Type` 是 string（"real"/"demo"），与 MT4 的 enum 不同。

## Account — MT5 完整信息

MT5 路径：`/mt5grpc.MT5/Account`

```go
resp, err := mt5Client.Account(ctx, &pb.AccountRequest{Id: sessionID})
rec := resp.GetResult()
```

### AccountRec 字段

| 字段 | 类型 | 说明 |
|------|------|------|
| `Login` | uint64 | 交易账号 |
| `Type` | string | 账户类型 |
| `UserName` | string | 用户名 |
| `TradeFlags` | int32 | 交易权限标志位 |
| `Country` | string | 国家 |
| `City` | string | 城市 |
| `State` | string | 省份 |
| `ZipCode` | string | 邮编 |
| `UserAddress` | string | 地址 |
| `Phone` | string | 电话 |
| `Email` | string | 邮箱 |
| `Balance` | double | 余额 |
| `Credit` | double | 信用 |
| `Blocked` | double | 冻结资金 |
| `Leverage` | int32 | 杠杆 |

## 与 ant 数据模型映射

`AccountSummary` → `mt_accounts` 表字段：

```
AccountSummary.Balance     → mt_accounts.balance
AccountSummary.Equity      → mt_accounts.equity
AccountSummary.Margin      → mt_accounts.margin
AccountSummary.FreeMargin  → mt_accounts.free_margin
AccountSummary.MarginLevel → mt_accounts.margin_level
AccountSummary.Leverage    → mt_accounts.leverage
AccountSummary.Currency    → mt_accounts.currency
AccountSummary.Type        → mt_accounts.account_type
AccountSummary.Profit      → mt_accounts.profit
AccountSummary.Credit      → mt_accounts.credit
```

## 使用场景

1. **绑定账户时**：CreateAccount 流程中，Connect 成功后调 AccountSummary，将真实数据写入 mt_accounts
2. **账户列表刷新**：前端列表页展示最新余额/净值
3. **实时更新**：通过 OnOrderUpdate stream 中的 `OrderUpdateSummary` 也可获取账户状态变更

## 参考代码

- MT4 proto：`/opt/ant/grpc/mt4.proto` L592-633
- MT5 proto：`/opt/ant/grpc/mt5.proto` L832-903
- MT4 适配器：`internal/mdgateway/adapter/mt4/gateway.go` → `SubscribeProfit`
- MT5 适配器：`internal/mdgateway/adapter/mt5/gateway.go` → `SubscribeProfit`
- mthub broker：`internal/mthub/types.go` → `AccountProfitEvent` / `AccountProfitBroker`
- stream handler：`internal/connect/stream_handler.go` → `profitEventToProto` / `sendInitialSnapshot` / `SubscribeUserSummary`
- Legacy 封装：`/opt/ant/backend/legacy/mt4client/account_methods.go`
- Legacy 封装：`/opt/ant/backend/legacy/mt5client/account_methods.go`

## 实时账户数据流（生产实现）

账户指标（余额/净值/保证金/盈亏）在 ant 中**全程推送，无轮询**，有两条通道：

### 通道 1：profit_update（每账户实时推送）

```
MT5/MT4 OnOrderUpdate stream
  │  OrderUpdateSummary { Balance, Equity, Margin, FreeMargin, MarginLevel, OpenedOrders[] }
  ▼
main.go OnOrderUpdate callback
  │  accountBroker.Publish(AccountProfitEvent)
  ▼
stream_handler.go SubscribeEvents
  │  profitCh → profitEventToProto() → SSE profit_update
  ▼
前端 onProfit handler
  │  更新 accountInfoMap（balance/equity/margin/profit...）
  │  更新 positions currentPrice/profit（仅 patch 已存在的行，不新增）
```

### 通道 2：user_summary（用户级聚合摘要）

```
前端订阅 SubscribeUserSummary
  │  首次: computeSummary() — 从 mt_accounts 表聚合所有账户
  │  后续: 每次 profit_update 触发 → re-aggregate from DB
  ▼
前端 onUserSummary handler
  │  setUserSummary(totalBalance, totalEquity, totalProfit, connectedCount...)
```

### profitEventToProto 映射

```go
func profitEventToProto(pev *mthub.AccountProfitEvent) *antv1.ProfitUpdateEvent {
    return &antv1.ProfitUpdateEvent{
        AccountId: pev.AccountID, Balance: pev.Balance, Credit: pev.Credit,
        Equity: pev.Equity, Profit: pev.Profit, Margin: pev.Margin,
        FreeMargin: pev.FreeMargin, MarginLevel: pev.MarginLevel,
        ProfitPercent: pev.ProfitPercent,
        Orders: []*OrderProfitItem{  // 每个持仓的 ticket/symbol/profit/volume/currentPrice
            {Ticket, Symbol, Profit, Volume, CurrentPrice}
        },
    }
}
```

注意：`OnOrderUpdate` 推送的 `OrderUpdateSummary` 已包含全部账户指标（Balance/Equity/Margin/FreeMargin/MarginLevel），因此**无需单独调用 `AccountSummary` RPC**。绑定时用一次 `AccountSummary` 填充初始值，后续全靠 stream 更新。

## MT4/MT5 字段差异（重要踩坑）

### Credit 获取

| 数据源 | MT4 | MT5 |
|--------|-----|-----|
| `AccountSummary.GetCredit()` | ✓ 有 | ✓ 有 |
| `OrderUpdateSummary.GetCredit()` | ✓ 有 | ✗ **没有** |
| `ProfitUpdate.GetCredit()` | ✓ 有 | ✓ 有 |

**MT5 注意**：`orderUpdateRecvLoop`（`quotes.go`）中 `OrderUpdateSummary` 没有 `GetCredit()`。对 MT5 设置 `Credit: 0`，真实 Credit 由另一个独立的 `OnAccountProfit` 流补充。

### OpenTime 获取

| 数据源 | MT4 | MT5 |
|--------|-----|-----|
| `Order.GetOpenTime()` | `*Timestamp` | `*Timestamp` |
| `Order.GetOpenTimestampUTC()` | ✗ 没有 | ✓ `int64` unix 秒 |

**MT5 注意**：broker 可能只填充 `OpenTimestampUTC`（int64），不填充 `OpenTime`（Timestamp pointer）。需要 fallback：
```go
func openTimeFromOrder(o *pb.Order) time.Time {
    if t := o.GetOpenTime(); t != nil && t.GetSeconds() > 0 {
        return t.AsTime()
    }
    if ts := o.GetOpenTimestampUTC(); ts > 0 {
        return time.Unix(ts, 0)
    }
    return time.Time{}
}
```

### Proto Timestamp 零值陷阱

`timestamppb.New(time.Unix(0, 0))` 创建 `seconds: 0` 的 Timestamp。
proto3 JSON `omitempty` 会**省略零值 Timestamp**，导致前端永远收不到该字段。

当 broker 返回零值 `OpenTime` 时：
1. `o.GetOpenTime().AsTime()` → `time.Unix(0, 0)` (epoch, **不是** Go zero time)
2. `timestamppb.New()` → `seconds: 0` (零值 Timestamp)
3. ConnectRPC JSON `omitempty` → **省略**
4. 前端收不到 → 显示为空
