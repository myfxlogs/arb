# MT4/MT5 历史 K 线（PriceHistory / QuoteHistory）

## 数据流总览

```
frontend SSE ← BarBroker ← mthub.PublishBar ← pipeline.go OnBar
                                    ↑
              bar_aggregator.go ← mdgateway recvLoop ← MT OnQuote stream ticks

frontend RPC ← PriceHistory RPC ← adapter/mt{4,5}/price_history.go
              ↓
         ClickHouse md_bars (backfill + 定时补充)
```

## MT4 vs MT5 — 5 个关键差异

### 1. RPC 名称不同

| 操作 | MT4 | MT5 |
|------|-----|-----|
| 历史 K 线 | `QuoteHistory` | `PriceHistory` |
| 实时报价流 | `OnQuote` | `OnQuote` |
| 订阅品种 | `SubscribeMany` | `SubscribeMany` |

### 2. K 线请求参数差异（最易踩坑）

| 参数 | MT4 | MT5 |
|------|-----|-----|
| 周期类型 | protobuf enum `Timeframe_Timeframe_M1` | `int32` 分钟数: `1,5,15,30,60,240,1440,10080` |
| 时间范围 | `From`(string) + `Count`(int32) | `From`(string) + `To`(string) |
| Count 计算 | `((to-from)*1000) / periodMs` 手动算 | 不需要（用 To 代替） |
| Count 上限 | **5000**，超过 clamp | 无硬限制 |
| From 格式 | `"2006-01-02T15:04:05"` UTC | `"2006-01-02T15:04:05"` UTC |

**MT4 陷阱**：用 `From` + `Count`，不是 `From/To`。Count 必须手动计算，上限 5000。
```go
count := int32(((to - from) * 1000) / periodMs(period))
if count <= 0 { count = 100 }
if count > 5000 { count = 5000 }
```

**MT5 陷阱**：用 `From` + `To`，是日期范围而非 Count。`To` 是结束时间戳。

### 3. Bar 字段名不同

| 字段 | MT4 | MT5 |
|------|-----|-----|
| 开盘价 | `b.GetOpen()` | `b.GetOpenPrice()` |
| 最高价 | `b.GetHigh()` | `b.GetHighPrice()` |
| 最低价 | `b.GetLow()` | `b.GetLowPrice()` |
| 收盘价 | `b.GetClose()` | `b.GetClosePrice()` |
| 成交量 | `b.GetVolume()` | `b.GetVolume()` |
| Tick 量 | **无** | `b.GetTickVolume()` |

### 4. gRPC 客户端模型

| 维度 | MT4 | MT5 |
|------|-----|-----|
| K 线客户端 | 复用主 `client` | **独立 `qhCli`**（PriceHistory 专用） |

**MT5 陷阱**：PriceHistory 需要单独的 gRPC 客户端 `qhCli`，在 `gateway.Open()` 中创建。复用主 client 会 nil pointer。

### 5. 周期映射

MT4 — protobuf enum：
```go
case "1m": return pb.Timeframe_Timeframe_M1, true
case "5m": return pb.Timeframe_Timeframe_M5, true
case "1h": return pb.Timeframe_Timeframe_H1, true
case "1d": return pb.Timeframe_Timeframe_D1, true
case "1w": return pb.Timeframe_Timeframe_W1, true
```

MT5 — int32 分钟数：
```go
case "1m": return 1     // 1 分钟
case "1h": return 60    // 60 分钟
case "1d": return 1440  // 1440 分钟 = 24*60
case "1w": return 10080 // MT5 PERIOD_W1 = 7*24*60
```

## 8 个常见陷阱

### 陷阱 1：MT4/MT5 价格字段名不同
- `mt4/price_history.go`: `b.GetOpen()`, `b.GetHigh()`, `b.GetLow()`, `b.GetClose()`
- `mt5/price_history.go`: `b.GetOpenPrice()`, `b.GetHighPrice()`, `b.GetLowPrice()`, `b.GetClosePrice()`
- **后果**: 写错编译不过（protobuf 生成的方法名不同）

### 陷阱 2：MT4 用 Count 而非 To
- MT4: `From`(时间字符串) + `Count`(条数)
- MT5: `From` + `To`
- **后果**: 给 MT4 传 `To` 参数被忽略，返回 `Count` 默认为 0 → 空结果

### 陷阱 3：MT5 PriceHistory 需要独立 gRPC 客户端
- MT4 复用主 `client`
- MT5 **必须**用 `qhCli` 字段
- **后果**: nil pointer panic

### 陷阱 4：periodMs() 重复定义
- `periodMs()` 在 `mt4/price_history.go` 和 `mt5/price_history.go` **各自定义**
- 这是有意为之 — MT4/MT5 adapter 禁止共享代码（除 `mdtick/` DTO）
- **后果**: 只改一个导致 MT4/MT5 行为不一致

### 陷阱 5：mdtick.Bar 用 decimal.Decimal
- 所有价格字段**必须**用 `decimal.NewFromFloat()` 转换
- `mdtick.Bar.Open`, `.High`, `.Low`, `.Close` 都是 `decimal.Decimal` 类型
- **后果**: 直接赋值 float64 → 价格精度丢失、回测不可复现

### 陷阱 6：CloseTsUnixMs 的计算
- `CloseTsUnixMs = OpenTsUnixMs + periodMs(period)`
- 假设 MT 返回的 bar 时间戳是**开盘时间**（MT4/MT5 的实际行为）
- **不能用** MT 返回的 time 直接作为 CloseTs
- **后果**: ClickHouse 时间范围查询偏移

### 陷阱 7：symbol 必须是券商原生符号
- `symbolRaw` 直接传 MT RPC，**不修改**
- 如有后缀（如 `EURUSD.z`），必须保留
- **后果**: 后缀剥离后 MT RPC 返回 "symbol not found"

### 陷阱 8：From 参数格式
- **必须** UTC：`time.Unix(from, 0).UTC().Format("2006-01-02T15:04:05")`
- 不能用 Unix 毫秒、不能用 `time.RFC3339`
- **后果**: 返回空或错误时间范围

## 认证（两个平台完全相同）

```go
md := metadata.New(map[string]string{
    "id":            sessionID,       // MT session ID
    "authorization": "Bearer " + token, // JWT token
})
authCtx := metadata.NewOutgoingContext(ctx, md)
```

## 关键数据转换

`mdtick.Bar` 是统一的 K 线 DTO（在 `adapter/mdtick/` 中共享）：
```go
type Bar struct {
    AccountID     string
    Period        string          // "1m","5m","1h",...
    OpenTsUnixMs  int64
    CloseTsUnixMs int64           // = OpenTsUnixMs + periodMs
    Open          decimal.Decimal  // ⚠️ 不是 float64
    High          decimal.Decimal
    Low           decimal.Decimal
    Close         decimal.Decimal
    Volume        float64
    TickCount     uint32          // MT5 特有
}
```

## 新增 K 线周期的 Checklist

1. [ ] `bar_aggregator.go` 周期列表添加（如需实时聚合）
2. [ ] `mt4/price_history.go` → `mt4PeriodToTimeframe()` 添加映射
3. [ ] `mt5/price_history.go` → `mt5PeriodToTimeframe()` 添加映射
4. [ ] 两个 adapter 的 `periodMs()` 添加毫秒值
5. [ ] 前端 `PriceChart.tsx` `TIMEFRAMES` 数组添加
6. [ ] 如需 SSE 推送，`bar_aggregator.go` 包含该周期
7. [ ] `go build ./internal/mdgateway/...` 编译验证
8. [ ] 真实账户测试 MT4 和 MT5 的 K 线拉取

## 源文件索引

| 文件 | 内容 |
|------|------|
| `backend/internal/mdgateway/adapter/mt4/price_history.go` | MT4 `GetPriceHistory` + `mt4PeriodToTimeframe` + `periodMs` |
| `backend/internal/mdgateway/adapter/mt5/price_history.go` | MT5 `GetPriceHistory` + `mt5PeriodToTimeframe` + `periodMs` |
| `backend/internal/mdgateway/adapter/mt4/quotes.go` | MT4 实时报价流 `Subscribe` + `OnQuote` |
| `backend/internal/mdgateway/adapter/mt5/quotes.go` | MT5 实时报价流 `Subscribe` + `OnQuote` |
| `backend/internal/mdgateway/adapter/mt4/order_history.go` | MT4 历史订单 |
| `backend/internal/mdgateway/adapter/mt5/order_history.go` | MT5 历史订单 |
| `backend/internal/mdgateway/adapter/mt4/order_stream.go` | MT4 订单流 `OnOrderProfit` |
| `backend/internal/mdgateway/adapter/mt5/order_stream.go` | MT5 订单流 `OnOrderProfit` |
| `backend/internal/mdgateway/adapter/mdtick/mdtick.go` | 共享 DTO（Bar, Tick, ProfitUpdate） |
| `backend/internal/mdgateway/bar_aggregator.go` | Tick → Bar 聚合 |
| `backend/internal/connect/system/mthub_service_extra.go` | `PriceHistory` ConnectRPC 端点 |
| `backend/internal/connect/system/mthub_service_backfill.go` | 自动 backfill 逻辑 |
| `backend/internal/mdgateway/backfiller/backfiller.go` | 定时 backfill 系统 |
